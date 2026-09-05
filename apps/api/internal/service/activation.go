package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/domain"
	"github.com/captchaflow/service-platform/api/internal/store"
)

// ActivationResult describes one CDK activation or re-entry. The default API
// key plaintext is only present on first activation; SessionToken is the
// one-time browser session token to set as cookie.
type ActivationResult struct {
	User           domain.User
	CDKPrefix      string
	DefaultAPIKey  string
	SessionToken   string
	NewlyActivated bool
}

// ActivateCDK exchanges a CDK code for a bound user identity, creating the
// user, the default API key and a browser session on first use. The CDK row
// is locked inside the transaction so concurrent activations serialize; the
// database unique constraint on bound_user_id backstops the race.
func (s *Services) ActivateCDK(ctx context.Context, cdkCode string) (ActivationResult, *ApplicationError) {
	normalized := crypto.NormalizeCDK(cdkCode)
	if normalized == "" {
		return ActivationResult{}, ErrInvalidCDK()
	}

	now := nowUTC()
	var result ActivationResult
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		cdk, batch, err := store.LockCdkByCodeHash(ctx, q, crypto.HMACSHA256(normalized, s.Settings.CDKPepper))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrCDKNotFound()
			}
			return err
		}

		if cdk.Status == domain.CDKStatusDisabled {
			return ErrCDKDisabled()
		}
		if cdk.Status == domain.CDKStatusActive && cdk.BoundUserID != nil {
			user, err := store.GetUser(ctx, q, *cdk.BoundUserID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return ErrCDKBindingInvalid()
				}
				return err
			}
			if user.Status != domain.UserStatusActive {
				return ErrAccountDisabled()
			}
			if err := store.TouchUserLogin(ctx, q, user.ID, now); err != nil {
				return err
			}
			token, err := s.issueSession(ctx, q, user.ID, now)
			if err != nil {
				return err
			}
			result = ActivationResult{User: user, CDKPrefix: cdk.CodePrefix, SessionToken: token, NewlyActivated: false}
			return nil
		}

		if cdk.Status != domain.CDKStatusUnactivated || cdk.BoundUserID != nil {
			return ErrCDKUnavailable()
		}
		if cdk.ActivationDeadline != nil && !cdk.ActivationDeadline.After(now) {
			return ErrCDKActivationExpiry()
		}
		if cdk.ExpiresAt != nil && !cdk.ExpiresAt.After(now) {
			return ErrCDKExpired()
		}
		if cdk.QuotaRemaining <= 0 {
			return ErrCDKExhausted()
		}

		user, err := store.CreateUser(ctx, q)
		if err != nil {
			return err
		}
		var expiresAt *time.Time
		if batch.ServiceDurationDays != nil {
			expiry := now.Add(time.Duration(*batch.ServiceDurationDays) * 24 * time.Hour)
			expiresAt = &expiry
		}
		if err := store.ActivateCdk(ctx, q, cdk.ID, user.ID, &now, expiresAt); err != nil {
			return err
		}
		secret, err := s.createAPIKeySecret(ctx, q, user.ID, "default")
		if err != nil {
			return err
		}
		token, err := s.issueSession(ctx, q, user.ID, now)
		if err != nil {
			return err
		}
		user.LastLoginAt = &now
		result = ActivationResult{User: user, CDKPrefix: cdk.CodePrefix, DefaultAPIKey: secret, SessionToken: token, NewlyActivated: true}
		return nil
	})

	if err != nil {
		if store.IsUniqueViolation(err) {
			return ActivationResult{}, ErrCDKUnavailable()
		}
		if appErr := asApplicationError(err); appErr != nil {
			return ActivationResult{}, appErr
		}
		slog.Error("activate cdk failed", "error", err)
		return ActivationResult{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return result, nil
}

// Logout revokes the current browser session only.
func (s *Services) Logout(ctx context.Context, sessionID uuid.UUID) *ApplicationError {
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		return store.RevokeUserSession(ctx, q, sessionID, nowUTC())
	})
	if err != nil {
		slog.Error("logout failed", "error", err)
		return NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return nil
}

// issueSession creates a revocable session and returns the one-time token.
func (s *Services) issueSession(ctx context.Context, q store.Querier, userID uuid.UUID, now time.Time) (string, error) {
	token, err := crypto.GenerateOpaqueToken()
	if err != nil {
		return "", err
	}
	expiresAt := now.Add(s.Settings.SessionTTL)
	if err := store.CreateUserSession(ctx, q, userID, crypto.HMACSHA256(token, s.Settings.SessionSecret), expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

// asApplicationError unwraps known application errors from store call chains.
// A typed-nil *ApplicationError wrapped in a non-nil interface is normalized
// to nil so it is never mistaken for a real failure.
func asApplicationError(err error) *ApplicationError {
	if appErr, ok := err.(*ApplicationError); ok && appErr != nil {
		return appErr
	}
	return nil
}
