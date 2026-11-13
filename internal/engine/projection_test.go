//go:build integration

package engine

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
	st "github.com/dstathis/swisstools"
)

// finishedTournament builds a tournament with `n` registered users, plays one
// round, and finishes it through WithTournamentEngine -- the same path the
// handlers and API use. Returns the tournament and its users.
func finishedTournament(t *testing.T, database *sql.DB, prefix string, n int) (*models.Tournament, []*models.User) {
	t.Helper()
	ctx := context.Background()

	org, err := db.CreateUser(ctx, database, prefix+"-org@example.com", prefix+"Org", "hash")
	if err != nil {
		t.Fatalf("create organizer: %v", err)
	}
	rounds := 1
	tourn := &models.Tournament{
		Name:        prefix + " Cup",
		MaxPlayers:  16,
		NumRounds:   &rounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: org.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}

	var users []*models.User
	for i := 0; i < n; i++ {
		name := prefix + string(rune('A'+i))
		u, err := db.CreateUser(ctx, database, prefix+string(rune('a'+i))+"@example.com", name, "hash")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register: %v", err)
		}
		users = append(users, u)
	}

	// Start: initialise the engine from the registrations.
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
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

	// Play round 1 and finish, through the same wrapper the app uses.
	err = WithTournamentEngine(ctx, database, tourn.ID,
		func(tx *sql.Tx, tr *models.Tournament, eng *st.Tournament) (string, error) {
			for _, p := range eng.GetRound() {
				if p.PlayerB() != st.BYE_OPPONENT_ID {
					if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
						return "", err
					}
				}
			}
			// FinishTournament calls UpdatePlayerStandings itself; calling it
			// here as well would count every result twice, which is why the
			// app never calls it directly either.
			if err := eng.FinishTournament(); err != nil {
				return "", err
			}
			return models.TournamentStatusFinished, nil
		})
	if err != nil {
		t.Fatalf("finish tournament: %v", err)
	}

	tourn, err = db.GetTournament(ctx, database, tourn.ID)
	if err != nil {
		t.Fatalf("reload tournament: %v", err)
	}
	return tourn, users
}

func countRows(t *testing.T, database *sql.DB, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := database.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestFinishingTournamentWritesProjection(t *testing.T) {
	database := testDB(t)
	tourn, _ := finishedTournament(t, database, "proj", 4)

	results := countRows(t, database, `SELECT count(*) FROM tournament_results WHERE tournament_id = $1`, tourn.ID)
	if results != 4 {
		t.Errorf("expected 4 tournament_results rows, got %d", results)
	}

	// Two pairings, mirrored.
	matches := countRows(t, database, `SELECT count(*) FROM matches WHERE tournament_id = $1`, tourn.ID)
	if matches != 4 {
		t.Errorf("expected 4 match rows, got %d", matches)
	}

	// Every player got a rank, and ranks start at 1.
	best := countRows(t, database, `SELECT min(final_rank) FROM tournament_results WHERE tournament_id = $1`, tourn.ID)
	if best != 1 {
		t.Errorf("expected a first place, got min rank %d", best)
	}
}

// The projection must be written in the same transaction that flips the
// status, so a finished tournament can never exist without its rows.
func TestProjectionIsAtomicWithStatus(t *testing.T) {
	database := testDB(t)
	tourn, _ := finishedTournament(t, database, "atomic", 4)

	if tourn.Status != models.TournamentStatusFinished {
		t.Fatalf("expected finished, got %q", tourn.Status)
	}
	if n := countRows(t, database, `SELECT count(*) FROM tournament_results WHERE tournament_id = $1`, tourn.ID); n == 0 {
		t.Error("tournament is finished but has no projection rows")
	}
}

// Running the finish path again must not duplicate rows.
func TestProjectionIsIdempotent(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn, _ := finishedTournament(t, database, "idem", 4)

	before := countRows(t, database, `SELECT count(*) FROM matches WHERE tournament_id = $1`, tourn.ID)

	// Re-enter the wrapper returning 'finished' again. The guard on the prior
	// status should skip the projection entirely; even if it did not, the
	// inserts are ON CONFLICT DO NOTHING.
	err := WithTournamentEngine(ctx, database, tourn.ID,
		func(tx *sql.Tx, tr *models.Tournament, eng *st.Tournament) (string, error) {
			return models.TournamentStatusFinished, nil
		})
	if err != nil {
		t.Fatalf("second finish: %v", err)
	}

	after := countRows(t, database, `SELECT count(*) FROM matches WHERE tournament_id = $1`, tourn.ID)
	if before != after {
		t.Errorf("projection duplicated rows: %d -> %d", before, after)
	}
}

// Deleting a tournament must take its projection with it. This guards the
// planned "delete past tournaments" feature: the schema has to be correct
// before that state gate is lifted.
func TestDeletingTournamentClearsProjection(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn, _ := finishedTournament(t, database, "del", 4)

	if n := countRows(t, database, `SELECT count(*) FROM matches WHERE tournament_id = $1`, tourn.ID); n == 0 {
		t.Fatal("expected match rows before delete")
	}
	if err := db.DeleteTournament(ctx, database, tourn.ID); err != nil {
		t.Fatalf("delete tournament: %v", err)
	}
	if n := countRows(t, database, `SELECT count(*) FROM matches WHERE tournament_id = $1`, tourn.ID); n != 0 {
		t.Errorf("expected matches to cascade away, got %d", n)
	}
	if n := countRows(t, database, `SELECT count(*) FROM tournament_results WHERE tournament_id = $1`, tourn.ID); n != 0 {
		t.Errorf("expected tournament_results to cascade away, got %d", n)
	}
}
