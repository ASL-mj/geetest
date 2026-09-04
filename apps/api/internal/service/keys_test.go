package service

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/domain"
)

func healthyCaller() (domain.APIKey, domain.User, domain.Cdk) {
	now := time.Now().UTC()
	expiry := now.Add(24 * time.Hour)
	key := domain.APIKey{ID: uuid.New(), Status: domain.APIKeyStatusActive}
	user := domain.User{ID: uuid.New(), Status: domain.UserStatusActive}
	cdk := domain.Cdk{ID: uuid.New(), Status: domain.CDKStatusActive, QuotaRemaining: 10, ExpiresAt: &expiry}
	return key, user, cdk
}

func TestValidateEffectiveCallerAcceptsHealthyState(t *testing.T) {
	key, user, cdk := healthyCaller()
	if appErr := validateEffectiveCaller(key, user, cdk); appErr != nil {
		t.Fatalf("healthy state must pass, got %v", appErr)
	}
}

func TestValidateEffectiveCallerIgnoresNilExpiry(t *testing.T) {
	key, user, cdk := healthyCaller()
	cdk.ExpiresAt = nil
	if appErr := validateEffectiveCaller(key, user, cdk); appErr != nil {
		t.Fatalf("nil expiry must not fail: %v", appErr)
	}
}

func TestValidateEffectiveCallerRejectsEachBrokenState(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*domain.APIKey, *domain.User, *domain.Cdk)
		wantCode string
	}{
		{"disabled key", func(k *domain.APIKey, _ *domain.User, _ *domain.Cdk) { k.Status = domain.APIKeyStatusDisabled }, "API_KEY_DISABLED"},
		{"deleted key", func(k *domain.APIKey, _ *domain.User, _ *domain.Cdk) { k.Status = domain.APIKeyStatusDeleted }, "API_KEY_DISABLED"},
		{"suspended user", func(_ *domain.APIKey, u *domain.User, _ *domain.Cdk) { u.Status = domain.UserStatusSuspended }, "ACCOUNT_DISABLED"},
		{"disabled cdk", func(_ *domain.APIKey, _ *domain.User, c *domain.Cdk) { c.Status = domain.CDKStatusDisabled }, "SERVICE_UNAVAILABLE"},
		{"unactivated cdk", func(_ *domain.APIKey, _ *domain.User, c *domain.Cdk) { c.Status = domain.CDKStatusUnactivated }, "SERVICE_UNAVAILABLE"},
		{"expired cdk", func(_ *domain.APIKey, _ *domain.User, c *domain.Cdk) {
			past := time.Now().UTC().Add(-time.Hour)
			c.ExpiresAt = &past
		}, "SERVICE_UNAVAILABLE"},
		{"exhausted quota", func(_ *domain.APIKey, _ *domain.User, c *domain.Cdk) { c.QuotaRemaining = 0 }, "QUOTA_EXHAUSTED"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			key, user, cdk := healthyCaller()
			testCase.mutate(&key, &user, &cdk)
			appErr := validateEffectiveCaller(key, user, cdk)
			if appErr == nil {
				t.Fatalf("expected rejection for %s", testCase.name)
			}
			if appErr.Code != testCase.wantCode {
				t.Fatalf("got code %q, want %q", appErr.Code, testCase.wantCode)
			}
		})
	}
}
