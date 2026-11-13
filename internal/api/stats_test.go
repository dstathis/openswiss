//go:build integration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

// finishedTournamentWithGuest builds and finishes a 3-user + 1-guest
// tournament so the API can be checked against real projected rows.
func finishedTournamentWithGuest(t *testing.T, database *sql.DB) []*models.User {
	t.Helper()
	ctx := context.Background()

	owner := mustCreateUser(t, database, "sowner-"+t.Name()+"@example.com", "SOwner-"+t.Name())
	rounds := 1
	tourn := &models.Tournament{
		Name: "API Stats " + t.Name(), MaxPlayers: 16, NumRounds: &rounds,
		PointsWin: 3, PointsDraw: 1, PointsLoss: 0,
		Status: models.TournamentStatusRegistrationOpen, OrganizerID: owner.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}

	var users []*models.User
	for i := 0; i < 3; i++ {
		u := mustCreateUser(t, database,
			string(rune('a'+i))+"-stats-"+t.Name()+"@example.com",
			string(rune('A'+i))+"-Stats-"+t.Name())
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register: %v", err)
		}
		users = append(users, u)
	}
	if _, err := db.CreateGuestRegistration(ctx, database, tourn.ID, "Walkin-"+t.Name()); err != nil {
		t.Fatalf("guest: %v", err)
	}

	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if err := engine.WithTournamentEngine(ctx, database, tourn.ID,
		func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
			state, err := engine.InitTournamentEngine(ctx, tx, tm, regs)
			if err != nil {
				return "", err
			}
			newEng, err := swisstools.LoadTournament(state)
			if err != nil {
				return "", err
			}
			*eng = newEng
			for _, p := range eng.GetRound() {
				if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
					continue
				}
				if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
					return "", err
				}
			}
			return models.TournamentStatusInProgress, nil
		}); err != nil {
		t.Fatalf("init engine: %v", err)
	}

	if err := engine.WithTournamentEngine(ctx, database, tourn.ID,
		func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
			// FinishTournament accumulates standings itself.
			if err := eng.FinishTournament(); err != nil {
				return "", err
			}
			return models.TournamentStatusFinished, nil
		}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	return users
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, into interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

func TestStatsAPI_GetStats(t *testing.T) {
	database := testDB(t)
	users := finishedTournamentWithGuest(t, database)
	api := &StatsAPI{DB: database}

	rec := httptest.NewRecorder()
	api.GetStats(rec, requestWithUser(http.MethodGet, "/", "", nil,
		map[string]string{"name": users[0].DisplayName}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got statsResponse
	decode(t, rec, &got)

	if got.Player.DisplayName != users[0].DisplayName {
		t.Errorf("player = %+v", got.Player)
	}
	if got.Career.Events != 1 {
		t.Errorf("Events = %d, want 1", got.Career.Events)
	}
	if len(got.Tournaments) != 1 {
		t.Fatalf("expected 1 tournament, got %d", len(got.Tournaments))
	}
	if got.Tournaments[0].FinalRank < 1 {
		t.Errorf("every player must have a rank, got %d", got.Tournaments[0].FinalRank)
	}
}

func TestStatsAPI_UnknownPlayerIs404(t *testing.T) {
	database := testDB(t)
	api := &StatsAPI{DB: database}

	for name, fn := range map[string]http.HandlerFunc{
		"stats":        api.GetStats,
		"matches":      api.ListMatches,
		"head-to-head": api.ListHeadToHead,
	} {
		rec := httptest.NewRecorder()
		fn(rec, requestWithUser(http.MethodGet, "/", "", nil, map[string]string{"name": "no-such-player"}))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rec.Code)
		}
		var body map[string]string
		decode(t, rec, &body)
		if body["error"] != "player not found" {
			t.Errorf("%s: error = %q", name, body["error"])
		}
	}
}

// A player with no finished events must serialize empty arrays, not null, so
// clients can iterate without a nil check.
func TestStatsAPI_EmptyPlayerSerializesArrays(t *testing.T) {
	database := testDB(t)
	u := mustCreateUser(t, database, "empty-"+t.Name()+"@example.com", "Empty-"+t.Name())
	api := &StatsAPI{DB: database}

	rec := httptest.NewRecorder()
	api.GetStats(rec, requestWithUser(http.MethodGet, "/", "", nil, map[string]string{"name": u.DisplayName}))
	if body := rec.Body.String(); !strings.Contains(body, `"tournaments":[]`) {
		t.Errorf("expected an empty array, got %s", body)
	}

	rec = httptest.NewRecorder()
	api.ListMatches(rec, requestWithUser(http.MethodGet, "/", "", nil, map[string]string{"name": u.DisplayName}))
	if body := rec.Body.String(); !strings.Contains(body, `"matches":[]`) {
		t.Errorf("expected an empty array, got %s", body)
	}

	rec = httptest.NewRecorder()
	api.ListHeadToHead(rec, requestWithUser(http.MethodGet, "/", "", nil, map[string]string{"name": u.DisplayName}))
	if body := rec.Body.String(); !strings.Contains(body, `"head_to_head":[]`) {
		t.Errorf("expected an empty array, got %s", body)
	}
}

