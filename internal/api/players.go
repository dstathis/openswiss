package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
	"github.com/go-chi/chi/v5"
)

type PlayersAPI struct {
	DB *sql.DB
}

func (a *PlayersAPI) List(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	regs, err := db.ListRegistrations(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list players")
		return
	}
	if regs == nil {
		regs = []models.Registration{}
	}

	type playerResponse struct {
		UserID      int64  `json:"user_id"`
		DisplayName string `json:"display_name"`
		Status      string `json:"status"`
	}
	var players []playerResponse
	for _, r := range regs {
		players = append(players, playerResponse{
			UserID:      r.UserID,
			DisplayName: r.DisplayName,
			Status:      r.Status,
		})
	}
	if players == nil {
		players = []playerResponse{}
	}
	jsonResponse(w, http.StatusOK, players)
}

func (a *PlayersAPI) Register(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if t.Status != models.TournamentStatusRegistrationOpen {
		jsonError(w, http.StatusBadRequest, "registration is not open")
		return
	}
	user := middleware.GetUser(r.Context())
	if t.MaxPlayers > 0 {
		count, _ := db.CountRegistrations(r.Context(), a.DB, id)
		if count >= t.MaxPlayers {
			jsonError(w, http.StatusBadRequest, "tournament is full")
			return
		}
	}

	var reg *models.Registration
	if t.RequireDecklist {
		reg, err = db.CreatePendingRegistration(r.Context(), a.DB, id, user.ID)
	} else {
		reg, err = db.CreateRegistration(r.Context(), a.DB, id, user.ID)
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, "already registered or error")
		return
	}
	jsonResponse(w, http.StatusCreated, reg)
}

func (a *PlayersAPI) Unregister(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if t.Status != models.TournamentStatusRegistrationOpen {
		jsonError(w, http.StatusBadRequest, "cannot unregister after tournament started")
		return
	}
	user := middleware.GetUser(r.Context())
	db.DeleteRegistration(r.Context(), a.DB, id, user.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *PlayersAPI) AddPlayer(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		PlayerName string `json:"player_name"`
	}
	if err := decodeJSON(r, &body); err != nil || body.PlayerName == "" {
		jsonError(w, http.StatusBadRequest, "player_name is required")
		return
	}

	// If tournament is in progress, add to engine directly
	if t.EngineState != nil {
		eng, err := swisstools.LoadTournament(t.EngineState)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load engine state")
			return
		}
		if err := eng.AddPlayer(body.PlayerName); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		data, _ := eng.DumpTournament()
		db.UpdateTournamentStatus(r.Context(), a.DB, id, t.Status) // triggers updated_at
		// Need to save engine state - use a simple update
		tx, _ := a.DB.BeginTx(r.Context(), nil)
		db.UpdateTournamentEngineState(r.Context(), tx, id, t.Status, data)
		tx.Commit()
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok", "player_name": body.PlayerName})
}

func (a *PlayersAPI) DropPlayer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	pid, _ := strconv.Atoi(chi.URLParam(r, "pid"))

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

	if t.EngineState != nil {
		eng, err := swisstools.LoadTournament(t.EngineState)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load engine state")
			return
		}
		if err := eng.RemovePlayerById(pid); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		data, _ := eng.DumpTournament()
		tx, _ := a.DB.BeginTx(r.Context(), nil)
		db.UpdateTournamentEngineState(r.Context(), tx, id, t.Status, data)
		tx.Commit()
	}

	// Also update registration status
	reg, err := db.GetRegistrationByEnginePlayerID(r.Context(), a.DB, id, pid)
	if err == nil {
		db.UpdateRegistrationStatus(r.Context(), a.DB, id, reg.UserID, models.RegistrationStatusDropped)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *PlayersAPI) GetDecklist(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	user := middleware.GetUser(r.Context())
	reg, err := db.GetRegistration(r.Context(), a.DB, id, user.ID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not registered")
		return
	}
	if reg.Decklist == nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"main": map[string]int{}, "sideboard": map[string]int{}})
		return
	}
	var dl swisstools.Decklist
	json.Unmarshal(reg.Decklist, &dl)
	jsonResponse(w, http.StatusOK, dl)
}

func (a *PlayersAPI) SubmitDecklist(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	user := middleware.GetUser(r.Context())

	var dl swisstools.Decklist
	if err := decodeJSON(r, &dl); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid decklist")
		return
	}
	data, _ := json.Marshal(dl)
	if err := db.UpdateRegistrationDecklist(r.Context(), a.DB, id, user.ID, data); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save decklist")
		return
	}
	jsonResponse(w, http.StatusOK, dl)
}

func (a *PlayersAPI) GetPlayerDecklist(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	pid, _ := strconv.ParseInt(chi.URLParam(r, "pid"), 10, 64)

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

	reg, err := db.GetRegistration(r.Context(), a.DB, id, pid)
	if err != nil {
		jsonError(w, http.StatusNotFound, "player not found")
		return
	}
	if reg.Decklist == nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{"main": map[string]int{}, "sideboard": map[string]int{}})
		return
	}
	var dl swisstools.Decklist
	json.Unmarshal(reg.Decklist, &dl)
	jsonResponse(w, http.StatusOK, dl)
}

func decodeJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("empty body")
	}
	return json.NewDecoder(r.Body).Decode(v)
}
