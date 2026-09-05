package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestCDKBindingUnique enforces the one-CDK-per-user constraint: two CDK rows
// must never bind the same user, which is what keeps activation races safe
// together with the FOR UPDATE row lock.
func TestCDKBindingUnique(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	cdkA, _ := h.seedCDK(nil)
	cdkB, _ := h.seedCDK(nil)

	user := uuid.New()
	if _, err := h.services.Pool.Exec(ctx, `INSERT INTO users (id, status) VALUES ($1, 'ACTIVE')`, user); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if _, err := h.services.Pool.Exec(ctx, `UPDATE cdks SET bound_user_id = $1 WHERE id = $2`, user, cdkA.ID); err != nil {
		t.Fatalf("bind first cdk: %v", err)
	}
	if _, err := h.services.Pool.Exec(ctx, `UPDATE cdks SET bound_user_id = $1 WHERE id = $2`, user, cdkB.ID); err == nil {
		t.Fatal("second cdk binding the same user must violate the unique constraint")
	}
}

// TestQuotaNonNegativeCheck rejects negative quota writes at the database.
func TestQuotaNonNegativeCheck(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	cdk, _ := h.seedCDK(nil)
	if _, err := h.services.Pool.Exec(ctx,
		`UPDATE cdks SET quota_remaining = -1 WHERE id = $1`, cdk.ID); err == nil {
		t.Fatal("negative quota must violate the check constraint")
	}
}

// TestAPIKeyHashUnique forbids two rows sharing one key hash.
func TestAPIKeyHashUnique(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	_, code := h.seedCDK(nil)
	activateResp := h.activate(code)
	if activateResp.StatusCode != 201 {
		t.Fatalf("activation failed: %d", activateResp.StatusCode)
	}

	userID := uuid.New()
	if _, err := h.services.Pool.Exec(ctx, `INSERT INTO users (id, status) VALUES ($1, 'ACTIVE')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	var existingHash []byte
	if err := h.services.Pool.QueryRow(ctx, `SELECT key_hash FROM api_keys LIMIT 1`).Scan(&existingHash); err != nil {
		t.Fatalf("read key hash: %v", err)
	}
	if _, err := h.services.Pool.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, name, key_prefix, key_last4, key_hash, status, total_calls)
		VALUES ($1, $2, 'dup', 'cf_live_dup', 'AAAA', $3, 'ACTIVE', 0)
	`, uuid.New(), userID, existingHash); err == nil {
		t.Fatal("duplicate key hash must violate the unique constraint")
	}
}

func TestAPIKeysStoreNoRecoverableSecretColumn(t *testing.T) {
	h := newHarness(t)
	var exists bool
	if err := h.services.Pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'api_keys' AND column_name = 'secret_ciphertext'
		)
	`).Scan(&exists); err != nil {
		t.Fatalf("inspect api_keys columns: %v", err)
	}
	if exists {
		t.Fatal("api_keys must not retain a recoverable secret column")
	}
}
