package httpapi

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/service"
)

// bearerSecret extracts the cf_live_ secret from the Authorization header.
func bearerSecret(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	secret := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return secret, secret != ""
}

// maskAndHashIP reduces the client IP to its /24 (v4) or /48 (v6) network
// and an irreversible correlation hash, per the privacy contract.
func maskAndHashIP(remoteAddr, pepper string) (string, []byte) {
	host := remoteAddr
	if hostOnly, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = hostOnly
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "0.0.0.0/32", crypto.HMACSHA256("unknown", pepper)
	}
	var masked *net.IPNet
	if ip.To4() != nil {
		_, masked, _ = net.ParseCIDR(ip.String() + "/24")
	} else {
		_, masked, _ = net.ParseCIDR(ip.String() + "/48")
	}
	if masked == nil {
		return "0.0.0.0/32", crypto.HMACSHA256(ip.String(), pepper)
	}
	return masked.String(), crypto.HMACSHA256(ip.String(), pepper)
}

// handleSolve is the public POST /v1/captcha/solve endpoint. It shares the
// quota, limiter and audit chain with every other invocation path.
func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	secret, ok := bearerSecret(r)
	if !ok {
		writeApplicationError(w, service.ErrAPIKeyInvalid())
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 256 {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	caller, appErr := s.services.AuthenticateAPIKey(r.Context(), secret)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}

	var payload struct {
		CaptchaID string `json:"captcha_id"`
		RiskType  string `json:"risk_type"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || json.Unmarshal(body, &payload) != nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	captchaID := strings.TrimSpace(payload.CaptchaID)
	if captchaID == "" || len(captchaID) > 256 || payload.RiskType != "slide" {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	clientIP, clientIPHash := maskAndHashIP(r.RemoteAddr, s.services.Settings.SessionSecret)
	if caller.APIKey.AllowedIPs != nil && *caller.APIKey.AllowedIPs != "" {
		if rawIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if !crypto.IPAllowed(rawIP, *caller.APIKey.AllowedIPs) {
				writeApplicationError(w, service.ErrKeyIPForbidden())
				return
			}
		}
	}
	outcome := s.solve.Solve(r.Context(), service.SolveInput{
		Caller:         caller,
		CaptchaID:      captchaID,
		IdempotencyKey: idempotencyKey,
		ClientIPMasked: clientIP,
		ClientIPHash:   clientIPHash,
		UserAgent:      truncateUserAgent(r.Header.Get("User-Agent")),
	})

	if outcome.SolveError != nil {
		if outcome.SolveError.HTTPStatus == http.StatusTooManyRequests && outcome.SolveError.RetryAfter > 0 {
			w.Header().Set("Retry-After", retryAfterSeconds(outcome.SolveError.RetryAfter))
		}
		writeJSON(w, outcome.SolveError.HTTPStatus, errorEnvelope{
			Success:   false,
			RequestID: firstNonEmpty(outcome.RequestID, newRequestID()),
			Error: errorBody{
				Code:    outcome.SolveError.Code,
				Message: outcome.SolveError.Message,
			},
		})
		return
	}

	writeSuccess(w, outcome.RequestID, http.StatusOK, map[string]any{
		"captcha_id":     outcome.Result.CaptchaID,
		"lot_number":     outcome.Result.LotNumber,
		"captcha_output": outcome.Result.CaptchaOutput,
		"pass_token":     outcome.Result.PassToken,
		"gen_time":       outcome.Result.GenTime,
	})
}

// handleConsoleSolve is the session-authenticated debug endpoint. It reuses
// the same admission chain with a server-generated idempotency key derived
// from the request id, so console tests never bypass quota or audit.
func (s *Server) handleConsoleSolve(w http.ResponseWriter, r *http.Request) {
	if s.solve == nil {
		writeApplicationError(w, service.NewError(503, "SOLVE_UNAVAILABLE", "Solve service is not configured."))
		return
	}
	user := userFromContext(r)

	var payload struct {
		KeyID     string `json:"key_id"`
		CaptchaID string `json:"captcha_id"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	captchaID := strings.TrimSpace(payload.CaptchaID)
	keyID := parseUUID(payload.KeyID)
	if captchaID == "" || len(captchaID) > 256 || keyID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	caller, appErr := s.services.AuthenticateAPIKeyForUser(r.Context(), *parseUUID(user.UserID), *keyID)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}

	clientIP, clientIPHash := maskAndHashIP(r.RemoteAddr, s.services.Settings.SessionSecret)
	if caller.APIKey.AllowedIPs != nil && *caller.APIKey.AllowedIPs != "" {
		if rawIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if !crypto.IPAllowed(rawIP, *caller.APIKey.AllowedIPs) {
				writeApplicationError(w, service.ErrKeyIPForbidden())
				return
			}
		}
	}
	outcome := s.solve.Solve(r.Context(), service.SolveInput{
		Caller:         caller,
		CaptchaID:      captchaID,
		IdempotencyKey: "console:" + newRequestID(),
		ClientIPMasked: clientIP,
		ClientIPHash:   clientIPHash,
		UserAgent:      truncateUserAgent(r.Header.Get("User-Agent")),
	})

	if outcome.SolveError != nil {
		writeJSON(w, outcome.SolveError.HTTPStatus, errorEnvelope{
			Success:   false,
			RequestID: firstNonEmpty(outcome.RequestID, newRequestID()),
			Error: errorBody{
				Code:    outcome.SolveError.Code,
				Message: outcome.SolveError.Message,
			},
		})
		return
	}
	writeSuccess(w, outcome.RequestID, http.StatusOK, map[string]any{
		"captcha_id":     outcome.Result.CaptchaID,
		"lot_number":     outcome.Result.LotNumber,
		"captcha_output": outcome.Result.CaptchaOutput,
		"pass_token":     outcome.Result.PassToken,
		"gen_time":       outcome.Result.GenTime,
	})
}

func truncateUserAgent(agent string) string {
	agent = strings.TrimSpace(agent)
	if len(agent) > 512 {
		return agent[:512]
	}
	if agent == "" {
		return "unknown"
	}
	return agent
}

func retryAfterSeconds(d time.Duration) string {
	seconds := int64((d + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return itoa(seconds)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
