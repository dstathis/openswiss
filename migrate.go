package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// runMigrate applies all pending migrations and exits. Intended to run as a
// one-shot pod / compose service before the app boots, so a multi-replica
// rollout doesn't have N replicas racing on migrate.Up().
func runMigrate(_ []string) {
	dsn := mustEnv("DATABASE_URL")

	database, err := openDB(dsn)
	if err != nil {
		fatal("connect db", "err", err)
	}
	defer database.Close()

	if err := applyMigrations(database); err != nil {
		fatal("apply migrations", "err", err)
	}
	slog.Info("migrations applied")
}

func applyMigrations(database *sql.DB) error {
	driver, err := postgres.WithInstance(database, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("driver: %w", err)
	}
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("up: %w", err)
	}
	return nil
}
