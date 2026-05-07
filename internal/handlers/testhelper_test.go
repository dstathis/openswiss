//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
	"github.com/go-chi/chi/v5"
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

func mustCreateUser(t *testing.T, database *sql.DB, email, name string, roles ...string) *models.User {
	t.Helper()
	ctx := context.Background()
	u, err := db.CreateUser(ctx, database, email, name, "hash")
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	if len(roles) > 0 {
		if err := db.UpdateUserRoles(ctx, database, u.ID, roles); err != nil {
			t.Fatalf("set roles for %s: %v", email, err)
		}
		u.Roles = roles
	}
	return u
}

func mustCreateTournament(t *testing.T, database *sql.DB, organizerID int64, status string) *models.Tournament {
	t.Helper()
	tourn := &models.Tournament{
		Name:        "Test " + t.Name(),
		MaxPlayers:  16,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      status,
		OrganizerID: organizerID,
	}
	if err := db.CreateTournament(context.Background(), database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	return tourn
}

// startedTournament returns an organizer + tournament that's been started with
// 4 players, all round-1 results submitted (so Finish/NextRound is valid).
func startedTournament(t *testing.T, database *sql.DB) (*models.User, *models.Tournament) {
	t.Helper()
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner-st-"+t.Name()+"@example.com", "Owner-st-"+t.Name())
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "Started " + t.Name(),
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: owner.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	for i := 0; i < 4; i++ {
		u := mustCreateUser(t, database, fmt.Sprintf("p%d-%s@example.com", i, t.Name()), fmt.Sprintf("P%d-%s", i, t.Name()))
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register player %d: %v", i, err)
		}
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if err := engine.WithTournamentEngine(ctx, database, tourn.ID,
		func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
			state, err := engine.InitTournamentEngine(ctx, tx, tm, regs)
			if err != nil {
				return "", err
			}
			ne, err := swisstools.LoadTournament(state)
			if err != nil {
				return "", err
			}
			*eng = ne
			for _, p := range eng.GetRound() {
				if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
					continue
				}
				if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
					return "", err
				}
			}
			return models.TournamentStatusInProgress, nil
		}); err != nil {
		t.Fatalf("init engine: %v", err)
	}
	tourn, err := db.GetTournament(ctx, database, tourn.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return owner, tourn
}

// requestWithUser builds a request with user in context and chi URL params resolved.
func requestWithUser(method, target, body string, user *models.User, params map[string]string) *http.Request {
	if target == "" {
		target = "/"
	}
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if user != nil {
		ctx := context.WithValue(req.Context(), middleware.UserContextKey, user)
		req = req.WithContext(ctx)
	}
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	return req
}
