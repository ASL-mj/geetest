package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/store"
)

// adminSessionCookie keeps operator sessions in a separate namespace from
// the user "session" cookie.
const adminSessionCookie = "admin_session"

// requireAdminAny guards /admin/v1 routes for any active operator.
func (s *Server) requireAdminAny(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAdminSession()(next)
}

// requireAdminSession guards /admin/v1 routes; roles limits access to the
// given operator roles (empty means any active operator).
func (s *Server) requireAdminSession(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(adminSessionCookie)
			if err != nil || cookie.Value == "" {
				writeApplicationError(w, service.ErrAdminSessionInvalid())
				return
			}
			session, appErr := s.services.ResolveAdminSession(r.Context(), cookie.Value)
			if appErr != nil {
				writeApplicationError(w, appErr)
				return
			}
			if len(roles) > 0 {
				allowed := false
				for _, role := range roles {
					if session.Role == role {
						allowed = true
						break
					}
				}
				if !allowed {
					writeApplicationError(w, service.ErrAdminForbidden())
					return
				}
			}
			next(w, r.WithContext(withAdminContext(r.Context(), session)))
		}
	}
}

type adminContextKey struct{}

func withAdminContext(ctx context.Context, session service.AdminSession) context.Context {
	return context.WithValue(ctx, adminContextKey{}, session)
}

func adminFromContext(r *http.Request) service.AdminSession {
	return r.Context().Value(adminContextKey{}).(service.AdminSession)
}

// auditIP reuses the platform masking for the operator source address.
func (s *Server) auditIP(r *http.Request) string {
	masked, _ := maskAndHashIP(r.RemoteAddr, s.services.Settings.SessionSecret)
	return masked
}

// requireReason enforces the audited-action contract: every write carries a
// non-empty reason.
func requireReason(w http.ResponseWriter, reason string) bool {
	if strings.TrimSpace(reason) == "" {
		writeApplicationError(w, service.ErrInvalidRequest())
		return false
	}
	return true
}

// handleAdminLogin exchanges username/password for an operator cookie.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if payload.Username == "" || payload.Password == "" {
		writeApplicationError(w, service.ErrAdminAuthFailed())
		return
	}
	token, appErr := s.services.AdminLogin(r.Context(), payload.Username, payload.Password)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(s.services.Settings.AdminSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{})
}

// handleAdminLogout revokes the presented operator session.
func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(adminSessionCookie)
	if err == nil && cookie.Value != "" {
		if appErr := s.services.AdminLogout(r.Context(), cookie.Value); appErr != nil {
			writeApplicationError(w, appErr)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminSessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{})
}

// handleAdminDashboard returns the landing aggregates.
func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, appErr := s.services.GetAdminDashboard(r.Context())
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{
		"users_total":        dashboard.UsersTotal,
		"users_active":       dashboard.UsersActive,
		"cdks_active":        dashboard.CdksActive,
		"cdks_unactivated":   dashboard.CdksUnactivated,
		"cdks_exhausted":     dashboard.CdksExhausted,
		"calls_today":        dashboard.CallsToday,
		"calls_failed_today": dashboard.CallsFailedToday,
		"quota_consumed":     dashboard.QuotaConsumed,
		"success_rate":       dashboard.SuccessRate,
	})
}

