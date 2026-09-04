package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/captchaflow/service-platform/api/internal/domain"
)

// CreateAPIKey inserts a new key; the plaintext secret is handled by the
// service layer and only the hash, prefix and last4 are persisted.
func CreateAPIKey(ctx context.Context, q Querier, key domain.APIKey) error {
	_, err := q.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, name, key_prefix, key_last4, key_hash, status, total_calls, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
	`, key.ID, key.UserID, key.Name, key.KeyPrefix, key.KeyLast4, key.KeyHash, string(key.Status), key.TotalCalls)
	return err
}

// ListAPIKeys returns the user's keys, newest first.
func ListAPIKeys(ctx context.Context, q Querier, userID uuid.UUID) ([]domain.APIKey, error) {
	rows, err := q.Query(ctx, `
		SELECT id, user_id, name, key_prefix, key_last4, key_hash, status, total_calls,
		       last_used_at, created_at, revoked_at
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
		var key domain.APIKey
		var status string
		if err := rows.Scan(
			&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.KeyLast4, &key.KeyHash, &status,
			&key.TotalCalls, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt,
		); err != nil {
			return nil, err
		}
		key.Status = domain.APIKeyStatus(status)
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// GetAPIKeyForUser fetches one key enforcing ownership; returns ErrNotFound
// when the key does not exist or belongs to someone else.
func GetAPIKeyForUser(ctx context.Context, q Querier, userID, keyID uuid.UUID) (domain.APIKey, error) {
	row := q.QueryRow(ctx, `
		SELECT id, user_id, name, key_prefix, key_last4, key_hash, status, total_calls,
		       last_used_at, created_at, revoked_at
		FROM api_keys
		WHERE id = $1 AND user_id = $2
	`, keyID, userID)

	var key domain.APIKey
	var status string
	err := row.Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.KeyLast4, &key.KeyHash, &status,
		&key.TotalCalls, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, ErrNotFound
		}
		return domain.APIKey{}, err
	}
	key.Status = domain.APIKeyStatus(status)
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

// GetAPIKeyForUserByHash resolves a key by its HMAC hash during
// authentication; no ownership scoping applies because the hash itself is the
// bearer credential.
func GetAPIKeyForUserByHash(ctx context.Context, q Querier, keyHash []byte) (domain.APIKey, error) {
	row := q.QueryRow(ctx, `
		SELECT id, user_id, name, key_prefix, key_last4, key_hash, status, total_calls,
		       last_used_at, created_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1
	`, keyHash)

	var key domain.APIKey
	var status string
	err := row.Scan(
		&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.KeyLast4, &key.KeyHash, &status,
		&key.TotalCalls, &key.LastUsedAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.APIKey{}, ErrNotFound
		}
		return domain.APIKey{}, err
	}
	key.Status = domain.APIKeyStatus(status)
	return key, nil
}

// GetCDKForUser resolves the CDK bound to a user; every user owns exactly one.
func GetCDKForUser(ctx context.Context, q Querier, userID uuid.UUID) (domain.Cdk, error) {
	row := q.QueryRow(ctx, `
		SELECT id, batch_id, code_prefix, code_hash, status, bound_user_id,
		       activation_deadline, expires_at, quota_total, quota_used,
		       quota_reserved, quota_remaining, activated_at, last_used_at,
		       created_at, updated_at
		FROM cdks
		WHERE bound_user_id = $1
	`, userID)

	var cdk domain.Cdk
	var status string
	err := row.Scan(
		&cdk.ID, &cdk.BatchID, &cdk.CodePrefix, &cdk.CodeHash, &status, &cdk.BoundUserID,
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
