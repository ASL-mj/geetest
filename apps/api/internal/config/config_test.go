package config

import (
	"strings"
	"testing"
)

func TestLoadLocalDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "local")
	settings, err := Load(t.TempDir() + "/missing.env")
	if err != nil {
		t.Fatalf("local load should not fail: %v", err)
	}
	if settings.Environment != EnvLocal {
		t.Fatalf("unexpected environment %q", settings.Environment)
	}
	if settings.SessionTTL <= 0 {
		t.Fatalf("session ttl must be positive")
	}
}

func TestLoadRejectsPlaceholderSecretsInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "change-me-session-secret")
	t.Setenv("API_KEY_PEPPER", "real-pepper")
	t.Setenv("CDK_PEPPER", "real-pepper")
	t.Setenv("GEETEST_SERVICE_API_KEY", "real-key")
	t.Setenv("DATABASE_URL", "postgres://db.internal:5432/platform")
	t.Setenv("REDIS_URL", "redis://cache.internal:6379/0")
	t.Setenv("GEETEST_SOLVER_URL", "https://solver.internal:8080")

	_, err := Load(t.TempDir() + "/missing.env")
	if err == nil || !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("expected placeholder secret rejection, got %v", err)
	}
}

func TestLoadRejectsLocalEndpointsInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "real-secret")
	t.Setenv("API_KEY_PEPPER", "real-pepper")
	t.Setenv("CDK_PEPPER", "real-pepper")
	t.Setenv("GEETEST_SERVICE_API_KEY", "real-key")
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/platform")
	t.Setenv("REDIS_URL", "redis://cache.internal:6379/0")
	t.Setenv("GEETEST_SOLVER_URL", "https://solver.internal:8080")

	_, err := Load(t.TempDir() + "/missing.env")
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected local endpoint rejection, got %v", err)
	}
}

func TestLoadAcceptsFullProductionConfiguration(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SESSION_SECRET", "real-secret")
	t.Setenv("API_KEY_PEPPER", "real-pepper")
	t.Setenv("CDK_PEPPER", "real-pepper")
	t.Setenv("GEETEST_SERVICE_API_KEY", "real-key")
	t.Setenv("DATABASE_URL", "postgres://db.internal:5432/platform")
	t.Setenv("REDIS_URL", "redis://cache.internal:6379/0")
	t.Setenv("GEETEST_SOLVER_URL", "https://solver.internal:8080")

	settings, err := Load(t.TempDir() + "/missing.env")
	if err != nil {
		t.Fatalf("production load should succeed: %v", err)
	}
	if settings.Environment != EnvProduction {
		t.Fatalf("unexpected environment %q", settings.Environment)
	}
}
