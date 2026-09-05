package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/captchaflow/service-platform/api/internal/config"
	"github.com/captchaflow/service-platform/api/internal/httpapi"
	"github.com/captchaflow/service-platform/api/internal/ratelimit"
	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/solver"
)

// fakeGateway is a scriptable solver gateway counting invocations.
type simpleGateway struct {
	calls atomic.Int32
	fn    func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error)
}

func (g *simpleGateway) Solve(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
	g.calls.Add(1)
	return g.fn(ctx, requestID, req)
}

func okResult(req solver.SolveRequest) *solver.SolveResult {
	return &solver.SolveResult{
		CaptchaID:     req.CaptchaID,
		LotNumber:     "lot-1",
		CaptchaOutput: "out-1",
		PassToken:     "token-1",
		GenTime:       "1700000000",
	}
}

// solveHarness wires the full solve stack with a scriptable gateway.
type solveHarness struct {
	h          *harness
	handler    http.Handler
	gateway    *simpleGateway
	bearerConf string
}

func newSolveHarness(t *testing.T, solverFn func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error)) *solveHarness {
	t.Helper()
	base := newHarness(t)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	limiter := ratelimit.NewFromClient(redisClient, testSettings)
	gateway := &simpleGateway{fn: solverFn}
	solveService := service.NewSolveService(base.services.Pool, gateway, limiter, testSettings.SessionSecret)
	handler := httpapi.NewRouter(base.services, solveService)
	return &solveHarness{h: base, handler: handler, gateway: gateway}
}

// activateUser activates a fresh CDK and returns the bearer secret.
func (s *solveHarness) activateUser() string {
	s.h.t.Helper()
	_, code := s.h.seedCDK(nil)
	resp := s.h.activate(code)
	if resp.StatusCode != http.StatusCreated {
		s.h.t.Fatalf("activation failed: %d", resp.StatusCode)
	}
	_, payload := decodeEnvelope(s.h.t, resp)
	data, _ := payload["data"].(map[string]any)
	secret, _ := data["default_api_key"].(string)
	if secret == "" {
		s.h.t.Fatalf("no default key: %v", payload)
	}
	return secret
}

