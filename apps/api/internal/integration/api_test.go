package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/domain"
)

func TestHealthz(t *testing.T) {
	h := newHarness(t)
	resp := h.do("GET", "/healthz", "")
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected body %v", payload)
	}
}

func TestActivateFirstTimeReturnsKeyAndSession(t *testing.T) {
	h := newHarness(t)
	_, code := h.seedCDK(nil)

	resp := h.activate(code)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %v", status, payload)
	}
	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected success envelope: %v", payload)
	}
	data, _ := payload["data"].(map[string]any)
	secret, _ := data["default_api_key"].(string)
	if !strings.HasPrefix(secret, "cf_live_") {
		t.Fatalf("default key must be cf_live_ prefixed: %v", data)
	}
	user, _ := data["user"].(map[string]any)
	if user["id"] == "" || user["cdk_prefix"] == "" {
		t.Fatalf("user payload incomplete: %v", data)
	}

	cookie := sessionCookie(t, resp)
	if !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("session cookie must be HttpOnly and Secure: %+v", cookie)
	}
}

func TestActivateReEntryIssuesSessionWithoutNewKey(t *testing.T) {
	h := newHarness(t)
	_, code := h.seedCDK(nil)

	first := h.activate(code)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first activation must be 201, got %d", first.StatusCode)
	}
	firstCookie := sessionCookie(t, first)

	second := h.activate(code)
	status, payload := decodeEnvelope(t, second)
	if status != http.StatusOK {
		t.Fatalf("re-entry must be 200, got %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	if _, hasKey := data["default_api_key"]; hasKey {
		t.Fatalf("re-entry must not return a key: %v", data)
	}
	secondCookie := sessionCookie(t, second)
	if firstCookie.Value == secondCookie.Value {
		t.Fatal("re-entry must issue a fresh session token")
	}
}

func TestActivateRejectsUnknownCDK(t *testing.T) {
	h := newHarness(t)
	resp := h.activate("UNKNOWN" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")))
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %v", status, payload)
	}
	expectError(t, payload, "CDK_NOT_FOUND")
}

func TestActivateValidatesFormat(t *testing.T) {
	h := newHarness(t)
	for _, code := range []string{"abc", "!!!!", "", strings.Repeat("a", 129)} {
		resp := h.activate(code)
		status, payload := decodeEnvelope(t, resp)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("code %q: expected 422, got %d: %v", code, status, payload)
		}
		expectError(t, payload, "INVALID_REQUEST")
	}
}

func TestActivateRejectsStateProblems(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*domain.Cdk)
		wantCode string
	}{
		{"disabled", func(c *domain.Cdk) { c.Status = domain.CDKStatusDisabled }, "CDK_DISABLED"},
		{"deadline passed", func(c *domain.Cdk) {
			past := time.Now().UTC().Add(-time.Hour)
			c.ActivationDeadline = &past
		}, "CDK_ACTIVATION_EXPIRED"},
		{"expired", func(c *domain.Cdk) {
			past := time.Now().UTC().Add(-time.Hour)
			c.ExpiresAt = &past
		}, "CDK_EXPIRED"},
		{"exhausted", func(c *domain.Cdk) { c.QuotaRemaining = 0 }, "CDK_EXHAUSTED"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			h := newHarness(t)
			_, code := h.seedCDK(testCase.mutate)
			status, payload := decodeEnvelope(t, h.activate(code))
			if status != http.StatusForbidden && status != http.StatusPaymentRequired {
				t.Fatalf("expected 402/403, got %d: %v", status, payload)
			}
			expectError(t, payload, testCase.wantCode)
		})
	}
}

