package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
	"github.com/go-chi/chi/v5"
)

// maxRoundMinutes bounds a start request. Rounds are tens of minutes; the cap
// keeps a typo from parking ends_at years into the future.
const maxRoundMinutes = 600

// RoundTimerAPI serves the round clock. Reading is public -- it is what the
// tournament page polls, and spectators and players need it as much as staff
// do. Writing is Judge-tier, the lowest staff tier, because running the clock
// is a floor-level job.
type RoundTimerAPI struct {
	DB *sql.DB
}

// roundTimerResponse is the shape the page polls. Every field the client needs
// to render and tick is here, so a poll is a single request.
//
// ServerNow is included so the client can correct for a wrong device clock
// instead of trusting it: a phone minutes out of sync would otherwise show a
// countdown minutes wrong.
type roundTimerResponse struct {
	// State is "running", "paused", or "none" (no round live, or no clock
	// started for it). "none" is a 200, not a 404 -- it is the ordinary
	// resting state of a round and the client polls this continuously.
	State string `json:"state"`

	Round      *int   `json:"round"`
	Playoff    bool   `json:"playoff"`
	RoundLabel string `json:"round_label,omitempty"`

	DurationSeconds *int       `json:"duration_seconds"`
	EndsAt          *time.Time `json:"ends_at"`
	PausedAt        *time.Time `json:"paused_at,omitempty"`

	// RemainingSeconds goes negative once the round is in overtime; the sign is
	// what tells the client to count up rather than down.
	RemainingSeconds *int `json:"remaining_seconds"`

	ServerNow time.Time `json:"server_now"`
}

// timerPayload builds the response for a tournament's live round.
func timerPayload(timer *models.RoundTimer, lr engine.LiveRound, roundIsLive bool) roundTimerResponse {
	now := time.Now().UTC()
	resp := roundTimerResponse{State: "none", ServerNow: now}
	if !roundIsLive {
		return resp
	}

	round := lr.Number
	resp.Round = &round
	resp.Playoff = lr.Playoff
	resp.RoundLabel = lr.Label
	if timer == nil {
		return resp
	}

	resp.State = "running"
	if timer.Paused() {
		resp.State = "paused"
	}
	duration := timer.DurationSeconds
	endsAt := timer.EndsAt.UTC()
	remaining := timer.RemainingSeconds(now)
	resp.DurationSeconds = &duration
	resp.EndsAt = &endsAt
	resp.PausedAt = timer.PausedAt
	resp.RemainingSeconds = &remaining
	return resp
}

// liveRoundFor resolves the tournament and the round a timer would belong to.
// It writes the error response itself and returns ok=false, keeping callers to
// a single guard clause.
func (a *RoundTimerAPI) liveRoundFor(w http.ResponseWriter, r *http.Request) (*models.Tournament, engine.LiveRound, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return nil, engine.LiveRound{}, false
	}
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return nil, engine.LiveRound{}, false
	}
	if len(t.EngineState) == 0 {
		return t, engine.LiveRound{}, false
	}
	eng, err := swisstools.LoadTournament(t.EngineState)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return nil, engine.LiveRound{}, false
	}
	lr, ok := engine.CurrentLiveRound(t, &eng)
	if !ok {
		return t, engine.LiveRound{}, false
	}
	return t, lr, true
}

// Get returns the clock for the live round. Public: this is the endpoint the
// tournament page polls, and the clock is for everyone in the room.
//
// Every open tournament page polls this same URL on a timer, so it must not be
// cached. Without no-store, browsers answer the second and later polls from
// their memory/HTTP cache and the page's view of the clock freezes at whatever
// it was when the page loaded -- a started timer never appears and a cleared
// one never goes away until the viewer refreshes by hand. The client also asks
// for cache: 'no-store'; this is the half that does not depend on the client
// getting it right.
func (a *RoundTimerAPI) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	t, lr, roundIsLive := a.liveRoundFor(w, r)
	if t == nil {
		return // liveRoundFor already wrote the error
	}
	if !roundIsLive {
		jsonResponse(w, http.StatusOK, timerPayload(nil, lr, false))
		return
	}
	timer, err := db.GetRoundTimer(r.Context(), a.DB, t.ID, lr.Number, lr.Playoff)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	jsonResponse(w, http.StatusOK, timerPayload(timer, lr, true))
}

// roundTimerRequest is the POST body: either {"minutes": N} to start/restart,
// or {"action": "pause"|"resume"}.
type roundTimerRequest struct {
	Minutes *int   `json:"minutes"`
	Action  string `json:"action"`
}

// Post starts, restarts, pauses or resumes the clock. Judge-tier.
//
// Starting over a running timer overwrites it rather than conflicting: that is
// how a judge corrects a duration they entered wrong.
func (a *RoundTimerAPI) Post(w http.ResponseWriter, r *http.Request) {
	var body roundTimerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Minutes == nil && body.Action == "" {
		jsonError(w, http.StatusBadRequest, `expected "minutes" or "action"`)
		return
	}
	if body.Minutes != nil && (*body.Minutes < 1 || *body.Minutes > maxRoundMinutes) {
		jsonError(w, http.StatusBadRequest, "minutes must be between 1 and 600")
		return
	}
	if body.Minutes == nil && body.Action != "pause" && body.Action != "resume" {
		jsonError(w, http.StatusBadRequest, `action must be "pause" or "resume"`)
		return
	}

	// Authorize before resolving the round so a non-staff caller learns nothing
	// about the tournament's state from the response.
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if !middleware.AuthorizeTournament(w, r, a.DB, id, models.TierJudge) {
		return
	}

	t, lr, roundIsLive := a.liveRoundFor(w, r)
	if t == nil {
		return
	}
	if !roundIsLive {
		jsonError(w, http.StatusBadRequest, "no round is in progress")
		return
	}

	var timer *models.RoundTimer
	switch {
	case body.Minutes != nil:
		var startedBy *int64
		if user := middleware.GetUser(r.Context()); user != nil {
			startedBy = &user.ID
		}
		timer, err = db.StartRoundTimer(r.Context(), a.DB, t.ID, lr.Number, lr.Playoff,
			time.Duration(*body.Minutes)*time.Minute, startedBy)
	case body.Action == "pause":
		timer, err = db.PauseRoundTimer(r.Context(), a.DB, t.ID, lr.Number, lr.Playoff)
	default:
		timer, err = db.ResumeRoundTimer(r.Context(), a.DB, t.ID, lr.Number, lr.Playoff)
	}
	if errors.Is(err, db.ErrNoRoundTimer) {
		jsonError(w, http.StatusNotFound, "no timer is running for this round")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	jsonResponse(w, http.StatusOK, timerPayload(timer, lr, true))
}

// Delete clears the clock for the live round, which is how a judge undoes a
// misclick. Judge-tier. Clearing an absent timer succeeds: the caller asked for
// a state that already holds.
func (a *RoundTimerAPI) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if !middleware.AuthorizeTournament(w, r, a.DB, id, models.TierJudge) {
		return
	}
	t, lr, roundIsLive := a.liveRoundFor(w, r)
	if t == nil {
		return
	}
	if !roundIsLive {
		jsonError(w, http.StatusBadRequest, "no round is in progress")
		return
	}
	if err := db.DeleteRoundTimer(r.Context(), a.DB, t.ID, lr.Number, lr.Playoff); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
