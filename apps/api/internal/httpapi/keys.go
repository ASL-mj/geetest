package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/domain"
	"github.com/captchaflow/service-platform/api/internal/service"
)

// serializeAPIKey renders the masked key representation; the plaintext
// secret never appears here. Timestamps are normalized to UTC per the V1
// contract.
func serializeAPIKey(key domain.APIKey) map[string]any {
	data := map[string]any{
		"id":          key.ID,
		"name":        key.Name,
		"prefix":      key.KeyPrefix,
		"last4":       key.KeyLast4,
		"status":      string(key.Status),
		"total_calls": key.TotalCalls,
		"created_at":  key.CreatedAt.UTC().Format(timeFormat),
	}
	if key.LastUsedAt != nil {
		data["last_used_at"] = key.LastUsedAt.UTC().Format(timeFormat)
	} else {
		data["last_used_at"] = nil
	}
	return data
}

const timeFormat = "2006-01-02T15:04:05.999999-07:00"

func parseUUID(value string) *uuid.UUID {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &parsed
}

// validateKeyName applies the shared 1..128 trimmed-length rule.
func validateKeyName(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if utf8Len(trimmed) < 1 || utf8Len(trimmed) > 128 {
		return "", false
	}
	return trimmed, true
}

func utf8Len(value string) int { return len([]rune(value)) }

// handleCreateKey returns 201 with the plaintext secret exactly once.
func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	name, ok := validateKeyName(payload.Name)
	if !ok {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	created, appErr := s.services.CreateAPIKey(r.Context(), *parseUUID(userFromContext(r).UserID), name)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}

	data := serializeAPIKey(created.Record)
	data["secret"] = created.Secret
	writeSuccess(w, newRequestID(), http.StatusCreated, data)
}

// handleListKeys returns the caller's keys, newest first, never with secrets.
func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	keys, appErr := s.services.ListAPIKeys(r.Context(), *parseUUID(userFromContext(r).UserID))
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, serializeAPIKey(key))
	}
	writeSuccess(w, newRequestID(), http.StatusOK, map[string]any{"items": items})
}

// handleUpdateKey renames or toggles a key; at least one field must change.
func (s *Server) handleUpdateKey(w http.ResponseWriter, r *http.Request) {
	keyID := parseUUID(r.PathValue("key_id"))
	if keyID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	var payload struct {
		Name   *string `json:"name"`
		Status *string `json:"status"`
	}
	if !decodeJSON(w, r, &payload) {
		return
	}
	if payload.Name == nil && payload.Status == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	var name *string
	if payload.Name != nil {
		trimmed, ok := validateKeyName(*payload.Name)
		if !ok {
			writeApplicationError(w, service.ErrInvalidRequest())
			return
		}
		name = &trimmed
	}
	var status *domain.APIKeyStatus
	if payload.Status != nil {
		parsed := domain.APIKeyStatus(*payload.Status)
		if parsed != domain.APIKeyStatusActive && parsed != domain.APIKeyStatusDisabled {
			writeApplicationError(w, service.ErrInvalidRequest())
			return
		}
		status = &parsed
	}

	updated, appErr := s.services.UpdateAPIKey(r.Context(), *parseUUID(userFromContext(r).UserID), *keyID, name, status)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, serializeAPIKey(updated))
}

// handleDeleteKey soft-deletes a key into the DELETED terminal state.
func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	keyID := parseUUID(r.PathValue("key_id"))
	if keyID == nil {
		writeApplicationError(w, service.ErrInvalidRequest())
		return
	}

	deleted, appErr := s.services.DeleteAPIKey(r.Context(), *parseUUID(userFromContext(r).UserID), *keyID)
	if appErr != nil {
		writeApplicationError(w, appErr)
		return
	}
	writeSuccess(w, newRequestID(), http.StatusOK, serializeAPIKey(deleted))
}
