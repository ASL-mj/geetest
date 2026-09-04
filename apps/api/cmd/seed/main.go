// Command seed inserts a CDK batch and one CDK into the local database for
// manual testing. It exists until the administrator APIs are implemented and
// must never run against production.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/captchaflow/service-platform/api/internal/config"
	"github.com/captchaflow/service-platform/api/internal/crypto"
	"github.com/captchaflow/service-platform/api/internal/store"
)

func main() {
	code := flag.String("code", "", "CDK plaintext code (required)")
	quota := flag.Int64("quota", 100, "total quota units for the CDK")
	batchName := flag.String("batch", "local-seed-batch", "batch name, reused when it exists")
	durationDays := flag.Int("days", 365, "service duration in days after activation")
	envFile := flag.String("env", "", "path to a .env file loaded for missing variables")
	flag.Parse()

	if *code == "" {
		fmt.Fprintln(os.Stderr, "usage: seed -code CAPTCHA-XXXX [-quota 100] [-batch name] [-days 365]")
		os.Exit(1)
	}

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

	normalized := crypto.NormalizeCDK(*code)
	if normalized == "" {
		slog.Error("CDK code contains no alphanumeric characters")
		os.Exit(1)
	}

	err = store.RunInTx(ctx, pool, func(ctx context.Context, q store.Querier) error {
		var batchID uuid.UUID
		err := q.QueryRow(ctx, `SELECT id FROM cdk_batches WHERE name = $1`, *batchName).Scan(&batchID)
		if err != nil {
			if err := q.QueryRow(ctx, `
				INSERT INTO cdk_batches (id, name, default_quota, service_duration_days)
				VALUES ($1, $2, $3, $4) RETURNING id
			`, uuid.New(), *batchName, *quota, *durationDays).Scan(&batchID); err != nil {
				return err
			}
		}

		codeHash := crypto.HMACSHA256(normalized, settings.CDKPepper)
		prefix := normalized
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		_, err = q.Exec(ctx, `
			INSERT INTO cdks (id, batch_id, code_prefix, code_hash, status,
			                  quota_total, quota_used, quota_reserved, quota_remaining)
			VALUES ($1, $2, $3, $4, 'UNACTIVATED', $5, 0, 0, $5)
		`, uuid.New(), batchID, prefix, codeHash, *quota)
		return err
	})
	if err != nil {
		slog.Error("seed cdk", "error", err)
		os.Exit(1)
	}

	fmt.Printf("seeded CDK %s (batch %s, quota %d, expires %d days after activation) at %s\n",
		normalized, *batchName, *quota, *durationDays, time.Now().Format(time.RFC3339))
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
