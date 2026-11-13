package results

import (
	"testing"

	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

func uid(n int64) *int64 { return &n }

// fixture builds a tournament with `names` players and a matching set of
// registrations. Engine player IDs are 1..n in the order the names are given,
// which is how AddPlayer assigns them.
type fixture struct {
	t    *models.Tournament
	eng  swisstools.Tournament
	regs []models.Registration
}

func newFixture(t *testing.T, names ...string) *fixture {
	t.Helper()
	f := &fixture{
		t:   &models.Tournament{ID: 42, Name: "Test Cup"},
		eng: swisstools.NewTournament(),
	}
	for i, n := range names {
		if err := f.eng.AddPlayer(n); err != nil {
			t.Fatalf("AddPlayer(%s): %v", n, err)
		}
		engineID := i + 1
		userID := uid(int64(100 + i))
		f.regs = append(f.regs, models.Registration{
			ID:             int64(900 + i),
			TournamentID:   42,
			UserID:         userID,
			DisplayName:    n,
			Status:         "confirmed",
			EnginePlayerID: &engineID,
		})
	}
	return f
}

// makeGuest converts the registration for `name` into a guest (no account),
// mirroring an organizer-added walk-in.
func (f *fixture) makeGuest(t *testing.T, name string) {
	t.Helper()
	for i := range f.regs {
		if f.regs[i].DisplayName == name {
			f.regs[i].UserID = nil
			f.regs[i].GuestName = &name
			return
		}
	}
	t.Fatalf("no registration named %s", name)
}

func (f *fixture) regIDFor(t *testing.T, name string) int64 {
	t.Helper()
	for _, r := range f.regs {
		if r.DisplayName == name {
			return r.ID
		}
	}
	t.Fatalf("no registration named %s", name)
	return 0
}

// enterResults gives every player A a 2-0 win over player B, mirroring what
// the app's SubmitResults handler does. It does not accumulate standings:
// NextRound and FinishTournament each call UpdatePlayerStandings themselves,
// and the app relies on that rather than calling it directly.
func (f *fixture) enterResults(t *testing.T) {
	t.Helper()
	for _, p := range f.eng.GetRound() {
		if p.PlayerB() != swisstools.BYE_OPPONENT_ID {
			if err := f.eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
				t.Fatalf("AddResult: %v", err)
			}
		}
	}
}

// playRound enters a round and accumulates it, for tests that need standings
// without advancing to the next round.
func (f *fixture) playRound(t *testing.T) {
	t.Helper()
	f.enterResults(t)
	if err := f.eng.UpdatePlayerStandings(); err != nil {
		t.Fatalf("UpdatePlayerStandings: %v", err)
	}
}

// advanceRound mirrors the app's NextRound handler: NextRound (which
// accumulates) then re-pair.
func (f *fixture) advanceRound(t *testing.T) {
	t.Helper()
	if err := f.eng.NextRound(); err != nil {
		t.Fatalf("NextRound: %v", err)
	}
	if err := f.eng.Pair(false); err != nil {
		t.Fatalf("Pair: %v", err)
	}
}

func (f *fixture) build(t *testing.T) ([]ResultRow, []MatchRow) {
	t.Helper()
	res, matches, err := BuildRows(f.t, &f.eng, f.regs)
	if err != nil {
		t.Fatalf("BuildRows: %v", err)
	}
	return res, matches
}

func TestBuildRowsMirrorsEveryMatchAndRecordsByeOnce(t *testing.T) {
	// Odd player count guarantees exactly one bye.
	f := newFixture(t, "Alice", "Bob", "Charlie", "Diana", "Eve")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.playRound(t)

	_, matches := f.build(t)

	var byes int
	byPlayer := make(map[int64]MatchRow)
	for _, m := range matches {
		if m.Outcome == OutcomeBye {
			byes++
			if m.OpponentRegID != nil || m.OpponentUserID != nil || m.OpponentName != nil {
				t.Errorf("bye row should have no opponent, got %+v", m)
			}
			continue
		}
		byPlayer[m.PlayerRegID] = m
	}

	if byes != 1 {
		t.Errorf("expected exactly 1 bye row, got %d", byes)
	}
	// Two real pairings for 5 players (one sits out), mirrored, plus the bye.
	if len(matches) != 5 {
		t.Fatalf("expected 5 match rows (2 pairings mirrored + 1 bye), got %d", len(matches))
	}

	// Every non-bye row must have a mirror that agrees on the game score.
	for regID, m := range byPlayer {
		mirror, ok := byPlayer[*m.OpponentRegID]
		if !ok {
			t.Fatalf("row for reg %d has no mirrored row for its opponent", regID)
		}
		if mirror.GameWins != m.GameLosses || mirror.GameLosses != m.GameWins {
			t.Errorf("mirror disagrees: %+v vs %+v", m, mirror)
		}
		if m.GameDraws != mirror.GameDraws {
			t.Errorf("draws differ between mirrored rows: %d vs %d", m.GameDraws, mirror.GameDraws)
		}
		if m.Outcome == OutcomeWin && mirror.Outcome != OutcomeLoss {
			t.Errorf("expected mirrored loss for a win, got %q", mirror.Outcome)
		}
		if m.RoundLabel != "Round 1" || m.Playoff {
			t.Errorf("unexpected round metadata: %+v", m)
		}
	}
}

