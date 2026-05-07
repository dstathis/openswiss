//go:build integration

package middleware

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
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
	database, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	driver, err := postgres.WithInstance(database, &postgres.Config{})
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
	for _, table := range []string{"password_resets", "registrations", "api_keys", "sessions", "tournaments", "users"} {
		if _, err := database.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			t.Fatalf("clean table %s: %v", table, err)
		}
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSessionAuth_NoCookie(t *testing.T) {
	database := testDB(t)
	called := false
	handler := SessionAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should be called")
	}
}

func TestSessionAuth_ValidSession(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	user, _ := db.CreateUser(ctx, database, "u@example.com", "U", "hash")
	token, _ := auth.GenerateSessionToken()
	if err := db.CreateSession(ctx, database, token, user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	var got int64
	handler := SessionAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			got = u.ID
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got != user.ID {
		t.Errorf("expected user.ID %d in context, got %d", user.ID, got)
	}
}

func TestSessionAuth_BadToken(t *testing.T) {
	database := testDB(t)
	handler := SessionAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user for bad token, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "nonexistent-token"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestSessionAuth_ExpiredSession(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	user, _ := db.CreateUser(ctx, database, "u@example.com", "U", "hash")
	token, _ := auth.GenerateSessionToken()
	if err := db.CreateSession(ctx, database, token, user.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	handler := SessionAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user for expired session, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestAPIKeyAuth_NoHeader(t *testing.T) {
	database := testDB(t)
	called := false
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should be called")
	}
}

func TestAPIKeyAuth_NotBearer(t *testing.T) {
	database := testDB(t)
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestAPIKeyAuth_TokenTooShort(t *testing.T) {
	database := testDB(t)
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer short")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestAPIKeyAuth_UnknownPrefix(t *testing.T) {
	database := testDB(t)
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer os_aaaaaaaaaa_doesnotexist1234567890123456789012345678901234567890")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	user, _ := db.CreateUser(ctx, database, "u@example.com", "U", "hash")
	full, prefix, _ := auth.GenerateAPIKey()
	hash, _ := auth.HashAPIKey(full)
	if _, err := db.CreateAPIKey(ctx, database, user.ID, hash, prefix, "Test", nil); err != nil {
		t.Fatalf("create key: %v", err)
	}

	var got int64
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			got = u.ID
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+full)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got != user.ID {
		t.Errorf("expected user.ID %d, got %d", user.ID, got)
	}
}

func TestAPIKeyAuth_BadKeyMatchingPrefix(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	user, _ := db.CreateUser(ctx, database, "u@example.com", "U", "hash")
	full, prefix, _ := auth.GenerateAPIKey()
	hash, _ := auth.HashAPIKey(full)
	if _, err := db.CreateAPIKey(ctx, database, user.ID, hash, prefix, "Test", nil); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Same prefix but wrong full key
	wrong := prefix + "_doesnotmatch1234567890123456789012345678901234567890123456"
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			t.Errorf("expected no user for invalid key, got %+v", u)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+wrong)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestAPIKeyAuth_SkipsIfUserAlreadySet(t *testing.T) {
	database := testDB(t)
	// SessionAuth populated the user; APIKeyAuth should pass through unchanged.
	preUser := &models.User{ID: 7, Email: "pre@example.com"}
	var got int64
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := GetUser(r.Context()); u != nil {
			got = u.ID
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer os_anything_at_all_should_be_ignored_here_when_user_already_set_x")
	ctx := context.WithValue(req.Context(), UserContextKey, preUser)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got != 7 {
		t.Errorf("expected pre-set user id 7, got %d", got)
	}
}
