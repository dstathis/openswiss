package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/openswiss/internal/results"
	st "github.com/dstathis/swisstools"
)

// runBackfillResults builds the statistics projection for finished
// tournaments that predate it, or rebuilds it after a projection bug.
//
// This is always safe to run: tournaments.engine_state remains the source of
// truth, so the matches and tournament_results tables can be regenerated from
// it at any time.
func runBackfillResults(args []string) {
	fs := flag.NewFlagSet("backfill-results", flag.ExitOnError)
	force := fs.Bool("force", false, "rebuild tournaments that already have a projection")
	if err := fs.Parse(args); err != nil {
		fatal("parse flags", "err", err)
	}

	dsn := mustEnv("DATABASE_URL")
	database, err := openDB(dsn)
	if err != nil {
		fatal("connect db", "err", err)
	}
	defer database.Close()

	ctx := context.Background()
	ids, err := finishedTournamentIDs(ctx, database, *force)
	if err != nil {
		fatal("list tournaments", "err", err)
	}

	var done, skipped int
	for _, id := range ids {
		if err := backfillOne(ctx, database, id, *force); err != nil {
			slog.Error("backfill failed", "tournament_id", id, "err", err)
			skipped++
			continue
		}
		done++
	}

	slog.Info("backfill complete", "projected", done, "failed", skipped, "force", *force)
	if skipped > 0 {
		fatal("some tournaments could not be projected", "failed", skipped)
	}
}

// finishedTournamentIDs returns the finished tournaments to project. Without
// --force, tournaments that already have rows are left alone.
func finishedTournamentIDs(ctx context.Context, database *sql.DB, force bool) ([]int64, error) {
	query := `SELECT t.id FROM tournaments t WHERE t.status = $1`
	if !force {
		query += ` AND NOT EXISTS (
			SELECT 1 FROM tournament_results r WHERE r.tournament_id = t.id
		)`
	}
	query += ` ORDER BY t.id`

	rows, err := database.QueryContext(ctx, query, models.TournamentStatusFinished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func backfillOne(ctx context.Context, database *sql.DB, tournamentID int64, force bool) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	t, err := db.GetTournamentForUpdate(ctx, tx, tournamentID)
	if err != nil {
		return fmt.Errorf("get tournament: %w", err)
	}
	if len(t.EngineState) == 0 {
		// A finished tournament with no engine state has no results to
		// project; nothing to do and nothing wrong.
		return nil
	}

	eng, err := st.LoadTournament(t.EngineState)
	if err != nil {
		return fmt.Errorf("load engine state: %w", err)
	}

	if force {
		if err := results.Clear(ctx, tx, tournamentID); err != nil {
			return err
		}
	}

	regs, err := db.ListRegistrationsTx(ctx, tx, tournamentID)
	if err != nil {
		return fmt.Errorf("list registrations: %w", err)
	}
	resultRows, matchRows, err := results.BuildRows(t, &eng, regs)
	if err != nil {
		return fmt.Errorf("build rows: %w", err)
	}
	if err := results.Project(ctx, tx, resultRows, matchRows); err != nil {
		return err
	}

	return tx.Commit()
}
