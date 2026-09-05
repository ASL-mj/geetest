package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/domain"
	"github.com/captchaflow/service-platform/api/internal/store"
)

func newTestUUID(t *testing.T) uuid.UUID {
	t.Helper()
	return uuid.New()
}

const (
	adminViewerUser = "viewer-test"
	adminPassword   = "correct-horse-battery"
)

// seedAdmin inserts an operator directly and returns its username.
func seedAdmin(t *testing.T, h *harness, username, role string) string {
	t.Helper()
	hash, err := crypto.HashPassword(adminPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := store.UpsertAdmin(context.Background(), h.services.Pool, store.AdminUser{
		ID:           newTestUUID(t),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return username
}

func adminLogin(t *testing.T, h *harness, username string) *http.Cookie {
	t.Helper()
	resp := h.do("POST", "/admin/v1/auth/login", fmt.Sprintf(`{"username": %q, "password": %q}`, username, adminPassword))
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusOK {
		t.Fatalf("admin login failed: %d: %v", status, payload)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "admin_session" {
			return cookie
		}
	}
	t.Fatal("admin_session cookie missing")
	return nil
}

func TestAdminLoginRejectsWrongPassword(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "boss", store.AdminRoleAdmin)

	resp := base.do("POST", "/admin/v1/auth/login", `{"username": "boss", "password": "nope"}`)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %v", status, payload)
	}
	expectError(t, payload, "ADMIN_AUTH_FAILED")
}

func TestAdminSurfaceRejectsUserSessionsAndAnons(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "boss", store.AdminRoleAdmin)

	status, payload := decodeEnvelope(t, base.do("GET", "/admin/v1/dashboard", ""))
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous must 401, got %d: %v", status, payload)
	}
	expectError(t, payload, "ADMIN_SESSION_INVALID")

	// A valid USER session must not open the admin surface.
	_, code := base.seedCDK(nil)
	userCookie := sessionCookie(t, base.activate(code))
	status, payload = decodeEnvelope(t, base.do("GET", "/admin/v1/dashboard", "", userCookie))
	if status != http.StatusUnauthorized {
		t.Fatalf("user session must 401 on admin surface, got %d: %v", status, payload)
	}
}

func TestViewerCannotWrite(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, adminViewerUser, store.AdminRoleViewer)
	viewer := adminLogin(t, base, adminViewerUser)

	// Reads succeed.
	status, payload := decodeEnvelope(t, base.do("GET", "/admin/v1/dashboard", "", viewer))
	if status != http.StatusOK {
		t.Fatalf("viewer read failed: %d: %v", status, payload)
	}

	// Writes are forbidden.
	body := `{"name": "batch-1", "quota": 10, "count": 1, "reason": "test"}`
	status, payload = decodeEnvelope(t, base.do("POST", "/admin/v1/cdk-batches", body, viewer))
	if status != http.StatusForbidden {
		t.Fatalf("viewer batch create must 403, got %d: %v", status, payload)
	}
	expectError(t, payload, "ADMIN_FORBIDDEN")
}

