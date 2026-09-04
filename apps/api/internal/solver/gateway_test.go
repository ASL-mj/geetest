package solver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/captchaflow/service-platform/api/internal/config"
)

func testSettings(baseURL string) config.Settings {
	return config.Settings{
		GeetestSolverURL:     baseURL,
		GeetestServiceAPIKey: "unit-service-key",
		SolverConnectTimeout: 2 * time.Second,
		SolverReadTimeout:    2 * time.Second,
		SolverTotalTimeout:   2 * time.Second,
	}
}

const validSolveBody = `{"success": true, "data": {"captcha_id": "cap1", "lot_number": "L1", "captcha_output": "O1", "pass_token": "T1", "gen_time": "1700000000"}}`

func TestGatewaySolvesAndSendsServiceKey(t *testing.T) {
	var gotKey, gotRequestID, gotRiskType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Service-Key")
		gotRequestID = r.Header.Get("X-Request-ID")
		if r.URL.Path != "/v1/geetest/solve" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		body := map[string]string{}
		_ = readJSONBody(r, &body)
		gotRiskType = body["risk_type"]
		w.Write([]byte(validSolveBody))
	}))
	defer server.Close()

	gateway := NewGateway(testSettings(server.URL))
	result, err := gateway.Solve(context.Background(), "req_abc", SolveRequest{CaptchaID: "cap1", RiskType: "slide"})
	if err != nil {
		t.Fatalf("solve failed: %v", err)
	}
	if gotKey != "unit-service-key" {
		t.Fatalf("service key not forwarded: %q", gotKey)
	}
	if gotRequestID != "req_abc" {
		t.Fatalf("request id not forwarded: %q", gotRequestID)
	}
	if gotRiskType != "slide" {
		t.Fatalf("risk type not forwarded: %q", gotRiskType)
	}
	if result.LotNumber != "L1" || result.PassToken != "T1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestGatewayMapsFailuresToTypedErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"solver 502", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(502) }, ErrSolver5xx},
		{"solver 422", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(422) }, ErrSolver422},
		{"bad json", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("not-json")) }, ErrSolverBad},
		{"success false", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"success": false, "message": "nope"}`))
		}, ErrSolver5xx},
		{"missing fields", func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"success": true, "data": {"captcha_id": "x"}}`))
		}, ErrSolverBad},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var gateway *Gateway
			if testCase.name == "unreachable" {
				// A closed port guarantees a transport-level failure.
				gateway = NewGateway(testSettings("http://127.0.0.1:1"))
			} else {
				server := httptest.NewServer(testCase.handler)
				defer server.Close()
				gateway = NewGateway(testSettings(server.URL))
			}
			_, err := gateway.Solve(context.Background(), "req_1", SolveRequest{CaptchaID: "c", RiskType: "slide"})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("got %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestGatewayTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(3 * time.Second)
		w.Write([]byte(validSolveBody))
	}))
	defer server.Close()

	gateway := NewGateway(testSettings(server.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := gateway.Solve(ctx, "req_1", SolveRequest{CaptchaID: "c", RiskType: "slide"})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

// readJSONBody decodes the request body into target for mock handlers.
func readJSONBody(r *http.Request, target *map[string]string) error {
	return json.NewDecoder(r.Body).Decode(target)
}
