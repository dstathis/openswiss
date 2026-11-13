//go:build integration

package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/dstathis/openswiss/internal/models"
)

// statsFixture builds a finished tournament and writes projection rows
// directly, so these tests exercise the queries rather than the projection.
//
// Layout: Alice beat Bob, drew with Carol, lost to a guest, and had a bye.
type statsFixture struct {
	alice, bob, carol *models.User
	tournamentID      int64
	guestRegID        int64
}

func insertMatch(t *testing.T, database *sql.DB, tournamentID int64, round int, label string,
	playerReg int64, playerUser *int64, playerName string,
	oppReg *int64, oppUser *int64, oppName *string,
	gw, gl, gd int, outcome string) {
	t.Helper()
	_, err := database.Exec(
		`INSERT INTO matches (tournament_id, round, playoff, round_label,
			player_reg_id, player_user_id, player_name,
			opponent_reg_id, opponent_user_id, opponent_name,
			game_wins, game_losses, game_draws, outcome)
		 VALUES ($1,$2,false,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		tournamentID, round, label, playerReg, playerUser, playerName,
		oppReg, oppUser, oppName, gw, gl, gd, outcome)
	if err != nil {
		t.Fatalf("insert match: %v", err)
	}
}

func insertResult(t *testing.T, database *sql.DB, tournamentID, regID int64, userID *int64,
	name string, rank, points, w, l, d int, dropped bool) {
	t.Helper()
	_, err := database.Exec(
		`INSERT INTO tournament_results (tournament_id, registration_id, user_id, display_name,
			final_rank, points, wins, losses, draws, game_wins, game_losses, game_draws,
			opp_match_win_pct, game_win_pct, opp_game_win_pct, dropped, player_count)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,0.5,0.5,0.5,$13,4)`,
		tournamentID, regID, userID, name, rank, points, w, l, d, w*2, l*2, d, dropped)
	if err != nil {
		t.Fatalf("insert result: %v", err)
	}
}

func newStatsFixture(t *testing.T, database *sql.DB) *statsFixture {
	t.Helper()
	ctx := context.Background()

	org, err := CreateUser(ctx, database, "s-org@example.com", "SOrg", "h")
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	alice, _ := CreateUser(ctx, database, "s-alice@example.com", "Alice", "h")
	bob, _ := CreateUser(ctx, database, "s-bob@example.com", "Bob", "h")
	carol, _ := CreateUser(ctx, database, "s-carol@example.com", "Carol", "h")

	tourn := &models.Tournament{
		Name: "Stats Cup", MaxPlayers: 8, PointsWin: 3, PointsDraw: 1, PointsLoss: 0,
		Status: models.TournamentStatusFinished, OrganizerID: org.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("tournament: %v", err)
	}

	regOf := func(u *models.User) int64 {
		r, err := CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName)
		if err != nil {
			t.Fatalf("register %s: %v", u.DisplayName, err)
		}
		return r.ID
	}
	aliceReg, bobReg, carolReg := regOf(alice), regOf(bob), regOf(carol)
	guest, err := CreateGuestRegistration(ctx, database, tourn.ID, "Walkin")
	if err != nil {
		t.Fatalf("guest: %v", err)
	}

	name := func(s string) *string { return &s }
	id := func(v int64) *int64 { return &v }

	// Alice's four rows: a win, a draw, a loss to a guest, and a bye.
	insertMatch(t, database, tourn.ID, 1, "Round 1", aliceReg, &alice.ID, "Alice", id(bobReg), &bob.ID, name("Bob"), 2, 0, 0, "win")
	insertMatch(t, database, tourn.ID, 2, "Round 2", aliceReg, &alice.ID, "Alice", id(carolReg), &carol.ID, name("Carol"), 1, 1, 1, "draw")
	insertMatch(t, database, tourn.ID, 3, "Round 3", aliceReg, &alice.ID, "Alice", id(guest.ID), nil, name("Walkin"), 0, 2, 0, "loss")
	insertMatch(t, database, tourn.ID, 4, "Round 4", aliceReg, &alice.ID, "Alice", nil, nil, nil, 2, 0, 0, "bye")
	// Bob's mirrored row, so the opponent side can be checked too.
	insertMatch(t, database, tourn.ID, 1, "Round 1", bobReg, &bob.ID, "Bob", id(aliceReg), &alice.ID, name("Alice"), 0, 2, 0, "loss")

	insertResult(t, database, tourn.ID, aliceReg, &alice.ID, "Alice", 1, 7, 1, 1, 1, false)
	insertResult(t, database, tourn.ID, bobReg, &bob.ID, "Bob", 3, 0, 0, 1, 0, false)
	insertResult(t, database, tourn.ID, carolReg, &carol.ID, "Carol", 2, 1, 0, 0, 1, true)

	return &statsFixture{alice: alice, bob: bob, carol: carol, tournamentID: tourn.ID, guestRegID: guest.ID}
}

func TestGetCareerStats(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	s, err := GetCareerStats(ctx, database, f.alice.ID)
	if err != nil {
		t.Fatalf("GetCareerStats: %v", err)
	}
	if s.Events != 1 {
		t.Errorf("Events = %d, want 1", s.Events)
	}
	if s.EventsWon != 1 {
		t.Errorf("EventsWon = %d, want 1", s.EventsWon)
	}
	if s.BestFinish != 1 {
		t.Errorf("BestFinish = %d, want 1", s.BestFinish)
	}
	if s.Wins != 1 || s.Losses != 1 || s.Draws != 1 {
		t.Errorf("record = %d-%d-%d, want 1-1-1", s.Wins, s.Losses, s.Draws)
	}
	if s.Byes != 1 {
		t.Errorf("Byes = %d, want 1", s.Byes)
	}
	want := 1.0 / 3.0
	if s.MatchWinPct < want-0.0001 || s.MatchWinPct > want+0.0001 {
		t.Errorf("MatchWinPct = %f, want %f", s.MatchWinPct, want)
	}
}

// A dropped event still counts toward career totals, and still has a rank.
func TestGetCareerStatsCountsDroppedEvents(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	s, err := GetCareerStats(ctx, database, f.carol.ID)
	if err != nil {
		t.Fatalf("GetCareerStats: %v", err)
	}
	if s.Events != 1 {
		t.Errorf("a dropped event must still count: Events = %d", s.Events)
	}
	if s.EventsDropped != 1 {
		t.Errorf("EventsDropped = %d, want 1", s.EventsDropped)
	}
	if s.BestFinish != 2 {
		t.Errorf("a dropped player still has a rank: BestFinish = %d, want 2", s.BestFinish)
	}
}

func TestGetCareerStatsEmptyPlayer(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	u, err := CreateUser(ctx, database, "nobody@example.com", "Nobody", "h")
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	s, err := GetCareerStats(ctx, database, u.ID)
	if err != nil {
		t.Fatalf("GetCareerStats: %v", err)
	}
	if s.Events != 0 || s.Wins != 0 || s.MatchWinPct != 0 || s.BestFinish != 0 {
		t.Errorf("expected a zeroed record, got %+v", s)
	}
}

// The central rule: head-to-head covers registered accounts only, while the
// same guest match still appears in history and still counts in career totals.
func TestListHeadToHeadExcludesGuestsAndByes(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	h2h, err := ListHeadToHeadForUser(ctx, database, f.alice.ID)
	if err != nil {
		t.Fatalf("ListHeadToHeadForUser: %v", err)
	}
	if len(h2h) != 2 {
		t.Fatalf("expected head-to-head rows for Bob and Carol only, got %d: %+v", len(h2h), h2h)
	}
	for _, h := range h2h {
		if h.OpponentDisplayName == "Walkin" {
			t.Error("guest opponent must not appear in head-to-head")
		}
		if h.OpponentUserID == 0 {
			t.Error("head-to-head opponents always have a user id")
		}
	}

	byOpponent := map[string]models.HeadToHead{}
	for _, h := range h2h {
		byOpponent[h.OpponentDisplayName] = h
	}
	if got := byOpponent["Bob"]; got.Wins != 1 || got.Matches != 1 {
		t.Errorf("vs Bob = %+v, want 1 win in 1 match", got)
	}
	if got := byOpponent["Carol"]; got.Draws != 1 || got.Matches != 1 {
		t.Errorf("vs Carol = %+v, want 1 draw in 1 match", got)
	}

	// The guest match and the bye are still in the history and the totals.
	matches, err := ListMatchesForUser(ctx, database, f.alice.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListMatchesForUser: %v", err)
	}
	if len(matches) != 4 {
		t.Fatalf("expected 4 match rows including the guest and the bye, got %d", len(matches))
	}
	var sawGuest, sawBye bool
	for _, m := range matches {
		if m.OpponentName == "Walkin" {
			sawGuest = true
			if m.OpponentUserID != nil {
				t.Error("guest opponent should have no user id")
			}
		}
		if m.IsBye() {
			sawBye = true
		}
	}
	if !sawGuest || !sawBye {
		t.Errorf("history lost the guest (%v) or the bye (%v)", sawGuest, sawBye)
	}

	// Head-to-head deliberately does not sum to the career match total.
	var h2hTotal int
	for _, h := range h2h {
		h2hTotal += h.Matches
	}
	if h2hTotal == len(matches) {
		t.Error("expected head-to-head to exclude the guest match and the bye")
	}
}

// Head-to-head follows a rename: it links by user id and reads the current
// display name, while the match row keeps the name played under.
func TestHeadToHeadUsesCurrentDisplayName(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	if err := UpdateUserDisplayName(ctx, database, f.bob.ID, "Bobby"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	h2h, err := ListHeadToHeadForUser(ctx, database, f.alice.ID)
	if err != nil {
		t.Fatalf("ListHeadToHeadForUser: %v", err)
	}
	var found bool
	for _, h := range h2h {
		if h.OpponentUserID == f.bob.ID {
			found = true
			if h.OpponentDisplayName != "Bobby" {
				t.Errorf("expected the current name Bobby, got %q", h.OpponentDisplayName)
			}
		}
	}
	if !found {
		t.Error("renamed opponent disappeared from head-to-head")
	}

	matches, _ := ListMatchesForUser(ctx, database, f.alice.ID, 50, 0)
	for _, m := range matches {
		if m.OpponentUserID != nil && *m.OpponentUserID == f.bob.ID {
			if m.OpponentName != "Bob" {
				t.Errorf("expected the historical name Bob, got %q", m.OpponentName)
			}
			if m.OpponentDisplayName != "Bobby" {
				t.Errorf("expected the current name Bobby for linking, got %q", m.OpponentDisplayName)
			}
		}
	}
}

func TestListMatchesForUserPaginates(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	total, err := CountMatchesForUser(ctx, database, f.alice.ID)
	if err != nil {
		t.Fatalf("CountMatchesForUser: %v", err)
	}
	if total != 4 {
		t.Fatalf("expected 4 matches, got %d", total)
	}

	first, err := ListMatchesForUser(ctx, database, f.alice.ID, 2, 0)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	second, err := ListMatchesForUser(ctx, database, f.alice.ID, 2, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected 2 rows per page, got %d and %d", len(first), len(second))
	}
	if first[0].Round == second[0].Round {
		t.Error("page 2 repeated page 1")
	}
}

func TestListTournamentResultsForUser(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	res, err := ListTournamentResultsForUser(ctx, database, f.carol.ID)
	if err != nil {
		t.Fatalf("ListTournamentResultsForUser: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if !res[0].Dropped {
		t.Error("expected Carol's result to be flagged as dropped")
	}
	if res[0].FinalRank != 2 {
		t.Errorf("FinalRank = %d, want 2", res[0].FinalRank)
	}
	if res[0].TournamentName != "Stats Cup" {
		t.Errorf("TournamentName = %q", res[0].TournamentName)
	}
}

// Deleting a tournament must remove it from every player's statistics --
// including opponents', not just the deleter's.
func TestDeletingTournamentRemovesItFromStats(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	if s, _ := GetCareerStats(ctx, database, f.alice.ID); s.Events != 1 {
		t.Fatalf("precondition: expected 1 event, got %d", s.Events)
	}
	if h, _ := ListHeadToHeadForUser(ctx, database, f.bob.ID); len(h) != 1 {
		t.Fatalf("precondition: expected Bob to have 1 head-to-head row, got %d", len(h))
	}

	if err := DeleteTournament(ctx, database, f.tournamentID); err != nil {
		t.Fatalf("DeleteTournament: %v", err)
	}

	for _, u := range []*models.User{f.alice, f.bob, f.carol} {
		s, err := GetCareerStats(ctx, database, u.ID)
		if err != nil {
			t.Fatalf("career stats for %s: %v", u.DisplayName, err)
		}
		if s.Events != 0 || s.Wins != 0 || s.Losses != 0 || s.Byes != 0 {
			t.Errorf("%s still has stats after the tournament was deleted: %+v", u.DisplayName, s)
		}
		h, err := ListHeadToHeadForUser(ctx, database, u.ID)
		if err != nil {
			t.Fatalf("head-to-head for %s: %v", u.DisplayName, err)
		}
		if len(h) != 0 {
			t.Errorf("%s still has head-to-head rows after delete: %+v", u.DisplayName, h)
		}
		m, err := ListMatchesForUser(ctx, database, u.ID, 50, 0)
		if err != nil {
			t.Fatalf("matches for %s: %v", u.DisplayName, err)
		}
		if len(m) != 0 {
			t.Errorf("%s still has match rows after delete: %d", u.DisplayName, len(m))
		}
	}
}

// Deleting a user must not erase their opponents' history. The opponent keeps
// the match with the name preserved, and simply loses the head-to-head row.
func TestDeletingUserPreservesOpponentHistory(t *testing.T) {
	database := testDB(t)
	f := newStatsFixture(t, database)
	ctx := context.Background()

	if _, err := database.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, f.alice.ID); err != nil {
		t.Fatalf("delete alice: %v", err)
	}

	matches, err := ListMatchesForUser(ctx, database, f.bob.ID, 50, 0)
	if err != nil {
		t.Fatalf("ListMatchesForUser: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Bob's match must survive Alice's account deletion, got %d rows", len(matches))
	}
	if matches[0].OpponentName != "Alice" {
		t.Errorf("opponent name should be preserved, got %q", matches[0].OpponentName)
	}
	if matches[0].OpponentUserID != nil {
		t.Error("opponent user id should be nulled after the account is deleted")
	}

	h2h, err := ListHeadToHeadForUser(ctx, database, f.bob.ID)
	if err != nil {
		t.Fatalf("ListHeadToHeadForUser: %v", err)
	}
	if len(h2h) != 0 {
		t.Errorf("a deleted account has no identity to tally against, got %+v", h2h)
	}
}