// A bye serializes a null opponent; a guest serializes a null user_id but
// keeps the name. Head-to-head excludes both.
func TestStatsAPI_ByesAndGuestsSerializeCorrectly(t *testing.T) {
	database := testDB(t)
	users := finishedTournamentWithGuest(t, database)
	api := &StatsAPI{DB: database}

	var sawBye, sawGuest bool
	for _, u := range users {
		rec := httptest.NewRecorder()
		api.ListMatches(rec, requestWithUser(http.MethodGet, "/", "", nil,
			map[string]string{"name": u.DisplayName}))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		var got matchesResponse
		decode(t, rec, &got)

		for _, m := range got.Matches {
			if m.Outcome == "bye" {
				sawBye = true
				if m.Opponent != nil {
					t.Errorf("a bye must serialize a null opponent, got %+v", m.Opponent)
				}
				continue
			}
			if m.Opponent == nil {
				t.Errorf("a played match must have an opponent: %+v", m)
				continue
			}
			if m.Opponent.UserID == nil {
				sawGuest = true
				if m.Opponent.NameAtEvent == "" {
					t.Error("a guest opponent must still carry a name")
				}
			}
		}

		// Head-to-head never contains a null opponent id.
		rec = httptest.NewRecorder()
		api.ListHeadToHead(rec, requestWithUser(http.MethodGet, "/", "", nil,
			map[string]string{"name": u.DisplayName}))
		var h2h headToHeadResponse
		decode(t, rec, &h2h)
		for _, e := range h2h.HeadToHead {
			if e.Opponent.ID == 0 {
				t.Errorf("head-to-head must exclude guests and byes, got %+v", e)
			}
		}
	}

	if !sawGuest {
		t.Error("expected at least one guest opponent in the fixture")
	}
	_ = sawBye // a bye only occurs with an odd field; not guaranteed here
}

func TestStatsAPI_ListMatchesPaginates(t *testing.T) {
	database := testDB(t)
	users := finishedTournamentWithGuest(t, database)
	api := &StatsAPI{DB: database}

	rec := httptest.NewRecorder()
	api.ListMatches(rec, requestWithUser(http.MethodGet, "/?page=1&per_page=1", "", nil,
		map[string]string{"name": users[0].DisplayName}))
	var got matchesResponse
	decode(t, rec, &got)

	if got.PerPage != 1 || got.Page != 1 {
		t.Errorf("paging echoed wrong: page=%d per_page=%d", got.Page, got.PerPage)
	}
	if got.Total < 1 {
		t.Errorf("total = %d, want at least 1", got.Total)
	}
	if len(got.Matches) > 1 {
		t.Errorf("per_page=1 returned %d rows", len(got.Matches))
	}
}

// /users/me/stats must agree with /players/{own name}/stats.
func TestStatsAPI_GetMyStatsMatchesPublicPage(t *testing.T) {
	database := testDB(t)
	users := finishedTournamentWithGuest(t, database)
	api := &StatsAPI{DB: database}
	me := users[0]

	pub := httptest.NewRecorder()
	api.GetStats(pub, requestWithUser(http.MethodGet, "/", "", nil,
		map[string]string{"name": me.DisplayName}))

	mine := httptest.NewRecorder()
	api.GetMyStats(mine, requestWithUser(http.MethodGet, "/", "", me, nil))

	if mine.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", mine.Code, mine.Body.String())
	}
	if pub.Body.String() != mine.Body.String() {
		t.Errorf("payloads differ:\npublic: %s\nme:     %s", pub.Body.String(), mine.Body.String())
	}
}

// An unfinished tournament contributes nothing: only finished events are
// projected.
func TestStatsAPI_InProgressTournamentIsAbsent(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	ctx := context.Background()

	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if len(regs) == 0 {
		t.Fatal("expected registrations")
	}
	user, err := db.GetUserByID(ctx, database, *regs[0].UserID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	api := &StatsAPI{DB: database}
	rec := httptest.NewRecorder()
	api.GetStats(rec, requestWithUser(http.MethodGet, "/", "", nil,
		map[string]string{"name": user.DisplayName}))

	var got statsResponse
	decode(t, rec, &got)
	if got.Career.Events != 0 {
		t.Errorf("an in-progress tournament must not appear in stats, got %d events", got.Career.Events)
	}
	if len(got.Tournaments) != 0 {
		t.Errorf("expected no tournaments, got %d", len(got.Tournaments))
	}
}
