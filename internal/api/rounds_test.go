//go:build integration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

// freshStarted returns a tournament that has just been started but has NO
// results submitted yet (round 1 pairings are open).
func freshStarted(t *testing.T, database *sql.DB) (*models.User, *models.Tournament) {
	t.Helper()
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner-fs-"+t.Name()+"@example.com", "Owner-fs-"+t.Name())
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "Fresh " + t.Name(),
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: owner.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	for i := 0; i < 4; i++ {
		u := mustCreateUser(t, database, "fp"+strconv.Itoa(i)+"-"+t.Name()+"@example.com", "FP"+strconv.Itoa(i)+"-"+t.Name())
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if err := engine.WithTournamentEngine(ctx, database, tourn.ID,
		func(tx *sql.Tx, tm *models.Tournament, eng *swisstools.Tournament) (string, error) {
			state, err := engine.InitTournamentEngine(ctx, tx, tm, regs)
			if err != nil {
				return "", err
			}
			ne, err := swisstools.LoadTournament(state)
			if err != nil {
				return "", err
			}
			*eng = ne
			return models.TournamentStatusInProgress, nil
		}); err != nil {
		t.Fatalf("init engine: %v", err)
	}
	tourn, _ = db.GetTournament(ctx, database, tourn.ID)
	return owner, tourn
}

func TestRoundsAPI_ListRounds_NotStarted(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.ListRounds(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Errorf("expected empty array for not-yet-started tournament, got %q", rec.Body.String())
	}
}

func TestRoundsAPI_ListRounds_NotFound(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.ListRounds(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRoundsAPI_ListRounds_Started(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.ListRounds(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var rounds []map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&rounds)
	if len(rounds) != 1 {
		t.Errorf("expected 1 round, got %d", len(rounds))
	}
}

func TestRoundsAPI_GetCurrentRound_Started(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["round_number"] == nil {
		t.Error("expected round_number in response")
	}
}

func TestRoundsAPI_GetCurrentRound_NotStarted(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRoundsAPI_GetCurrentRound_NotFound(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRoundsAPI_GetRound(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"round": "1",
	})
	rec := httptest.NewRecorder()
	api.GetRound(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRoundsAPI_GetRound_Invalid(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"round": "99",
	})
	rec := httptest.NewRecorder()
	api.GetRound(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid round, got %d", rec.Code)
	}
}

func TestRoundsAPI_GetRound_NotStarted(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/", "", nil, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"round": "1",
	})
	rec := httptest.NewRecorder()
	api.GetRound(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRoundsAPI_GetRound_NotFound(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999", "round": "1"})
	rec := httptest.NewRecorder()
	api.GetRound(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestRoundsAPI_SubmitResults(t *testing.T) {
	database := testDB(t)
	owner, tourn := freshStarted(t, database)
	api := &RoundsAPI{DB: database}

	// Build a result batch from the actual round-1 pairings
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	tourn, _ = db.GetTournament(context.Background(), database, tourn.ID)
	eng, _ := swisstools.LoadTournament(tourn.EngineState)
	pairings := eng.GetRound()
	type entry struct {
		PlayerID int `json:"player_id"`
		Wins     int `json:"wins"`
		Losses   int `json:"losses"`
		Draws    int `json:"draws"`
	}
	var results []entry
	for _, p := range pairings {
		if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
			continue
		}
		results = append(results, entry{PlayerID: p.PlayerA(), Wins: 2, Losses: 0, Draws: 0})
	}
	body, _ := json.Marshal(map[string]interface{}{"results": results})

	r := requestWithUser("POST", "/", string(body), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitResults(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	_ = regs
}

func TestRoundsAPI_SubmitResults_BadJSON(t *testing.T) {
	database := testDB(t)
	owner, tourn := freshStarted(t, database)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("POST", "/", "not-json", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitResults(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRoundsAPI_SubmitResults_Forbidden(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	other := mustCreateUser(t, database, "other-rounds@example.com", "OtherRounds")
	api := &RoundsAPI{DB: database}

	r := requestWithUser("POST", "/", `{"results":[]}`, other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitResults(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRoundsAPI_NextRound(t *testing.T) {
	database := testDB(t)
	owner, tourn := startedTournament(t, database) // round 1 results already in
	api := &RoundsAPI{DB: database}

	r := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.NextRound(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoundsAPI_NextRound_Forbidden(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-next@example.com", "OtherNext")
	api := &RoundsAPI{DB: database}

	r := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.NextRound(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRoundsAPI_GetStandings_NotStarted(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetStandings(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Errorf("expected empty array for not-started, got %q", rec.Body.String())
	}
}

func TestRoundsAPI_GetStandings_Started(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetStandings(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var standings []interface{}
	json.NewDecoder(rec.Body).Decode(&standings)
	if len(standings) == 0 {
		t.Error("expected non-empty standings")
	}
}

func TestRoundsAPI_GetStandings_NotFound(t *testing.T) {
	database := testDB(t)
	api := &RoundsAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.GetStandings(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestFormatPairings_BasicAndBye(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	for _, n := range []string{"A", "B", "C"} {
		eng.AddPlayer(n)
	}
	if err := eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}
	resp := formatPairings(&eng, eng.GetRound())
	if len(resp) == 0 {
		t.Fatal("expected at least one pairing")
	}
	var sawBye bool
	for _, p := range resp {
		if p.IsBye {
			sawBye = true
		}
	}
	if !sawBye {
		t.Error("expected a bye in 3-player tournament")
	}
}

// TestFormatPairings_ResolvesNames is the regression test for the bug where
// pairings rendered raw player IDs instead of names: every non-bye player in a
// pairing must carry the name it was registered under, and the numeric ID must
// still be present alongside it.
func TestFormatPairings_ResolvesNames(t *testing.T) {
	eng := swisstools.NewTournamentWithConfig(swisstools.TournamentConfig{
		PointsForWin:  3,
		PointsForDraw: 1,
		PointsForLoss: 0,
		ByeWins:       swisstools.BYE_WINS,
		ByeLosses:     swisstools.BYE_LOSSES,
		ByeDraws:      swisstools.BYE_DRAWS,
	})
	names := []string{"Alice", "Bob", "Carol", "Dave"}
	for _, n := range names {
		eng.AddPlayer(n)
	}
	if err := eng.StartTournament(); err != nil {
		t.Fatalf("StartTournament: %v", err)
	}

	resp := formatPairings(&eng, eng.GetRound())
	if len(resp) == 0 {
		t.Fatal("expected at least one pairing")
	}
	for i, p := range resp {
		// PlayerA is always a real player; its name must resolve and must not
		// be the stringified ID.
		nameA, ok := nameOf(&eng, p.PlayerA)
		if !ok {
			t.Fatalf("pairing %d: PlayerA id %d not found in engine", i, p.PlayerA)
		}
		if p.PlayerAName != nameA {
			t.Errorf("pairing %d: PlayerAName = %q, want %q (id %d)", i, p.PlayerAName, nameA, p.PlayerA)
		}
		if p.PlayerAName == "" {
			t.Errorf("pairing %d: PlayerAName is empty (would render as a blank/ID cell)", i)
		}
		// PlayerB carries a name only when it is a real opponent (not a bye).
		if p.IsBye {
			continue
		}
		nameB, ok := nameOf(&eng, p.PlayerB)
		if !ok {
			t.Fatalf("pairing %d: PlayerB id %d not found in engine", i, p.PlayerB)
		}
		if p.PlayerBName != nameB {
			t.Errorf("pairing %d: PlayerBName = %q, want %q (id %d)", i, p.PlayerBName, nameB, p.PlayerB)
		}
	}
}

func nameOf(eng *swisstools.Tournament, id int) (string, bool) {
	player, ok := eng.GetPlayerById(id)
	if !ok {
		return "", false
	}
	return player.Name, true
}