func (s *solveHarness) post(secret, idempotencyKey, body string) *http.Response {
	s.h.t.Helper()
	req := httptest.NewRequest("POST", "/v1/captcha/solve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, req)
	return recorder.Result()
}

func quotaOf(t *testing.T, h *harness, code string) (remaining, used, reserved int64) {
	t.Helper()
	err := h.services.Pool.QueryRow(context.Background(),
		`SELECT quota_remaining, quota_used, quota_reserved FROM cdks WHERE code_prefix = $1`,
		code[:8]).Scan(&remaining, &used, &reserved)
	if err != nil {
		t.Fatalf("quota lookup: %v", err)
	}
	return remaining, used, reserved
}

func ledgerCount(t *testing.T, h *harness, entryType string) int {
	t.Helper()
	var count int
	if err := h.services.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM quota_ledger WHERE entry_type = $1`, entryType).Scan(&count); err != nil {
		t.Fatalf("ledger count: %v", err)
	}
	return count
}

func TestSolveSuccessSettlesQuota(t *testing.T) {
	h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	})
	secret := h.activateUser()

	resp := h.post(secret, "idem-success-1", `{"captcha_id": "cap-1", "risk_type": "slide"}`)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, payload)
	}
	data, _ := payload["data"].(map[string]any)
	if data["lot_number"] != "lot-1" || data["pass_token"] != "token-1" {
		t.Fatalf("unexpected solve data: %v", data)
	}
	if requestID, _ := payload["request_id"].(string); !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("missing request_id: %v", payload)
	}
	if h.gateway.calls.Load() != 1 {
		t.Fatalf("solver must be called once, got %d", h.gateway.calls.Load())
	}

	// Quota moved from remaining to used; nothing left reserved.
	var remaining, used, reserved int64
	if err := h.h.services.Pool.QueryRow(context.Background(),
		`SELECT quota_remaining, quota_used, quota_reserved FROM cdks WHERE quota_used > 0`).
		Scan(&remaining, &used, &reserved); err != nil {
		t.Fatalf("quota lookup: %v", err)
	}
	if remaining != 99 || used != 1 || reserved != 0 {
		t.Fatalf("quota counters wrong: remaining=%d used=%d reserved=%d", remaining, used, reserved)
	}
	var keyLastUsed *time.Time
	if err := h.h.services.Pool.QueryRow(context.Background(), `SELECT last_used_at FROM api_keys WHERE total_calls = 1 LIMIT 1`).Scan(&keyLastUsed); err != nil {
		t.Fatalf("api key last-used lookup: %v", err)
	}
	if keyLastUsed == nil {
		t.Fatal("successful solve must update api key last_used_at")
	}
	if got := ledgerCount(t, h.h, "RESERVE"); got != 1 {
		t.Fatalf("expected 1 RESERVE, got %d", got)
	}
	if got := ledgerCount(t, h.h, "CONFIRM"); got != 1 {
		t.Fatalf("expected 1 CONFIRM, got %d", got)
	}
}

func TestSolveIdempotentReplaySolvesOnce(t *testing.T) {
	h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	})
	secret := h.activateUser()

	first := h.post(secret, "idem-dup", `{"captcha_id": "cap-1", "risk_type": "slide"}`)
	second := h.post(secret, "idem-dup", `{"captcha_id": "cap-1", "risk_type": "slide"}`)

	_, firstPayload := decodeEnvelope(t, first)
	status, secondPayload := decodeEnvelope(t, second)
	if status != http.StatusOK {
		t.Fatalf("replay expected 200, got %d: %v", status, secondPayload)
	}
	if firstPayload["request_id"] != secondPayload["request_id"] {
		t.Fatalf("replay must return the original request_id: %v vs %v", firstPayload["request_id"], secondPayload["request_id"])
	}
	if firstPayload["data"] == nil || !reflect.DeepEqual(firstPayload["data"], secondPayload["data"]) {
		t.Fatalf("replay must return the original response body: %v vs %v", firstPayload["data"], secondPayload["data"])
	}
	if h.gateway.calls.Load() != 1 {
		t.Fatalf("duplicate idempotency key must solve once, got %d", h.gateway.calls.Load())
	}
	// Only one reserve and one confirm despite two requests.
	if got := ledgerCount(t, h.h, "RESERVE"); got != 1 {
		t.Fatalf("expected 1 RESERVE, got %d", got)
	}
	if got := ledgerCount(t, h.h, "CONFIRM"); got != 1 {
		t.Fatalf("expected 1 CONFIRM, got %d", got)
	}
}

func TestSolveQuotaExhaustedNeverReachesSolver(t *testing.T) {
	h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	})
	secret := h.activateUser()
	if _, err := h.h.services.Pool.Exec(context.Background(), `UPDATE cdks SET quota_remaining = 0`); err != nil {
		t.Fatal(err)
	}

	resp := h.post(secret, "idem-exhausted", `{"captcha_id": "cap-1", "risk_type": "slide"}`)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %v", status, payload)
	}
	expectError(t, payload, "QUOTA_EXHAUSTED")
	if h.gateway.calls.Load() != 0 {
		t.Fatalf("exhausted CDK must not reach the solver, got %d calls", h.gateway.calls.Load())
	}
}

func TestSolveRefundsExactlyOnceOnSolverFailure(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		solverErr   error
		wantStatus  int
		wantErrCode string
	}{
		{"solver 502", solver.ErrSolver5xx, http.StatusBadGateway, "SOLVER_FAILED"},
		{"solver timeout", solver.ErrTimeout, http.StatusGatewayTimeout, "SOLVER_TIMEOUT"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
				return nil, testCase.solverErr
			})
			secret := h.activateUser()

			resp := h.post(secret, "idem-fail-"+testCase.name, `{"captcha_id": "cap-1", "risk_type": "slide"}`)
			status, payload := decodeEnvelope(t, resp)
			if status != testCase.wantStatus {
				t.Fatalf("expected %d, got %d: %v", testCase.wantStatus, status, payload)
			}
			expectError(t, payload, testCase.wantErrCode)

			// Refund restores remaining, drains reserved, and is recorded once.
			remaining, used, reserved := 0, 0, 0
			if err := h.h.services.Pool.QueryRow(context.Background(),
				`SELECT quota_remaining, quota_used, quota_reserved FROM cdks WHERE quota_used >= 0 LIMIT 1`).
				Scan(&remaining, &used, &reserved); err != nil {
				t.Fatal(err)
			}
			if remaining != 100 || reserved != 0 {
				t.Fatalf("refund incomplete: remaining=%d reserved=%d", remaining, reserved)
			}
			if got := ledgerCount(t, h.h, "REFUND"); got != 1 {
				t.Fatalf("expected exactly 1 REFUND, got %d", got)
			}
			if got := ledgerCount(t, h.h, "CONFIRM"); got != 0 {
				t.Fatalf("failed call must not confirm, got %d", got)
			}

			var refunded bool
			if err := h.h.services.Pool.QueryRow(context.Background(),
				`SELECT quota_refunded FROM api_calls WHERE status = 'FAILED_REFUNDED'`).Scan(&refunded); err != nil {
				t.Fatalf("no FAILED_REFUNDED record: %v", err)
			}
			if !refunded {
				t.Fatal("api_calls must record the refund")
			}
		})
	}
}

func TestSolveNilSolverResultRefundsQuota(t *testing.T) {
	h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return nil, nil
	})
	secret := h.activateUser()

	resp := h.post(secret, "idem-nil-result", `{"captcha_id":"cap-1","risk_type":"slide"}`)
	status, payload := decodeEnvelope(t, resp)
	if status != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %v", status, payload)
	}
	expectError(t, payload, "SOLVER_FAILED")
	errBody, _ := payload["error"].(map[string]any)
	if retryable, _ := errBody["retryable"].(bool); !retryable {
		t.Fatalf("nil solver result should be retryable: %v", errBody)
	}
	var remaining, used, reserved int64
	if err := h.h.services.Pool.QueryRow(context.Background(),
		`SELECT quota_remaining, quota_used, quota_reserved FROM cdks LIMIT 1`).Scan(&remaining, &used, &reserved); err != nil {
		t.Fatalf("quota lookup: %v", err)
	}
	if remaining != 100 || used != 0 || reserved != 0 {
		t.Fatalf("nil result must refund exactly once: remaining=%d used=%d reserved=%d", remaining, used, reserved)
	}
	if got := ledgerCount(t, h.h, "REFUND"); got != 1 {
		t.Fatalf("expected exactly one refund, got %d", got)
	}
}

func TestSolveRateLimitedWritesNoLedger(t *testing.T) {
	base := newHarness(t)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	tightSettings := testSettings
	tightSettings.RateLimitPerMinute = 1
	limiter := ratelimit.NewFromClient(redisClient, tightSettings)
	gateway := &simpleGateway{fn: func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	}}
	solveService := service.NewSolveService(base.services.Pool, gateway, limiter, testSettings.SessionSecret)
	handler := httpapi.NewRouter(base.services, solveService)

	_, code := base.seedCDK(nil)
	activateResp := base.activate(code)
	if activateResp.StatusCode != http.StatusCreated {
		t.Fatalf("activation failed: %d", activateResp.StatusCode)
	}
	_, payload := decodeEnvelope(t, activateResp)
	data, _ := payload["data"].(map[string]any)
	secret, _ := data["default_api_key"].(string)

	post := func(idem string) *http.Response {
		req := httptest.NewRequest("POST", "/v1/captcha/solve", strings.NewReader(`{"captcha_id": "c", "risk_type": "slide"}`))
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Idempotency-Key", idem)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result()
	}

	first := post("idem-r1")
	if status, _ := decodeEnvelope(t, first); status != http.StatusOK {
		t.Fatalf("first call must pass, got %d", first.StatusCode)
	}
	second := post("idem-r2")
	status, payload := decodeEnvelope(t, second)
	if status != http.StatusTooManyRequests {
		t.Fatalf("second call must be rate limited, got %d: %v", status, payload)
	}
	expectError(t, payload, "RATE_LIMITED")
	if errBody, _ := payload["error"].(map[string]any); errBody == nil || errBody["retryable"] != true {
		t.Fatalf("rate limit must be marked retryable: %v", payload)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
	if got := ledgerCount(t, base, "RESERVE"); got != 1 {
		t.Fatalf("rejected call must not touch the ledger, RESERVE count %d", got)
	}
}

func TestSolveConcurrentHonorsRemainingQuota(t *testing.T) {
	h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	})
	secret := h.activateUser()
	// Quota 2 for 3 concurrent calls: exactly two may reach the solver.
	if _, err := h.h.services.Pool.Exec(context.Background(), `UPDATE cdks SET quota_remaining = 2, quota_total = 2`); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	statuses := make([]int, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp := h.post(secret, fmt.Sprintf("idem-conc-%d", i), `{"captcha_id": "c", "risk_type": "slide"}`)
			statuses[i] = resp.StatusCode
		}(i)
	}
	wg.Wait()

	successes := 0
	paymentRequired := 0
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusPaymentRequired:
			paymentRequired++
		}
	}
	if successes != 2 || paymentRequired != 1 {
		t.Fatalf("expected 2 success + 1 quota rejected, got %v", statuses)
	}
	if calls := h.gateway.calls.Load(); calls != 2 {
		t.Fatalf("solver must see exactly 2 calls, got %d", calls)
	}
}

func TestSolveValidatesRequestAndAuth(t *testing.T) {
	h := newSolveHarness(t, func(ctx context.Context, requestID string, req solver.SolveRequest) (*solver.SolveResult, error) {
		return okResult(req), nil
	})
	secret := h.activateUser()

	cases := []struct {
		name       string
		secret     string
		idem       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"missing bearer", "", "idem-a", `{"captcha_id": "c", "risk_type": "slide"}`, 401, "API_KEY_INVALID"},
		{"bad bearer", "cf_live_wrong", "idem-b", `{"captcha_id": "c", "risk_type": "slide"}`, 401, "API_KEY_INVALID"},
		{"missing idempotency", secret, "", `{"captcha_id": "c", "risk_type": "slide"}`, 422, "INVALID_REQUEST"},
		{"empty captcha id", secret, "idem-c", `{"captcha_id": "", "risk_type": "slide"}`, 422, "INVALID_REQUEST"},
		{"wrong risk type", secret, "idem-d", `{"captcha_id": "c", "risk_type": "recaptcha"}`, 422, "INVALID_REQUEST"},
		{"bad json", secret, "idem-e", `{not-json`, 422, "INVALID_REQUEST"},
		{"trailing json", secret, "idem-f", `{"captcha_id":"c","risk_type":"slide"}{}`, 422, "INVALID_REQUEST"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resp := h.post(testCase.secret, testCase.idem, testCase.body)
			status, payload := decodeEnvelope(t, resp)
			if status != testCase.wantStatus {
				t.Fatalf("expected %d, got %d: %v", testCase.wantStatus, status, payload)
			}
			expectError(t, payload, testCase.wantCode)
		})
	}
}

// config import guard for testSettings typing.
var _ = config.EnvTest
