package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/google/uuid"
)

// Ledger entry types per the V1 quota model.
const (
	EntryReserve         = "RESERVE"
	EntryConfirm         = "CONFIRM"
	EntryRefund          = "REFUND"
	EntryAdminAdjustment = "ADMIN_ADJUSTMENT"
)

// ErrQuotaExhausted is the typed signal from ReserveQuota when no row was
// updated because quota_remaining hit zero.
var ErrQuotaExhausted = errors.New("no remaining quota")

// QuotaSnapshot captures the quota counters around a ledger mutation.
type QuotaSnapshot struct {
	Remaining int64
	Used      int64
	Reserved  int64
	Total     int64
}

func readQuota(ctx context.Context, q Querier, cdkID uuid.UUID) (QuotaSnapshot, error) {
	var snap QuotaSnapshot
	err := q.QueryRow(ctx, `SELECT quota_remaining, quota_used, quota_reserved, quota_total FROM cdks WHERE id = $1`, cdkID).
		Scan(&snap.Remaining, &snap.Used, &snap.Reserved, &snap.Total)
	return snap, err
}

// InsertQuotaLedgerEntry appends an operator ledger row (e.g.
// ADMIN_ADJUSTMENT) without touching counters; the caller performs the
// counter update inside the same transaction. userID may be nil for CDKs
// that no user has activated yet.
func InsertQuotaLedgerEntry(ctx context.Context, q Querier, cdkID uuid.UUID, userID *uuid.UUID, adminID uuid.UUID, after QuotaSnapshot, delta int64, entryType, reason string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO quota_ledger (id, cdk_id, user_id, entry_type,
		                          available_before, delta_available, available_after,
		                          used_before, used_after, reserved_before, reserved_after,
		                          reason, request_id, actor_type, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, $9, $10, $11, 'admin', $12)
	`, uuid.New(), cdkID, userID, entryType,
		after.Remaining-delta, delta, after.Remaining, after.Used, after.Reserved,
		reason, "admin:"+adminID.String(), adminID)
	return err
}

// ReserveQuota atomically moves one unit remaining -> reserved and appends a
// RESERVE ledger row in the same transaction. The conditional UPDATE is the
// admission gate: zero rows affected means QUOTA_EXHAUSTED.
func ReserveQuota(ctx context.Context, pool *pgxpool.Pool, cdkID, userID, apiCallID uuid.UUID, requestID, reason string) error {
	return RunInTx(ctx, pool, func(ctx context.Context, q Querier) error {
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
	})
}

// ConfirmQuota moves one unit reserved -> used and appends CONFIRM. The
// unique (api_call_id, entry_type) constraint makes retries safe.
func ConfirmQuota(ctx context.Context, pool *pgxpool.Pool, cdkID, userID, apiCallID uuid.UUID, requestID, reason string) error {
	return RunInTx(ctx, pool, func(ctx context.Context, q Querier) error {
		tag, err := q.Exec(ctx, `
			UPDATE cdks
			SET quota_used     = quota_used + 1,
			    quota_reserved = quota_reserved - 1,
			    quota_remaining = quota_remaining,
			    last_used_at   = now(),
			    updated_at     = now()
			WHERE id = $1 AND quota_reserved > 0
		`, cdkID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound // nothing reserved to confirm; treat as no-op
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
	})
}

// RefundQuota moves one unit reserved -> remaining and appends REFUND.
// Calling it twice for the same api_call_id fails on the ledger unique
// constraint, enforcing exactly-once refunds.
func RefundQuota(ctx context.Context, pool *pgxpool.Pool, cdkID, userID, apiCallID uuid.UUID, requestID, reason string) error {
	return RunInTx(ctx, pool, func(ctx context.Context, q Querier) error {
		// Exactly-once: check the ledger row BEFORE touching counters so a
		// repeated refund can never double-apply the movement.
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM quota_ledger WHERE api_call_id = $1 AND entry_type = $2)`, apiCallID, EntryRefund).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}

		tag, err := q.Exec(ctx, `
			UPDATE cdks
			SET quota_reserved  = quota_reserved - 1,
			    quota_remaining = quota_remaining + 1,
			    updated_at      = now()
			WHERE id = $1 AND quota_reserved > 0
		`, cdkID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // nothing reserved (already settled): keep exactly-once
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
		return err
	})
}

// QuotaTotals returns the current counters for a CDK.
func QuotaTotals(ctx context.Context, q Querier, cdkID uuid.UUID) (QuotaSnapshot, error) {
	return readQuota(ctx, q, cdkID)
}
