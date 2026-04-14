package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
	"github.com/go-chi/chi/v5"
)

type TournamentAPI struct {
	DB *sql.DB
}

func (a *TournamentAPI) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	page, perPage := paginationParams(r)
	tournaments, err := db.ListTournaments(r.Context(), a.DB, status, page, perPage)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list tournaments")
		return
	}
	if tournaments == nil {
		tournaments = []models.Tournament{}
	}
	jsonResponse(w, http.StatusOK, tournaments)
}

func (a *TournamentAPI) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	jsonResponse(w, http.StatusOK, t)
}

func (a *TournamentAPI) Create(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	var t models.Tournament
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t.OrganizerID = user.ID
	if t.Status == "" {
		t.Status = models.TournamentStatusScheduled
	}
	if t.PointsWin == 0 {
		t.PointsWin = 3
	}
	if t.PointsDraw == 0 {
		t.PointsDraw = 1
	}

	if err := db.CreateTournament(r.Context(), a.DB, &t); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create tournament")
		return
	}
	jsonResponse(w, http.StatusCreated, t)
}

func (a *TournamentAPI) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	user := middleware.GetUser(r.Context())
	if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
		jsonError(w, http.StatusForbidden, "forbidden")
		return
	}
	if t.Status != models.TournamentStatusScheduled && t.Status != models.TournamentStatusRegistrationOpen {
		jsonError(w, http.StatusBadRequest, "tournament cannot be modified in current state")
		return
	}

	var update models.Tournament
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if update.Name != "" {
		t.Name = update.Name
	}
	if update.Description != nil {
		t.Description = update.Description
	}
	if update.Location != nil {
		t.Location = update.Location
	}
	if update.ScheduledAt != nil {
		t.ScheduledAt = update.ScheduledAt
	}
	if update.MaxPlayers != 0 {
		t.MaxPlayers = update.MaxPlayers
	}
	if update.NumRounds != nil {
		t.NumRounds = update.NumRounds
	}
	if update.TopCut != 0 {
		t.TopCut = update.TopCut
	}

	if err := db.UpdateTournament(r.Context(), a.DB, t); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to update tournament")
		return
	}
	jsonResponse(w, http.StatusOK, t)
}

func (a *TournamentAPI) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	user := middleware.GetUser(r.Context())
	if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
		jsonError(w, http.StatusForbidden, "forbidden")
		return
	}
	if t.Status != models.TournamentStatusScheduled && t.Status != models.TournamentStatusRegistrationOpen {
		jsonError(w, http.StatusBadRequest, "tournament can only be deleted in scheduled or registration_open state")
		return
	}
	db.DeleteTournament(r.Context(), a.DB, id)
	w.WriteHeader(http.StatusNoContent)
}

func (a *TournamentAPI) OpenRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	user := middleware.GetUser(r.Context())
	if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
		jsonError(w, http.StatusForbidden, "forbidden")
		return
	}
	if t.Status != models.TournamentStatusScheduled {
		jsonError(w, http.StatusBadRequest, "tournament is not in scheduled state")
		return
	}
	db.UpdateTournamentStatus(r.Context(), a.DB, id, models.TournamentStatusRegistrationOpen)
	t.Status = models.TournamentStatusRegistrationOpen
	jsonResponse(w, http.StatusOK, t)
}

func (a *TournamentAPI) Start(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	regs, _ := db.ListRegistrations(r.Context(), a.DB, id)

	err := engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if t.Status != models.TournamentStatusRegistrationOpen && t.Status != models.TournamentStatusScheduled {
				return "", fmt.Errorf("tournament cannot be started from state %s", t.Status)
			}
			state, err := engine.InitTournamentEngine(r.Context(), tx, t, regs)
			if err != nil {
				return "", err
			}
			newEng, err := swisstools.LoadTournament(state)
			if err != nil {
				return "", err
			}
			*eng = newEng
			return models.TournamentStatusInProgress, nil
		})

	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, _ := db.GetTournament(r.Context(), a.DB, id)
	jsonResponse(w, http.StatusOK, t)
}

func (a *TournamentAPI) Finish(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if err := eng.FinishTournament(); err != nil {
				return "", err
			}
			return models.TournamentStatusFinished, nil
		})

	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, _ := db.GetTournament(r.Context(), a.DB, id)
	jsonResponse(w, http.StatusOK, t)
}