func TestAdminBatchCreationAndQuotaAdjustmentAreAudited(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "boss", store.AdminRoleAdmin)
	admin := adminLogin(t, base, "boss")

	// Batch generation returns plaintext codes exactly once.
	body := `{"name": "e2e-batch", "quota": 25, "count": 3, "service_duration_days": 30, "reason": "reseller order 7"}`
	status, payload := decodeEnvelope(t, base.do("POST", "/admin/v1/cdk-batches", body, admin))
	if status != http.StatusCreated {
		t.Fatalf("batch create failed: %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	_, _ = data["batch_id"]
	codes, _ := data["codes"].([]any)
	if len(codes) != 3 {
		t.Fatalf("expected 3 codes, got %v", codes)
	}

	// Only hashes persist; the plaintext must not appear in code_prefix.
	var matches int
	if err := base.services.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cdks WHERE code_hash = convert_to($1, 'UTF8')`, codes[0].(string)).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if matches != 0 {
		t.Fatal("plaintext CDK code must never be stored")
	}

	// Quota adjustment on an unactivated CDK from the batch.
	cdks, err := store.ListCdks(context.Background(), base.services.Pool, nil, "UNACTIVATED", 100)
	if err != nil || len(cdks) < 3 {
		t.Fatalf("cdk list failed: %v (%d)", err, len(cdks))
	}
	cdkID := cdks[0].ID
	adjust := fmt.Sprintf(`{"delta": 100, "reason": "support case 42"}`)
	status, payload = decodeEnvelope(t, base.do("POST", fmt.Sprintf("/admin/v1/cdks/%s/quota-adjustments", cdkID), adjust, admin))
	if status != http.StatusCreated {
		t.Fatalf("adjustment failed: %d: %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	if data["quota_total"].(float64) != 125 || data["quota_remaining"].(float64) != 125 {
		t.Fatalf("adjustment counters wrong: %v", data)
	}

	// Missing reason is rejected.
	status, _ = decodeEnvelope(t, base.do("POST", fmt.Sprintf("/admin/v1/cdks/%s/quota-adjustments", cdkID), `{"delta": 1}`, admin))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("missing reason must 422, got %d", status)
	}
	// A negative adjustment may not consume more than the available quota.
	status, payload = decodeEnvelope(t, base.do("POST", fmt.Sprintf("/admin/v1/cdks/%s/quota-adjustments", cdkID), `{"delta": -1000, "reason": "invalid correction"}`, admin))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("oversized negative adjustment must 422, got %d: %v", status, payload)
	}
	expectError(t, payload, "QUOTA_ADJUSTMENT_INVALID")

	// The adjustment produced both a ledger row and an audit entry.
	var entryType string
	if err := base.services.Pool.QueryRow(context.Background(),
		`SELECT entry_type FROM quota_ledger WHERE entry_type = 'ADMIN_ADJUSTMENT' LIMIT 1`).Scan(&entryType); err != nil {
		t.Fatalf("ledger row missing: %v", err)
	}
	status, payload = decodeEnvelope(t, base.do("GET", "/admin/v1/audit-logs", "", admin))
	if status != http.StatusOK {
		t.Fatalf("audit list failed: %d: %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	items, _ := data["items"].([]any)
	found := false
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if entry["action"] == "cdk.quota_adjusted" && entry["reason"] == "support case 42" {
			found = true
		}
	}
	if !found {
		t.Fatalf("quota adjustment audit entry missing: %v", items)
	}
}

func TestAdminDisablesCdkAndUser(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "boss", store.AdminRoleAdmin)
	admin := adminLogin(t, base, "boss")

	_, code := base.seedCDK(nil)
	userResp := base.activate(code)
	userCookie := sessionCookie(t, userResp)
	status, payload := decodeEnvelope(t, userResp)
	data, _ := payload["data"].(map[string]any)
	_ = status
	userID, _ := data["user"].(map[string]any)["id"].(string)

	// Disable the CDK.
	var cdkID string
	if err := base.services.Pool.QueryRow(context.Background(),
		`SELECT id FROM cdks WHERE code_prefix = $1`, code[:8]).Scan(&cdkID); err != nil {
		t.Fatal(err)
	}
	disable := `{"status": "DISABLED", "reason": "abuse report"}`
	status, payload = decodeEnvelope(t, base.do("PATCH", "/admin/v1/cdks/"+cdkID, disable, admin))
	if status != http.StatusOK {
		t.Fatalf("cdk disable failed: %d: %v", status, payload)
	}

	// Suspend the user; their session stops resolving.
	suspend := `{"status": "SUSPENDED", "reason": "chargeback"}`
	status, payload = decodeEnvelope(t, base.do("PATCH", "/admin/v1/users/"+userID, suspend, admin))
	if status != http.StatusOK {
		t.Fatalf("user suspend failed: %d: %v", status, payload)
	}
	status, payload = decodeEnvelope(t, base.do("GET", "/v1/account", "", userCookie))
	if status == http.StatusOK {
		t.Fatalf("suspended user session must stop resolving: %d %v", status, payload)
	}
}

func TestAdminEnableExpiredUnboundCdkKeepsExpiredStatus(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "expiry-admin", store.AdminRoleAdmin)
	admin := adminLogin(t, base, "expiry-admin")
	past := time.Now().UTC().Add(-time.Hour)
	cdk, _ := base.seedCDK(func(c *domain.Cdk) {
		c.Status = domain.CDKStatusDisabled
		c.ActivationDeadline = &past
	})

	status, payload := decodeEnvelope(t, base.do("PATCH", fmt.Sprintf("/admin/v1/cdks/%s", cdk.ID), `{"status":"ACTIVE","reason":"reopen review"}`, admin))
	if status != http.StatusOK {
		t.Fatalf("enable expired cdk failed: %d %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	if data["status"] != "EXPIRED" {
		t.Fatalf("expired unbound cdk must remain expired when enabled: %v", data)
	}
}

func TestAdminQuerySurfacesFilterAndRedactData(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "query-admin", store.AdminRoleAdmin)
	admin := adminLogin(t, base, "query-admin")
	_, code := base.seedCDK(nil)
	activation := base.activate(code)
	status, payload := decodeEnvelope(t, activation)
	if status != http.StatusCreated {
		t.Fatalf("activation failed: %d %v", status, payload)
	}
	user, _ := payload["data"].(map[string]any)
	userID, _ := uuid.Parse(user["user"].(map[string]any)["id"].(string))
	created, appErr := base.services.CreateAPIKey(context.Background(), userID, "worker")
	if appErr != nil {
		t.Fatalf("create second key: %v", appErr)
	}
	var cdkID uuid.UUID
	if err := base.services.Pool.QueryRow(context.Background(), `SELECT id FROM cdks WHERE bound_user_id = $1`, userID).Scan(&cdkID); err != nil {
		t.Fatalf("load cdk: %v", err)
	}
	callID := uuid.New()
	acceptedAt := time.Now().UTC().Add(-time.Minute)
	_, err := base.services.Pool.Exec(context.Background(), `
		INSERT INTO api_calls (
			id, request_id, operation, idempotency_key_hash, user_id, cdk_id, api_key_id,
			api_key_name_snapshot, api_key_prefix_snapshot, captcha_id, risk_type,
			status, http_status, error_code, error_summary, accepted_at, completed_at,
			duration_ms, quota_reserved, quota_refunded, client_ip_masked, client_ip_hash, user_agent
		) VALUES ($1, 'req_admin_query_1', 'captcha.solve', $2, $3, $4, $5,
			'worker', $6, 'cap-admin', 'slide', 'FAILED_REFUNDED', 502,
			'SOLVER_FAILED', 'downstream failed', $7, $7, 83, true, true,
			'192.0.2.0/24', $8, 'admin-query-test')
	`, callID, []byte("admin-query-idem"), userID, cdkID, created.Record.ID,
		created.Record.KeyPrefix, acceptedAt, []byte("ip-hash"))
	if err != nil {
		t.Fatalf("insert call: %v", err)
	}
	for _, entryType := range []string{"RESERVE", "REFUND"} {
		_, err = base.services.Pool.Exec(context.Background(), `
		INSERT INTO quota_ledger (
			id, cdk_id, user_id, api_call_id, entry_type,
			available_before, delta_available, available_after,
			used_before, used_after, reserved_before, reserved_after,
			reason, request_id, actor_type, created_at
		) VALUES ($1, $2, $3, $4, $5, 100, $6, $7, 0, 0, 0, 0, $8, 'req_admin_query_1', 'user', $9)
	`, uuid.New(), cdkID, userID, callID, entryType,
			map[string]int64{"RESERVE": -1, "REFUND": 1}[entryType],
			map[string]int64{"RESERVE": 99, "REFUND": 100}[entryType],
			"query test "+entryType, acceptedAt)
		if err != nil {
			t.Fatalf("insert ledger %s: %v", entryType, err)
		}
	}

	status, payload = decodeEnvelope(t, base.do("GET", fmt.Sprintf("/admin/v1/api-keys?user_id=%s&status=ACTIVE&limit=1", userID), "", admin))
	if status != http.StatusOK {
		t.Fatalf("admin key query failed: %d %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 1 || data["next_cursor"] == nil {
		t.Fatalf("key query must paginate active keys: %v", data)
	}
	serialized, _ := json.Marshal(items[0])
	if strings.Contains(string(serialized), created.Secret) || strings.Contains(string(serialized), "key_hash") {
		t.Fatalf("admin key query leaked secret material: %s", serialized)
	}

	from := url.QueryEscape(acceptedAt.Add(-time.Minute).Format(time.RFC3339Nano))
	to := url.QueryEscape(acceptedAt.Add(time.Minute).Format(time.RFC3339Nano))
	status, payload = decodeEnvelope(t, base.do("GET", fmt.Sprintf("/admin/v1/calls?user_id=%s&cdk_id=%s&api_key_id=%s&captcha_id=cap-admin&status=FAILED_REFUNDED&from=%s&to=%s", userID, cdkID, created.Record.ID, from, to), "", admin))
	if status != http.StatusOK {
		t.Fatalf("admin call query failed: %d %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	items, _ = data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["error_summary"] != "downstream failed" {
		t.Fatalf("call filter or error summary missing: %v", data)
	}
	serialized, _ = json.Marshal(items[0])
	if strings.Contains(string(serialized), "response_json") || strings.Contains(string(serialized), "pass_token") {
		t.Fatalf("admin call query leaked sensitive response data: %s", serialized)
	}

	status, payload = decodeEnvelope(t, base.do("GET", fmt.Sprintf("/admin/v1/quota-ledger?cdk_id=%s&user_id=%s&entry_type=REFUND&request_id=req_admin_query_1&from=%s&to=%s", cdkID, userID, from, to), "", admin))
	if status != http.StatusOK {
		t.Fatalf("admin ledger query failed: %d %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	items, _ = data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["delta_available"].(float64) != 1 {
		t.Fatalf("ledger filter mismatch: %v", data)
	}
}

func TestAdminSolverHealthReportsSanitizedStatus(t *testing.T) {
	base := newHarness(t)
	seedAdmin(t, base, "boss", store.AdminRoleAdmin)
	admin := adminLogin(t, base, "boss")

	status, payload := decodeEnvelope(t, base.do("GET", "/admin/v1/solver-health", "", admin))
	if status != http.StatusOK {
		t.Fatalf("solver health failed: %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	healthStatus, _ := data["status"].(string)
	if healthStatus == "" {
		t.Fatalf("status missing: %v", data)
	}
	// The test settings point at https://solver.test, which cannot resolve;
	// the endpoint must report a sanitized failure, never the URL or key.
	serialized := fmt.Sprintf("%v", data)
	if strings.Contains(serialized, "solver.test") || strings.Contains(serialized, "X-Service-Key") {
		t.Fatalf("solver health leaked configuration: %s", serialized)
	}
	if healthStatus == "UNREACHABLE" && data["failure"] == "" {
		t.Fatalf("unreachable status requires failure category: %v", data)
	}
}
