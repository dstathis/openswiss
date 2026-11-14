//go:build integration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func timerParams(id int64) map[string]string {
	return map[string]string{"id": fmt.Sprint(id)}
}

func decodeTimer(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func mustAPIStaff(t *testing.T, database *sql.DB, tournamentID int64, tier models.TournamentTier, label string) *models.User {
	t.Helper()
	u := mustCreateUser(t, database, fmt.Sprintf("%s-%s@example.com", label, t.Name()), label+"-"+t.Name())
	if err := db.AddTournamentStaff(context.Background(), database, &models.TournamentStaff{
		TournamentID: tournamentID,
		UserID:       u.ID,
		Tier:         tier,
	}); err != nil {
		t.Fatalf("grant %s: %v", tier, err)
	}
	return u
}

// A round with no clock is the ordinary resting state, so it is a 200 with
// state "none" rather than a 404 -- the page polls this endpoint continuously
// and a 404 would be indistinguishable from a broken deployment.
func TestRoundTimerAPI_GetAbsentIsStateNone(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &RoundTimerAPI{DB: database}

	rec := httptest.NewRecorder()
	api.Get(rec, requestWithUser("GET", "/", "", nil, timerParams(tourn.ID)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodeTimer(t, rec)
	if body["state"] != "none" {
		t.Errorf("state = %v, want \"none\"", body["state"])
	}
	if body["server_now"] == nil {
		t.Error("server_now missing; the client needs it to correct clock skew")
	}
}

func TestRoundTimerAPI_GetRunning(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := freshStarted(t, database)
	if _, err := db.StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}

	api := &RoundTimerAPI{DB: database}
	rec := httptest.NewRecorder()
	api.Get(rec, requestWithUser("GET", "/", "", nil, timerParams(tourn.ID)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodeTimer(t, rec)
	if body["state"] != "running" {
		t.Errorf("state = %v, want \"running\"", body["state"])
	}
	if body["round_label"] != "Round 1" {
		t.Errorf("round_label = %v, want \"Round 1\"", body["round_label"])
	}
	if got := body["remaining_seconds"].(float64); got < 2990 || got > 3000 {
		t.Errorf("remaining_seconds = %v, want ~3000", got)
	}
	if body["ends_at"] == nil {
		t.Error("ends_at missing; the client ticks down from it")
	}
}

// Overtime is carried as a negative remainder, which is how the page knows to
// switch from counting down to counting up.
func TestRoundTimerAPI_GetOvertimeIsNegative(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := freshStarted(t, database)
	if _, err := db.StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	// Drag ends_at into the past rather than waiting 50 minutes.
	if _, err := database.ExecContext(ctx,
		`UPDATE round_timers SET ends_at = now() - interval '2 minutes'
		 WHERE tournament_id = $1`, tourn.ID); err != nil {
		t.Fatalf("backdate ends_at: %v", err)
	}

	api := &RoundTimerAPI{DB: database}
	rec := httptest.NewRecorder()
	api.Get(rec, requestWithUser("GET", "/", "", nil, timerParams(tourn.ID)))

	body := decodeTimer(t, rec)
	if got := body["remaining_seconds"].(float64); got > -119 || got < -121 {
		t.Errorf("remaining_seconds = %v, want ~-120", got)
	}
}

func TestRoundTimerAPI_PostStartAsJudge(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	judge := mustAPIStaff(t, database, tourn.ID, models.TierJudge, "judge")
	api := &RoundTimerAPI{DB: database}

	rec := httptest.NewRecorder()
	api.Post(rec, requestWithUser("POST", "/", `{"minutes":50}`, judge, timerParams(tourn.ID)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := decodeTimer(t, rec)
	if body["state"] != "running" {
		t.Errorf("state = %v, want \"running\"", body["state"])
	}
	if body["duration_seconds"].(float64) != 3000 {
		t.Errorf("duration_seconds = %v, want 3000", body["duration_seconds"])
	}
}

func TestRoundTimerAPI_PostPauseAndResume(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	judge := mustAPIStaff(t, database, tourn.ID, models.TierJudge, "judge")
	api := &RoundTimerAPI{DB: database}

	start := httptest.NewRecorder()
	api.Post(start, requestWithUser("POST", "/", `{"minutes":50}`, judge, timerParams(tourn.ID)))
	if start.Code != http.StatusOK {
		t.Fatalf("start: status = %d", start.Code)
	}

	pause := httptest.NewRecorder()
	api.Post(pause, requestWithUser("POST", "/", `{"action":"pause"}`, judge, timerParams(tourn.ID)))
	if pause.Code != http.StatusOK {
		t.Fatalf("pause: status = %d; body: %s", pause.Code, pause.Body.String())
	}
	if body := decodeTimer(t, pause); body["state"] != "paused" {
		t.Errorf("state = %v, want \"paused\"", body["state"])
	}

	resume := httptest.NewRecorder()
	api.Post(resume, requestWithUser("POST", "/", `{"action":"resume"}`, judge, timerParams(tourn.ID)))
	if resume.Code != http.StatusOK {
		t.Fatalf("resume: status = %d", resume.Code)
	}
	if body := decodeTimer(t, resume); body["state"] != "running" {
		t.Errorf("state = %v, want \"running\"", body["state"])
	}
}

func TestRoundTimerAPI_PostRejectsBadBody(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	judge := mustAPIStaff(t, database, tourn.ID, models.TierJudge, "judge")
	api := &RoundTimerAPI{DB: database}

	for _, body := range []string{`{}`, `{"minutes":0}`, `{"minutes":601}`, `{"action":"explode"}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.Post(rec, requestWithUser("POST", "/", body, judge, timerParams(tourn.ID)))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestRoundTimerAPI_Delete(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := freshStarted(t, database)
	judge := mustAPIStaff(t, database, tourn.ID, models.TierJudge, "judge")
	if _, err := db.StartRoundTimer(ctx, database, tourn.ID, 1, false, 50*time.Minute, nil); err != nil {
		t.Fatalf("StartRoundTimer: %v", err)
	}
	api := &RoundTimerAPI{DB: database}

	rec := httptest.NewRecorder()
	api.Delete(rec, requestWithUser("DELETE", "/", "", judge, timerParams(tourn.ID)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	timer, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("GetRoundTimer: %v", err)
	}
	if timer != nil {
		t.Errorf("timer survived DELETE: %+v", timer)
	}
}

// Writing is Judge-tier; reading is public. Anonymous and plain-player
// requests must both be refused on the write paths.
func TestRoundTimerAPI_WritesRequireJudge(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	outsider := mustCreateUser(t, database, "outsider-"+t.Name()+"@example.com", "Outsider-"+t.Name())
	api := &RoundTimerAPI{DB: database}

	cases := map[string]*models.User{"anonymous": nil, "non-staff player": outsider}
	for name, user := range cases {
		t.Run(name+" post", func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.Post(rec, requestWithUser("POST", "/", `{"minutes":50}`, user, timerParams(tourn.ID)))
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
		t.Run(name+" delete", func(t *testing.T) {
			rec := httptest.NewRecorder()
			api.Delete(rec, requestWithUser("DELETE", "/", "", user, timerParams(tourn.ID)))
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}

	// Reading stays open to everyone.
	rec := httptest.NewRecorder()
	api.Get(rec, requestWithUser("GET", "/", "", nil, timerParams(tourn.ID)))
	if rec.Code != http.StatusOK {
		t.Errorf("anonymous GET status = %d, want 200", rec.Code)
	}
}

// With no round being played there is nothing to time, and the poll must say so
// rather than inventing a round number.
func TestRoundTimerAPI_GetWithNoLiveRound(t *testing.T) {
	database := testDB(t)
	owner := mustCreateUser(t, database, "owner-nl-"+t.Name()+"@example.com", "Owner-nl-"+t.Name())
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	api := &RoundTimerAPI{DB: database}

	rec := httptest.NewRecorder()
	api.Get(rec, requestWithUser("GET", "/", "", nil, timerParams(tourn.ID)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodeTimer(t, rec)
	if body["state"] != "none" {
		t.Errorf("state = %v, want \"none\"", body["state"])
	}
	if body["round"] != nil {
		t.Errorf("round = %v, want null when no round is live", body["round"])
	}
}

// The page polls this endpoint repeatedly at one unchanging URL. Without an
// explicit no-store, browsers serve the second and later polls from their
// memory/HTTP cache -- the page's view of the clock then freezes at whatever it
// was when the page loaded, so a started timer never appears and a cleared one
// never goes away until a manual refresh.
func TestRoundTimerAPI_GetIsNotCacheable(t *testing.T) {
	database := testDB(t)
	_, tourn := freshStarted(t, database)
	api := &RoundTimerAPI{DB: database}

	rec := httptest.NewRecorder()
	api.Get(rec, requestWithUser("GET", "/", "", nil, timerParams(tourn.ID)))

	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}
}
