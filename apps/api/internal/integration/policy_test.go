package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/domain"
	"github.com/captchaflow/service-platform/api/internal/solver"
	"github.com/captchaflow/service-platform/api/internal/store"
)

// activateWithSession activates a fresh CDK and returns the bearer secret
// plus the session cookie for follow-up console calls.
func activateWithSession(t *testing.T, h *harness) (string, *http.Cookie) {
	t.Helper()
	_, code := h.seedCDK(nil)
	resp := h.activate(code)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("activation failed: %d", resp.StatusCode)
	}
	_, payload := decodeEnvelope(t, resp)
	data, _ := payload["data"].(map[string]any)
	secret, _ := data["default_api_key"].(string)
	if secret == "" {
		t.Fatalf("no default key: %v", payload)
	}
	return secret, sessionCookie(t, resp)
}

// TestAPIKeyPolicyRestrictsCalls covers per-key quota and IP allowlists.
func TestAPIKeyPolicyRestrictsCalls(t *testing.T) {
	sh := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	})
	h := sh.h

	_, session := activateWithSession(t, h)

	resp := h.do("POST", "/v1/keys", `{"name": "受限 Key"}`, session)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusCreated {
		t.Fatalf("create key failed: %d %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	keyID, _ := data["id"].(string)
	originalSecret, _ := data["secret"].(string)
	if originalSecret == "" {
		t.Fatalf("created key must return plaintext once: %v", data)
	}

	// Policy: per-key quota of 1 and an allowlist excluding the default
	// httptest source address (192.0.2.1).
	patch := func(ips string) *http.Response {
		return h.do("PATCH", "/v1/keys/"+keyID, fmt.Sprintf(`{"quota_limit": 1, "allowed_ips": %q}`, ips), session)
	}
	if status, payload := decodeEnvelope(t, patch("10.0.0.0/8, 10.1.1.1")); status != http.StatusOK {
		t.Fatalf("policy patch failed: %d %v", status, payload)
	}

	// Off-allowlist source IP: rejected before admission, no solver call.
	denied := sh.post(originalSecret, "idem-ip-denied", `{"captcha_id": "cap-1", "risk_type": "slide"}`)
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for off-allowlist IP, got %d", denied.StatusCode)
	}
	status, payload = decodeEnvelope(t, denied)
	expectError(t, payload, "KEY_IP_FORBIDDEN")
	_ = status
	if sh.gateway.calls.Load() != 0 {
		t.Fatalf("blocked call must not reach the solver")
	}

	// Allow the caller IP; quota_limit=1 lets exactly one solve through.
	if status, _ := decodeEnvelope(t, patch("192.0.2.1")); status != http.StatusOK {
		t.Fatalf("allowlist patch failed")
	}
	first := sh.post(originalSecret, "idem-ip-allowed", `{"captcha_id": "cap-1", "risk_type": "slide"}`)
	if first.StatusCode != http.StatusOK {
		_, body := decodeEnvelope(t, first)
		t.Fatalf("expected first solve to pass, got %d %v", first.StatusCode, body)
	}
	second := sh.post(originalSecret, "idem-ip-second", `{"captcha_id": "cap-2", "risk_type": "slide"}`)
	if second.StatusCode != http.StatusPaymentRequired {
		_, body := decodeEnvelope(t, second)
		t.Fatalf("expected per-key quota rejection (402), got %d %v", second.StatusCode, body)
	}
	status, payload = decodeEnvelope(t, second)
	expectError(t, payload, "KEY_QUOTA_EXHAUSTED")

	// Historical plaintext cannot be recovered through the API. The original
	// secret is intentionally retained only in this test for authenticated calls.
	resp = h.do("GET", "/v1/keys/"+keyID+"/secret", "", session)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("historical secret endpoint must be unavailable: %d", resp.StatusCode)
	}
}

