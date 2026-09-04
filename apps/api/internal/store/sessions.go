package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/captchaflow/service-platform/api/internal/domain"
)

// CreateUserSession persists a hashed session token with its expiry.
func CreateUserSession(ctx context.Context, q Querier, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, token_hash, expires_at)
		VALUES (gen_random_uuid(), $1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

// GetActiveUserSession resolves a token hash to an unrevoked, unexpired
// session together with its user.
func GetActiveUserSession(ctx context.Context, q Querier, tokenHash []byte) (domain.UserSession, domain.User, error) {
	row := q.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.token_hash, s.expires_at, s.revoked_at, s.created_at,
		       u.id, u.status, u.last_login_at, u.created_at, u.updated_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
	`, tokenHash)

	var session domain.UserSession
	var user domain.User
	var userStatus string
	err := row.Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.ExpiresAt, &session.RevokedAt, &session.CreatedAt,
		&user.ID, &userStatus, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserSession{}, domain.User{}, ErrNotFound
		}
		return domain.UserSession{}, domain.User{}, err
	}
	user.Status = domain.UserStatus(userStatus)
	return session, user, nil
}

// RevokeUserSession marks the given session revoked; it is a no-op when the
// session was already revoked.
func RevokeUserSession(ctx context.Context, q Querier, sessionID uuid.UUID, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE user_sessions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL
	`, sessionID, at)
	return err
}
