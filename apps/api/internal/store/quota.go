package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Ledger entry types per the V1 quota model.
const (
	EntryReserve = "RESERVE"
	EntryConfirm = "CONFIRM"
	EntryRefund  = "REFUND"
)

// ErrQuotaExhausted is the typed signal from ReserveQuota when no row was
// updated because quota_remaining hit zero.
var ErrQuotaExhausted = errors.New("no remaining quota")

// QuotaSnapshot captures the three counters around a ledger mutation.
type QuotaSnapshot struct {
	Remaining int64
	Used      int64
	Reserved  int64
}

func readQuota(ctx context.Context, q Querier, cdkID uuid.UUID) (QuotaSnapshot, error) {
	var snap QuotaSnapshot
	err := q.QueryRow(ctx, `SELECT quota_remaining, quota_used, quota_reserved FROM cdks WHERE id = $1`, cdkID).
		Scan(&snap.Remaining, &snap.Used, &snap.Reserved)
	return snap, err
}

// ReserveQuota atomically moves one unit remaining -> reserved and appends a
// RESERVE ledger row in the same transaction. The conditional UPDATE is the
// admission gate: zero rows affected means QUOTA_EXHAUSTED.
func ReserveQuota(ctx context.Context, q Querier, cdkID, userID, apiCallID uuid.UUID, requestID, reason string) error {
	tag, err := q.Exec(ctx, `
		UPDATE cdks
		SET quota_remaining = quota_remaining - 1,
		    quota_reserved  = quota_reserved + 1,
		    updated_at      = now()
		WHERE id = $1 AND quota_remaining > 0
	`, cdkID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrQuotaExhausted
	}

	before, err := readQuota(ctx, q, cdkID)
	if err != nil {
		return err
	}
	// After the update: remaining+1 was the pre-value, reserved-1 likewise.
	_, err = q.Exec(ctx, `
		INSERT INTO quota_ledger (id, cdk_id, user_id, api_call_id, entry_type,
		                          available_before, delta_available, available_after,
		                          used_before, used_after, reserved_before, reserved_after,
		                          reason, request_id, actor_type)
		VALUES ($1, $2, $3, $4, $5, $6, -1, $7, $8, $8, $9, $10, $11, $12, 'user')
	`, uuid.New(), cdkID, userID, apiCallID, EntryReserve,
		before.Remaining+1, before.Remaining,
		before.Used, before.Reserved-1, before.Reserved,
		reason, requestID)
	return err
}

// ConfirmQuota moves one unit reserved -> used and appends CONFIRM. The
// unique (api_call_id, entry_type) constraint makes retries safe.
func ConfirmQuota(ctx context.Context, q Querier, cdkID, userID, apiCallID uuid.UUID, requestID, reason string) error {
	_, err := q.Exec(ctx, `
		UPDATE cdks
		SET quota_used     = quota_used + 1,
		    quota_reserved = quota_reserved - 1,
		    quota_remaining = quota_remaining,
		    last_used_at   = now(),
		    updated_at     = now()
		WHERE id = $1
	`, cdkID)
	if err != nil {
		return err
	}

	before, err := readQuota(ctx, q, cdkID)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO quota_ledger (id, cdk_id, user_id, api_call_id, entry_type,
		                          available_before, delta_available, available_after,
		                          used_before, used_after, reserved_before, reserved_after,
		                          reason, request_id, actor_type)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $6, $7, $8, $9, $10, $11, $12, 'user')
	`, uuid.New(), cdkID, userID, apiCallID, EntryConfirm,
		before.Remaining, before.Used-1, before.Used, before.Reserved-1, before.Reserved,
		reason, requestID)
	return err
}

// RefundQuota moves one unit reserved -> remaining and appends REFUND.
// Calling it twice for the same api_call_id fails on the ledger unique
// constraint, enforcing exactly-once refunds.
func RefundQuota(ctx context.Context, q Querier, cdkID, userID, apiCallID uuid.UUID, requestID, reason string) error {
	_, err := q.Exec(ctx, `
		UPDATE cdks
		SET quota_reserved  = quota_reserved - 1,
		    quota_remaining = quota_remaining + 1,
		    updated_at      = now()
		WHERE id = $1
	`, cdkID)
	if err != nil {
		return err
	}

	before, err := readQuota(ctx, q, cdkID)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO quota_ledger (id, cdk_id, user_id, api_call_id, entry_type,
		                          available_before, delta_available, available_after,
		                          used_before, used_after, reserved_before, reserved_after,
		                          reason, request_id, actor_type)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, $8, $8, $9, $10, $11, $12, 'system')
	`, uuid.New(), cdkID, userID, apiCallID, EntryRefund,
		before.Remaining-1, before.Remaining, before.Used, before.Reserved-1, before.Reserved,
		reason, requestID)
	if IsUniqueViolation(err) {
		return nil // refund already recorded: exactly-once satisfied
	}
	return err
}

// QuotaTotals returns the current counters for a CDK.
func QuotaTotals(ctx context.Context, q Querier, cdkID uuid.UUID) (QuotaSnapshot, error) {
	return readQuota(ctx, q, cdkID)
}