// TestAdmissionGatesRejectLateEntitlementDisable simulates the gap between
// authentication and admission. Disabling any entitlement during that gap
// must keep the solve from reserving quota or reaching the gateway.
func TestAdmissionGatesRejectLateEntitlementDisable(t *testing.T) {
	h := newHarness(t)
	secret, _ := activateWithSession(t, h)
	caller, appErr := h.services.AuthenticateAPIKey(context.Background(), secret)
	if appErr != nil {
		t.Fatalf("authenticate seeded key: %v", appErr)
	}

	keyStatus := domain.APIKeyStatusDisabled
	if err := store.UpdateAPIKeyFields(context.Background(), h.services.Pool, caller.APIKeyID, nil, &keyStatus, nil); err != nil {
		t.Fatalf("disable key: %v", err)
	}
	admitted, err := store.ConsumeAPIKeyQuota(context.Background(), h.services.Pool, caller.APIKeyID)
	if err != nil {
		t.Fatalf("consume disabled key: %v", err)
	}
	if admitted {
		t.Fatal("disabled API key must not pass the atomic admission gate")
	}

	keyStatus = domain.APIKeyStatusActive
	if err := store.UpdateAPIKeyFields(context.Background(), h.services.Pool, caller.APIKeyID, nil, &keyStatus, nil); err != nil {
		t.Fatalf("restore key: %v", err)
	}
	if err := store.SetUserStatus(context.Background(), h.services.Pool, caller.UserID, "DISABLED"); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	admitted, err = store.ConsumeAPIKeyQuota(context.Background(), h.services.Pool, caller.APIKeyID)
	if err != nil {
		t.Fatalf("consume disabled user's key: %v", err)
	}
	if admitted {
		t.Fatal("disabled user must not pass the atomic key admission gate")
	}

	if err := store.SetUserStatus(context.Background(), h.services.Pool, caller.UserID, string(domain.UserStatusActive)); err != nil {
		t.Fatalf("restore user: %v", err)
	}
	if err := store.SetCdkStatus(context.Background(), h.services.Pool, caller.CDKID, string(domain.CDKStatusDisabled)); err != nil {
		t.Fatalf("disable cdk: %v", err)
	}
	if err := store.ReserveQuota(context.Background(), h.services.Pool, caller.CDKID, caller.UserID, uuid.New(), "late-disable", "late entitlement disable"); err == nil {
		t.Fatal("disabled CDK must not reserve quota after authentication")
	}

	summary, err := store.GetCDKSummaryForUser(context.Background(), h.services.Pool, caller.UserID)
	if err != nil {
		t.Fatalf("read quota summary: %v", err)
	}
	if summary.QuotaRemaining != 100 || summary.QuotaReserved != 0 || summary.QuotaUsed != 0 {
		t.Fatalf("late entitlement disable must leave quota unchanged: %+v", summary)
	}
}

