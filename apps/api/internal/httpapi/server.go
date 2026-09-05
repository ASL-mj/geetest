// Package httpapi exposes the platform HTTP API: envelope responses, session
// cookie handling and the V1 user routes.
package httpapi

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/store"
)

//go:embed all:web
var webFS embed.FS

// Server carries the shared dependencies for all handlers.
type Server struct {
	services         *service.Services
	solve            *service.SolveService
	trustedProxyHops int
	adminLoginGate   *ipWindowLimiter
	activateGate     *ipWindowLimiter
}

// NewRouter builds the V1 route table. Go 1.22+ ServeMux patterns provide
// method matching and path parameters. The solve endpoint requires its own
// service; passing nil omits it (useful for focused test routers).
// When a web build is embedded (deploy images), unmatched GETs fall back to
// the SPA so history routes like /console/keys survive a hard refresh.
func NewRouter(services *service.Services, solve *service.SolveService) http.Handler {
	server := &Server{
		services:         services,
		solve:            solve,
		trustedProxyHops: services.Settings.TrustedProxyHops,
		// Unauthenticated hot spots get a small per-IP fixed window: admin
		// login is also a CPU amplifier (Argon2id), activation hammers the DB.
		adminLoginGate: newIPWindowLimiter(10, time.Minute),
		activateGate:   newIPWindowLimiter(30, time.Minute),
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", server.handleHealthz)
	mux.HandleFunc("POST /v1/auth/activate", server.requireThrottle(server.activateGate, server.handleActivate))
	mux.HandleFunc("POST /v1/auth/logout", server.requireUserSession(server.handleLogout))
	mux.HandleFunc("POST /v1/keys", server.requireUserSession(server.handleCreateKey))
	mux.HandleFunc("GET /v1/keys", server.requireUserSession(server.handleListKeys))
	mux.HandleFunc("PATCH /v1/keys/{key_id}", server.requireUserSession(server.handleUpdateKey))
	mux.HandleFunc("GET /v1/keys/{key_id}/secret", server.requireUserSession(server.handleRevealKeySecret))
	mux.HandleFunc("DELETE /v1/keys/{key_id}", server.requireUserSession(server.handleDeleteKey))
	mux.HandleFunc("GET /v1/account", server.requireUserSession(server.handleGetAccount))
	mux.HandleFunc("GET /v1/usage", server.requireUserSession(server.handleGetUsage))
	mux.HandleFunc("GET /v1/calls", server.requireUserSession(server.handleListCalls))
	mux.HandleFunc("GET /v1/calls/{request_id}", server.requireUserSession(server.handleGetCall))
	mux.HandleFunc("POST /v1/tools/captcha/solve", server.requireUserSession(server.handleConsoleSolve))
	if solve != nil {
		mux.HandleFunc("POST /v1/captcha/solve", server.handleSolve)
	}

	// Administrator surface: /admin/v1 with its own session cookie and RBAC.
	mux.HandleFunc("POST /admin/v1/auth/login", server.requireThrottle(server.adminLoginGate, server.handleAdminLogin))
	mux.HandleFunc("POST /admin/v1/auth/logout", server.requireAdminAny(server.handleAdminLogout))
	mux.HandleFunc("GET /admin/v1/dashboard", server.requireAdminAny(server.handleAdminDashboard))
	mux.HandleFunc("POST /admin/v1/cdk-batches", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminCreateBatch))
	mux.HandleFunc("GET /admin/v1/cdk-batches", server.requireAdminAny(server.handleAdminListBatches))
	mux.HandleFunc("GET /admin/v1/cdks", server.requireAdminAny(server.handleAdminListCdks))
	mux.HandleFunc("POST /admin/v1/cdks/{cdk_id}/quota-adjustments", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminAdjustQuota))
	mux.HandleFunc("PATCH /admin/v1/cdks/{cdk_id}", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminSetCdkStatus))
	mux.HandleFunc("GET /admin/v1/users", server.requireAdminAny(server.handleAdminListUsers))
	mux.HandleFunc("PATCH /admin/v1/users/{user_id}", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminSetUserStatus))
	mux.HandleFunc("GET /admin/v1/audit-logs", server.requireAdminAny(server.handleAdminListAuditLogs))
	mux.HandleFunc("GET /admin/v1/solver-health", server.requireAdminAny(server.handleAdminSolverHealth))
	mux.HandleFunc("GET /admin/v1/cdks/{cdk_id}/code", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminRevealCdkCode))
	mux.HandleFunc("PATCH /admin/v1/cdks/{cdk_id}/remark", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminSetCdkRemark))
	mux.HandleFunc("GET /admin/v1/system/config", server.requireAdminAny(server.handleAdminGetSystemConfig))
	mux.HandleFunc("PUT /admin/v1/system/config", server.requireAdminSession(store.AdminRoleAdmin)(server.handleAdminSetSystemConfig))

	// Anonymous metadata for the docs pages: the display base URL only.
	mux.HandleFunc("GET /v1/meta", server.handlePublicMeta)

	// Embedded web console (production image). Serving is skipped entirely
	// when apps/api/web is empty, so local/test routers behave as before.
	if spa := newSPAHandler(); spa != nil {
		mux.HandleFunc("GET /", spa)
	}

	return mux
}

// newSPAHandler returns a handler serving the embedded web build with
// history-mode fallback, or nil when no build is embedded.
func newSPAHandler() http.HandlerFunc {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil
	}
	return spaHandlerFor(sub)
}

func spaHandlerFor(files fs.FS) http.HandlerFunc {
	entries, err := fs.ReadDir(files, ".")
	if err != nil || len(entries) == 0 {
		return nil
	}
	fileServer := http.FileServerFS(files)
	index, indexErr := fs.ReadFile(files, "index.html")
	if indexErr != nil {
		return nil
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// API namespaces always answer with the JSON envelope, never HTML.
		if isAPIPath(r.URL.Path) {
			writeApplicationError(w, service.ErrNotFound())
			return
		}
		// Serve real assets by extension; everything else gets index.html so
		// SPA routes like /console/keys or /admin render on hard refresh.
		if path.Ext(r.URL.Path) != "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	}
}

// isAPIPath reports whether a request path belongs to the platform API
// surface (which must never fall back to the web console). /admin itself is
// an SPA route; only the /admin/v1 API namespace is excluded.
func isAPIPath(p string) bool {
	return p == "/healthz" || p == "/v1" || p == "/admin/v1" ||
		strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/admin/v1/")
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

// maxBodyBytes bounds every JSON body (1 MiB is far above any platform
// payload) so unauthenticated endpoints can never stream unbounded input
// into memory.
const maxBodyBytes = 1 << 20

// decodeJSON reads a JSON body; every transport problem is a 422 and an
// oversized body is rejected before it can be buffered.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return false
	}
	return true
}
