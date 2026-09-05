package store

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CDKSummary is the CDK state shown on the user account page.
type CDKSummary struct {
	CodePrefix     string
	Status         string
	ActivatedAt    *time.Time
	ExpiresAt      *time.Time
	QuotaTotal     int64
	QuotaUsed      int64
	QuotaReserved  int64
	QuotaRemaining int64
}

// GetCDKSummaryForUser loads the CDK bound to a user; every user has exactly
// one CDK (activation creates the binding).
func GetCDKSummaryForUser(ctx context.Context, q Querier, userID uuid.UUID) (CDKSummary, error) {
	var summary CDKSummary
	err := q.QueryRow(ctx, `
		SELECT code_prefix, status, activated_at, expires_at,
		       quota_total, quota_used, quota_reserved, quota_remaining
		FROM cdks WHERE bound_user_id = $1
	`, userID).Scan(&summary.CodePrefix, &summary.Status, &summary.ActivatedAt, &summary.ExpiresAt,
		&summary.QuotaTotal, &summary.QuotaUsed, &summary.QuotaReserved, &summary.QuotaRemaining)
	if err != nil {
		if err == pgx.ErrNoRows {
			return CDKSummary{}, ErrNotFound
		}
		return CDKSummary{}, err
	}
	return summary, nil
}

// UsageCounters aggregates one user's call history for the usage endpoint.
type UsageCounters struct {
	CallsToday    int64
	CallsTotal    int64
	SuccessTotal  int64
	FailedTotal   int64
	RejectedTotal int64
}

// GetUsageCounters returns lifetime and current-day call aggregates. Days
// follow UTC so the counter resets deterministically.
func GetUsageCounters(ctx context.Context, q Querier, userID uuid.UUID) (UsageCounters, error) {
	var counters UsageCounters
	err := q.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE accepted_at >= date_trunc('day', now() AT TIME ZONE 'utc')),
			count(*),
			count(*) FILTER (WHERE status = 'SUCCEEDED'),
			count(*) FILTER (WHERE status = 'FAILED_REFUNDED'),
			count(*) FILTER (WHERE status = 'REJECTED')
		FROM api_calls WHERE user_id = $1
	`, userID).Scan(&counters.CallsToday, &counters.CallsTotal, &counters.SuccessTotal,
		&counters.FailedTotal, &counters.RejectedTotal)
	return counters, err
}

// CallRecord is a redacted call-log entry; it never carries response bodies
// or client identity beyond the masked network.
type CallRecord struct {
	RequestID      string
	APIKeyName     string
	APIKeyPrefix   string
	CaptchaID      string
	RiskType       string
	Status         string
	HTTPStatus     int
	ErrorCode      *string
	ErrorSummary   *string
	AcceptedAt     time.Time
	CompletedAt    *time.Time
	DurationMS     *int
	QuotaReserved  bool
	QuotaRefunded  bool
	ClientIPMasked string
	UserAgent      string
}

// CallCursor is the stable boundary for descending call-log pagination. The
// request id breaks accepted_at ties created by concurrent calls.
type CallCursor struct {
	AcceptedAt time.Time
	RequestID  string
}

// ListCallsForUser returns the newest calls of one user with cursor
// pagination. Both cursor fields participate in the exclusive boundary so
// records sharing a timestamp are neither skipped nor duplicated.
func ListCallsForUser(ctx context.Context, q Querier, userID uuid.UUID, cursor *CallCursor, limit int) ([]CallRecord, error) {
	sql := `
		SELECT request_id, api_key_name_snapshot, api_key_prefix_snapshot, captcha_id, risk_type,
		       status, http_status, error_code, error_summary, accepted_at, completed_at, duration_ms,
		       quota_reserved, quota_refunded, client_ip_masked::text, user_agent
		FROM api_calls
		WHERE user_id = $1`
	args := []any{userID}
	if cursor != nil {
		sql += ` AND (accepted_at, request_id) < ($2, $3)`
		args = append(args, cursor.AcceptedAt, cursor.RequestID)
	}
	sql += ` ORDER BY accepted_at DESC, request_id DESC LIMIT $` + itoa(len(args)+1)
	args = append(args, limit)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]CallRecord, 0, limit)
	for rows.Next() {
		var record CallRecord
		if err := rows.Scan(&record.RequestID, &record.APIKeyName, &record.APIKeyPrefix, &record.CaptchaID,
			&record.RiskType, &record.Status, &record.HTTPStatus, &record.ErrorCode, &record.ErrorSummary, &record.AcceptedAt,
			&record.CompletedAt, &record.DurationMS, &record.QuotaReserved, &record.QuotaRefunded,
			&record.ClientIPMasked, &record.UserAgent); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// GetCallForUser resolves one call by request id with ownership enforced in
// the WHERE clause so foreign request ids are indistinguishable from
// nonexistent ones.
func GetCallForUser(ctx context.Context, q Querier, userID uuid.UUID, requestID string) (CallRecord, error) {
	var record CallRecord
	err := q.QueryRow(ctx, `
		SELECT request_id, api_key_name_snapshot, api_key_prefix_snapshot, captcha_id, risk_type,
		       status, http_status, error_code, error_summary, accepted_at, completed_at, duration_ms,
		       quota_reserved, quota_refunded, client_ip_masked::text, user_agent
		FROM api_calls
		WHERE user_id = $1 AND request_id = $2
	`, userID, requestID).Scan(&record.RequestID, &record.APIKeyName, &record.APIKeyPrefix, &record.CaptchaID,
		&record.RiskType, &record.Status, &record.HTTPStatus, &record.ErrorCode, &record.ErrorSummary, &record.AcceptedAt,
		&record.CompletedAt, &record.DurationMS, &record.QuotaReserved, &record.QuotaRefunded,
		&record.ClientIPMasked, &record.UserAgent)
	if err != nil {
		if err == pgx.ErrNoRows {
			return CallRecord{}, ErrNotFound
		}
		return CallRecord{}, err
	}
	return record, nil
}

func itoa(n int) string { return strconv.Itoa(n) }
