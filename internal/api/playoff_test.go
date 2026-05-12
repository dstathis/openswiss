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

// playoffReady builds a tournament that has finished swiss and is ready to
// have the playoff started (top_cut configured).
func playoffReady(t *testing.T, database *sql.DB) (*models.User, *models.Tournament) {
	t.Helper()
	ctx := context.Background()

	owner := mustCreateUser(t, database, "po-owner-"+t.Name()+"@example.com", "POOwner-"+t.Name())
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "Playoff " + t.Name(),
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		TopCut:      4,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: owner.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	for i := 0; i < 8; i++ {
		u := mustCreateUser(t, database, "po"+strconv.Itoa(i)+"-"+t.Name()+"@example.com", "PO"+strconv.Itoa(i)+"-"+t.Name())
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)

	// Init + run all swiss rounds with results
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
			for round := 1; round <= numRounds; round++ {
				for _, p := range eng.GetRound() {
					if p.PlayerB() == swisstools.BYE_OPPONENT_ID {
						continue
					}
					if err := eng.AddResult(p.PlayerA(), 2, 0, 0); err != nil {
						return "", err
					}
				}
				if err := eng.NextRound(); err != nil {
					return "", err
				}
				if round < numRounds {
					if err := eng.Pair(false); err != nil {
						return "", err
					}
				}
			}
			return models.TournamentStatusInProgress, nil
		}); err != nil {
		t.Fatalf("setup playoff-ready: %v", err)
	}
	tourn, _ = db.GetTournament(ctx, database, tourn.ID)
	return owner, tourn
}

func TestPlayoffAPI_Start(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Start(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Status != models.TournamentStatusPlayoff {
		t.Errorf("status = %q, want playoff", got.Status)
	}
}

func TestPlayoffAPI_Start_Forbidden(t *testing.T) {
	database := testDB(t)
	_, tourn := playoffReady(t, database)
	other := mustCreateUser(t, database, "other-pf@example.com", "OtherPF")
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Start(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestPlayoffAPI_Start_NoTopCut(t *testing.T) {
	database := testDB(t)
	owner, tourn := startedTournament(t, database) // has top_cut=0
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Start(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayoffAPI_Get_NotStarted(t *testing.T) {
	database := testDB(t)
	api := &PlayoffAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Get(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayoffAPI_Get_NotFound(t *testing.T) {
	database := testDB(t)
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.Get(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayoffAPI_Get_NoPlayoff(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Get(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when no playoff started, got %d", rec.Code)
	}
}

func TestPlayoffAPI_Get_AfterStart(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}

	startReq := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	api.Start(httptest.NewRecorder(), startReq)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Get(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["seeds"] == nil {
		t.Error("expected seeds in response")
	}
}

func TestPlayoffAPI_GetCurrentRound_NotStarted(t *testing.T) {
	database := testDB(t)
	api := &PlayoffAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayoffAPI_GetCurrentRound_NotFound(t *testing.T) {
	database := testDB(t)
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayoffAPI_GetCurrentRound_NoPlayoff(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &PlayoffAPI{DB: database}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when no playoff, got %d", rec.Code)
	}
}

func TestPlayoffAPI_GetCurrentRound_AfterStart(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}
	api.Start(httptest.NewRecorder(), requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetCurrentRound(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlayoffAPI_SubmitResults_BadJSON(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}
	api.Start(httptest.NewRecorder(), requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))

	r := requestWithUser("POST", "/", "not-json", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitResults(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayoffAPI_SubmitResults_Forbidden(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}
	api.Start(httptest.NewRecorder(), requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))

	other := mustCreateUser(t, database, "po-other@example.com", "POOther")
	r := requestWithUser("POST", "/", `{"results":[]}`, other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitResults(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestPlayoffAPI_SubmitResults_AndNextRound(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}
	api.Start(httptest.NewRecorder(), requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))

	// Read current playoff round, build results
	tourn, _ = db.GetTournament(ctx, database, tourn.ID)
	eng, _ := swisstools.LoadTournament(tourn.EngineState)
	pairings := eng.GetPlayoffRound()
	type entry struct {
		PlayerID int `json:"player_id"`
		Wins     int `json:"wins"`
		Losses   int `json:"losses"`
		Draws    int `json:"draws"`
	}
	var results []entry
	for _, p := range pairings {
		results = append(results, entry{PlayerID: p.PlayerA(), Wins: 2, Losses: 0, Draws: 0})
	}
	body, _ := json.Marshal(map[string]interface{}{"results": results})

	r := requestWithUser("POST", "/", string(body), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitResults(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit: %d, body=%s", rec.Code, rec.Body.String())
	}

	// Advance the playoff
	r = requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec = httptest.NewRecorder()
	api.NextRound(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("next round: %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestPlayoffAPI_NextRound_Forbidden(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	api := &PlayoffAPI{DB: database}
	api.Start(httptest.NewRecorder(), requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))

	other := mustCreateUser(t, database, "po-other-nr@example.com", "POOtherNR")
	r := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.NextRound(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