func TestBuildRowsRecordsDraws(t *testing.T) {
	f := newFixture(t, "Alice", "Bob")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	p := f.eng.GetRound()[0]
	// 1 win, 1 loss, 1 draw on both sides is a drawn match.
	if err := f.eng.AddResult(p.PlayerA(), 1, 1, 1); err != nil {
		t.Fatalf("AddResult: %v", err)
	}
	if err := f.eng.UpdatePlayerStandings(); err != nil {
		t.Fatalf("UpdatePlayerStandings: %v", err)
	}

	_, matches := f.build(t)
	if len(matches) != 2 {
		t.Fatalf("expected 2 mirrored rows, got %d", len(matches))
	}
	for _, m := range matches {
		if m.Outcome != OutcomeDraw {
			t.Errorf("expected a draw on both sides, got %q", m.Outcome)
		}
		if m.GameDraws != 1 {
			t.Errorf("expected 1 game draw, got %d", m.GameDraws)
		}
	}
}

func TestBuildRowsSkipsUnplayedPairings(t *testing.T) {
	f := newFixture(t, "Alice", "Bob")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	// No results entered at all.
	_, matches := f.build(t)
	if len(matches) != 0 {
		t.Fatalf("expected no rows for an unplayed round, got %d: %+v", len(matches), matches)
	}
}

func TestBuildRowsDroppedPlayerKeepsRankAndPreDropMatches(t *testing.T) {
	f := newFixture(t, "Alice", "Bob", "Charlie", "Diana")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.enterResults(t)

	// Advance to round 2 before dropping. RemovePlayerById rewrites only the
	// *current* round, so dropping during round 2 leaves round 1 intact --
	// which is how a real drop happens.
	f.advanceRound(t)

	// Drop the leader, who is on a win.
	leader := f.eng.GetStandings()[0]
	if err := f.eng.RemovePlayerById(leader.PlayerID); err != nil {
		t.Fatalf("RemovePlayerById: %v", err)
	}

	res, matches := f.build(t)

	if len(res) != 4 {
		t.Fatalf("expected all 4 players in the results, got %d", len(res))
	}

	var dropped *ResultRow
	for i := range res {
		if res[i].DisplayName == leader.Name {
			dropped = &res[i]
		}
	}
	if dropped == nil {
		t.Fatal("dropped player missing from results; BuildRows must use GetFinalStandings")
	}
	if !dropped.Dropped {
		t.Error("expected Dropped to be true")
	}
	if dropped.DroppedInRound == nil || *dropped.DroppedInRound != 2 {
		t.Errorf("expected DroppedInRound 2, got %v", dropped.DroppedInRound)
	}
	if dropped.FinalRank < 1 {
		t.Errorf("a dropped player must still get a rank, got %d", dropped.FinalRank)
	}
	// They were winning, so they must not be ranked below the players on zero.
	for _, r := range res {
		if r.Points < dropped.Points && r.FinalRank <= dropped.FinalRank {
			t.Errorf("dropped player rank %d should beat %s rank %d",
				dropped.FinalRank, r.DisplayName, r.FinalRank)
		}
	}

	// Their pre-drop match survives, and no match exists after the drop.
	droppedRegID := f.regIDFor(t, leader.Name)
	var seen int
	for _, m := range matches {
		if m.PlayerRegID == droppedRegID {
			seen++
			if m.Round != 1 {
				t.Errorf("dropped player has a match in round %d, after dropping in round 2", m.Round)
			}
		}
	}
	if seen == 0 {
		t.Error("dropped player lost their pre-drop match history")
	}
}

// A player who clicks "Request Drop" has registrations.status = 'dropped' but
// is untouched in the engine. That is a request, not a drop, and must not be
// recorded as one.
func TestBuildRowsIgnoresRegistrationStatusForDropped(t *testing.T) {
	f := newFixture(t, "Alice", "Bob")
	for i := range f.regs {
		f.regs[i].Status = models.RegistrationStatusDropped
	}
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.playRound(t)

	res, _ := f.build(t)
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	for _, r := range res {
		if r.Dropped {
			t.Errorf("%s was never removed from the engine and must not count as dropped", r.DisplayName)
		}
		if r.DroppedInRound != nil {
			t.Errorf("%s: expected nil DroppedInRound, got %v", r.DisplayName, *r.DroppedInRound)
		}
	}
}

