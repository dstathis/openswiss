//go:build integration

package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/models"
)

func createTimerTournament(t *testing.T, database *sql.DB) *models.Tournament {
	t.Helper()
	ctx := context.Background()
	org := createTestOrganizer(t, database)
	tourn := &models.Tournament{
		Name:        "Timer Cup",
		PointsWin:   3,
		PointsDraw:  1,
		Status:      models.TournamentStatusInProgress,
		OrganizerID: org.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("CreateTournament: %v", err)
	}
	return tourn
}

func TestStartAndGetRoundTimer(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	timer, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil)
	if err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	if timer.DurationSeconds != 3000 {
		t.Errorf("duration_seconds = %d, want 3000", timer.DurationSeconds)
	}
	if timer.Paused() {
		t.Error("a freshly started timer must not be paused")
	}

	got, err := GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("GetRoundTimer: %v", err)
	}
	if got == nil {
		t.Fatal("GetRoundTimer returned nil for a timer that was just started")
	}
	// Allow a little slack: ends_at is computed from the database's now().
	if r := got.RemainingSeconds(time.Now()); r < 2990 || r > 3000 {
		t.Errorf("remaining = %d, want ~3000", r)
	}
}

// A missing timer is not an error -- it is the ordinary state of a round
// nobody has started a clock for, and the page renders nothing for it.
func TestGetRoundTimerAbsentIsNotAnError(t *testing.T) {
	database := testDB(t)
	tourn := createTimerTournament(t, database)

	got, err := GetRoundTimer(context.Background(), database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("GetRoundTimer on an unstarted round: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil timer, got %+v", got)
	}
}

// The whole point of keying on (tournament, round, playoff): once the round
// advances, the previous round's clock must not be visible as the current one.
func TestRoundTimerIsScopedToItsRound(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 3, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}

	got, err := GetRoundTimer(ctx, database, tourn.ID, 4, false)
	if err != nil {
		t.Fatalf("GetRoundTimer round 4: %v", err)
	}
	if got != nil {
		t.Errorf("round 3's timer leaked into round 4: %+v", got)
	}
}

// Swiss round 1 and playoff round 1 are different rounds and must not collide.
func TestRoundTimerSwissAndPlayoffAreSeparate(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("start swiss timer: %v", err)
	}
	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, true, 90*time.Minute, nil); err != nil {
		t.Fatalf("start playoff timer: %v", err)
	}

	swiss, err := GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil || swiss == nil {
		t.Fatalf("get swiss timer: %v, %+v", err, swiss)
	}
	playoff, err := GetRoundTimer(ctx, database, tourn.ID, 1, true)
	if err != nil || playoff == nil {
		t.Fatalf("get playoff timer: %v, %+v", err, playoff)
	}
	if swiss.DurationSeconds != 3000 {
		t.Errorf("swiss duration = %d, want 3000", swiss.DurationSeconds)
	}
	if playoff.DurationSeconds != 5400 {
		t.Errorf("playoff duration = %d, want 5400", playoff.DurationSeconds)
	}
}

// Starting over an existing timer restarts it rather than erroring, which is
// how a judge fixes a wrong duration.
func TestStartRoundTimerOverwrites(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := PauseRoundTimer(ctx, database, tourn.ID, 1, false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 10*time.Minute, nil); err != nil {
		t.Fatalf("restart: %v", err)
	}

	got, err := GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil || got == nil {
		t.Fatalf("GetRoundTimer: %v, %+v", err, got)
	}
	if got.DurationSeconds != 600 {
		t.Errorf("duration_seconds = %d, want 600 after restart", got.DurationSeconds)
	}
	if got.Paused() {
		t.Error("restarting must clear a previous pause")
	}
}

