//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func timerHandler(database *sql.DB) *TournamentHandler {
	return &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
}

// mustStaff grants tier on the tournament and returns the user holding it.
func mustStaff(t *testing.T, database *sql.DB, tournamentID int64, tier models.TournamentTier, label string) *models.User {
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

// A judge is the lowest tier allowed to run the clock -- that is the whole
// point of the feature, so it is the first thing asserted.
func TestStartRoundTimerAsJudge(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := startedTournament(t, database)
	judge := mustStaff(t, database, tourn.ID, models.TierJudge, "judge")

	h := timerHandler(database)
	w := httptest.NewRecorder()
	req := requestWithUser("POST", fmt.Sprintf("/tournaments/%d/round-timer/start", tourn.ID),
		"minutes=50", judge, map[string]string{"id": fmt.Sprint(tourn.ID)})
	h.StartRoundTimer(w, req)

	if w.Code != 303 {
		t.Fatalf("status = %d, want 303; body: %s", w.Code, w.Body.String())
	}
	want := fmt.Sprintf("/tournaments/%d/manage", tourn.ID)
	if got := w.Header().Get("Location"); got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	timer, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil || timer == nil {
		t.Fatalf("timer not persisted: %v, %+v", err, timer)
	}
	if timer.DurationSeconds != 3000 {
		t.Errorf("duration_seconds = %d, want 3000", timer.DurationSeconds)
	}
	if timer.StartedBy == nil || *timer.StartedBy != judge.ID {
		t.Errorf("started_by = %v, want %d", timer.StartedBy, judge.ID)
	}
}

func TestStartRoundTimerRejectsNonStaff(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	outsider := mustCreateUser(t, database, "outsider-"+t.Name()+"@example.com", "Outsider-"+t.Name())

	h := timerHandler(database)
	w := httptest.NewRecorder()
	req := requestWithUser("POST", fmt.Sprintf("/tournaments/%d/round-timer/start", tourn.ID),
		"minutes=50", outsider, map[string]string{"id": fmt.Sprint(tourn.ID)})
	h.StartRoundTimer(w, req)

	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	timer, err := db.GetRoundTimer(context.Background(), database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("GetRoundTimer: %v", err)
	}
	if timer != nil {
		t.Error("a non-staff request started a timer")
	}
}

func TestStartRoundTimerRejectsBadMinutes(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	judge := mustStaff(t, database, tourn.ID, models.TierJudge, "judge")

	for _, body := range []string{"minutes=0", "minutes=-5", "minutes=abc", "minutes=", "minutes=601"} {
		t.Run(body, func(t *testing.T) {
			h := timerHandler(database)
			w := httptest.NewRecorder()
			req := requestWithUser("POST", fmt.Sprintf("/tournaments/%d/round-timer/start", tourn.ID),
				body, judge, map[string]string{"id": fmt.Sprint(tourn.ID)})
			h.StartRoundTimer(w, req)
			if w.Code != 400 {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

// A tournament with no live round has nothing to time, so there is no round to
// key the timer on and the request must be refused rather than guessed at.
func TestStartRoundTimerRejectedWhenNoRoundIsLive(t *testing.T) {
	database := testDB(t)
	owner := mustCreateUser(t, database, "owner-nl-"+t.Name()+"@example.com", "Owner-nl-"+t.Name())
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	h := timerHandler(database)
	w := httptest.NewRecorder()
	req := requestWithUser("POST", fmt.Sprintf("/tournaments/%d/round-timer/start", tourn.ID),
		"minutes=50", owner, map[string]string{"id": fmt.Sprint(tourn.ID)})
	h.StartRoundTimer(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestPauseResumeClearRoundTimer(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := startedTournament(t, database)
	judge := mustStaff(t, database, tourn.ID, models.TierJudge, "judge")
	params := map[string]string{"id": fmt.Sprint(tourn.ID)}
	h := timerHandler(database)

	start := httptest.NewRecorder()
	h.StartRoundTimer(start, requestWithUser("POST", "/", "minutes=50", judge, params))
	if start.Code != 303 {
		t.Fatalf("start: status = %d, want 303", start.Code)
	}

	pause := httptest.NewRecorder()
	h.PauseRoundTimer(pause, requestWithUser("POST", "/", "", judge, params))
	if pause.Code != 303 {
		t.Fatalf("pause: status = %d, want 303", pause.Code)
	}
	paused, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil || paused == nil {
		t.Fatalf("get after pause: %v, %+v", err, paused)
	}
	if !paused.Paused() {
		t.Fatal("timer not paused after PauseRoundTimer")
	}
	frozen := paused.RemainingSeconds(time.Now())

	resume := httptest.NewRecorder()
	h.ResumeRoundTimer(resume, requestWithUser("POST", "/", "", judge, params))
	if resume.Code != 303 {
		t.Fatalf("resume: status = %d, want 303", resume.Code)
	}
	resumed, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil || resumed == nil {
		t.Fatalf("get after resume: %v, %+v", err, resumed)
	}
	if resumed.Paused() {
		t.Fatal("timer still paused after ResumeRoundTimer")
	}
	if got := resumed.RemainingSeconds(time.Now()); got < frozen-1 || got > frozen {
		t.Errorf("resumed remainder = %d, want ~%d", got, frozen)
	}

	clear := httptest.NewRecorder()
	h.ClearRoundTimer(clear, requestWithUser("POST", "/", "", judge, params))
	if clear.Code != 303 {
		t.Fatalf("clear: status = %d, want 303", clear.Code)
	}
	gone, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if gone != nil {
		t.Errorf("timer survived clear: %+v", gone)
	}
}

func TestPauseResumeClearRejectNonStaff(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	judge := mustStaff(t, database, tourn.ID, models.TierJudge, "judge")
	outsider := mustCreateUser(t, database, "outsider2-"+t.Name()+"@example.com", "Outsider2-"+t.Name())
	params := map[string]string{"id": fmt.Sprint(tourn.ID)}
	h := timerHandler(database)

	start := httptest.NewRecorder()
	h.StartRoundTimer(start, requestWithUser("POST", "/", "minutes=50", judge, params))
	if start.Code != 303 {
		t.Fatalf("start: status = %d", start.Code)
	}

	for name, fn := range map[string]func(w *httptest.ResponseRecorder){
		"pause": func(w *httptest.ResponseRecorder) {
			h.PauseRoundTimer(w, requestWithUser("POST", "/", "", outsider, params))
		},
		"resume": func(w *httptest.ResponseRecorder) {
			h.ResumeRoundTimer(w, requestWithUser("POST", "/", "", outsider, params))
		},
		"clear": func(w *httptest.ResponseRecorder) {
			h.ClearRoundTimer(w, requestWithUser("POST", "/", "", outsider, params))
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			fn(w)
			if w.Code != 403 {
				t.Errorf("status = %d, want 403", w.Code)
			}
		})
	}

	// The timer must still be there and untouched.
	timer, err := db.GetRoundTimer(context.Background(), database, tourn.ID, 1, false)
	if err != nil || timer == nil {
		t.Fatalf("timer gone after rejected requests: %v, %+v", err, timer)
	}
	if timer.Paused() {
		t.Error("a rejected pause still froze the clock")
	}
}

// Advancing the round must leave the new round with no clock, which is how
// "nothing is displayed until a judge starts it" holds across a round change.
func TestRoundTimerDoesNotSurviveRoundAdvance(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := startedTournament(t, database)
	params := map[string]string{"id": fmt.Sprint(tourn.ID)}
	h := timerHandler(database)

	start := httptest.NewRecorder()
	h.StartRoundTimer(start, requestWithUser("POST", "/", "minutes=50", owner, params))
	if start.Code != 303 {
		t.Fatalf("start: status = %d, want 303", start.Code)
	}

	next := httptest.NewRecorder()
	h.NextRound(next, requestWithUser("POST", "/", "", owner, params))
	if next.Code != 303 {
		t.Fatalf("next round: status = %d, want 303; body: %s", next.Code, next.Body.String())
	}

	// Round 1's timer is still on disk, but round 2 -- the live one -- has none.
	round2, err := db.GetRoundTimer(ctx, database, tourn.ID, 2, false)
	if err != nil {
		t.Fatalf("GetRoundTimer round 2: %v", err)
	}
	if round2 != nil {
		t.Errorf("round 1's timer carried into round 2: %+v", round2)
	}
}

// Detail must hand the template everything it needs to decide whether to emit
// the timer section at all, and what to label it.
func TestDetailExposesLiveRoundAndTimer(t *testing.T) {
	database := testDB(t)
	_, tourn := startedTournament(t, database)
	judge := mustStaff(t, database, tourn.ID, models.TierJudge, "judge")
	params := map[string]string{"id": fmt.Sprint(tourn.ID)}

	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}

	// Before anyone starts a clock: a round is live, but there is no timer.
	rec := httptest.NewRecorder()
	h.Detail(rec, requestWithUser("GET", "/", "", nil, params))
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["RoundIsLive"] != true {
		t.Errorf("RoundIsLive = %v, want true", data["RoundIsLive"])
	}
	if data["RoundTimer"] != (*models.RoundTimer)(nil) && data["RoundTimer"] != nil {
		t.Errorf("RoundTimer = %v, want nil before a timer is started", data["RoundTimer"])
	}
	if data["PairingsLabel"] != "Round 1" {
		t.Errorf("PairingsLabel = %v, want %q", data["PairingsLabel"], "Round 1")
	}

	// After a judge starts one, the same page carries it.
	start := httptest.NewRecorder()
	h.StartRoundTimer(start, requestWithUser("POST", "/", "minutes=50", judge, params))
	if start.Code != 303 {
		t.Fatalf("start: status = %d", start.Code)
	}

	tmpl2 := &mockTemplate{}
	h2 := &TournamentHandler{DB: database, Tmpl: tmpl2}
	rec2 := httptest.NewRecorder()
	h2.Detail(rec2, requestWithUser("GET", "/", "", nil, params))
	data2 := tmpl2.calls[0].Data.(map[string]interface{})
	timer, ok := data2["RoundTimer"].(*models.RoundTimer)
	if !ok || timer == nil {
		t.Fatalf("RoundTimer = %v, want a timer", data2["RoundTimer"])
	}
	if timer.Round != 1 || timer.Playoff {
		t.Errorf("timer is for round %d (playoff=%v), want round 1 swiss", timer.Round, timer.Playoff)
	}
}

// A tournament with no live round must report that, so the detail page emits no
// timer markup at all rather than a hidden-but-present section.
func TestDetailReportsNoLiveRoundWhenNotRunning(t *testing.T) {
	database := testDB(t)
	owner := mustCreateUser(t, database, "owner-nr-"+t.Name()+"@example.com", "Owner-nr-"+t.Name())
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	rec := httptest.NewRecorder()
	h.Detail(rec, requestWithUser("GET", "/", "", nil, map[string]string{"id": fmt.Sprint(tourn.ID)}))

	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["RoundIsLive"] != false {
		t.Errorf("RoundIsLive = %v, want false", data["RoundIsLive"])
	}
}

// During the playoff the detail page must show the bracket round, not the last
// Swiss round it used to keep displaying, and must label it by bracket stage.
func TestDetailShowsPlayoffPairingsDuringPlayoff(t *testing.T) {
	database := testDB(t)
	owner, tourn := playoffReady(t, database)
	params := map[string]string{"id": fmt.Sprint(tourn.ID)}

	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	start := httptest.NewRecorder()
	h.StartPlayoff(start, requestWithUser("POST", "/", "", owner, params))
	if start.Code != 303 {
		t.Fatalf("StartPlayoff: status = %d, body: %s", start.Code, start.Body.String())
	}

	tmpl := &mockTemplate{}
	h2 := &TournamentHandler{DB: database, Tmpl: tmpl}
	rec := httptest.NewRecorder()
	h2.Detail(rec, requestWithUser("GET", "/", "", nil, params))
	data := tmpl.calls[0].Data.(map[string]interface{})

	if data["RoundIsLive"] != true {
		t.Errorf("RoundIsLive = %v, want true during the playoff", data["RoundIsLive"])
	}
	// Top cut of 4 is a two-round bracket, so the first round is the semifinals.
	if data["PairingsLabel"] != "Semifinals" {
		t.Errorf("PairingsLabel = %v, want %q", data["PairingsLabel"], "Semifinals")
	}
	pairings, ok := data["Pairings"].([]resolvedPairing)
	if !ok {
		t.Fatalf("Pairings = %T, want []resolvedPairing", data["Pairings"])
	}
	if len(pairings) != 2 {
		t.Errorf("got %d pairings, want 2 for a top cut of 4", len(pairings))
	}
}

// A judge can time a playoff round too, keyed separately from the Swiss rounds.
func TestStartRoundTimerDuringPlayoff(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := playoffReady(t, database)
	params := map[string]string{"id": fmt.Sprint(tourn.ID)}
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	start := httptest.NewRecorder()
	h.StartPlayoff(start, requestWithUser("POST", "/", "", owner, params))
	if start.Code != 303 {
		t.Fatalf("StartPlayoff: status = %d", start.Code)
	}

	judge := mustStaff(t, database, tourn.ID, models.TierJudge, "pfjudge")
	timerRec := httptest.NewRecorder()
	h.StartRoundTimer(timerRec, requestWithUser("POST", "/", "minutes=90", judge, params))
	if timerRec.Code != 303 {
		t.Fatalf("StartRoundTimer: status = %d, body: %s", timerRec.Code, timerRec.Body.String())
	}

	timer, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, true)
	if err != nil || timer == nil {
		t.Fatalf("playoff timer not persisted: %v, %+v", err, timer)
	}
	if timer.DurationSeconds != 5400 {
		t.Errorf("duration_seconds = %d, want 5400", timer.DurationSeconds)
	}
	// It must not have landed on Swiss round 1.
	swiss, err := db.GetRoundTimer(ctx, database, tourn.ID, 1, false)
	if err != nil {
		t.Fatalf("GetRoundTimer swiss: %v", err)
	}
	if swiss != nil {
		t.Errorf("playoff timer leaked onto swiss round 1: %+v", swiss)
	}
}