// TestSystemConfigHoldsDisplayBaseURL covers the docs-facing base URL: it is
// a display value only and must never touch the solver configuration.
func TestSystemConfigHoldsDisplayBaseURL(t *testing.T) {
	h := newHarness(t)
	seedAdmin(t, h, "config-admin", store.AdminRoleAdmin)
	login := adminLogin(t, h, "config-admin")

	resp := h.do("GET", "/admin/v1/system/config", "", login)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusOK {
		t.Fatalf("get config failed: %d %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	if data["api_base_url"] != "" || data["has_override"] != false {
		t.Fatalf("expected empty default config, got %v", data)
	}
	// Public meta mirrors the unset state.
	resp = h.do("GET", "/v1/meta", "")
	_, payload = decodeEnvelope(t, resp)
	if data, _ = payload["data"].(map[string]any); data["api_base_url"] != "" {
		t.Fatalf("public meta should be empty by default, got %v", data)
	}

	body := `{"api_base_url": "https://api.captchaflow.example.com", "reason": "接入文档更新"}`
	resp = h.do("PUT", "/admin/v1/system/config", body, login)
	if status, payload := decodeEnvelope(t, resp); status != http.StatusOK {
		t.Fatalf("put config failed: %d %v", status, payload)
	}

	resp = h.do("GET", "/admin/v1/system/config", "", login)
	_, payload = decodeEnvelope(t, resp)
	data, _ = payload["data"].(map[string]any)
	if data["api_base_url"] != "https://api.captchaflow.example.com" || data["has_override"] != true {
		t.Fatalf("override not stored: %v", data)
	}
	// Public meta now serves the display URL.
	resp = h.do("GET", "/v1/meta", "")
	_, payload = decodeEnvelope(t, resp)
	if data, _ = payload["data"].(map[string]any); data["api_base_url"] != "https://api.captchaflow.example.com" {
		t.Fatalf("public meta not updated: %v", data)
	}

	// Invalid URL is a 422 and does not overwrite the stored value.
	resp = h.do("PUT", "/admin/v1/system/config", `{"api_base_url": "not-a-url", "reason": "x"}`, login)
	if status, _ := decodeEnvelope(t, resp); status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid url should 422")
	}
	resp = h.do("GET", "/v1/meta", "")
	_, payload = decodeEnvelope(t, resp)
	if data, _ = payload["data"].(map[string]any); data["api_base_url"] != "https://api.captchaflow.example.com" {
		t.Fatalf("failed write must not clobber config: %v", data)
	}
}

// TestAdminRevealsCdkPlaintext covers sealed-code re-display and remarks.
func TestAdminRevealsCdkPlaintext(t *testing.T) {
	h := newHarness(t)
	seedAdmin(t, h, "reveal-admin", store.AdminRoleAdmin)
	login := adminLogin(t, h, "reveal-admin")

	body := `{"name": "reveal-batch", "quota": 10, "count": 2, "service_duration_days": 30, "reason": "test"}`
	resp := h.do("POST", "/admin/v1/cdk-batches", body, login)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusCreated {
		t.Fatalf("batch create failed: %d %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	codes, _ := data["codes"].([]any)
	if len(codes) != 2 {
		t.Fatalf("expected 2 codes, got %v", data)
	}

	resp = h.do("GET", "/admin/v1/cdks", "", login)
	_, payload = decodeEnvelope(t, resp)
	items, _ := payload["data"].(map[string]any)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 listed cdks, got %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	id, _ := first["id"].(string)
	if first["batch_name"] != "reveal-batch" {
		t.Fatalf("batch_name should join through, got %v", first["batch_name"])
	}
	if first["remark"] != "reveal-batch" {
		t.Fatalf("single-purpose batch should seed the remark, got %v", first["remark"])
	}

	resp = h.do("GET", "/admin/v1/cdks/"+id+"/code", "", login)
	status, payload = decodeEnvelope(t, resp)
	if status != http.StatusOK {
		t.Fatalf("reveal code failed: %d %v", status, payload)
	}
	revealed, _ := payload["data"].(map[string]any)["code"].(string)
	found := false
	for _, item := range codes {
		if s, _ := item.(string); s == revealed && revealed != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("revealed code %q not among generated codes %v", revealed, codes)
	}

	// Remark updates persist.
	resp = h.do("PATCH", "/admin/v1/cdks/"+id+"/remark", `{"remark": "渠道 A 专用"}`, login)
	if status, _ := decodeEnvelope(t, resp); status != http.StatusOK {
		t.Fatalf("remark update failed")
	}
	resp = h.do("GET", "/admin/v1/cdks", "", login)
	_, payload = decodeEnvelope(t, resp)
	items, _ = payload["data"].(map[string]any)["items"].([]any)
	for _, item := range items {
		row, _ := item.(map[string]any)
		if row["id"] == id && row["remark"] != "渠道 A 专用" {
			t.Fatalf("remark not persisted: %v", row)
		}
	}
}
