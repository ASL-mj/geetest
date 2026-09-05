// Package integration runs the full HTTP stack against a real PostgreSQL.
// Set TEST_DATABASE_URL to an empty database to enable the suite; tests are
// skipped otherwise, mirroring the Python conftest behavior.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/config"
	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/domain"
	"github.com/captchaflow/service-platform/api/internal/httpapi"
	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/store"
)

var testSettings config.Settings

func TestMain(m *testing.M) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		fmt.Println("TEST_DATABASE_URL not set; skipping integration suite")
		os.Exit(0)
	}

	settings := config.Settings{
		Environment:          config.EnvTest,
		DatabaseURL:          databaseURL,
		RedisURL:             "redis://localhost:6379/0",
		SessionSecret:        "test-session-secret",
		SessionTTL:           time.Hour,
		APIKeyPepper:         "test-api-key-pepper",
		CDKPepper:            "test-cdk-pepper",
		GeetestSolverURL:     "https://solver.test",
		GeetestServiceAPIKey: "test-solver-key",
		SolverConnectTimeout: 2 * time.Second,
		SolverReadTimeout:    2 * time.Second,
		SolverTotalTimeout:   2 * time.Second,
		RateLimitPerMinute:   60,
		ConcurrencyLimit:     4,
		AdminSessionTTL:      time.Hour,
	}

	ctx := context.Background()
	pool, err := store.Connect(ctx, settings.DatabaseURL)
	if err != nil {
		fmt.Printf("connect test database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// The suite expects an empty database; refuse to run on unknown schemas.
	var tableCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tableCount); err != nil {
		fmt.Printf("inspect test database: %v\n", err)
		os.Exit(1)
	}
	if tableCount == 0 {
		if err := store.Migrate(ctx, pool); err != nil {
			fmt.Printf("migrate test database: %v\n", err)
			os.Exit(1)
		}
	}

	testSettings = settings
	os.Exit(m.Run())
}

// harness wires one HTTP handler onto the shared test database.
type harness struct {
	t        *testing.T
	handler  http.Handler
	services *service.Services
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL must point to an empty PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := store.Connect(ctx, testSettings.DatabaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	// Tests assert on whole-table counts, so clear the mutable tables for
	// isolation between tests and across suite runs.
	if _, err := pool.Exec(ctx, `TRUNCATE quota_ledger, api_calls, cdks, cdk_batches, api_keys, users, user_sessions, system_settings CASCADE`); err != nil {
		t.Fatalf("reset tables: %v", err)
	}
	services := service.NewServices(pool, testSettings)
	return &harness{t: t, handler: httpapi.NewRouter(services, nil), services: services}
}

// seedCDK inserts a CDK with optional field overrides and returns its code.
func (h *harness) seedCDK(mutate func(*domain.Cdk)) (domain.Cdk, string) {
	h.t.Helper()
	ctx := context.Background()
	code := "TEST" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	hash := crypto.HMACSHA256(code, h.services.Settings.CDKPepper)

	batchID := uuid.New()
	if _, err := h.services.Pool.Exec(ctx, `
		INSERT INTO cdk_batches (id, name, default_quota, service_duration_days)
		VALUES ($1, $2, 100, 365)
	`, batchID, "batch-"+uuid.NewString()); err != nil {
		h.t.Fatalf("seed batch: %v", err)
	}

	cdk := domain.Cdk{
		ID:             uuid.New(),
		BatchID:        batchID,
		CodePrefix:     code[:8],
		CodeHash:       hash,
		Status:         domain.CDKStatusUnactivated,
		QuotaTotal:     100,
		QuotaRemaining: 100,
	}
	if mutate != nil {
		mutate(&cdk)
	}
	if _, err := h.services.Pool.Exec(ctx, `
		INSERT INTO cdks (id, batch_id, code_prefix, code_hash, status, activation_deadline,
		                  expires_at, quota_total, quota_used, quota_reserved, quota_remaining)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 0, 0, $9)
	`, cdk.ID, cdk.BatchID, cdk.CodePrefix, cdk.CodeHash, string(cdk.Status),
		cdk.ActivationDeadline, cdk.ExpiresAt, cdk.QuotaTotal, cdk.QuotaRemaining); err != nil {
		h.t.Fatalf("seed cdk: %v", err)
	}
	return cdk, code
}

func (h *harness) do(method, path, body string, cookies ...*http.Cookie) *http.Response {
	h.t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, req)
	return recorder.Result()
}

func (h *harness) activate(code string, cookies ...*http.Cookie) *http.Response {
	return h.do("POST", "/v1/auth/activate", fmt.Sprintf(`{"cdk": %q}`, code), cookies...)
}

// sessionCookie extracts the session cookie from a response.
func sessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session" {
			return cookie
		}
	}
	t.Fatal("session cookie missing from response")
	return nil
}

// decodeEnvelope reads the platform envelope and asserts success flag.
func decodeEnvelope(t *testing.T, resp *http.Response) (int, map[string]any) {
	t.Helper()
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, payload
}

func expectError(t *testing.T, payload map[string]any, code string) {
	t.Helper()
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected failure envelope, got %v", payload)
	}
	errBody, _ := payload["error"].(map[string]any)
	if errBody == nil || errBody["code"] != code {
		t.Fatalf("expected error code %q, got %v", code, payload)
	}
	if requestID, _ := payload["request_id"].(string); !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("error envelope must carry request_id, got %v", payload)
	}
}
