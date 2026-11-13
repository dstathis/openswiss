package db

import (
	"context"
	"database/sql"

	"github.com/dstathis/openswiss/internal/models"
)

// Every statistics query joins tournaments, including the aggregates that
// could read the projection tables directly. The join is on the primary key so
// it costs nothing, and it is the single seam where a future soft delete
// becomes one extra predicate instead of a silent correctness bug -- deleted
// tournaments would otherwise keep counting toward career totals.
const (
	statsResultsFrom = `FROM tournament_results r
		JOIN tournaments t ON t.id = r.tournament_id`
	statsMatchesFrom = `FROM matches m
		JOIN tournaments t ON t.id = m.tournament_id`
)

// GetCareerStats aggregates a player's record across every finished
// tournament. Dropped events count: the rounds they played really happened.
func GetCareerStats(ctx context.Context, database *sql.DB, userID int64) (*models.CareerStats, error) {
	var s models.CareerStats
	var bestFinish sql.NullInt64

	err := database.QueryRowContext(ctx,
		`SELECT
			count(*),
			count(*) FILTER (WHERE r.final_rank = 1),
			count(*) FILTER (WHERE r.dropped),
			min(r.final_rank),
			coalesce(sum(r.wins), 0),
			coalesce(sum(r.losses), 0),
			coalesce(sum(r.draws), 0),
			coalesce(sum(r.game_wins), 0),
			coalesce(sum(r.game_losses), 0),
			coalesce(sum(r.game_draws), 0)
		 `+statsResultsFrom+`
		 WHERE r.user_id = $1`,
		userID,
	).Scan(&s.Events, &s.EventsWon, &s.EventsDropped, &bestFinish,
		&s.Wins, &s.Losses, &s.Draws, &s.GameWins, &s.GameLosses, &s.GameDraws)
	if err != nil {
		return nil, err
	}
	if bestFinish.Valid {
		s.BestFinish = int(bestFinish.Int64)
	}

	// Byes live on the match rows, not the result rows.
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) `+statsMatchesFrom+`
		 WHERE m.player_user_id = $1 AND m.outcome = 'bye'`,
		userID,
	).Scan(&s.Byes); err != nil {
		return nil, err
	}

	// Percentages are computed here rather than in SQL so the divide-by-zero
	// guard is explicit for a player with no matches yet.
	if total := s.Wins + s.Losses + s.Draws; total > 0 {
		s.MatchWinPct = float64(s.Wins) / float64(total)
	}
	if total := s.GameWins + s.GameLosses + s.GameDraws; total > 0 {
		s.GameWinPct = float64(s.GameWins) / float64(total)
	}

	return &s, nil
}

// ListTournamentResultsForUser returns a player's finishes, newest first.
func ListTournamentResultsForUser(ctx context.Context, database *sql.DB, userID int64) ([]models.TournamentResult, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT t.id, t.name, t.scheduled_at, r.final_rank, r.player_count, r.points,
		        r.wins, r.losses, r.draws, r.game_wins, r.game_losses, r.game_draws,
		        r.opp_match_win_pct, r.game_win_pct, r.opp_game_win_pct,
		        r.dropped, r.dropped_in_round
		 `+statsResultsFrom+`
		 WHERE r.user_id = $1
		 ORDER BY t.scheduled_at DESC NULLS LAST, t.id DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.TournamentResult
	for rows.Next() {
		var tr models.TournamentResult
		if err := rows.Scan(&tr.TournamentID, &tr.TournamentName, &tr.ScheduledAt,
			&tr.FinalRank, &tr.PlayerCount, &tr.Points,
			&tr.Wins, &tr.Losses, &tr.Draws, &tr.GameWins, &tr.GameLosses, &tr.GameDraws,
			&tr.OppMatchWinPct, &tr.GameWinPct, &tr.OppGameWinPct,
			&tr.Dropped, &tr.DroppedInRound); err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}

// CountMatchesForUser returns how many match rows a player has, for paging.
func CountMatchesForUser(ctx context.Context, database *sql.DB, userID int64) (int, error) {
	var n int
	err := database.QueryRowContext(ctx,
		`SELECT count(*) `+statsMatchesFrom+` WHERE m.player_user_id = $1`,
		userID,
	).Scan(&n)
	return n, err
}

// ListMatchesForUser returns a page of a player's match history, newest first.
//
// The users join supplies the opponent's *current* display name for linking,
// while matches.opponent_name preserves the name they played under. It is a
// LEFT join because an opponent may be a guest, a bye, or a deleted account.
func ListMatchesForUser(ctx context.Context, database *sql.DB, userID int64, limit, offset int) ([]models.PlayerMatch, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT t.id, t.name, m.round, m.playoff, m.round_label, m.outcome,
		        m.game_wins, m.game_losses, m.game_draws,
		        m.opponent_user_id, coalesce(m.opponent_name, ''), coalesce(u.display_name, '')
		 `+statsMatchesFrom+`
		 LEFT JOIN users u ON u.id = m.opponent_user_id
		 WHERE m.player_user_id = $1
		 ORDER BY t.scheduled_at DESC NULLS LAST, t.id DESC, m.playoff, m.round
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PlayerMatch
	for rows.Next() {
		var m models.PlayerMatch
		if err := rows.Scan(&m.TournamentID, &m.TournamentName, &m.Round, &m.Playoff,
			&m.RoundLabel, &m.Outcome, &m.GameWins, &m.GameLosses, &m.GameDraws,
			&m.OpponentUserID, &m.OpponentName, &m.OpponentDisplayName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListHeadToHeadForUser returns a player's record against each opponent who
// has a user account, most-played first.
//
// opponent_user_id IS NOT NULL excludes byes and guests in one condition.
// Guests are excluded deliberately: they have no identity that is stable
// across tournaments, so two unrelated walk-ins sharing a name would otherwise
// merge into one bogus record. The consequence is that these counts do not sum
// to the career match total, which the UI states explicitly.
func ListHeadToHeadForUser(ctx context.Context, database *sql.DB, userID int64) ([]models.HeadToHead, error) {
	rows, err := database.QueryContext(ctx,
		`SELECT m.opponent_user_id, coalesce(u.display_name, max(m.opponent_name)),
		        count(*) FILTER (WHERE m.outcome = 'win'),
		        count(*) FILTER (WHERE m.outcome = 'loss'),
		        count(*) FILTER (WHERE m.outcome = 'draw'),
		        count(*)
		 `+statsMatchesFrom+`
		 LEFT JOIN users u ON u.id = m.opponent_user_id
		 WHERE m.player_user_id = $1 AND m.opponent_user_id IS NOT NULL
		 GROUP BY m.opponent_user_id, u.display_name
		 ORDER BY count(*) DESC, coalesce(u.display_name, max(m.opponent_name))`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.HeadToHead
	for rows.Next() {
		var h models.HeadToHead
		if err := rows.Scan(&h.OpponentUserID, &h.OpponentDisplayName,
			&h.Wins, &h.Losses, &h.Draws, &h.Matches); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
