package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/store"
)

// TestSweepStaleConfirmedCallReversesSettlement covers a crash after quota
// confirmation but before the call row is finalized. Recovery declares the
// call failed, so it must also reverse the confirmed quota and key counter.
func TestSweepStaleConfirmedCallReversesSettlement(t *testing.T) {
	h := newHarness(t)
	secret, _ := activateWithSession(t, h)
	caller, appErr := h.services.AuthenticateAPIKey(context.Background(), secret)
	if appErr != nil {
		t.Fatalf("authenticate setup key: %v", appErr)
	}

	call := store.APICall{
		ID:                   uuid.New(),
		RequestID:            "req_crash_after_confirm",
		Operation:            "captcha.solve",
		IdempotencyKeyHash:   []byte("crash-after-confirm"),
		UserID:               caller.UserID,
		CDKID:                caller.CDKID,
		APIKeyID:             caller.APIKeyID,
		APIKeyNameSnapshot:   caller.APIKey.Name,
		APIKeyPrefixSnapshot: caller.APIKey.KeyPrefix,
		CaptchaID:            "cap-crash",
		RiskType:             "slide",
		ClientIPMasked:       "192.0.2.0/24",
		ClientIPHash:         []byte("test-ip-hash"),
		UserAgent:            "recovery-test",
	}
	ctx := context.Background()
	if err := store.CreateAPICall(ctx, h.services.Pool, call); err != nil {
		t.Fatalf("create call: %v", err)
	}
	if admitted, err := store.ConsumeAPIKeyQuota(ctx, h.services.Pool, call.APIKeyID); err != nil || !admitted {
		t.Fatalf("admit key: admitted=%t err=%v", admitted, err)
	}
	if err := store.ReserveQuota(ctx, h.services.Pool, call.CDKID, call.UserID, call.ID, call.RequestID, "reserve"); err != nil {
		t.Fatalf("reserve quota: %v", err)
	}
	if err := store.ReserveAPICall(ctx, h.services.Pool, call.ID); err != nil {
		t.Fatalf("mark reserved: %v", err)
	}
	if err := store.DispatchAPICall(ctx, h.services.Pool, call.ID); err != nil {
		t.Fatalf("mark dispatched: %v", err)
	}
	if err := store.ConfirmQuota(ctx, h.services.Pool, call.CDKID, call.UserID, call.ID, call.RequestID, "confirm"); err != nil {
		t.Fatalf("confirm quota: %v", err)
	}
	if _, err := h.services.Pool.Exec(ctx, `UPDATE api_calls SET accepted_at = now() - interval '1 hour' WHERE id = $1`, call.ID); err != nil {
		t.Fatalf("age call: %v", err)
	}

	reaped, err := store.SweepStaleInFlightCalls(ctx, h.services.Pool, time.Minute)
	if err != nil {
		t.Fatalf("sweep stale call: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("expected one reaped call, got %d", reaped)
	}

	summary, err := store.GetCDKSummaryForUser(ctx, h.services.Pool, caller.UserID)
	if err != nil {
		t.Fatalf("quota summary: %v", err)
	}
	if summary.QuotaRemaining != 100 || summary.QuotaUsed != 0 || summary.QuotaReserved != 0 {
		t.Fatalf("confirmed quota must be reversed during recovery: %+v", summary)
	}
	key, err := store.GetAPIKeyForUser(ctx, h.services.Pool, caller.UserID, caller.APIKeyID)
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	if key.TotalCalls != 0 {
		t.Fatalf("recovery must release the key admission counter, got %d", key.TotalCalls)
	}

	var refundRows int
	if err := h.services.Pool.QueryRow(ctx, `SELECT count(*) FROM quota_ledger WHERE api_call_id = $1 AND entry_type = 'REFUND'`, call.ID).Scan(&refundRows); err != nil {
		t.Fatalf("count refund ledger rows: %v", err)
	}
	if refundRows != 1 {
		t.Fatalf("recovery must append exactly one refund ledger row, got %d", refundRows)
	}
	if err := store.CompleteAPICallSuccess(ctx, h.services.Pool, call.ID, time.Millisecond); err != nil {
		t.Fatalf("late success transition should be harmless: %v", err)
	}
	var status string
	if err := h.services.Pool.QueryRow(ctx, `SELECT status FROM api_calls WHERE id = $1`, call.ID).Scan(&status); err != nil {
		t.Fatalf("read recovered call status: %v", err)
	}
	if status != store.CallStatusFailedRefunded {
		t.Fatalf("recovered call must remain failed after a late success attempt, got %s", status)
	}
}

func TestSweepStaleReceivedCallWithReserveLedger(t *testing.T) {
	h := newHarness(t)
	secret, _ := activateWithSession(t, h)
	caller, appErr := h.services.AuthenticateAPIKey(context.Background(), secret)
	if appErr != nil {
		t.Fatalf("authenticate setup key: %v", appErr)
	}
	call := store.APICall{
		ID:                   uuid.New(),
		RequestID:            "req_crash_before_mark_reserved",
		Operation:            "captcha.solve",
		IdempotencyKeyHash:   []byte("crash-before-mark-reserved"),
		UserID:               caller.UserID,
		CDKID:                caller.CDKID,
		APIKeyID:             caller.APIKeyID,
		APIKeyNameSnapshot:   caller.APIKey.Name,
		APIKeyPrefixSnapshot: caller.APIKey.KeyPrefix,
		CaptchaID:            "cap-crash-received",
		RiskType:             "slide",
		ClientIPMasked:       "192.0.2.0/24",
		ClientIPHash:         []byte("test-ip-hash"),
		UserAgent:            "recovery-test",
	}
	ctx := context.Background()
	if err := store.CreateAPICall(ctx, h.services.Pool, call); err != nil {
		t.Fatalf("create call: %v", err)
	}
	if admitted, err := store.ConsumeAPIKeyQuota(ctx, h.services.Pool, call.APIKeyID); err != nil || !admitted {
		t.Fatalf("admit key: admitted=%t err=%v", admitted, err)
	}
	if err := store.ReserveQuota(ctx, h.services.Pool, call.CDKID, call.UserID, call.ID, call.RequestID, "reserve"); err != nil {
		t.Fatalf("reserve quota: %v", err)
	}
	if _, err := h.services.Pool.Exec(ctx, `UPDATE api_calls SET accepted_at = now() - interval '1 hour' WHERE id = $1`, call.ID); err != nil {
		t.Fatalf("age call: %v", err)
	}

	reaped, err := store.SweepStaleInFlightCalls(ctx, h.services.Pool, time.Minute)
	if err != nil {
		t.Fatalf("sweep stale received call: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("expected one reaped received call, got %d", reaped)
	}
	summary, err := store.GetCDKSummaryForUser(ctx, h.services.Pool, caller.UserID)
	if err != nil {
		t.Fatalf("quota summary: %v", err)
	}
	if summary.QuotaRemaining != 100 || summary.QuotaUsed != 0 || summary.QuotaReserved != 0 {
		t.Fatalf("received-call recovery must refund quota: %+v", summary)
	}
	key, err := store.GetAPIKeyForUser(ctx, h.services.Pool, caller.UserID, caller.APIKeyID)
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	if key.TotalCalls != 0 {
		t.Fatalf("received-call recovery must release key counter, got %d", key.TotalCalls)
	}
}
