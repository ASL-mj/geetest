package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/store"
)

const adminCursorSeparator = "~"

func adminLimit(raw string) (int, bool) {
	if raw == "" {
		return 20, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, false
	}
	return limit, true
}

func adminTime(raw string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, false
	}
	value = value.UTC()
	return &value, true
}

func parseAdminKeyCursor(raw string) (*store.AdminAPIKeyCursor, bool) {
	createdRaw, idRaw, ok := strings.Cut(raw, adminCursorSeparator)
	if !ok || createdRaw == "" || idRaw == "" {
		return nil, false
	}
	created, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return nil, false
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return nil, false
	}
	return &store.AdminAPIKeyCursor{CreatedAt: created, ID: id}, true
}

func formatAdminKeyCursor(cursor store.AdminAPIKeyCursor) string {
	return cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + adminCursorSeparator + cursor.ID.String()
}

func parseLedgerCursor(raw string) (*store.LedgerCursor, bool) {
	createdRaw, idRaw, ok := strings.Cut(raw, adminCursorSeparator)
	if !ok || createdRaw == "" || idRaw == "" {
		return nil, false
	}
	created, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return nil, false
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return nil, false
	}
	return &store.LedgerCursor{CreatedAt: created, ID: id}, true
}

func formatLedgerCursor(cursor store.LedgerCursor) string {
	return cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + adminCursorSeparator + cursor.ID.String()
}

func parseAdminUUIDQuery(r *http.Request, name string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, false
	}
	return &id, true
}

func serializeAdminAPIKey(item store.AdminAPIKeyRow) map[string]any {
	return map[string]any{
		"id":           item.ID,
		"user_id":      item.UserID,
		"cdk_id":       item.CDKID,
		"cdk_prefix":   item.CDKPrefix,
		"user_status":  item.UserStatus,
		"cdk_status":   item.CDKStatus,
		"name":         item.Name,
		"prefix":       item.KeyPrefix,
		"last4":        item.KeyLast4,
		"status":       item.Status,
		"total_calls":  item.TotalCalls,
		"created_at":   item.CreatedAt.UTC().Format(timeFormatUTC),
		"last_used_at": formatTimePtr(item.LastUsedAt),
	}
}

func serializeAdminCall(item store.AdminCallRow) map[string]any {
	data := serializeCall(item.CallRecord)
	data["user_id"] = item.UserID
	data["cdk_id"] = item.CDKID
	data["api_key_id"] = item.APIKeyID
	return data
}

func serializeLedgerEntry(item store.AdminQuotaLedgerEntry) map[string]any {
	return map[string]any{
		"id":               item.ID,
		"cdk_id":           item.CDKID,
		"user_id":          item.UserID,
		"api_call_id":      item.APICallID,
		"entry_type":       item.EntryType,
		"available_before": item.AvailableBefore,
		"delta_available":  item.DeltaAvailable,
		"available_after":  item.AvailableAfter,
		"used_before":      item.UsedBefore,
		"used_after":       item.UsedAfter,
		"reserved_before":  item.ReservedBefore,
		"reserved_after":   item.ReservedAfter,
		"reason":           item.Reason,
		"request_id":       item.RequestID,
		"actor_type":       item.ActorType,
		"actor_id":         item.ActorID,
		"created_at":       item.CreatedAt.UTC().Format(timeFormatUTC),
	}
}