// handleAdminCreateBatch generates a CDK batch; the plaintext codes appear
// exactly once in this response.
func (s *Server) handleAdminCreateBatch(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name                string     `json:"name"`
		Description         string     `json:"description"`
		Quota               int64      `json:"quota"`
		Count               int        `json:"count"`
		ActivationDeadline  *time.Time `json:"activation_deadline"`
		ServiceDurationDays *int       `json:"service_duration_days"`
		Reason              string     `json:"reason"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if !requireReason(w, payload.Reason) {
		return
	}

	result, appErr := s.services.CreateCdkBatch(r.Context(), adminFromContext(r), service.CreateCdkBatchRequest{
		Name:                strings.TrimSpace(payload.Name),
		Description:         strings.TrimSpace(payload.Description),
		Quota:               payload.Quota,
		Count:               payload.Count,
		ActivationDeadline:  payload.ActivationDeadline,
		ServiceDurationDays: payload.ServiceDurationDays,
		Reason:              strings.TrimSpace(payload.Reason),
	}, s.auditIP(r))
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusCreated, map[string]any{
		"batch_id": result.BatchID,
		"codes":    result.Codes,
	})
}

// handleAdminListBatches lists generated batches with counts.
func (s *Server) handleAdminListBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := store.ListCdkBatches(r.Context(), s.services.Pool, 50)
	if err != nil {
		slog.Error("admin batch list failed", "error", err)
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	items := make([]map[string]any, 0, len(batches))
	for _, batch := range batches {
		items = append(items, map[string]any{
			"id":                    batch.ID,
			"name":                  batch.Name,
			"description":           batch.Description,
			"default_quota":         batch.DefaultQuota,
			"activation_deadline":   formatTimePtr(batch.ActivationDeadline),
			"service_duration_days": batch.ServiceDurationDays,
			"total_cdks":            batch.TotalCDKs,
			"active_cdks":           batch.ActiveCDKs,
			"created_at":            batch.CreatedAt.UTC().Format(timeFormatUTC),
		})
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": items})
}

// handleAdminListCdks lists CDK rows with optional filters.
func (s *Server) handleAdminListCdks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var batchID *uuid.UUID
	if raw := query.Get("batch_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeApplicationError(w, service.ErrInvalidRequest())
			return
		}
		batchID = &parsed
	}
	status := query.Get("status")
	cdks, err := store.ListCdks(r.Context(), s.services.Pool, batchID, status, 100)
	if err != nil {
		slog.Error("admin cdk list failed", "error", err)
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	items := make([]map[string]any, 0, len(cdks))
	for _, cdk := range cdks {
		items = append(items, map[string]any{
			"id":               cdk.ID,
			"batch_id":         cdk.BatchID,
			"code_prefix":      cdk.CodePrefix,
			"status":           cdk.Status,
			"bound_user_id":    cdk.BoundUserID,
			"bound_user_state": cdk.BoundUserState,
			"remark":           cdk.Remark,
			"batch_name":       cdk.BatchName,
			"expires_at":       formatTimePtr(cdk.ExpiresAt),
			"quota_total":      cdk.QuotaTotal,
			"quota_used":       cdk.QuotaUsed,
			"quota_reserved":   cdk.QuotaReserved,
			"quota_remaining":  cdk.QuotaRemaining,
			"activated_at":     formatTimePtr(cdk.ActivatedAt),
			"created_at":       cdk.CreatedAt.UTC().Format(timeFormatUTC),
		})
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": items})
}

// handleAdminAdjustQuota applies an audited ledger-based quota adjustment.
func (s *Server) handleAdminAdjustQuota(w http.ResponseWriter, r *http.Request) {
	cdkID := parseUUID(r.PathValue("cdk_id"))
	if cdkID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var payload struct {
		Delta  int64  `json:"delta"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if !requireReason(w, payload.Reason) {
		return
	}
	snapshot, appErr := s.services.AdjustCdkQuota(r.Context(), adminFromContext(r), *cdkID,
		service.QuotaAdjustmentRequest{Delta: payload.Delta, Reason: strings.TrimSpace(payload.Reason)}, s.auditIP(r))
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusCreated, map[string]any{
		"quota_total":     snapshot.Total,
		"quota_used":      snapshot.Used,
		"quota_reserved":  snapshot.Reserved,
		"quota_remaining": snapshot.Remaining,
	})
}

// handleAdminSetCdkStatus enables or disables one CDK.
func (s *Server) handleAdminSetCdkStatus(w http.ResponseWriter, r *http.Request) {
	cdkID := parseUUID(r.PathValue("cdk_id"))
	if cdkID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var payload struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if !requireReason(w, payload.Reason) {
		return
	}
	enable := payload.Status == "ACTIVE"
	if payload.Status != "ACTIVE" && payload.Status != "DISABLED" {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	status, appErr := s.services.SetCdkEnabled(r.Context(), adminFromContext(r), *cdkID,
		enable, strings.TrimSpace(payload.Reason), s.auditIP(r))
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"status": status})
}

// handleAdminRevealCdkCode decrypts a CDK's sealed plaintext for re-copy.
func (s *Server) handleAdminRevealCdkCode(w http.ResponseWriter, r *http.Request) {
	cdkID := parseUUID(r.PathValue("cdk_id"))
	if cdkID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	code, appErr := s.services.RevealCdkCode(r.Context(), *cdkID)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"code": code})
}