func TestPauseAndResumeRoundTimer(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}

	paused, err := PauseRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("PauseRoundTimer: %v", err)
	}
	if !paused.Paused() {
		t.Fatal("timer is not paused after PauseRoundTimer")
	}
	frozen := paused.RemainingSeconds(time.Now())

	// The remainder must not drift while paused, however much time passes.
	time.Sleep(1100 * time.Millisecond)
	stillPaused, err := GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil || stillPaused == nil {
		t.Fatalf("GetRoundTimer while paused: %v, %+v", err, stillPaused)
	}
	if got := stillPaused.RemainingSeconds(time.Now()); got != frozen {
		t.Errorf("paused remainder drifted: %d, want %d", got, frozen)
	}

	resumed, err := ResumeRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("ResumeRoundTimer: %v", err)
	}
	if resumed.Paused() {
		t.Error("timer still paused after ResumeRoundTimer")
	}
	// Resuming continues from where it froze rather than recomputing from the
	// original start, so the paused second must not have been consumed.
	if got := resumed.RemainingSeconds(time.Now()); got < frozen-1 || got > frozen {
		t.Errorf("resumed remainder = %d, want ~%d", got, frozen)
	}
}

// Pausing twice must not compound: the second pause has nothing to freeze.
func TestPauseRoundTimerIsIdempotent(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	first, err := PauseRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("first pause: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	second, err := PauseRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("second pause: %v", err)
	}
	if !second.PausedAt.Equal(*first.PausedAt) {
		t.Errorf("second pause moved paused_at from %v to %v", first.PausedAt, second.PausedAt)
	}
}

func TestResumeRunningRoundTimerIsANoop(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	started, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil)
	if err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	resumed, err := ResumeRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("ResumeRoundTimer: %v", err)
	}
	if !resumed.EndsAt.Equal(started.EndsAt) {
		t.Errorf("resuming a running timer moved ends_at from %v to %v", started.EndsAt, resumed.EndsAt)
	}
}

func TestPauseAbsentRoundTimer(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	got, err := PauseRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != ErrNoRoundTimer {
		t.Errorf("PauseRoundTimer with no timer: err = %v, want ErrNoRoundTimer", err)
	}
	if got != nil {
		t.Errorf("expected nil timer, got %+v", got)
	}
}

func TestDeleteRoundTimer(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	if err := DeleteRoundTimer(ctx, database, tourn.ID, 1, false); err != nil {
		t.Fatalf("DeleteRoundTimer: %v", err)
	}
	got, err := GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("GetRoundTimer after delete: %v", err)
	}
	if got != nil {
		t.Errorf("timer survived delete: %+v", got)
	}
}

// Deleting a tournament must not leave orphaned timer rows behind.
func TestDeleteTournamentCascadesRoundTimers(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tourn := createTimerTournament(t, database)

	if _, err := StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	if err := DeleteTournament(ctx, database, tourn.ID); err != nil {
		t.Fatalf("DeleteTournament: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx,
		"SELECT count(*) FROM round_timers WHERE tournament_id = $1", tourn.ID).Scan(&count); err != nil {
		t.Fatalf("count round_timers: %v", err)
	}
	if count != 0 {
		t.Errorf("round_timers left %d orphaned rows", count)
	}
}

// round_minutes is a tournament setting, so it must survive a create/read and
// an update like every other column.
func TestTournamentRoundMinutesRoundTrips(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	org := createTestOrganizer(t, database)

	fifty := 50
	tourn := &models.Tournament{
		Name:         "Round Minutes Cup",
		RoundMinutes: &fifty,
		Status:       models.TournamentStatusScheduled,
		OrganizerID:  org.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("CreateTournament: %v", err)
	}

	got, err := GetTournament(ctx, database, tourn.ID)
	if err != nil {
		t.Fatalf("GetTournament: %v", err)
	}
	if got.RoundMinutes == nil || *got.RoundMinutes != 50 {
		t.Fatalf("round_minutes = %v, want 50", got.RoundMinutes)
	}

	thirty := 30
	got.RoundMinutes = &thirty
	if err := UpdateTournament(ctx, database, got); err != nil {
		t.Fatalf("UpdateTournament: %v", err)
	}
	again, err := GetTournament(ctx, database, tourn.ID)
	if err != nil {
		t.Fatalf("GetTournament after update: %v", err)
	}
	if again.RoundMinutes == nil || *again.RoundMinutes != 30 {
		t.Fatalf("round_minutes after update = %v, want 30", again.RoundMinutes)
	}
}