func TestKeyLifecycle(t *testing.T) {
	h := newHarness(t)
	_, code := h.seedCDK(nil)
	activateResp := h.activate(code)
	if activateResp.StatusCode != http.StatusCreated {
		t.Fatalf("setup activation failed: %d", activateResp.StatusCode)
	}
	cookies := []*http.Cookie{sessionCookie(t, activateResp)}

	// Create returns the secret exactly once.
	createResp := h.do("POST", "/v1/keys", `{"name": "worker-a"}`, cookies...)
	status, payload := decodeEnvelope(t, createResp)
	if status != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	secret, _ := data["secret"].(string)
	if !strings.HasPrefix(secret, "cf_live_") {
		t.Fatalf("secret missing: %v", data)
	}
	keyID, _ := data["id"].(string)
	if keyID == "" {
		t.Fatalf("key id missing: %v", data)
	}

	// List never contains the secret.
	listResp := h.do("GET", "/v1/keys", "", cookies...)
	status, payload = decodeEnvelope(t, listResp)
	if status != http.StatusOK {
		t.Fatalf("list keys: expected 200, got %d: %v", status, payload)
	}
	listData, _ := payload["data"].(map[string]any)
	items, _ := listData["items"].([]any)
	if len(items) != 2 { // default + worker-a
		t.Fatalf("expected 2 keys, got %d", len(items))
	}
	listBody := fmt.Sprintf("%v", listData)
	if strings.Contains(listBody, secret) {
		t.Fatal("listing leaked the key secret")
	}
	firstItem, _ := items[0].(map[string]any)
	if firstItem["name"] != "worker-a" {
		t.Fatalf("keys must sort newest first, got %v", items)
	}

	// Rename.
	patchResp := h.do("PATCH", "/v1/keys/"+keyID, `{"name": "worker-b"}`, cookies...)
	status, payload = decodeEnvelope(t, patchResp)
	if status != http.StatusOK {
		t.Fatalf("rename: expected 200, got %d: %v", status, payload)
	}
	if renamed, _ := payload["data"].(map[string]any); renamed["name"] != "worker-b" {
		t.Fatalf("rename failed: %v", payload)
	}

	// Disable sets revoked status.
	patchResp = h.do("PATCH", "/v1/keys/"+keyID, `{"status": "DISABLED"}`, cookies...)
	status, payload = decodeEnvelope(t, patchResp)
	if status != http.StatusOK {
		t.Fatalf("disable: expected 200, got %d: %v", status, payload)
	}
	if disabled, _ := payload["data"].(map[string]any); disabled["status"] != "DISABLED" {
		t.Fatalf("disable failed: %v", payload)
	}

	// Re-enable.
	patchResp = h.do("PATCH", "/v1/keys/"+keyID, `{"status": "ACTIVE"}`, cookies...)
	status, payload = decodeEnvelope(t, patchResp)
	if status != http.StatusOK {
		t.Fatalf("re-enable: expected 200, got %d: %v", status, payload)
	}

	// Validation: empty change and invalid status.
	for _, body := range []string{`{}`, `{"status": "ENABLED"}`} {
		patchResp = h.do("PATCH", "/v1/keys/"+keyID, body, cookies...)
		status, payload = decodeEnvelope(t, patchResp)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("patch %s: expected 422, got %d: %v", body, status, payload)
		}
	}

	// Delete is terminal.
	deleteResp := h.do("DELETE", "/v1/keys/"+keyID, "", cookies...)
	status, payload = decodeEnvelope(t, deleteResp)
	if status != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %v", status, payload)
	}
	if deleted, _ := payload["data"].(map[string]any); deleted["status"] != "DELETED" {
		t.Fatalf("delete failed: %v", payload)
	}
	patchResp = h.do("PATCH", "/v1/keys/"+keyID, `{"name": "zombie"}`, cookies...)
	status, payload = decodeEnvelope(t, patchResp)
	if status != http.StatusConflict {
		t.Fatalf("patch deleted: expected 409, got %d: %v", status, payload)
	}
	expectError(t, payload, "API_KEY_DELETED")
}

