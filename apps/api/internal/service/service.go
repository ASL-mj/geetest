package service

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/captchaflow/service-platform/api/internal/config"
)

// Services bundles the pool-backed use cases with their runtime settings.
type Services struct {
	Pool     *pgxpool.Pool
	Settings config.Settings
}

// NewServices wires the application services onto a connection pool.
func NewServices(pool *pgxpool.Pool, settings config.Settings) *Services {
	return &Services{Pool: pool, Settings: settings}
}

// nowUTC is the single clock used by application logic for testability.
func nowUTC() time.Time { return time.Now().UTC() }

// uuidPtr returns nil for zero UUIDs, keeping optional SQL args type-safe.
func uuidPtr(id uuid.UUID) *uuid.UUID {
	if id == (uuid.UUID{}) {
		return nil
	}
	return &id
}
