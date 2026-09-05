package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminAPIKeyFilter contains the server-side filters exposed to operators.
// Empty fields mean no filter; secrets are deliberately absent from this
// query model.
type AdminAPIKeyFilter struct {
	UserID    *uuid.UUID
	Status    string
	KeyPrefix string
}

// AdminAPIKeyCursor is the stable descending boundary for key pagination.
type AdminAPIKeyCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// AdminAPIKeyRow is an operator-visible key record with effective ownership
// state. It never contains the bearer secret or its hash.
type AdminAPIKeyRow struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	CDKID      *uuid.UUID
	CDKPrefix  *string
	UserStatus string
	CDKStatus  *string
	Name       string
	KeyPrefix  string
	KeyLast4   string
	Status     string
	TotalCalls int64
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// ListAPIKeysForAdmin returns newest keys first with ownership and status
// filters. The caller supplies limit+1 for look-ahead pagination.
func ListAPIKeysForAdmin(ctx context.Context, q Querier, filter AdminAPIKeyFilter, cursor *AdminAPIKeyCursor, limit int) ([]AdminAPIKeyRow, error) {
	sql := `
		SELECT k.id, k.user_id, c.id, c.code_prefix, u.status, c.status,
		       k.name, k.key_prefix, k.key_last4, k.status, k.total_calls,
		       k.created_at, k.last_used_at
		FROM api_keys k
		JOIN users u ON u.id = k.user_id
		LEFT JOIN cdks c ON c.bound_user_id = k.user_id
		WHERE true`
	args := make([]any, 0, 6)
	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		sql += ` AND k.user_id = $` + itoa(len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		sql += ` AND k.status = $` + itoa(len(args))
	}
	if filter.KeyPrefix != "" {
		args = append(args, filter.KeyPrefix+"%")
		sql += ` AND k.key_prefix LIKE $` + itoa(len(args))
	}
	if cursor != nil {
		args = append(args, cursor.CreatedAt, cursor.ID)
		sql += ` AND (k.created_at, k.id) < ($` + itoa(len(args)-1) + `, $` + itoa(len(args)) + `)`
	}
	args = append(args, limit)
	sql += ` ORDER BY k.created_at DESC, k.id DESC LIMIT $` + itoa(len(args))

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminAPIKeyRow, 0, limit)
	for rows.Next() {
		var item AdminAPIKeyRow
		if err := rows.Scan(&item.ID, &item.UserID, &item.CDKID, &item.CDKPrefix, &item.UserStatus,
			&item.CDKStatus, &item.Name, &item.KeyPrefix, &item.KeyLast4, &item.Status,
			&item.TotalCalls, &item.CreatedAt, &item.LastUsedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// AdminCallFilter contains operator call-log filters. From is inclusive and
// To is exclusive, both interpreted as UTC instants.
type AdminCallFilter struct {
	UserID    *uuid.UUID
	CDKID     *uuid.UUID
	APIKeyID  *uuid.UUID
	CaptchaID string
	Status    string
	RequestID string
	From      *time.Time
	To        *time.Time
}

// AdminCallRow is a redacted call record plus stable ownership identifiers.
type AdminCallRow struct {
	CallRecord
	UserID   uuid.UUID
	CDKID    uuid.UUID
	APIKeyID uuid.UUID
}

// ListCallsForAdmin returns filtered, redacted call records. No response JSON,
// API key material, solver token or client IP hash is selected.
func ListCallsForAdmin(ctx context.Context, q Querier, filter AdminCallFilter, cursor *CallCursor, limit int) ([]AdminCallRow, error) {
	sql := `
		SELECT a.request_id, a.user_id, a.cdk_id, a.api_key_id,
		       a.api_key_name_snapshot, a.api_key_prefix_snapshot, a.captcha_id, a.risk_type,
		       a.status, a.http_status, a.error_code, a.error_summary, a.accepted_at,
		       a.completed_at, a.duration_ms, a.quota_reserved, a.quota_refunded,
		       a.client_ip_masked::text, a.user_agent
		FROM api_calls a
		WHERE true`
	args := make([]any, 0, 12)
	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		sql += ` AND a.user_id = $` + itoa(len(args))
	}
	if filter.CDKID != nil {
		args = append(args, *filter.CDKID)
		sql += ` AND a.cdk_id = $` + itoa(len(args))
	}
	if filter.APIKeyID != nil {
		args = append(args, *filter.APIKeyID)
		sql += ` AND a.api_key_id = $` + itoa(len(args))
	}
	if filter.CaptchaID != "" {
		args = append(args, filter.CaptchaID)
		sql += ` AND a.captcha_id = $` + itoa(len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		sql += ` AND a.status = $` + itoa(len(args))
	}
	if filter.RequestID != "" {
		args = append(args, filter.RequestID)
		sql += ` AND a.request_id = $` + itoa(len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		sql += ` AND a.accepted_at >= $` + itoa(len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		sql += ` AND a.accepted_at < $` + itoa(len(args))
	}
	if cursor != nil {
		args = append(args, cursor.AcceptedAt, cursor.RequestID)
		sql += ` AND (a.accepted_at, a.request_id) < ($` + itoa(len(args)-1) + `, $` + itoa(len(args)) + `)`
	}
	args = append(args, limit)
	sql += ` ORDER BY a.accepted_at DESC, a.request_id DESC LIMIT $` + itoa(len(args))

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminCallRow, 0, limit)
	for rows.Next() {
		var item AdminCallRow
		if err := rows.Scan(&item.RequestID, &item.UserID, &item.CDKID, &item.APIKeyID,
			&item.APIKeyName, &item.APIKeyPrefix, &item.CaptchaID, &item.RiskType,
			&item.Status, &item.HTTPStatus, &item.ErrorCode, &item.ErrorSummary,
			&item.AcceptedAt, &item.CompletedAt, &item.DurationMS, &item.QuotaReserved,
			&item.QuotaRefunded, &item.ClientIPMasked, &item.UserAgent); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// AdminLedgerFilter contains operator quota-ledger filters.
type AdminLedgerFilter struct {
	CDKID     *uuid.UUID
	UserID    *uuid.UUID
	EntryType string
	RequestID string
	From      *time.Time
	To        *time.Time
}

// LedgerCursor is the stable descending boundary for ledger pagination.
type LedgerCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// AdminQuotaLedgerEntry exposes the immutable quota movement and its audit
// context. Nullable owners are allowed for pre-activation administrator
// adjustments.
type AdminQuotaLedgerEntry struct {
	ID              uuid.UUID
	CDKID           uuid.UUID
	UserID          *uuid.UUID
	APICallID       *uuid.UUID
	EntryType       string
	AvailableBefore int64
	DeltaAvailable  int64
	AvailableAfter  int64
	UsedBefore      int64
	UsedAfter       int64
	ReservedBefore  int64
	ReservedAfter   int64
	Reason          string
	RequestID       string
	ActorType       string
	ActorID         *uuid.UUID
	CreatedAt       time.Time
}

// ListQuotaLedgerForAdmin returns newest ledger entries with server-side
// filters and cursor pagination.
func ListQuotaLedgerForAdmin(ctx context.Context, q Querier, filter AdminLedgerFilter, cursor *LedgerCursor, limit int) ([]AdminQuotaLedgerEntry, error) {
	sql := `
		SELECT id, cdk_id, user_id, api_call_id, entry_type,
		       available_before, delta_available, available_after,
		       used_before, used_after, reserved_before, reserved_after,
		       reason, request_id, actor_type, actor_id, created_at
		FROM quota_ledger
		WHERE true`
	args := make([]any, 0, 10)
	if filter.CDKID != nil {
		args = append(args, *filter.CDKID)
		sql += ` AND cdk_id = $` + itoa(len(args))
	}
	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		sql += ` AND user_id = $` + itoa(len(args))
	}
	if filter.EntryType != "" {
		args = append(args, filter.EntryType)
		sql += ` AND entry_type = $` + itoa(len(args))
	}
	if filter.RequestID != "" {
		args = append(args, filter.RequestID)
		sql += ` AND request_id = $` + itoa(len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		sql += ` AND created_at >= $` + itoa(len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		sql += ` AND created_at < $` + itoa(len(args))
	}
	if cursor != nil {
		args = append(args, cursor.CreatedAt, cursor.ID)
		sql += ` AND (created_at, id) < ($` + itoa(len(args)-1) + `, $` + itoa(len(args)) + `)`
	}
	args = append(args, limit)
	sql += ` ORDER BY created_at DESC, id DESC LIMIT $` + itoa(len(args))

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdminQuotaLedgerEntry, 0, limit)
	for rows.Next() {
		var item AdminQuotaLedgerEntry
		if err := rows.Scan(&item.ID, &item.CDKID, &item.UserID, &item.APICallID, &item.EntryType,
			&item.AvailableBefore, &item.DeltaAvailable, &item.AvailableAfter,
			&item.UsedBefore, &item.UsedAfter, &item.ReservedBefore, &item.ReservedAfter,
			&item.Reason, &item.RequestID, &item.ActorType, &item.ActorID, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
