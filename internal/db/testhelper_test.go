//go:build integration

package db

import (
"database/sql"
"fmt"
"os"
"testing"

"github.com/golang-migrate/migrate/v4"
"github.com/golang-migrate/migrate/v4/database/postgres"
_ "github.com/golang-migrate/migrate/v4/source/file"
_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
t.Helper()
dsn := os.Getenv("TEST_DATABASE_URL")
if dsn == "" {
t.Skip("TEST_DATABASE_URL not set, skipping integration test")
}
db, err := sql.Open("postgres", dsn)
if err != nil {
t.Fatalf("open db: %v", err)
}
if err := db.Ping(); err != nil {
t.Fatalf("ping db: %v", err)
}

// Run migrations
driver, err := postgres.WithInstance(db, &postgres.Config{})
if err != nil {
t.Fatalf("create migration driver: %v", err)
}
m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
if err != nil {
t.Fatalf("create migrator: %v", err)
}
if err := m.Up(); err != nil && err != migrate.ErrNoChange {
t.Fatalf("run migrations: %v", err)
}

// Clean all tables before each test
for _, table := range []string{"password_resets", "registrations", "api_keys", "sessions", "tournaments", "users"} {
if _, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
t.Fatalf("clean table %s: %v", table, err)
}
}

t.Cleanup(func() {
db.Close()
})

return db
}
