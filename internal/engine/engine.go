package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/openswiss/internal/results"
	st "github.com/dstathis/swisstools"
)

// WithTournamentEngine loads the tournament engine state within a transaction,
// calls the provided function, then saves the state back. The callback receives
// the tournament model and the loaded swisstools Tournament engine.
func WithTournamentEngine(ctx context.Context, database *sql.DB, tournamentID int64, fn func(tx *sql.Tx, t *models.Tournament, eng *st.Tournament) (string, error)) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	t, err := db.GetTournamentForUpdate(ctx, tx, tournamentID)
	if err != nil {
		return fmt.Errorf("get tournament: %w", err)
	}

	var eng st.Tournament
	if t.EngineState != nil && len(t.EngineState) > 0 {
		eng, err = st.LoadTournament(t.EngineState)
		if err != nil {
			return fmt.Errorf("load engine state: %w", err)
		}
	} else {
		eng = st.NewTournamentWithConfig(st.TournamentConfig{
			PointsForWin:  t.PointsWin,
			PointsForDraw: t.PointsDraw,
			PointsForLoss: t.PointsLoss,
			ByeWins:       st.BYE_WINS,
			ByeLosses:     st.BYE_LOSSES,
			ByeDraws:      st.BYE_DRAWS,
		})
	}

	newStatus, err := fn(tx, t, &eng)
	if err != nil {
		return err
	}

	data, err := eng.DumpTournament()
	if err != nil {
		return fmt.Errorf("dump engine state: %w", err)
	}

	if newStatus == "" {
		newStatus = t.Status
	}
	if err := db.UpdateTournamentEngineState(ctx, tx, tournamentID, newStatus, data); err != nil {
		return fmt.Errorf("save engine state: %w", err)
	}

	// Finishing a tournament is the trigger for building its statistics
	// projection. This is hooked here rather than at each caller because
	// WithTournamentEngine is the only path to the 'finished' status --
	// db.UpdateTournamentStatus is only ever called with registration_open or
	// in_progress -- and there are six separate places that return it.
	// Running inside the transaction makes the projection atomic with the
	// status change, so a tournament can never be finished without its rows.
	if newStatus == models.TournamentStatusFinished && t.Status != models.TournamentStatusFinished {
		if err := projectResults(ctx, tx, t, &eng); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// projectResults derives and writes the statistics projection for a finished
// tournament. Safe to run more than once: the writes are idempotent.
func projectResults(ctx context.Context, tx *sql.Tx, t *models.Tournament, eng *st.Tournament) error {
	regs, err := db.ListRegistrationsTx(ctx, tx, t.ID)
	if err != nil {
		return fmt.Errorf("list registrations for projection: %w", err)
	}
	resultRows, matchRows, err := results.BuildRows(t, eng, regs)
	if err != nil {
		return fmt.Errorf("build results projection: %w", err)
	}
	if err := results.Project(ctx, tx, resultRows, matchRows); err != nil {
		return fmt.Errorf("write results projection: %w", err)
	}
	return nil
}

// InitTournamentEngine creates a new engine with the tournament's config,
// adds all confirmed registrations as players, and returns the engine state.
func InitTournamentEngine(ctx context.Context, tx *sql.Tx, t *models.Tournament, regs []models.Registration) ([]byte, error) {
	eng := st.NewTournamentWithConfig(st.TournamentConfig{
		PointsForWin:  t.PointsWin,
		PointsForDraw: t.PointsDraw,
		PointsForLoss: t.PointsLoss,
		ByeWins:       st.BYE_WINS,
		ByeLosses:     st.BYE_LOSSES,
		ByeDraws:      st.BYE_DRAWS,
	})

	if t.NumRounds != nil && *t.NumRounds > 0 {
		eng.SetMaxRounds(*t.NumRounds)
	}

	for _, r := range regs {
		if r.Status != models.RegistrationStatusConfirmed {
			continue
		}

		if err := eng.AddPlayer(r.DisplayName); err != nil {
			return nil, fmt.Errorf("add player %s: %w", r.DisplayName, err)
		}

		playerID, ok := eng.GetPlayerID(r.DisplayName)
		if !ok {
			return nil, fmt.Errorf("player %s not found after adding", r.DisplayName)
		}

		// Guests have no user account, so no external ID to link.
		if r.UserID != nil {
			if err := eng.SetPlayerExternalID(playerID, int(*r.UserID)); err != nil {
				return nil, fmt.Errorf("set external ID for %s: %w", r.DisplayName, err)
			}
		}

		// Set decklist if available
		if r.Decklist != nil {
			var dl st.Decklist
			if err := json.Unmarshal(r.Decklist, &dl); err == nil {
				eng.SetPlayerDecklist(playerID, dl)
			}
		}

		if err := db.UpdateRegistrationEnginePlayerID(ctx, tx, r.ID, playerID); err != nil {
			return nil, fmt.Errorf("update engine player id: %w", err)
		}
	}

	if err := eng.StartTournament(); err != nil {
		return nil, fmt.Errorf("start tournament: %w", err)
	}

	return eng.DumpTournament()
}
