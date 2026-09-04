// Command api runs the CaptchaFlow platform backend.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/captchaflow/service-platform/api/internal/config"
	"github.com/captchaflow/service-platform/api/internal/httpapi"
	"github.com/captchaflow/service-platform/api/internal/ratelimit"
	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/solver"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := store.Connect(ctx, settings.DatabaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	limiter := ratelimit.New(ctx, settings)
	gateway := solver.NewGateway(settings)
	services := service.NewServices(pool, settings)
	solveService := service.NewSolveService(pool, gateway, limiter, settings.SessionSecret)
	handler := httpapi.NewRouter(services, solveService)

	server := &http.Server{
		Addr:              ":8000",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       settings.SolverTotalTimeout + 10*time.Second,
		WriteTimeout:      settings.SolverTotalTimeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("captchaflow api listening", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
	slog.Info("captchaflow api stopped")
}

// resolveEnvFile prefers an explicit path, then the repository root .env,
// then the working directory .env.
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
