// Package solver isolates the GeeTest HTTP service behind a narrow gateway.
// It is the only component permitted to know the solver URL and service key;
// failures surface as typed domain errors, never downstream bodies.
package solver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/captchaflow/service-platform/api/internal/config"
)

// Gateway exposes the single V1 operation: solve one slide captcha. The
// base URL can be overridden at runtime from the admin system settings.
type Gateway struct {
	url        atomic.Value // holds string
	serviceKey string
	client     *http.Client
}

// SetBaseURL hot-swaps the solver endpoint; concurrent Solve calls observe
// either the old or the new value, never a torn one.
func (g *Gateway) SetBaseURL(url string) {
	g.url.Store(url)
}

// currentBaseURL resolves the runtime override.
func (g *Gateway) currentBaseURL() string {
	if v, ok := g.url.Load().(string); ok && v != "" {
		return v
	}
	return ""
}

// NewGateway builds the HTTP client with strict connect/read/total timeouts.
func NewGateway(settings config.Settings) *Gateway {
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: settings.SolverConnectTimeout}).DialContext,
	}
	g := &Gateway{
		serviceKey: settings.GeetestServiceAPIKey,
		client: &http.Client{
			Transport: transport,
			Timeout:   settings.SolverTotalTimeout,
		},
	}
	g.url.Store(settings.GeetestSolverURL)
	return g
}

// SolveRequest is the platform-facing input.
type SolveRequest struct {
	CaptchaID string `json:"captcha_id"`
	RiskType  string `json:"risk_type"`
}

// SolveResult carries the normalized GeeTest payload.
type SolveResult struct {
	CaptchaID     string `json:"captcha_id"`
	LotNumber     string `json:"lot_number"`
	CaptchaOutput string `json:"captcha_output"`
	PassToken     string `json:"pass_token"`
	GenTime       string `json:"gen_time"`
}

// Typed failure categories rendered to stable platform error codes.
var (
	ErrTimeout   = errors.New("solver timeout")
	ErrUnreach   = errors.New("solver unreachable")
	ErrSolver5xx = errors.New("solver failed")
	ErrSolverBad = errors.New("solver response malformed")
	ErrSolver422 = errors.New("solver rejected parameters")
)

type solverResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    *SolveResult `json:"data"`
}

// Solve calls POST {base}/v1/geetest/solve with the service key header.
func (g *Gateway) Solve(ctx context.Context, requestID string, req SolveRequest) (*SolveResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSolverBad, err)
	}
	endpoint := g.currentBaseURL()
	if !endsWithV1Solve(endpoint) {
		endpoint = trimTrailingSlash(endpoint) + "/v1/geetest/solve"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreach, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Service-Key", g.serviceKey)
	httpReq.Header.Set("X-Request-ID", requestID)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err) {
			return nil, ErrTimeout
		}
		return nil, ErrUnreach
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, ErrSolverBad
	}

	switch {
	case resp.StatusCode == http.StatusUnprocessableEntity:
		return nil, ErrSolver422
	case resp.StatusCode >= 500:
		return nil, ErrSolver5xx
	case resp.StatusCode >= 400:
		return nil, ErrSolverBad
	}

	var parsed solverResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, ErrSolverBad
	}
	if !parsed.Success || parsed.Data == nil {
		return nil, ErrSolver5xx
	}
	if parsed.Data.LotNumber == "" || parsed.Data.CaptchaOutput == "" || parsed.Data.PassToken == "" {
		return nil, ErrSolverBad
	}
	return parsed.Data, nil
}

func trimTrailingSlash(url string) string {
	for len(url) > 0 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	return url
}

func endsWithV1Solve(url string) bool {
	return len(url) >= len("/v1/geetest/solve") && url[len(url)-len("/v1/geetest/solve"):] == "/v1/geetest/solve"
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
