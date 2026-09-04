// Package httpapi exposes the platform HTTP API: envelope responses, session
// cookie handling and the V1 user routes.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/service"
)

// Server carries the shared dependencies for all handlers.
type Server struct {
	services *service.Services
	solve    *service.SolveService
}

// NewRouter builds the V1 route table. Go 1.22+ ServeMux patterns provide
// method matching and path parameters. The solve endpoint requires its own
// service; passing nil omits it (useful for focused test routers).
func NewRouter(services *service.Services, solve *service.SolveService) http.Handler {
	server := &Server{services: services, solve: solve}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", server.handleHealthz)
	mux.HandleFunc("POST /v1/auth/activate", server.handleActivate)
	mux.HandleFunc("POST /v1/auth/logout", server.requireUserSession(server.handleLogout))
	mux.HandleFunc("POST /v1/keys", server.requireUserSession(server.handleCreateKey))
	mux.HandleFunc("GET /v1/keys", server.requireUserSession(server.handleListKeys))
	mux.HandleFunc("PATCH /v1/keys/{key_id}", server.requireUserSession(server.handleUpdateKey))
	mux.HandleFunc("DELETE /v1/keys/{key_id}", server.requireUserSession(server.handleDeleteKey))
	if solve != nil {
		mux.HandleFunc("POST /v1/captcha/solve", server.handleSolve)
	}

	return mux
}

// handleHealthz is a public liveness probe; it must not check the solver.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// successEnvelope mirrors the platform contract {"success","request_id","data"}.
type successEnvelope struct {
	Success   bool   `json:"success"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`
}

// errorEnvelope mirrors {"success","request_id","error"} for every failure.
type errorEnvelope struct {
	Success   bool      `json:"success"`
	RequestID string    `json:"request_id"`
	Error     errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSuccess(w http.ResponseWriter, requestID string, statusCode int, data any) {
	writeJSON(w, statusCode, successEnvelope{Success: true, RequestID: requestID, Data: data})
}

// writeApplicationError renders a use-case failure without leaking internals.
func writeApplicationError(w http.ResponseWriter, appErr *service.ApplicationError) {
	writeJSON(w, appErr.StatusCode, errorEnvelope{
		Success:   false,
		RequestID: crypto.NewRequestID(),
		Error:     errorBody{Code: appErr.Code, Message: appErr.Message, Retryable: false},
	})
}

func (s *Server) writeInternalError(w http.ResponseWriter, err error, context string) {
	slog.Error(context, "error", err)
	writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
}

// decodeJSON reads a JSON body; every transport problem is a 422.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return false
	}
	return true
}
