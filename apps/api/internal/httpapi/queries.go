package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/store"
)

var errInvalidRequest = service.ErrInvalidRequest()

// timeFormatUTC is the shared ISO 8601 UTC rendering for query responses.
const timeFormatUTC = "2006-01-02T15:04:05.999999Z"

func formatTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(timeFormatUTC)
}

// serializeCall renders a redacted call record; response bodies, client
// identity hashes and key material never appear.
func serializeCall(record store.CallRecord) map[string]any {
	return map[string]any{
		"request_id":     record.RequestID,
		"api_key_name":   record.APIKeyName,
		"api_key_prefix": record.APIKeyPrefix,
		"captcha_id":     record.CaptchaID,
		"risk_type":      record.RiskType,
		"status":         record.Status,
		"http_status":    record.HTTPStatus,
		"error_code":     record.ErrorCode,
		"accepted_at":    record.AcceptedAt.UTC().Format(timeFormatUTC),
		"completed_at":   formatTimePtr(record.CompletedAt),
		"duration_ms":    record.DurationMS,
		"quota_reserved": record.QuotaReserved,
		"quota_refunded": record.QuotaRefunded,
		"client_ip":      record.ClientIPMasked,
		"user_agent":     record.UserAgent,
	}
}

// handleGetAccount returns the bound CDK and quota summary.
func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	userID := *parseUUID(userFromContext(r).UserID)
	summary, appErr := s.services.GetAccount(r.Context(), userID)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{
		"cdk": map[string]any{
			"code_prefix":     summary.CodePrefix,
			"status":          summary.Status,
			"activated_at":    formatTimePtr(summary.ActivatedAt),
			"expires_at":      formatTimePtr(summary.ExpiresAt),
			"quota_total":     summary.QuotaTotal,
			"quota_used":      summary.QuotaUsed,
			"quota_reserved":  summary.QuotaReserved,
			"quota_remaining": summary.QuotaRemaining,
		},
	})
}

// handleGetUsage returns quota and aggregate call statistics.
func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	userID := *parseUUID(userFromContext(r).UserID)
	report, appErr := s.services.GetUsage(r.Context(), userID)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{
		"quota_total":     report.QuotaTotal,
		"quota_used":      report.QuotaUsed,
		"quota_reserved":  report.QuotaReserved,
		"quota_remaining": report.QuotaRemaining,
		"calls_today":     report.CallsToday,
		"calls_total":     report.CallsTotal,
		"success_total":   report.SuccessTotal,
		"failed_total":    report.FailedTotal,
		"rejected_total":  report.RejectedTotal,
		"success_rate":    report.SuccessRate,
	})
}

// handleListCalls serves the owner-scoped call log with cursor pagination.
func (s *Server) handleListCalls(w http.ResponseWriter, r *http.Request) {
	userID := *parseUUID(userFromContext(r).UserID)

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeApplicationError(w, errInvalidRequest)
			return
		}
		limit = parsed
	}
	var cursor *time.Time
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeApplicationError(w, errInvalidRequest)
			return
		}
		cursor = &parsed
	}

	page, appErr := s.services.ListCalls(r.Context(), userID, cursor, limit)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, serializeCall(record))
	}
	var nextCursor any
	if page.NextCursor != nil {
		nextCursor = page.NextCursor.UTC().Format(time.RFC3339Nano)
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": nextCursor,
	})
}

// handleGetCall resolves one call by request id with ownership enforced.
func (s *Server) handleGetCall(w http.ResponseWriter, r *http.Request) {
	userID := *parseUUID(userFromContext(r).UserID)
	requestID := r.PathValue("request_id")
	if requestID == "" {
		writeApplicationError(w, errInvalidRequest)
		return
	}
	record, appErr := s.services.GetCall(r.Context(), userID, requestID)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, serializeCall(record))
}