// handleAdminListAPIKeys returns masked key records with ownership filters.
func (s *Server) handleAdminListAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseAdminUUIDQuery(r, "user_id")
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	limit, ok := adminLimit(r.URL.Query().Get("limit"))
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var cursor *store.AdminAPIKeyCursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, ok = parseAdminKeyCursor(raw)
		if !ok {
			writeApplicationError(w, service.ErrInvalidRequest())
			return
		}
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))
	if len(status) > 32 || len(prefix) > 32 {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	items, err := store.ListAPIKeysForAdmin(r.Context(), s.services.Pool, store.AdminAPIKeyFilter{
		UserID: userID, Status: status, KeyPrefix: prefix,
	}, cursor, limit+1)
	if err != nil {
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	var next any
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = formatAdminKeyCursor(store.AdminAPIKeyCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	serialized := make([]map[string]any, 0, len(items))
	for _, item := range items {
		serialized = append(serialized, serializeAdminAPIKey(item))
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": serialized, "next_cursor": next})
}

// handleAdminListCalls returns redacted calls with operator filters.
func (s *Server) handleAdminListCalls(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	userID, ok := parseAdminUUIDQuery(r, "user_id")
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	cdkID, ok := parseAdminUUIDQuery(r, "cdk_id")
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	apiKeyID, ok := parseAdminUUIDQuery(r, "api_key_id")
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	from, ok := adminTime(query.Get("from"))
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	to, ok := adminTime(query.Get("to"))
	if !ok || (from != nil && to != nil && !from.Before(*to)) {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	limit, ok := adminLimit(query.Get("limit"))
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var cursor *store.CallCursor
	if raw := query.Get("cursor"); raw != "" {
		cursor, ok = parseCallCursor(raw)
		if !ok {
			writeApplicationError(w, service.ErrInvalidRequest())
			return
		}
	}
	filter := store.AdminCallFilter{
		UserID: userID, CDKID: cdkID, APIKeyID: apiKeyID,
		CaptchaID: strings.TrimSpace(query.Get("captcha_id")),
		Status:    strings.TrimSpace(query.Get("status")),
		RequestID: strings.TrimSpace(query.Get("request_id")),
		From:      from, To: to,
	}
	if len(filter.CaptchaID) > 256 || len(filter.Status) > 32 || len(filter.RequestID) > 64 {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	items, err := store.ListCallsForAdmin(r.Context(), s.services.Pool, filter, cursor, limit+1)
	if err != nil {
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	var next any
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = formatCallCursor(store.CallCursor{AcceptedAt: last.AcceptedAt, RequestID: last.RequestID})
	}
	serialized := make([]map[string]any, 0, len(items))
	for _, item := range items {
		serialized = append(serialized, serializeAdminCall(item))
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": serialized, "next_cursor": next})
}

// handleAdminListQuotaLedger returns immutable quota movements with filters.
func (s *Server) handleAdminListQuotaLedger(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cdkID, ok := parseAdminUUIDQuery(r, "cdk_id")
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	userID, ok := parseAdminUUIDQuery(r, "user_id")
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	from, ok := adminTime(query.Get("from"))
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	to, ok := adminTime(query.Get("to"))
	if !ok || (from != nil && to != nil && !from.Before(*to)) {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	limit, ok := adminLimit(query.Get("limit"))
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	var cursor *store.LedgerCursor
	if raw := query.Get("cursor"); raw != "" {
		cursor, ok = parseLedgerCursor(raw)
		if !ok {
			writeApplicationError(w, service.ErrInvalidRequest())
			return
		}
	}
	entryType := strings.TrimSpace(query.Get("entry_type"))
	requestID := strings.TrimSpace(query.Get("request_id"))
	if len(entryType) > 32 || len(requestID) > 64 {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}
	items, err := store.ListQuotaLedgerForAdmin(r.Context(), s.services.Pool, store.AdminLedgerFilter{
		CDKID: cdkID, UserID: userID, EntryType: entryType, RequestID: requestID,
		From: from, To: to,
	}, cursor, limit+1)
	if err != nil {
		writeApplicationError(w, service.NewError(500, "INTERNAL_ERROR", "Internal server error."))
		return
	}
	var next any
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = formatLedgerCursor(store.LedgerCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	serialized := make([]map[string]any, 0, len(items))
	for _, item := range items {
		serialized = append(serialized, serializeLedgerEntry(item))
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": serialized, "next_cursor": next})
}
