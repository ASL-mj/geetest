package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/captchaflow/service-platform/api/internal/domain"
)

// CreateUser inserts a new active internal user and returns it.
func CreateUser(ctx context.Context, q Querier) (domain.User, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO users (id, status) VALUES (gen_random_uuid(), 'ACTIVE')
		RETURNING id, status, last_login_at, created_at, updated_at
	`)
	var user domain.User
	var status string
	err := row.Scan(&user.ID, &status, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	user.Status = domain.UserStatus(status)
	return user, nil
}

// GetUser fetches one user by ID.
func GetUser(ctx context.Context, q Querier, id uuid.UUID) (domain.User, error) {
	row := q.QueryRow(ctx, `SELECT id, status, last_login_at, created_at, updated_at FROM users WHERE id = $1`, id)
	var user domain.User
	var status string
	err := row.Scan(&user.ID, &status, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, ErrNotFound
		}
		return domain.User{}, err
	}
	user.Status = domain.UserStatus(status)
	return user, nil
}

// TouchUserLogin updates last_login_at for the user.
func TouchUserLogin(ctx context.Context, q Querier, id uuid.UUID, at time.Time) error {
	_, err := q.Exec(ctx, `UPDATE users SET last_login_at = $2, updated_at = now() WHERE id = $1`, id, at)
	return err
}

// LockCdkByCodeHash selects the CDK and its batch with FOR UPDATE so
// concurrent activations serialize on the row.
func LockCdkByCodeHash(ctx context.Context, q Querier, codeHash []byte) (domain.Cdk, domain.CdkBatch, error) {
	row := q.QueryRow(ctx, `
		SELECT c.id, c.batch_id, c.code_prefix, c.code_hash, c.status, c.bound_user_id,
		       c.activation_deadline, c.expires_at, c.quota_total, c.quota_used,
		       c.quota_reserved, c.quota_remaining, c.activated_at, c.last_used_at,
		       c.created_at, c.updated_at,
		       b.id, b.name, b.description, b.default_quota, b.activation_deadline,
		       b.service_duration_days, b.created_by, b.created_at
		FROM cdks c
		JOIN cdk_batches b ON c.batch_id = b.id
		WHERE c.code_hash = $1
		FOR UPDATE OF c
	`, codeHash)

	var cdk domain.Cdk
	var batch domain.CdkBatch
	var cdkStatus string
	err := row.Scan(
		&cdk.ID, &cdk.BatchID, &cdk.CodePrefix, &cdk.CodeHash, &cdkStatus, &cdk.BoundUserID,
		&cdk.ActivationDeadline, &cdk.ExpiresAt, &cdk.QuotaTotal, &cdk.QuotaUsed,
		&cdk.QuotaReserved, &cdk.QuotaRemaining, &cdk.ActivatedAt, &cdk.LastUsedAt,
		&cdk.CreatedAt, &cdk.UpdatedAt,
		&batch.ID, &batch.Name, &batch.Description, &batch.DefaultQuota, &batch.ActivationDeadline,
		&batch.ServiceDurationDays, &batch.CreatedBy, &batch.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Cdk{}, domain.CdkBatch{}, ErrNotFound
		}
		return domain.Cdk{}, domain.CdkBatch{}, err
	}
	cdk.Status = domain.CDKStatus(cdkStatus)
	return cdk, batch, nil
}

// ActivateCdk binds the CDK to a fresh user and stamps the activation fields.
func ActivateCdk(ctx context.Context, q Querier, cdkID, userID uuid.UUID, activatedAt, expiresAt *time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE cdks
		SET status = 'ACTIVE', bound_user_id = $2, activated_at = $3, expires_at = $4, updated_at = now()
		WHERE id = $1
	`, cdkID, userID, activatedAt, expiresAt)
	return err
}
