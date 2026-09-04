package httpapi

import (
	"net/http"
	"unicode"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/service"
)

const sessionCookieName = "session"

// sessionContext carries the resolved user session through the request.
type sessionContext struct {
	Token     string
	SessionID string
	UserID    string
}

// requireUserSession guards handlers behind a valid HttpOnly session cookie.
func (s *Server) requireUserSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeApplicationError(w, service.ErrSessionRequired())
			return
		}
		user, session, appErr := s.services.ResolveUserSession(r.Context(), cookie.Value)
		if appErr != nil {
			writeApplicationError(w, appErr)
			return
		}
		r = r.WithContext(withUserContext(r.Context(), sessionContext{
			Token:     cookie.Value,
			SessionID: session.ID.String(),
			UserID:    user.ID.String(),
		}))
		next(w, r)
	}
}

// setSessionCookie issues the HttpOnly session cookie for this response.
func setSessionCookie(w http.ResponseWriter, token string, maxAgeSeconds int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie client-side.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// newRequestID is a local alias keeping handler bodies terse.
func newRequestID() string { return crypto.NewRequestID() }

// hasLetterOrDigit reports whether any rune is alphanumeric, matching the
// V1 activation validation rule.
func hasLetterOrDigit(value string) bool {
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return true
		}
	}
	return false
}