// handleAdminSetCdkRemark updates the operator note on one CDK.
func (s *Server) handleAdminSetCdkRemark(w http.ResponseWriter, r *http.Request) {
	cdkID := parseUUID(r.PathValue("cdk_id"))
	if cdkID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var payload struct {
		Remark string `json:"remark"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if appErr := s.services.SetCdkRemark(r.Context(), adminFromContext(r), *cdkID, strings.TrimSpace(payload.Remark)); appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"remark": strings.TrimSpace(payload.Remark)})
}

// handleAdminGetSystemConfig reports the effective runtime configuration.
func (s *Server) handleAdminGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	view, appErr := s.services.GetSystemConfig(r.Context())
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{
		"solver_base_url": view.SolverBaseURL,
		"solver_source":   view.SolverSource,
	})
}

// handleAdminSetSystemConfig stores, audits and live-applies an override.
func (s *Server) handleAdminSetSystemConfig(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SolverBaseURL string `json:"solver_base_url"`
		Reason        string `json:"reason"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if !requireReason(w, payload.Reason) {
		return
	}
	if appErr := s.services.SetSolverBaseURL(r.Context(), adminFromContext(r),
		payload.SolverBaseURL, strings.TrimSpace(payload.Reason), s.solverGateway, s.auditIP(r)); appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"solver_base_url": strings.TrimSpace(payload.SolverBaseURL)})
}

// handleAdminListUsers lists platform users.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := store.ListUsers(r.Context(), s.services.Pool, 100)
	if err != nil {
		slog.Error("admin user list failed", "error", err)
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		items = append(items, map[string]any{
			"id":            user.ID,
			"status":        user.Status,
			"created_at":    user.CreatedAt.UTC().Format(timeFormatUTC),
			"last_login_at": formatTimePtr(user.LastLoginAt),
			"cdk_prefix":    user.CDKPrefix,
			"cdk_status":    user.CDKStatus,
			"cdk_remaining": user.CDKRemaining,
		})
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": items})
}

// handleAdminSetUserStatus suspends or restores a user.
func (s *Server) handleAdminSetUserStatus(w http.ResponseWriter, r *http.Request) {
	userID := parseUUID(r.PathValue("user_id"))
	if userID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var payload struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if !requireReason(w, payload.Reason) {
		return
	}
	if payload.Status != "ACTIVE" && payload.Status != "SUSPENDED" {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	status, appErr := s.services.SetUserEnabled(r.Context(), adminFromContext(r), *userID,
		payload.Status == "ACTIVE", strings.TrimSpace(payload.Reason), s.auditIP(r))
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"status": status})
}

// handleAdminListAuditLogs returns the newest audit entries.
func (s *Server) handleAdminListAuditLogs(w http.ResponseWriter, r *http.Request) {
	entries, err := store.ListAdminAuditLogs(r.Context(), s.services.Pool, 100)
	if err != nil {
		slog.Error("admin audit list failed", "error", err)
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, map[string]any{
			"id":            entry.ID,
			"admin_user_id": entry.AdminUserID,
			"action":        entry.Action,
			"target_type":   entry.TargetType,
			"target_id":     entry.TargetID,
			"before":        json.RawMessage(entry.BeforeJSON),
			"after":         json.RawMessage(entry.AfterJSON),
			"reason":        entry.Reason,
			"ip_masked":     entry.IPMasked,
			"created_at":    entry.CreatedAt.UTC().Format(timeFormatUTC),
		})
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": items})
}

// handleAdminSolverHealth probes the solver base URL with a strict timeout
// and reports only sanitized status, latency and a failure category.
func (s *Server) handleAdminSolverHealth(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 3 * time.Second}
	started := time.Now()
	resp, err := client.Get(strings.TrimSuffix(s.services.Settings.GeetestSolverURL, "/") + "/healthz")
	latency := time.Since(started)

	health := map[string]any{
		"latency_ms": latency.Milliseconds(),
		"checked_at": time.Now().UTC().Format(timeFormatUTC),
	}
	if err != nil {
		health["status"] = "UNREACHABLE"
		health["failure"] = classifySolverFailure(err)
	} else {
		defer resp.Body.Close()
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			health["status"] = "OK"
		case resp.StatusCode == 401 || resp.StatusCode == 403:
			health["status"] = "AUTH_FAILED"
		default:
			health["status"] = "DEGRADED"
		}
	}
	writeSuccess(w, newRequestID(), http.StatusOK, health)
}

func classifySolverFailure(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "TIMEOUT"
	}
	return "UNREACHABLE"
}
