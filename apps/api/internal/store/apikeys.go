package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/captchaflow/service-platform/api/internal/domain"
)

// apiKeyColumns is the projection shared by every key lookup.
const apiKeyColumns = `id, user_id, name, key_prefix, key_last4, key_hash, secret_ciphertext,
	       status, quota_limit, allowed_ips, total_calls, last_used_at, created_at, revoked_at`

func scanAPIKey(scan func(dest ...any) error) (domain.APIKey, error) {
	var key domain.APIKey
	var status string
	if err := scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.KeyLast4, &key.KeyHash, &key.SecretCiphertext,
		&status, &key.QuotaLimit, &key.AllowedIPs, &key.TotalCalls, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt,
	); err != nil {
		return domain.APIKey{}, err
	}
	key.Status = domain.APIKeyStatus(status)
	return key, nil
}

// CreateAPIKey inserts a new key. The peppered HMAC hash drives
// authentication; the AES-GCM ciphertext is what lets the owner re-copy the
// plaintext later.
func CreateAPIKey(ctx context.Context, q Querier, key domain.APIKey) error {
	_, err := q.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, name, key_prefix, key_last4, key_hash, secret_ciphertext,
		                      status, quota_limit, allowed_ips, total_calls, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
	`, key.ID, key.UserID, key.Name, key.KeyPrefix, key.KeyLast4, key.KeyHash, key.SecretCiphertext,
		string(key.Status), key.QuotaLimit, key.AllowedIPs, key.TotalCalls)
	return err
}

// ListAPIKeys returns the user's keys, newest first.
func ListAPIKeys(ctx context.Context, q Querier, userID uuid.UUID) ([]domain.APIKey, error) {
	rows, err := q.Query(ctx, `
		SELECT `+apiKeyColumns+`
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []domain.APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// GetAPIKeyForUser fetches one key enforcing ownership; returns ErrNotFound
// when the key does not exist or belongs to someone else.
func GetAPIKeyForUser(ctx context.Context, q Querier, userID, keyID uuid.UUID) (domain.APIKey, error) {
	row := q.QueryRow(ctx, `
		SELECT `+apiKeyColumns+`
		FROM api_keys
		WHERE id = $1 AND user_id = $2
	`, keyID, userID)

	key, err := scanAPIKey(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, ErrNotFound
		}
		return domain.APIKey{}, err
	}
	return key, nil
}

// UpdateAPIKeyFields applies a partial update; nil fields keep their value.
func UpdateAPIKeyFields(ctx context.Context, q Querier, keyID uuid.UUID, name *string, status *domain.APIKeyStatus, revokedAt *time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE api_keys
		SET name        = COALESCE($2, name),
		    status      = COALESCE($3, status),
		    revoked_at  = $4
		WHERE id = $1
	`, keyID, name, status, revokedAt)
	return err
}

// UpdateAPIKeyPolicy applies the operator-editable policy fields: display
// name, per-key quota ceiling and the IP allowlist. nil keeps the current
// value; explicit values (including 0 / empty string = unlimited) replace it.
func UpdateAPIKeyPolicy(ctx context.Context, q Querier, keyID uuid.UUID, name *string, quotaLimit *int64, allowedIPs *string) error {
	_, err := q.Exec(ctx, `
		UPDATE api_keys
		SET name        = COALESCE($2, name),
		    quota_limit = COALESCE($3, quota_limit),
		    allowed_ips = COALESCE($4, allowed_ips)
		WHERE id = $1
	`, keyID, name, quotaLimit, allowedIPs)
	return err
}

// ConsumeAPIKeyQuota is the atomic admission gate for per-key ceilings:
// one conditional UPDATE admits the call or refuses it, so concurrent
// solves cannot overshoot the limit. A NULL/negative-or-zero limit is
// unlimited (the UI documents 0 as 不限).
func ConsumeAPIKeyQuota(ctx context.Context, q Querier, keyID uuid.UUID) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE api_keys SET total_calls = total_calls + 1
		WHERE id = $1 AND (quota_limit IS NULL OR quota_limit <= 0 OR total_calls < quota_limit)
	`, keyID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseAPIKeyQuota rolls the admission gate back for a call that failed
// after admission, keeping total_calls equal to settled successes.
func ReleaseAPIKeyQuota(ctx context.Context, q Querier, keyID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE api_keys SET total_calls = GREATEST(total_calls - 1, 0) WHERE id = $1`, keyID)
	return err
}

// GetAPIKeyForUserByHash resolves a key by its HMAC hash during
// authentication; no ownership scoping applies because the hash itself is the
// bearer credential.
func GetAPIKeyForUserByHash(ctx context.Context, q Querier, keyHash []byte) (domain.APIKey, error) {
	row := q.QueryRow(ctx, `
		SELECT `+apiKeyColumns+`
		FROM api_keys
		WHERE key_hash = $1
	`, keyHash)

	key, err := scanAPIKey(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, ErrNotFound
		}
		return domain.APIKey{}, err
	}
	return key, nil
}

// GetCDKForUser resolves the CDK bound to a user; every user owns exactly one.
func GetCDKForUser(ctx context.Context, q Querier, userID uuid.UUID) (domain.Cdk, error) {
	row := q.QueryRow(ctx, `
		SELECT id, batch_id, code_prefix, code_hash, code_ciphertext, remark, status, bound_user_id,
		       activation_deadline, expires_at, quota_total, quota_used,
		       quota_reserved, quota_remaining, activated_at, last_used_at,
		       created_at, updated_at
		FROM cdks
		WHERE bound_user_id = $1
	`, userID)

	var cdk domain.Cdk
	var status string
	err := row.Scan(
		&cdk.ID, &cdk.BatchID, &cdk.CodePrefix, &cdk.CodeHash, &cdk.CodeCiphertext, &cdk.Remark, &status, &cdk.BoundUserID,
		&cdk.ActivationDeadline, &cdk.ExpiresAt, &cdk.QuotaTotal, &cdk.QuotaUsed,
		&cdk.QuotaReserved, &cdk.QuotaRemaining, &cdk.ActivatedAt, &cdk.LastUsedAt,
		&cdk.CreatedAt, &cdk.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Cdk{}, ErrNotFound
		}
		return domain.Cdk{}, err
	}
	cdk.Status = domain.CDKStatus(status)
	return cdk, nil
}
