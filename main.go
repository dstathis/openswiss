package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"

	mw "github.com/dstathis/openswiss/internal/middleware"
)

func main() {
	// Structured JSON logging for production. Wrap with ContextLogHandler so
	// per-request attrs (request ID) are added automatically when callers use
	// the *Context variants.
	slog.SetDefault(slog.New(mw.ContextLogHandler{
		Handler: slog.NewJSONHandler(os.Stderr, nil),
	}))

	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}

	switch cmd {
	case "serve":
		runServe(args)
	case "migrate":
		runMigrate(args)
	case "-h", "--help", "help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", cmd)
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `openswiss — Swiss-system tournament server

Usage:
  openswiss serve     Run the HTTP server (default)
  openswiss migrate   Apply database migrations and exit
  openswiss help      Show this message

Configuration is via environment variables. See README.md.
`)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fatal("missing required env var", "name", key)
	}
	return v
}

// fatal logs a structured error and exits 1. Use only at startup; a running
// process should never abort the whole server for one bad request.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// openDB opens the PostgreSQL connection, configures pool limits, and waits
// for the database to accept connections. Used by both serve and migrate.
func openDB(dsn string) (*sql.DB, error) {
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(25)
	database.SetConnMaxLifetime(30 * time.Minute)
	database.SetConnMaxIdleTime(5 * time.Minute)

	for i := 0; i < 30; i++ {
		if err = database.Ping(); err == nil {
			return database, nil
		}
		time.Sleep(time.Second)
	}
	database.Close()
	return nil, fmt.Errorf("ping: %w", err)
}
