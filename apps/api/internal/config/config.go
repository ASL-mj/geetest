// Package config loads runtime settings from the environment and an optional
// .env file at the repository root. Placeholder secrets are rejected when the
// platform runs in production mode.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Environment string

const (
	EnvLocal      Environment = "local"
	EnvTest       Environment = "test"
	EnvProduction Environment = "production"
)

// Settings mirrors the V1 runtime contract shared with the deployment
// environment; every name matches the variables in .env.example.
type Settings struct {
	Environment Environment

	DatabaseURL          string
	RedisURL             string
	SessionSecret        string
	SessionTTL           time.Duration
	APIKeyPepper         string
	CDKPepper            string
	GeetestSolverURL     string
	GeetestServiceAPIKey string

	SolverConnectTimeout time.Duration
	SolverReadTimeout    time.Duration
	SolverTotalTimeout   time.Duration
	RateLimitPerMinute   int
	ConcurrencyLimit     int
}

// Load reads the environment, applying defaults that match .env.example. When
// envFile is non-empty and exists, missing variables are filled from it
// without overriding real environment variables.
func Load(envFile string) (Settings, error) {
	_ = loadDotEnv(envFile)

	settings := Settings{
		Environment:          Environment(getEnv("APP_ENV", string(EnvLocal))),
		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/geetest_platform"),
		RedisURL:             getEnv("REDIS_URL", "redis://localhost:6379/0"),
		SessionSecret:        getEnv("SESSION_SECRET", "change-me-session-secret"),
		SessionTTL:           time.Duration(getEnvInt("SESSION_TTL_SECONDS", 604800)) * time.Second,
		APIKeyPepper:         getEnv("API_KEY_PEPPER", "change-me-api-key-pepper"),
		CDKPepper:            getEnv("CDK_PEPPER", "change-me-cdk-pepper"),
		GeetestSolverURL:     getEnv("GEETEST_SOLVER_URL", "https://solver.internal.example.com"),
		GeetestServiceAPIKey: getEnv("GEETEST_SERVICE_API_KEY", "change-me-geetest-service-api-key"),

		SolverConnectTimeout: time.Duration(getEnvInt("SOLVER_CONNECT_TIMEOUT_SECONDS", 10)) * time.Second,
		SolverReadTimeout:    time.Duration(getEnvInt("SOLVER_READ_TIMEOUT_SECONDS", 120)) * time.Second,
		SolverTotalTimeout:   time.Duration(getEnvInt("SOLVER_TOTAL_TIMEOUT_SECONDS", 130)) * time.Second,
		RateLimitPerMinute:   getEnvInt("RATE_LIMIT_PER_MINUTE", 60),
		ConcurrencyLimit:     getEnvInt("CONCURRENCY_LIMIT", 4),
	}

	if err := settings.validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s Settings) validate() error {
	switch s.Environment {
	case EnvLocal, EnvTest:
		return nil
	case EnvProduction:
	default:
		return fmt.Errorf("unknown APP_ENV %q", s.Environment)
	}

	placeholderPrefixes := []string{"change-me-", "sample-", "placeholder-"}
	for name, value := range map[string]string{
		"SESSION_SECRET":          s.SessionSecret,
		"API_KEY_PEPPER":          s.APIKeyPepper,
		"CDK_PEPPER":              s.CDKPepper,
		"GEETEST_SERVICE_API_KEY": s.GeetestServiceAPIKey,
	} {
		if value == "" || hasPrefix(value, placeholderPrefixes) {
			return fmt.Errorf("%s must be set to a production secret", name)
		}
	}
	for name, value := range map[string]string{
		"DATABASE_URL":       s.DatabaseURL,
		"REDIS_URL":          s.RedisURL,
		"GEETEST_SOLVER_URL": s.GeetestSolverURL,
	} {
		if value == "" || strings.Contains(value, "example.com") || strings.Contains(value, "localhost") {
			return fmt.Errorf("%s must be set for production", name)
		}
	}
	return nil
}

func hasPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

// loadDotEnv applies KEY=VALUE pairs from path for variables that are not
// already present in the environment. Missing files are ignored.
func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
