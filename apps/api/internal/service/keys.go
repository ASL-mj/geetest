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

// CreatedAPIKey carries the one-time plaintext secret alongside its record.
type CreatedAPIKey struct {
	Record domain.APIKey
	Secret string
}

// CreateAPIKey generates a cf_live_ secret, persists only its peppered HMAC
// hash plus display prefix/last4, and returns the plaintext exactly once.
func (s *Services) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string) (CreatedAPIKey, *ApplicationError) {
	var created CreatedAPIKey
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		secret, appErr := s.createAPIKeySecret(ctx, q, userID, name)
		if appErr != nil {
			return appErr
		}
		// Fetch the persisted record back for a clean response.
		key, err := store.GetAPIKeyForUserByHash(ctx, q, crypto.HMACSHA256(secret, s.Settings.APIKeyPepper))
		if err != nil {
			return err
		}
		created = CreatedAPIKey{Record: key, Secret: secret}
		return nil
	})
	if err != nil {
		if appErr := asApplicationError(err); appErr != nil {
			return CreatedAPIKey{}, appErr
		}
		slog.Error("create api key failed", "error", err)
		return CreatedAPIKey{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return created, nil
}

// createAPIKeySecret is the transactional primitive shared with activation.
// It returns plain error so a nil result never becomes a typed-nil pointer
// stored inside an error interface.
func (s *Services) createAPIKeySecret(ctx context.Context, q store.Querier, userID uuid.UUID, name string) (string, error) {
	secret, err := crypto.GenerateOpaqueToken()
	if err != nil {
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	full := "cf_live_" + secret
	record := domain.APIKey{
		ID:        uuid.New(),
		UserID:    userID,
		Name:      name,
		KeyPrefix: full[:16],
		KeyLast4:  full[len(full)-4:],
		KeyHash:   crypto.HMACSHA256(full, s.Settings.APIKeyPepper),
		Status:    domain.APIKeyStatusActive,
	}
	if err := store.CreateAPIKey(ctx, q, record); err != nil {
		return "", NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return full, nil
}

// ListAPIKeys returns the caller's keys without secrets.
func (s *Services) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]domain.APIKey, *ApplicationError) {
	keys, err := store.ListAPIKeys(ctx, s.Pool, userID)
	if err != nil {
		slog.Error("list api keys failed", "error", err)
		return nil, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return keys, nil
}

// UpdateAPIKey renames or toggles a key. Deleted keys are frozen.
func (s *Services) UpdateAPIKey(ctx context.Context, userID, keyID uuid.UUID, name *string, status *domain.APIKeyStatus) (domain.APIKey, *ApplicationError) {
	var updated domain.APIKey
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		key, err := store.GetAPIKeyForUser(ctx, q, userID, keyID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrAPIKeyNotFound()
			}
			return err
		}
		if key.Status == domain.APIKeyStatusDeleted {
			return ErrAPIKeyDeleted()
		}

		var revokedAt *time.Time
		if status != nil {
			if *status == domain.APIKeyStatusDisabled {
				now := nowUTC()
				revokedAt = &now
			} else {
				revokedAt = nil
			}
		}
		if err := store.UpdateAPIKeyFields(ctx, q, keyID, name, status, revokedAt); err != nil {
			return err
		}
		updated, err = store.GetAPIKeyForUser(ctx, q, userID, keyID)
		return err
	})
	if err != nil {
		if appErr := asApplicationError(err); appErr != nil {
			return domain.APIKey{}, appErr
		}
		slog.Error("update api key failed", "error", err)
		return domain.APIKey{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return updated, nil
}

// DeleteAPIKey soft-deletes a key as the DELETED terminal state.
func (s *Services) DeleteAPIKey(ctx context.Context, userID, keyID uuid.UUID) (domain.APIKey, *ApplicationError) {
	var deleted domain.APIKey
	err := store.RunInTx(ctx, s.Pool, func(ctx context.Context, q store.Querier) error {
		if _, err := store.GetAPIKeyForUser(ctx, q, userID, keyID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrAPIKeyNotFound()
			}
			return err
		}
		now := nowUTC()
		deletedStatus := domain.APIKeyStatusDeleted
		if err := store.UpdateAPIKeyFields(ctx, q, keyID, nil, &deletedStatus, &now); err != nil {
			return err
		}
		fetched, err := store.GetAPIKeyForUser(ctx, q, userID, keyID)
		if err != nil {
			return err
		}
		deleted = fetched
		return nil
	})
	if err != nil {
		if appErr := asApplicationError(err); appErr != nil {
			return domain.APIKey{}, appErr
		}
		slog.Error("delete api key failed", "error", err)
		return domain.APIKey{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return deleted, nil
}

// AuthenticatedCaller is the immutable identity set resolved per request.
type AuthenticatedCaller struct {
	UserID   uuid.UUID
	CDKID    uuid.UUID
	APIKeyID uuid.UUID
	APIKey   domain.APIKey
	User     domain.User
	CDK      domain.Cdk
}

// AuthenticateAPIKey validates a Bearer secret through the full effective
// status chain: key status, user status, CDK status, expiry, quota. There is
// no cached state; every call recomputes entitlements.
func (s *Services) AuthenticateAPIKey(ctx context.Context, secret string) (AuthenticatedCaller, *ApplicationError) {
	key, err := store.GetAPIKeyForUserByHash(ctx, s.Pool, crypto.HMACSHA256(secret, s.Settings.APIKeyPepper))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return AuthenticatedCaller{}, ErrAPIKeyInvalid()
		}
		slog.Error("authenticate api key failed", "error", err)
		return AuthenticatedCaller{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}

	user, cdk, appErr := s.loadCallerState(ctx, key.UserID)
	if appErr != nil {
		return AuthenticatedCaller{}, appErr
	}
	if appErr := validateEffectiveCaller(key, user, cdk); appErr != nil {
		return AuthenticatedCaller{}, appErr
	}
	return AuthenticatedCaller{UserID: user.ID, CDKID: cdk.ID, APIKeyID: key.ID, APIKey: key, User: user, CDK: cdk}, nil
}

// loadCallerState fetches the user and the single bound CDK.
func (s *Services) loadCallerState(ctx context.Context, userID uuid.UUID) (domain.User, domain.Cdk, *ApplicationError) {
	user, err := store.GetUser(ctx, s.Pool, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.User{}, domain.Cdk{}, ErrServiceUnavailable()
		}
		slog.Error("load caller user failed", "error", err)
		return domain.User{}, domain.Cdk{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	cdk, err := store.GetCDKForUser(ctx, s.Pool, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.User{}, domain.Cdk{}, ErrServiceUnavailable()
		}
		slog.Error("load caller cdk failed", "error", err)
		return domain.User{}, domain.Cdk{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	return user, cdk, nil
}

// validateEffectiveCaller applies the V1 rule ordering exactly.
func validateEffectiveCaller(key domain.APIKey, user domain.User, cdk domain.Cdk) *ApplicationError {
	if key.Status != domain.APIKeyStatusActive {
		return ErrAPIKeyDisabled()
	}
	if user.Status != domain.UserStatusActive {
		return ErrAccountDisabled()
	}
	if cdk.Status != domain.CDKStatusActive {
		return ErrServiceUnavailable()
	}
	if cdk.ExpiresAt != nil && !cdk.ExpiresAt.After(nowUTC()) {
		return ErrServiceUnavailable()
	}
	if cdk.QuotaRemaining <= 0 {
		return ErrQuotaExhausted()
	}
	return nil
}

// ResolveUserSession validates the session cookie into user + session.
func (s *Services) ResolveUserSession(ctx context.Context, token string) (domain.User, domain.UserSession, *ApplicationError) {
	if token == "" {
		return domain.User{}, domain.UserSession{}, ErrSessionRequired()
	}
	session, user, err := store.GetActiveUserSession(ctx, s.Pool, crypto.HMACSHA256(token, s.Settings.SessionSecret))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.User{}, domain.UserSession{}, ErrSessionInvalid()
		}
		slog.Error("resolve session failed", "error", err)
		return domain.User{}, domain.UserSession{}, NewError(500, "INTERNAL_ERROR", "Internal server error.")
	}
	if user.Status != domain.UserStatusActive {
		return domain.User{}, domain.UserSession{}, ErrAccountDisabled()
	}
	return user, session, nil
}
