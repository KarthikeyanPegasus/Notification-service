package main

import (
	"context"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spidey/notification-service/internal/config"
	"github.com/spidey/notification-service/internal/repository"
	"github.com/spidey/notification-service/internal/service"
)

func main() {
	name := "local-dev"
	if len(os.Args) > 1 && os.Args[1] != "" {
		name = os.Args[1]
	}

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "loading config:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := repository.NewDB(ctx, cfg.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connecting db:", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := runMigrations(cfg.Database.DSN, cfg.Database.MigrationDir); err != nil {
		fmt.Fprintln(os.Stderr, "running migrations:", err)
		os.Exit(1)
	}

	apiKeyRepo := repository.NewAPIKeyRepository(db)
	apiKeySvc := service.NewAPIKeyService(apiKeyRepo)

	out, err := apiKeySvc.Create(ctx, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "creating api key:", err)
		os.Exit(1)
	}

	// Print the API key only (store it securely; it is not retrievable again).
	fmt.Println(out.APIKey)
}

func runMigrations(dsn, dir string) error {
	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running up migrations: %w", err)
	}
	return nil
}
