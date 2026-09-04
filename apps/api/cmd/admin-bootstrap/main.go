// Command admin-bootstrap creates or refreshes the operator account named by
// ADMIN_BOOTSTRAP_USERNAME/ADMIN_BOOTSTRAP_PASSWORD (or their flags). The
// password is only hashed into admin_users and never logged.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/captchaflow/service-platform/api/internal/config"
	"github.com/captchaflow/service-platform/api/internal/service"
	"github.com/captchaflow/service-platform/api/internal/store"
)

func main() {
	envFile := flag.String("env", "../../.env", "path to the environment file")
	username := flag.String("username", "", "administrator username (overrides the environment)")
	password := flag.String("password", "", "administrator password (overrides the environment)")
	flag.Parse()

	settings, err := config.Load(*envFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if *username != "" {
		settings.AdminUsername = *username
	}
	if *password != "" {
		settings.AdminPassword = *password
	}

	pool, err := store.Connect(context.Background(), settings.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	services := service.NewServices(pool, settings)
	result, appErr := services.BootstrapAdmin(context.Background())
	if appErr != nil {
		fmt.Fprintf(os.Stderr, "bootstrap admin: %s\n", strings.TrimSpace(appErr.Message))
		os.Exit(1)
	}
	fmt.Printf("bootstrap admin ready: %s (%s)\n", result.Username, result.AdminID)
}
