// Package results derives the normalized statistics projection from a
// tournament's swisstools engine state.
//
// All play data is stored in tournaments.engine_state as an opaque
// DumpTournament() blob, which no SQL query can reach into. When a tournament
// finishes, BuildRows converts that blob into rows for the tournament_results
// and matches tables, and Project writes them. Those tables are what the
// player statistics pages and API read.
//
// The projection is derived and disposable: engine_state stays authoritative,
// so the tables can always be rebuilt from it.
package results

import (
	"fmt"

	"github.com/dstathis/openswiss/internal/export"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

// Match outcomes, from the perspective of the row's player.
const (
	OutcomeWin  = "win"
	OutcomeLoss = "loss"
	OutcomeDraw = "draw"
	OutcomeBye  = "bye"
)

// ResultRow is one player's final placing in one tournament.
type ResultRow struct {
	TournamentID   int64
	RegistrationID int64
	UserID         *int64 // nil for guests
	DisplayName    string // name at the time of the event
	FinalRank      int
	Points         int
	Wins           int
	Losses         int
	Draws          int
	GameWins       int
	GameLosses     int
	GameDraws      int
	OppMatchWinPct float64
	GameWinPct     float64
	OppGameWinPct  float64
	Dropped        bool
	DroppedInRound *int
	PlayerCount    int
}

// MatchRow is one match from one player's perspective. Every non-bye pairing
// produces two of these, mirrored; a bye produces one.
type MatchRow struct {
	TournamentID   int64
	Round          int
	Playoff        bool
	RoundLabel     string
	PlayerRegID    int64
	PlayerUserID   *int64
	PlayerName     string
	OpponentRegID  *int64  // nil for a bye, or an opponent with no registration row
	OpponentUserID *int64  // nil for a bye or a guest
	OpponentName   *string // nil for a bye
	GameWins       int
	GameLosses     int
	GameDraws      int
	Outcome        string
}

// BuildRows derives the projection for one tournament. It is pure: it touches
// no database and mutates neither argument.
//
// Players the engine knows about but that have no registration row are
// skipped, since a row cannot be written without a registration to key on.
func BuildRows(t *models.Tournament, eng *swisstools.Tournament, regs []models.Registration) ([]ResultRow, []MatchRow, error) {
	if t == nil || eng == nil {
		return nil, nil, fmt.Errorf("results: nil tournament or engine")
	}

	byEngineID := make(map[int]models.Registration, len(regs))
	for _, r := range regs {
		if r.EnginePlayerID != nil {
			byEngineID[*r.EnginePlayerID] = r
		}
	}

	players := eng.GetPlayers()
	playerCount := eng.GetPlayerCount()

	// GetFinalStandings, not GetStandings: the latter omits dropped players
	// entirely, so they would silently vanish from their own statistics. The
	// final view ranks them in place.
	var resultRows []ResultRow
	for _, s := range eng.GetFinalStandings() {
		reg, ok := byEngineID[s.PlayerID]
		if !ok {
			continue
		}
		p := players[s.PlayerID]

		row := ResultRow{
			TournamentID:   t.ID,
			RegistrationID: reg.ID,
			UserID:         reg.UserID,
			DisplayName:    s.Name,
			FinalRank:      s.Rank,
			Points:         s.Points,
			Wins:           s.Wins,
			Losses:         s.Losses,
			Draws:          s.Draws,
			GameWins:       p.GameWins,
			GameLosses:     p.GameLosses,
			GameDraws:      p.GameDraws,
			OppMatchWinPct: s.Tiebreakers.OpponentMatchWinPct,
			GameWinPct:     s.Tiebreakers.GameWinPercentage,
			OppGameWinPct:  s.Tiebreakers.OpponentGameWinPct,
			// From the engine, not registrations.status: the player-facing
			// "Request Drop" button sets the registration status without
			// removing the player from the engine, and a request an organizer
			// never actioned is not a drop.
			Dropped:     s.Removed,
			PlayerCount: playerCount,
		}
		if s.Removed {
			round := s.RemovedInRound
			row.DroppedInRound = &round
		}
		resultRows = append(resultRows, row)
	}

	var matchRows []MatchRow

	for round := 1; round <= eng.GetCurrentRound(); round++ {
		pairings, err := eng.GetRoundByNumber(round)
		if err != nil {
			// Round not paired yet; nothing to record.
			continue
		}
		label := fmt.Sprintf("Round %d", round)
		for _, p := range pairings {
			matchRows = append(matchRows, pairingRows(t.ID, round, false, label, p, byEngineID)...)
		}
	}

	if playoff := eng.GetPlayoff(); playoff != nil {
		for i, round := range playoff.Rounds {
			// Same labels the OTR export uses, so a bracket match reads
			// identically wherever it is shown.
			label := export.PlayoffRoundName(len(playoff.Rounds), i)
			for _, p := range round {
				matchRows = append(matchRows, pairingRows(t.ID, i+1, true, label, p, byEngineID)...)
			}
		}
	}

	return resultRows, matchRows, nil
}

// pairingRows converts one pairing into its per-player rows: two mirrored rows
// for a real match, one row for a bye, none for a pairing with no result yet.
func pairingRows(tournamentID int64, round int, playoff bool, label string, p swisstools.Pairing, byEngineID map[int]models.Registration) []MatchRow {
	a, b := p.PlayerA(), p.PlayerB()
	aWins, bWins, draws := p.PlayerAWins(), p.PlayerBWins(), p.Draws()

	// A pairing that has not been played yet carries sentinel results.
	if aWins == swisstools.UNINITIALIZED_RESULT ||
		bWins == swisstools.UNINITIALIZED_RESULT ||
		draws == swisstools.UNINITIALIZED_RESULT {
		return nil
	}

	// Byes. Normally the bye sits on side B, but RemovePlayerById can leave it
	// on either, so handle both rather than trusting the layout.
	if b == swisstools.BYE_OPPONENT_ID {
		return byeRow(tournamentID, round, playoff, label, a, aWins, bWins, draws, byEngineID)
	}
	if a == swisstools.BYE_OPPONENT_ID {
		return byeRow(tournamentID, round, playoff, label, b, bWins, aWins, draws, byEngineID)
	}

	regA, okA := byEngineID[a]
	regB, okB := byEngineID[b]
	if !okA || !okB {
		// Cannot attribute the match without both registration rows.
		return nil
	}

	base := MatchRow{
		TournamentID: tournamentID,
		Round:        round,
		Playoff:      playoff,
		RoundLabel:   label,
		GameDraws:    draws,
	}

	// The outcome is derived by comparing game wins; swisstools has no result
	// enum and decides match wins the same way in UpdatePlayerStandings.
	rowA, rowB := base, base

	rowA.PlayerRegID, rowA.PlayerUserID, rowA.PlayerName = regA.ID, regA.UserID, regA.DisplayName
	rowA.OpponentRegID, rowA.OpponentUserID, rowA.OpponentName = &regB.ID, regB.UserID, &regB.DisplayName
	rowA.GameWins, rowA.GameLosses = aWins, bWins
	rowA.Outcome = outcome(aWins, bWins)

	rowB.PlayerRegID, rowB.PlayerUserID, rowB.PlayerName = regB.ID, regB.UserID, regB.DisplayName
	rowB.OpponentRegID, rowB.OpponentUserID, rowB.OpponentName = &regA.ID, regA.UserID, &regA.DisplayName
	rowB.GameWins, rowB.GameLosses = bWins, aWins
	rowB.Outcome = outcome(bWins, aWins)

	return []MatchRow{rowA, rowB}
}

func byeRow(tournamentID int64, round int, playoff bool, label string, playerID, wins, losses, draws int, byEngineID map[int]models.Registration) []MatchRow {
	reg, ok := byEngineID[playerID]
	if !ok {
		return nil
	}
	return []MatchRow{{
		TournamentID: tournamentID,
		Round:        round,
		Playoff:      playoff,
		RoundLabel:   label,
		PlayerRegID:  reg.ID,
		PlayerUserID: reg.UserID,
		PlayerName:   reg.DisplayName,
		GameWins:     wins,
		GameLosses:   losses,
		GameDraws:    draws,
		Outcome:      OutcomeBye,
	}}
}

func outcome(own, opp int) string {
	switch {
	case own > opp:
		return OutcomeWin
	case own < opp:
		return OutcomeLoss
	default:
		return OutcomeDraw
	}
}
