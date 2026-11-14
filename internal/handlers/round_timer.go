package handlers

import (
	"database/sql"
	"fmt"
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

// maxRoundMinutes bounds the start form. Rounds are tens of minutes; ten hours
// is already absurd, and the cap keeps a typo from parking ends_at years out.
const maxRoundMinutes = 600

// loadLiveRound resolves the tournament and its live round for a timer request.
// It writes the error response itself and returns ok=false, so callers stay a
// single guard clause like the other tournament handlers.
func (h *TournamentHandler) loadLiveRound(w http.ResponseWriter, r *http.Request, id int64) (*models.Tournament, engine.LiveRound, bool) {
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, engine.LiveRound{}, false
	}
	if len(t.EngineState) == 0 {
		http.Error(w, "No round is in progress", http.StatusBadRequest)
		return nil, engine.LiveRound{}, false
	}
	eng, err := swisstools.LoadTournament(t.EngineState)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return nil, engine.LiveRound{}, false
	}
	lr, ok := engine.CurrentLiveRound(t, &eng)
	if !ok {
		http.Error(w, "No round is in progress", http.StatusBadRequest)
		return nil, engine.LiveRound{}, false
	}
	return t, lr, true
}

// timerRequest runs the authorization and live-round lookup every timer action
// shares, then hands the resolved round to fn. All four actions are Judge-tier:
// the clock is a floor-level concern and judges are the staff standing on the
// floor.
func (h *TournamentHandler) timerRequest(w http.ResponseWriter, r *http.Request, fn func(id int64, lr engine.LiveRound) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !middleware.AuthorizeTournament(w, r, h.DB, id, models.TierJudge) {
		return
	}
	_, lr, ok := h.loadLiveRound(w, r, id)
	if !ok {
		return
	}
	if err := fn(id, lr); err != nil {
		if err == db.ErrNoRoundTimer {
			http.Error(w, "No timer is running for this round", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

// StartRoundTimer starts or restarts the clock for the live round. Restarting
// over a running timer is deliberate rather than an error: it is how a judge
// corrects a duration they entered wrong.
func (h *TournamentHandler) StartRoundTimer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	minutes, err := strconv.Atoi(r.FormValue("minutes"))
	if err != nil || minutes < 1 || minutes > maxRoundMinutes {
		http.Error(w, fmt.Sprintf("Round length must be a whole number of minutes between 1 and %d", maxRoundMinutes), http.StatusBadRequest)
		return
	}

	var startedBy *int64
	if user := middleware.GetUser(r.Context()); user != nil {
		startedBy = &user.ID
	}

	h.timerRequest(w, r, func(id int64, lr engine.LiveRound) error {
		_, err := db.StartRoundTimer(r.Context(), h.DB, id, lr.Number, lr.Playoff,
			time.Duration(minutes)*time.Minute, startedBy)
		return err
	})
}

// PauseRoundTimer freezes the clock -- a fire alarm, a long judge call.
func (h *TournamentHandler) PauseRoundTimer(w http.ResponseWriter, r *http.Request) {
	h.timerRequest(w, r, func(id int64, lr engine.LiveRound) error {
		_, err := db.PauseRoundTimer(r.Context(), h.DB, id, lr.Number, lr.Playoff)
		return err
	})
}

// ResumeRoundTimer continues a frozen clock from where it stopped.
func (h *TournamentHandler) ResumeRoundTimer(w http.ResponseWriter, r *http.Request) {
	h.timerRequest(w, r, func(id int64, lr engine.LiveRound) error {
		_, err := db.ResumeRoundTimer(r.Context(), h.DB, id, lr.Number, lr.Playoff)
		return err
	})
}

// ClearRoundTimer removes the clock entirely, which undoes a misclick and
// returns the page to showing no timer at all.
func (h *TournamentHandler) ClearRoundTimer(w http.ResponseWriter, r *http.Request) {
	h.timerRequest(w, r, func(id int64, lr engine.LiveRound) error {
		return db.DeleteRoundTimer(r.Context(), h.DB, id, lr.Number, lr.Playoff)
	})
}

// roundTimerFor loads the timer for a tournament's live round, for the pages
// that render it. A tournament with no live round, or a live round with no
// clock, both come back as a nil timer -- the page shows nothing either way.
func roundTimerFor(r *http.Request, database *sql.DB, t *models.Tournament, eng *swisstools.Tournament) (*models.RoundTimer, engine.LiveRound, bool) {
	lr, ok := engine.CurrentLiveRound(t, eng)
	if !ok {
		return nil, engine.LiveRound{}, false
	}
	timer, err := db.GetRoundTimer(r.Context(), database, t.ID, lr.Number, lr.Playoff)
	if err != nil {
		return nil, lr, true
	}
	return timer, lr, true
}
