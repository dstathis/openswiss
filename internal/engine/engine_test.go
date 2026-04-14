//go:build integration

package engine

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
	st "github.com/dstathis/swisstools"
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

func setupTournamentWithPlayers(t *testing.T, database *sql.DB, numPlayers int) (*models.Tournament, []models.Registration) {
	t.Helper()
	ctx := context.Background()

	prefix := t.Name()
	org, err := db.CreateUser(ctx, database, fmt.Sprintf("org-%s@example.com", prefix), "Org-"+prefix, "hash")
	if err != nil {
		t.Fatalf("create organizer: %v", err)
	}

	numRounds := 3
	tourn := &models.Tournament{
		Name:        "Engine Test",
		MaxPlayers:  32,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: org.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}

	var regs []models.Registration
	for i := 0; i < numPlayers; i++ {
		email := fmt.Sprintf("player%d-%s@example.com", i, prefix)
		name := fmt.Sprintf("Player%d-%s", i, prefix)
		u, err := db.CreateUser(ctx, database, email, name, "hash")
		if err != nil {
			t.Fatalf("create player %d: %v", i, err)
		}
		reg, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID)
		if err != nil {
			t.Fatalf("register player %d: %v", i, err)
		}
		reg.DisplayName = name
		regs = append(regs, *reg)
	}

	return tourn, regs
}

func TestInitTournamentEngine(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	tourn, regs := setupTournamentWithPlayers(t, database, 4)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	state, err := InitTournamentEngine(ctx, tx, tourn, regs)
	if err != nil {
		t.Fatalf("InitTournamentEngine: %v", err)
	}
	if len(state) == 0 {
		t.Fatal("expected non-empty engine state")
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	for _, reg := range regs {
		got, err := db.GetRegistration(ctx, database, tourn.ID, reg.UserID)
		if err != nil {
			t.Fatalf("get registration for user %d: %v", reg.UserID, err)
		}
		if got.EnginePlayerID == nil {
			t.Errorf("player %d: expected engine_player_id to be set", reg.UserID)
		}
	}
}

func TestWithTournamentEngine(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	tourn, regs := setupTournamentWithPlayers(t, database, 4)

	// Initialize the engine
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	state, err := InitTournamentEngine(ctx, tx, tourn, regs)
	if err != nil {
		t.Fatalf("InitTournamentEngine: %v", err)
	}
	if err := db.UpdateTournamentEngineState(ctx, tx, tourn.ID, models.TournamentStatusInProgress, state); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Use WithTournamentEngine to load and verify
	err = WithTournamentEngine(ctx, database, tourn.ID, func(wtx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
		round := eng.GetCurrentRound()
		if round == 0 {
			return "", fmt.Errorf("expected non-zero current round")
		}
		return models.TournamentStatusInProgress, nil
	})
	if err != nil {
		t.Fatalf("WithTournamentEngine: %v", err)
	}

	got, err := db.GetTournament(ctx, database, tourn.ID)
	if err != nil {
		t.Fatalf("get tournament: %v", err)
	}
	if got.Status != models.TournamentStatusInProgress {
		t.Errorf("status = %q, want %q", got.Status, models.TournamentStatusInProgress)
	}
	if len(got.EngineState) == 0 {
		t.Error("expected non-empty engine state after WithTournamentEngine")
	}
}

func TestWithTournamentEngine_ErrorRollback(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	tourn, regs := setupTournamentWithPlayers(t, database, 4)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	state, err := InitTournamentEngine(ctx, tx, tourn, regs)
	if err != nil {
		t.Fatalf("InitTournamentEngine: %v", err)
	}
	if err := db.UpdateTournamentEngineState(ctx, tx, tourn.ID, models.TournamentStatusInProgress, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Callback returns an error - should rollback
	testErr := fmt.Errorf("intentional error")
	err = WithTournamentEngine(ctx, database, tourn.ID, func(wtx *sql.Tx, tm *models.Tournament, eng *st.Tournament) (string, error) {
		return "", testErr
	})
	if err != testErr {
		t.Errorf("expected testErr, got %v", err)
	}

	got, _ := db.GetTournament(ctx, database, tourn.ID)
	if got.Status != models.TournamentStatusInProgress {
		t.Errorf("status = %q, want %q (should not have changed)", got.Status, models.TournamentStatusInProgress)
	}
}
