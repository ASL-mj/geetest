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

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/captchaflow/service-platform/api/internal/httpapi"
	"github.com/captchaflow/service-platform/api/internal/ratelimit"
	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/solver"
)

// queryHarness couples a solve-capable router with base session helpers.
type queryHarness struct {
	solve   *solveHarness
	base    *harness
	secrets map[string]string // secret by user label
	cookies map[string]*http.Cookie
}

// newQueryHarness wires the full solve stack for multi-user query tests.
func newQueryHarness(t *testing.T) *queryHarness {
	t.Helper()
	base := newHarness(t)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	limiter := ratelimit.NewFromClient(redisClient, testSettings)
	gateway := &simpleGateway{fn: func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	}}
	solveService := service.NewSolveService(base.services.Pool, gateway, limiter, testSettings.SessionSecret)
	handler := httpapi.NewRouter(base.services, solveService)
	base.handler = handler

	return &queryHarness{
		solve:   &solveHarness{h: base, handler: handler, gateway: gateway},
		base:    base,
		secrets: map[string]string{},
		cookies: map[string]*http.Cookie{},
	}
}

// addUser activates a fresh CDK and records the bearer secret and cookie.
func (q *queryHarness) addUser(label string) string {
	q.base.t.Helper()
	_, code := q.base.seedCDK(nil)
	resp := q.base.activate(code)
	status, payload := decodeEnvelope(q.base.t, resp)
	if status != http.StatusCreated {
		q.base.t.Fatalf("activation failed: %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	secret, _ := data["default_api_key"].(string)
	if secret == "" {
		q.base.t.Fatalf("no default key: %v", payload)
	}
	q.secrets[label] = secret
	q.cookies[label] = sessionCookie(q.base.t, resp)
	return secret
}

// sessionFor re-enters the user's CDK for a fresh session cookie. NewCDKs
// are one-per-user, so re-entry (200) keeps the same identity.
func (q *queryHarness) sessionFor(label, code string) *http.Cookie {
	q.base.t.Helper()
	resp := q.base.activate(code)
	status, payload := decodeEnvelope(q.base.t, resp)
	if status != http.StatusOK {
		q.base.t.Fatalf("re-entry failed: %d: %v", status, payload)
	}
	cookie := sessionCookie(q.base.t, resp)
	q.cookies[label] = cookie
	return cookie
}

// solveOnce performs one successful solve, returning its request_id.
func (q *queryHarness) solveOnce(label, idemKey string) string {
	q.base.t.Helper()
	resp := q.solve.post(q.secrets[label], idemKey, `{"captcha_id": "cap-q", "risk_type": "slide"}`)
	if resp.StatusCode != http.StatusOK {
		q.base.t.Fatalf("solve setup failed: %d", resp.StatusCode)
	}
	_, payload := decodeEnvelope(q.base.t, resp)
	requestID, _ := payload["request_id"].(string)
	return requestID
}

func TestAccountReturnsCDKSummary(t *testing.T) {
	q := newQueryHarness(t)
	q.addUser("alice")

	status, payload := decodeEnvelope(t, q.base.do("GET", "/v1/account", "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	cdk, _ := data["cdk"].(map[string]any)
	if cdk["status"] != "ACTIVE" {
		t.Fatalf("unexpected cdk summary: %v", cdk)
	}
	if cdk["quota_total"].(float64) != 100 || cdk["quota_remaining"].(float64) != 100 {
		t.Fatalf("quota fields wrong: %v", cdk)
	}
}

func TestAccountRequiresSession(t *testing.T) {
	q := newQueryHarness(t)
	status, payload := decodeEnvelope(t, q.base.do("GET", "/v1/account", ""))
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %v", status, payload)
	}
	expectError(t, payload, "SESSION_INVALID")
}

func TestUsageAggregatesCalls(t *testing.T) {
	q := newQueryHarness(t)
	q.addUser("alice")
	q.solveOnce("alice", "usage-1")
	q.solveOnce("alice", "usage-2")

	status, payload := decodeEnvelope(t, q.base.do("GET", "/v1/usage", "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	if data["calls_total"].(float64) != 2 || data["success_total"].(float64) != 2 {
		t.Fatalf("call counters wrong: %v", data)
	}
	if data["quota_used"].(float64) != 2 || data["quota_remaining"].(float64) != 98 {
		t.Fatalf("quota counters wrong: %v", data)
	}
	if rate, _ := data["success_rate"].(float64); rate < 0.999 || rate > 1.001 {
		t.Fatalf("success rate wrong: %v", data)
	}
}

func TestCallsListIsOwnerScopedAndPaginated(t *testing.T) {
	q := newQueryHarness(t)
	q.addUser("alice")
	q.addUser("bob")
	aliceCall := q.solveOnce("alice", "page-1")
	// Second alice call exercises pagination.
	q.solveOnce("alice", "page-2")
	bobCall := q.solveOnce("bob", "page-bob")

	status, payload := decodeEnvelope(t, q.base.do("GET", "/v1/calls?limit=1", "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item for alice page, got %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["request_id"] == bobCall {
		t.Fatal("alice page must not contain bob call")
	}
	if data["next_cursor"] == nil {
		t.Fatal("limit=1 over 2 alice calls must produce next_cursor")
	}
	serialized, _ := json.Marshal(first)
	if strings.Contains(string(serialized), "pass_token") || strings.Contains(string(serialized), "lot_number") {
		t.Fatalf("call log must be redacted: %s", serialized)
	}

	// Cursor pagination reaches the remaining page without duplicates.
	cursor, _ := data["next_cursor"].(string)
	status, payload = decodeEnvelope(t, q.base.do("GET", "/v1/calls?limit=1&cursor="+url.QueryEscape(cursor), "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("cursor page failed: %d: %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	items, _ = data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["request_id"] == first["request_id"] {
		t.Fatalf("cursor page must return the other call: %v", items)
	}

	// Ownership: alice cannot fetch bob's call; owner lookup succeeds.
	status, payload = decodeEnvelope(t, q.base.do("GET", "/v1/calls/"+bobCall, "", q.cookies["alice"]))
	if status != http.StatusNotFound {
		t.Fatalf("foreign request_id must 404, got %d: %v", status, payload)
	}
	expectError(t, payload, "CALL_NOT_FOUND")

	status, payload = decodeEnvelope(t, q.base.do("GET", "/v1/calls/"+aliceCall, "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("owner lookup failed: %d: %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	if data["request_id"] != aliceCall {
		t.Fatalf("detail mismatch: %v", data)
	}
}

func TestCallsPaginationDoesNotSkipSameTimestampRecords(t *testing.T) {
	q := newQueryHarness(t)
	q.addUser("alice")
	caller, appErr := q.base.services.AuthenticateAPIKey(context.Background(), q.secrets["alice"])
	if appErr != nil {
		t.Fatalf("authenticate setup key: %v", appErr)
	}

	acceptedAt := time.Date(2026, time.January, 2, 3, 4, 5, 6000000, time.UTC)
	for i := range 3 {
		if _, err := q.base.services.Pool.Exec(context.Background(), `
			INSERT INTO api_calls (
				id, request_id, operation, idempotency_key_hash, user_id, cdk_id, api_key_id,
				api_key_name_snapshot, api_key_prefix_snapshot, captcha_id, risk_type,
				status, http_status, quota_reserved, quota_refunded,
				client_ip_masked, client_ip_hash, user_agent, accepted_at
			) VALUES (
				$1, $2, 'captcha.solve', $3, $4, $5, $6,
				$7, $8, $9, 'slide', 'SUCCEEDED', 200, false, false,
				'192.0.2.0/24', $10, 'pagination-test', $11
			)
		`, uuid.New(), fmt.Sprintf("req_tie_%d", i), []byte(fmt.Sprintf("tie-%d", i)),
			caller.UserID, caller.CDKID, caller.APIKeyID, caller.APIKey.Name, caller.APIKey.KeyPrefix,
			fmt.Sprintf("cap-tie-%d", i), []byte("test-ip-hash"), acceptedAt); err != nil {
			t.Fatalf("seed same-timestamp call %d: %v", i, err)
		}
	}

	status, payload := decodeEnvelope(t, q.base.do("GET", "/v1/calls?limit=1", "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("first page failed: %d %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	firstItems, _ := data["items"].([]any)
	if len(firstItems) != 1 {
		t.Fatalf("expected one first-page record, got %v", firstItems)
	}
	cursor, _ := data["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("same-timestamp records must produce a next cursor")
	}

	status, payload = decodeEnvelope(t, q.base.do("GET", "/v1/calls?limit=1&cursor="+url.QueryEscape(cursor), "", q.cookies["alice"]))
	if status != http.StatusOK {
		t.Fatalf("second page failed: %d %v", status, payload)
	}
	data, _ = payload["data"].(map[string]any)
	secondItems, _ := data["items"].([]any)
	if len(secondItems) != 1 {
		t.Fatalf("same-timestamp cursor must not skip the next record: %v", secondItems)
	}
	if firstItems[0].(map[string]any)["request_id"] == secondItems[0].(map[string]any)["request_id"] {
		t.Fatalf("same-timestamp cursor must not duplicate the first record: %v", secondItems)
	}
}

func TestCallsListRejectsBadLimit(t *testing.T) {
	q := newQueryHarness(t)
	q.addUser("alice")

	status, payload := decodeEnvelope(t, q.base.do("GET", "/v1/calls?limit=zero", "", q.cookies["alice"]))
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %v", status, payload)
	}
	expectError(t, payload, "INVALID_REQUEST")
}