func TestCrossUserIsolation(t *testing.T) {
	h := newHarness(t)
	_, aliceCode := h.seedCDK(nil)
	_, bobCode := h.seedCDK(nil)

	aliceResp := h.activate(aliceCode)
	if aliceResp.StatusCode != http.StatusCreated {
		t.Fatalf("alice activation failed: %d", aliceResp.StatusCode)
	}
	alice := []*http.Cookie{sessionCookie(t, aliceResp)}

	bobResp := h.activate(bobCode)
	if bobResp.StatusCode != http.StatusCreated {
		t.Fatalf("bob activation failed: %d", bobResp.StatusCode)
	}
	bob := []*http.Cookie{sessionCookie(t, bobResp)}

	createResp := h.do("POST", "/v1/keys", `{"name": "alice-key"}`, alice...)
	_, payload := decodeEnvelope(t, createResp)
	data, _ := payload["data"].(map[string]any)
	aliceKeyID, _ := data["id"].(string)

	// Bob cannot read, change or delete Alice's key.
	resp := h.do("PATCH", "/v1/keys/"+aliceKeyID, `{"name": "hijacked"}`, bob...)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusNotFound {
		t.Fatalf("cross-user patch must 404, got %d: %v", status, payload)
	}
	expectError(t, payload, "API_KEY_NOT_FOUND")

	resp = h.do("DELETE", "/v1/keys/"+aliceKeyID, "", bob...)
	status, payload = decodeEnvelope(t, resp)
	if status != http.StatusNotFound {
		t.Fatalf("cross-user delete must 404, got %d: %v", status, payload)
	}
}

func TestSessionGuardAndLogout(t *testing.T) {
	h := newHarness(t)
	_, code := h.seedCDK(nil)
	activateResp := h.activate(code)
	if activateResp.StatusCode != http.StatusCreated {
		t.Fatalf("activation failed: %d", activateResp.StatusCode)
	}
	cookies := []*http.Cookie{sessionCookie(t, activateResp)}

	// Missing and forged cookies are rejected.
	resp := h.do("GET", "/v1/keys", "")
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusUnauthorized {
		t.Fatalf("missing cookie must 401, got %d: %v", status, payload)
	}
	expectError(t, payload, "SESSION_INVALID")

	resp = h.do("GET", "/v1/keys", "", &http.Cookie{Name: "session", Value: "forged"})
	status, payload = decodeEnvelope(t, resp)
	if status != http.StatusUnauthorized {
		t.Fatalf("forged cookie must 401, got %d: %v", status, payload)
	}

	// Logout revokes exactly the current session.
	resp = h.do("POST", "/v1/auth/logout", "", cookies...)
	status, payload = decodeEnvelope(t, resp)
	if status != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d: %v", status, payload)
	}
	resp = h.do("GET", "/v1/keys", "", cookies...)
	status, payload = decodeEnvelope(t, resp)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked session must 401, got %d: %v", status, payload)
	}
}

func TestDisabledUserLosesSessionAccess(t *testing.T) {
	h := newHarness(t)
	_, code := h.seedCDK(nil)
	activateResp := h.activate(code)
	if activateResp.StatusCode != http.StatusCreated {
		t.Fatalf("activation failed: %d", activateResp.StatusCode)
	}
	cookies := []*http.Cookie{sessionCookie(t, activateResp)}

	// Resolve the activated user, then suspend exactly that account.
	status, payload := decodeEnvelope(t, activateResp)
	data, _ := payload["data"].(map[string]any)
	user, _ := data["user"].(map[string]any)
	userID, _ := user["id"].(string)
	if _, err := h.services.Pool.Exec(context.Background(),
		`UPDATE users SET status = 'SUSPENDED' WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend user: %v", err)
	}

	resp := h.do("GET", "/v1/keys", "", cookies...)
	status, payload = decodeEnvelope(t, resp)
	if status != http.StatusForbidden {
		t.Fatalf("suspended user must 403, got %d: %v", status, payload)
	}
	expectError(t, payload, "ACCOUNT_DISABLED")
}
