package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/dstathis/openswiss/internal/models"
)

// ErrNoRoundTimer is returned by the operations that act on an existing timer
// (pause, resume) when there is nothing to act on. Callers translate it to a
// 404/409 rather than a generic 500. GetRoundTimer deliberately does *not*
// return it -- "no timer" is the ordinary resting state of a round, not an
// error condition.
var ErrNoRoundTimer = errors.New("round timer: not found")

const roundTimerColumns = `tournament_id, round, playoff, duration_seconds,
	ends_at, paused_at, started_by, started_at`

func scanRoundTimer(row interface {
	Scan(dest ...any) error
}) (*models.RoundTimer, error) {
	var t models.RoundTimer
	err := row.Scan(&t.TournamentID, &t.Round, &t.Playoff, &t.DurationSeconds,
		&t.EndsAt, &t.PausedAt, &t.StartedBy, &t.StartedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetRoundTimer returns the timer for one round, or (nil, nil) when no one has
// started one. The absent case is not an error: the detail page renders
// nothing for a round with no clock, and the JSON endpoint reports
// state "none", so every caller wants to distinguish absent from broken.
func GetRoundTimer(ctx context.Context, database DBTX, tournamentID int64, round int, playoff bool) (*models.RoundTimer, error) {
	timer, err := scanRoundTimer(database.QueryRowContext(ctx,
		`SELECT `+roundTimerColumns+` FROM round_timers
		 WHERE tournament_id = $1 AND round = $2 AND playoff = $3`,
		tournamentID, round, playoff))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return timer, nil
}

// StartRoundTimer starts (or restarts) the clock for one round. ends_at is
// computed from the database's now() rather than the app server's, so the
// timer is anchored to the same clock that later serves it and pause/resume
// arithmetic can stay in SQL.
//
// Starting over a running or paused timer overwrites it and clears any pause.
// That is how a judge corrects a wrong duration, so it is an upsert rather
// than a conflict.
func StartRoundTimer(ctx context.Context, database DBTX, tournamentID int64, round int, playoff bool, d time.Duration, startedBy *int64) (*models.RoundTimer, error) {
	seconds := int(d / time.Second)
	return scanRoundTimer(database.QueryRowContext(ctx,
		`INSERT INTO round_timers (tournament_id, round, playoff, duration_seconds,
			ends_at, paused_at, started_by, started_at)
		 VALUES ($1, $2, $3, $4, now() + ($4::int * interval '1 second'), NULL, $5, now())
		 ON CONFLICT (tournament_id, round, playoff) DO UPDATE
		 SET duration_seconds = EXCLUDED.duration_seconds,
		     ends_at          = EXCLUDED.ends_at,
		     paused_at        = NULL,
		     started_by       = EXCLUDED.started_by,
		     started_at       = EXCLUDED.started_at
		 RETURNING `+roundTimerColumns,
		tournamentID, round, playoff, seconds, startedBy))
}

// PauseRoundTimer freezes the clock by stamping paused_at. It is idempotent:
// pausing an already-paused timer leaves the original paused_at alone, so a
// double click can't quietly hand the round extra time.
func PauseRoundTimer(ctx context.Context, database DBTX, tournamentID int64, round int, playoff bool) (*models.RoundTimer, error) {
	return updateRoundTimer(ctx, database,
		`UPDATE round_timers SET paused_at = now()
		 WHERE tournament_id = $1 AND round = $2 AND playoff = $3 AND paused_at IS NULL
		 RETURNING `+roundTimerColumns,
		tournamentID, round, playoff)
}

// ResumeRoundTimer restarts a frozen clock by pushing ends_at forward by however
// long it was paused, so the round continues from where it stopped rather than
// losing the paused interval. Resuming a running timer is a no-op.
func ResumeRoundTimer(ctx context.Context, database DBTX, tournamentID int64, round int, playoff bool) (*models.RoundTimer, error) {
	return updateRoundTimer(ctx, database,
		`UPDATE round_timers SET ends_at = ends_at + (now() - paused_at), paused_at = NULL
		 WHERE tournament_id = $1 AND round = $2 AND playoff = $3 AND paused_at IS NOT NULL
		 RETURNING `+roundTimerColumns,
		tournamentID, round, playoff)
}

// updateRoundTimer runs a conditional UPDATE ... RETURNING and distinguishes
// "no such timer" from "the timer was already in the requested state". Both
// update zero rows, so the row has to be re-read to tell them apart; the
// already-in-state case returns the current timer and no error, which keeps
// pause and resume idempotent for the caller.
func updateRoundTimer(ctx context.Context, database DBTX, query string, tournamentID int64, round int, playoff bool) (*models.RoundTimer, error) {
	timer, err := scanRoundTimer(database.QueryRowContext(ctx, query, tournamentID, round, playoff))
	if errors.Is(err, sql.ErrNoRows) {
		existing, getErr := GetRoundTimer(ctx, database, tournamentID, round, playoff)
		if getErr != nil {
			return nil, getErr
		}
		if existing == nil {
			return nil, ErrNoRoundTimer
		}
		return existing, nil
	}
	if err != nil {
		return nil, err
	}
	return timer, nil
}

// DeleteRoundTimer clears the clock for one round, which is how a judge undoes
// a misclick. Deleting an absent timer succeeds -- the caller asked for a state
// that already holds.
func DeleteRoundTimer(ctx context.Context, database DBTX, tournamentID int64, round int, playoff bool) error {
	_, err := database.ExecContext(ctx,
		`DELETE FROM round_timers WHERE tournament_id = $1 AND round = $2 AND playoff = $3`,
		tournamentID, round, playoff)
	return err
}
