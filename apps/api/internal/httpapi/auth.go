package httpapi

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/captchaflow/service-platform/api/internal/service"
)

type userContextKey struct{}

func withUserContext(ctx context.Context, session sessionContext) context.Context {
	return context.WithValue(ctx, userContextKey{}, session)
}

func userFromContext(r *http.Request) sessionContext {
	return r.Context().Value(userContextKey{}).(sessionContext)
}

// handleActivate exchanges a CDK for a bound user identity and session.
// First activation returns 201 with the default API key plaintext once.
func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		CDK string `json:"cdk"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}

	code := strings.TrimSpace(payload.CDK)
	if utf8.RuneCountInString(code) < 4 || utf8.RuneCountInString(code) > 128 || !hasLetterOrDigit(code) {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	result, appErr := s.services.ActivateCDK(r.Context(), code)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}

	statusCode := http.StatusOK
	data := map[string]any{
		"user": map[string]any{
			"id":         result.User.ID,
			"cdk_prefix": result.CDKPrefix,
		},
	}
	if result.NewlyActivated {
		statusCode = http.StatusCreated
		data["default_api_key"] = result.DefaultAPIKey
	}

	setSessionCookie(w, result.SessionToken, int(s.services.Settings.SessionTTL.Seconds()))
	writeSuccess(w, newRequestID(), statusCode, data)
}

// handleLogout revokes only the current session and clears the cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	session := userFromContext(r)
	sessionID := parseUUID(session.SessionID)
	if sessionID == nil {
		writeApplicationError(w, service.ErrSessionInvalid())
		return
	}
	if appErr := s.services.Logout(r.Context(), *sessionID); appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	clearSessionCookie(w)
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{})
}
