package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
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
		RegistrationID int64  `json:"registration_id"`
		UserID         *int64 `json:"user_id,omitempty"`
		DisplayName    string `json:"display_name"`
		IsGuest        bool   `json:"is_guest"`
		Status         string `json:"status"`
	}
	var players []playerResponse
	for _, r := range regs {
		players = append(players, playerResponse{
			RegistrationID: r.ID,
			UserID:         r.UserID,
			DisplayName:    r.DisplayName,
			IsGuest:        r.IsGuest(),
			Status:         r.Status,
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
		reg, err = db.CreatePendingRegistration(r.Context(), a.DB, id, user.ID, user.DisplayName)
	} else {
		reg, err = db.CreateRegistration(r.Context(), a.DB, id, user.ID, user.DisplayName)
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

// AddPlayer manually adds a guest player. Pre-tournament writes a guest
// registration only; mid-tournament also registers the player in the engine.
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
	switch t.Status {
	case models.TournamentStatusScheduled,
		models.TournamentStatusRegistrationOpen,
		models.TournamentStatusInProgress:
	default:
		jsonError(w, http.StatusBadRequest, "cannot add players in this tournament state")
		return
	}

	var body struct {
		PlayerName string `json:"player_name"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.PlayerName) == "" {
		jsonError(w, http.StatusBadRequest, "player_name is required")
		return
	}

	reg, err := db.CreateGuestRegistration(r.Context(), a.DB, id, body.PlayerName)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if t.Status == models.TournamentStatusInProgress {
		err := engine.WithTournamentEngine(r.Context(), a.DB, id,
			func(tx *sql.Tx, _ *models.Tournament, eng *swisstools.Tournament) (string, error) {
				if err := eng.AddPlayer(reg.DisplayName); err != nil {
					return "", err
				}
				playerID, ok := eng.GetPlayerID(reg.DisplayName)
				if !ok {
					return "", fmt.Errorf("player %s not found after adding", reg.DisplayName)
				}
				return "", db.UpdateRegistrationEnginePlayerID(r.Context(), tx, reg.ID, playerID)
			})
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	jsonResponse(w, http.StatusCreated, reg)
}

// DropPlayer removes a player. URL param pid is the engine_player_id when the
// tournament is in progress, or the registration_id when it's not yet started.
func (a *PlayersAPI) DropPlayer(w http.ResponseWriter, r *http.Request) {
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

	// Pre-tournament: pid is interpreted as a registration id and the row is deleted.
	if t.Status == models.TournamentStatusScheduled || t.Status == models.TournamentStatusRegistrationOpen {
		if err := db.DeleteRegistrationByID(r.Context(), a.DB, pid); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	enginePlayerID := int(pid)
	err = engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, _ *models.Tournament, eng *swisstools.Tournament) (string, error) {
			return "", eng.RemovePlayerById(enginePlayerID)
		})
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if reg, err := db.GetRegistrationByEnginePlayerID(r.Context(), a.DB, id, enginePlayerID); err == nil {
		db.UpdateRegistrationStatusByID(r.Context(), a.DB, reg.ID, models.RegistrationStatusDropped)
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetRegistrationDecklist returns the decklist for any registration in a
// tournament the organizer manages (real user or guest).
func (a *PlayersAPI) GetRegistrationDecklist(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	regID, _ := strconv.ParseInt(chi.URLParam(r, "regID"), 10, 64)

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
	reg, err := db.GetRegistrationByID(r.Context(), a.DB, regID)
	if err != nil || reg.TournamentID != id {
		jsonError(w, http.StatusNotFound, "registration not found")
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

// SetRegistrationDecklist lets the organizer submit/replace a decklist on
// behalf of any registration (real user or guest).
func (a *PlayersAPI) SetRegistrationDecklist(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	regID, _ := strconv.ParseInt(chi.URLParam(r, "regID"), 10, 64)

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
	reg, err := db.GetRegistrationByID(r.Context(), a.DB, regID)
	if err != nil || reg.TournamentID != id {
		jsonError(w, http.StatusNotFound, "registration not found")
		return
	}

	var dl swisstools.Decklist
	if err := decodeJSON(r, &dl); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid decklist")
		return
	}
	data, _ := json.Marshal(dl)
	if err := db.UpdateRegistrationDecklistByID(r.Context(), a.DB, regID, data); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to save decklist")
		return
	}
	jsonResponse(w, http.StatusOK, dl)
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
