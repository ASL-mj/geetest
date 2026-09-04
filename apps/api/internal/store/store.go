// Package store provides PostgreSQL access via pgx, the embedded forward-only
// migrations and the query helpers used by the application services.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting the same
// query methods run inside or outside a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// MigrationFS embeds the forward-only SQL migrations applied by Migrate.
//
//go:embed migrations/*.sql
var MigrationFS embed.FS

// NormalizeDatabaseURL rewrites SQLAlchemy-style DSNs (+asyncpg, +psycopg)
// into the plain postgres:// form pgx expects.
func NormalizeDatabaseURL(databaseURL string) string {
	scheme, rest, found := strings.Cut(databaseURL, "://")
	if !found {
		return databaseURL
	}
	if base, _, hasDriver := strings.Cut(scheme, "+"); hasDriver {
		scheme = base
	}
	if scheme == "postgresql" {
		scheme = "postgres"
	}
	return scheme + "://" + rest
}

// Connect opens a connection pool for the application.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, NormalizeDatabaseURL(databaseURL))
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// IsUniqueViolation reports whether err is the PostgreSQL 23505 constraint.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// RunInTx executes fn inside one transaction, committing on nil and rolling
// back on error, including *service.ApplicationError values.
func RunInTx(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context, q Querier) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Migrate applies every embedded migration in filename order exactly once.
// It is forward-only: no down migrations exist, matching the delivery rule.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrationFS, err := fs.Sub(MigrationFS, "migrations")
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		applied, err := migrationApplied(ctx, pool, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		for _, statement := range splitStatements(string(sqlBytes)) {
			if statement == "" {
				continue
			}
			if _, err := tx.Exec(ctx, statement); err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("apply %s: %w", name, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// splitStatements splits a migration script on semicolons. The platform
// migrations are plain DDL without functions or string literals containing
// semicolons, so a naive split is sufficient.
func splitStatements(script string) []string {
	return strings.Split(script, ";")
}

func migrationApplied(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = $1)`, name).Scan(&exists)
	return exists, err
}

// RevokeLedgerWrites enforces the append-only quota ledger policy when the
// dedicated application role is present in the deployment.
func RevokeLedgerWrites(ctx context.Context, pool *pgxpool.Pool) error {
	var roleExists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'geetest_platform_app')`).Scan(&roleExists)
	if err != nil || !roleExists {
		return err
	}
	_, err = pool.Exec(ctx, `REVOKE UPDATE, DELETE ON TABLE quota_ledger FROM geetest_platform_app`)
	return err
}