func TestBuildRowsGuestOpponentHasNoUserID(t *testing.T) {
	f := newFixture(t, "Alice", "Bob")
	f.makeGuest(t, "Bob")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.playRound(t)

	res, matches := f.build(t)

	bobRegID := f.regIDFor(t, "Bob")
	for _, m := range matches {
		switch m.PlayerRegID {
		case bobRegID:
			if m.PlayerUserID != nil {
				t.Errorf("guest row should have a nil PlayerUserID, got %d", *m.PlayerUserID)
			}
		default:
			if m.OpponentUserID != nil {
				t.Errorf("Alice's opponent is a guest; OpponentUserID should be nil, got %d", *m.OpponentUserID)
			}
			// The guest's name is still preserved for display.
			if m.OpponentName == nil || *m.OpponentName != "Bob" {
				t.Errorf("guest opponent name not preserved: %v", m.OpponentName)
			}
		}
	}
	for _, r := range res {
		if r.DisplayName == "Bob" && r.UserID != nil {
			t.Errorf("guest result row should have nil UserID, got %d", *r.UserID)
		}
	}
}

func TestBuildRowsIncludesPlayoffMatches(t *testing.T) {
	f := newFixture(t, "Alice", "Bob", "Charlie", "Diana")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.enterResults(t)
	if err := f.eng.FinishTournament(); err != nil {
		t.Fatalf("FinishTournament: %v", err)
	}
	if err := f.eng.StartPlayoff(4); err != nil {
		t.Fatalf("StartPlayoff: %v", err)
	}
	for _, p := range f.eng.GetPlayoffRound() {
		if err := f.eng.AddPlayoffResult(p.PlayerA(), 2, 0, 0); err != nil {
			t.Fatalf("AddPlayoffResult: %v", err)
		}
	}

	_, matches := f.build(t)

	var playoffRows int
	for _, m := range matches {
		if !m.Playoff {
			continue
		}
		playoffRows++
		if m.RoundLabel != "Semifinals" {
			t.Errorf("expected the first round of a top 4 to be Semifinals, got %q", m.RoundLabel)
		}
	}
	// Two semifinal matches, mirrored.
	if playoffRows != 4 {
		t.Errorf("expected 4 playoff rows (2 matches mirrored), got %d", playoffRows)
	}
}

func TestBuildRowsSkipsPlayersWithoutRegistrations(t *testing.T) {
	f := newFixture(t, "Alice", "Bob")
	// Drop Bob's registration entirely; his rows cannot be keyed.
	f.regs = f.regs[:1]
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.playRound(t)

	res, matches := f.build(t)
	if len(res) != 1 {
		t.Errorf("expected only the registered player in results, got %d", len(res))
	}
	if len(matches) != 0 {
		t.Errorf("a match needs both registrations to be attributable, got %d rows", len(matches))
	}
}

func TestBuildRowsRejectsNilArguments(t *testing.T) {
	if _, _, err := BuildRows(nil, nil, nil); err == nil {
		t.Error("expected an error for nil arguments")
	}
}

// Dropping a player rewrites their current-round pairing into a bye for the
// opponent, discarding whatever was there. This is swisstools behavior, not
// something the projection can undo: the original pairing is gone from
// engine_state, so a bye caused by a drop is indistinguishable from a real
// one. Pinned here so the limitation is visible rather than surprising.
func TestBuildRowsDropTurnsOpponentsCurrentRoundIntoABye(t *testing.T) {
	f := newFixture(t, "Alice", "Bob", "Charlie", "Diana")
	if err := f.eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	f.enterResults(t)
	f.advanceRound(t)

	// Find a round-2 pairing and note both sides before the drop.
	round2 := f.eng.GetRound()
	if len(round2) == 0 {
		t.Fatal("expected round 2 pairings")
	}
	victim := round2[0].PlayerA()
	opponent := round2[0].PlayerB()
	if opponent == swisstools.BYE_OPPONENT_ID {
		t.Skip("first pairing is a bye; nothing to observe")
	}

	if err := f.eng.RemovePlayerById(victim); err != nil {
		t.Fatalf("RemovePlayerById: %v", err)
	}

	_, matches := f.build(t)

	var opponentReg int64
	for _, r := range f.regs {
		if r.EnginePlayerID != nil && *r.EnginePlayerID == opponent {
			opponentReg = r.ID
		}
	}

	var found bool
	for _, m := range matches {
		if m.PlayerRegID == opponentReg && m.Round == 2 && !m.Playoff {
			found = true
			if m.Outcome != OutcomeBye {
				t.Errorf("expected the abandoned opponent to receive a bye, got %q", m.Outcome)
			}
			if m.OpponentName != nil {
				t.Errorf("a bye has no opponent, but got %q -- the drop should have erased it", *m.OpponentName)
			}
		}
	}
	if !found {
		t.Error("expected a round 2 row for the abandoned opponent")
	}
}
