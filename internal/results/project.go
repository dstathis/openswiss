package results

import (
	"context"
	"database/sql"
	"fmt"
)

// Project writes a tournament's projection inside the caller's transaction, so
// it commits atomically with whatever moved the tournament to 'finished'.
//
// It is idempotent. Re-running it is a no-op rather than a duplicate, which
// matters for the backfill command and for any retry: matches is guarded by
// idx_matches_unique and tournament_results by its primary key.
func Project(ctx context.Context, tx *sql.Tx, results []ResultRow, matches []MatchRow) error {
	for _, r := range results {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tournament_results (
				tournament_id, registration_id, user_id, display_name, final_rank, points,
				wins, losses, draws, game_wins, game_losses, game_draws,
				opp_match_win_pct, game_win_pct, opp_game_win_pct,
				dropped, dropped_in_round, player_count
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
			ON CONFLICT (tournament_id, registration_id) DO NOTHING`,
			r.TournamentID, r.RegistrationID, r.UserID, r.DisplayName, r.FinalRank, r.Points,
			r.Wins, r.Losses, r.Draws, r.GameWins, r.GameLosses, r.GameDraws,
			r.OppMatchWinPct, r.GameWinPct, r.OppGameWinPct,
			r.Dropped, r.DroppedInRound, r.PlayerCount,
		)
		if err != nil {
			return fmt.Errorf("insert tournament_result (tournament %d, registration %d): %w",
				r.TournamentID, r.RegistrationID, err)
		}
	}

	for _, m := range matches {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO matches (
				tournament_id, round, playoff, round_label,
				player_reg_id, player_user_id, player_name,
				opponent_reg_id, opponent_user_id, opponent_name,
				game_wins, game_losses, game_draws, outcome
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (tournament_id, playoff, round, player_reg_id) DO NOTHING`,
			m.TournamentID, m.Round, m.Playoff, m.RoundLabel,
			m.PlayerRegID, m.PlayerUserID, m.PlayerName,
			m.OpponentRegID, m.OpponentUserID, m.OpponentName,
			m.GameWins, m.GameLosses, m.GameDraws, m.Outcome,
		)
		if err != nil {
			return fmt.Errorf("insert match (tournament %d, round %d, registration %d): %w",
				m.TournamentID, m.Round, m.PlayerRegID, err)
		}
	}

	return nil
}

// Clear removes a tournament's projection so it can be rebuilt. Used by
// `openswiss backfill-results --force`; engine_state remains the source of
// truth, so this is always recoverable.
func Clear(ctx context.Context, tx *sql.Tx, tournamentID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM matches WHERE tournament_id = $1`, tournamentID); err != nil {
		return fmt.Errorf("clear matches for tournament %d: %w", tournamentID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tournament_results WHERE tournament_id = $1`, tournamentID); err != nil {
		return fmt.Errorf("clear tournament_results for tournament %d: %w", tournamentID, err)
	}
	return nil
}
