//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
)

// playoffReady mirrors the api package helper but in handlers.
func playoffReady(t *testing.T, database *sql.DB) (*models.User, *models.Tournament) {
	t.Helper()
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner-pf-"+t.Name()+"@example.com", "OwnerPF-"+t.Name())
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
		u := mustCreateUser(t, database, "pf"+strconv.Itoa(i)+"-"+t.Name()+"@example.com", "PF"+strconv.Itoa(i)+"-"+t.Name())
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

func TestTournamentHandler_StartPlayoff(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.StartPlayoff(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Status != models.TournamentStatusPlayoff {
		t.Errorf("status = %q, want playoff", got.Status)
	}
}

func TestTournamentHandler_PlayoffResults(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	// Start playoff first
	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	h.StartPlayoff(httptest.NewRecorder(), req)

	// Read playoff round and submit results
	tourn, _ = db.GetTournament(context.Background(), database, tourn.ID)
	eng, _ := swisstools.LoadTournament(tourn.EngineState)
	pairings := eng.GetPlayoffRound()

	form := url.Values{}
	for _, p := range pairings {
		pid := strconv.Itoa(p.PlayerA())
		form.Set("wins_a_"+pid, "2")
		form.Set("wins_b_"+pid, "0")
		form.Set("draws_"+pid, "0")
	}
	req2 := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.PlayoffResults(rec, req2)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTournamentHandler_NextPlayoffRound(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	// Start playoff and submit first-round results
	h.StartPlayoff(httptest.NewRecorder(), requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))
	tourn, _ = db.GetTournament(context.Background(), database, tourn.ID)
	eng, _ := swisstools.LoadTournament(tourn.EngineState)
	pairings := eng.GetPlayoffRound()
	form := url.Values{}
	for _, p := range pairings {
		pid := strconv.Itoa(p.PlayerA())
		form.Set("wins_a_"+pid, "2")
		form.Set("wins_b_"+pid, "0")
		form.Set("draws_"+pid, "0")
	}
	h.PlayoffResults(httptest.NewRecorder(), requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)}))

	// Advance the playoff
	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.NextPlayoffRound(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
