// Command migrate applies the embedded forward-only migrations to the
// configured database and enforces the append-only ledger policy afterwards.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/captchaflow/service-platform/api/internal/config"
	"github.com/captchaflow/service-platform/api/internal/store"
)

func main() {
	envFile := flag.String("env", "", "path to a .env file loaded for missing variables")
	flag.Parse()

	settings, err := config.Load(resolveEnvFile(*envFile))
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := store.Connect(ctx, settings.DatabaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool); err != nil {
		slog.Error("migrate", "error", err)
		os.Exit(1)
	}
	if err := store.RevokeLedgerWrites(ctx, pool); err != nil {
		slog.Error("enforce ledger policy", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}

func resolveEnvFile(explicit string) string {
	if explicit != "" {
		return explicit
	}
	for _, candidate := range []string{"../../.env", ".env"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
