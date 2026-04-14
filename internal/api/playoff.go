package api

import (
	"database/sql"
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

type PlayoffAPI struct {
	DB *sql.DB
}

func (a *PlayoffAPI) Start(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if t.TopCut <= 0 {
				return "", fmt.Errorf("tournament has no top cut configured")
			}
			if err := eng.StartPlayoff(t.TopCut); err != nil {
				return "", err
			}
			return models.TournamentStatusPlayoff, nil
		})

	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *PlayoffAPI) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if t.EngineState == nil {
		jsonError(w, http.StatusBadRequest, "tournament not started")
		return
	}
	eng, err := swisstools.LoadTournament(t.EngineState)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to load engine state")
		return
	}
	playoff := eng.GetPlayoff()
	if playoff == nil {
		jsonError(w, http.StatusBadRequest, "no playoff started")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":        eng.GetPlayoffStatus(),
		"current_round": playoff.CurrentRound,
		"seeds":         playoff.Seeds,
		"finished":      playoff.Finished,
	})
}

func (a *PlayoffAPI) GetCurrentRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if t.EngineState == nil {
		jsonError(w, http.StatusBadRequest, "tournament not started")
		return
	}
	eng, err := swisstools.LoadTournament(t.EngineState)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to load engine state")
		return
	}
	pairings := eng.GetPlayoffRound()
	if pairings == nil {
		jsonError(w, http.StatusBadRequest, "no playoff started")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"pairings": formatPairings(pairings),
	})
}

func (a *PlayoffAPI) SubmitResults(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	var batch resultBatch
	if err := decodeJSON(r, &batch); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			for _, res := range batch.Results {
				if err := eng.AddPlayoffResult(res.PlayerID, res.Wins, res.Losses, res.Draws); err != nil {
					return "", fmt.Errorf("player %d: %w", res.PlayerID, err)
				}
			}
			return "", nil
		})

	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *PlayoffAPI) NextRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if err := eng.NextPlayoffRound(); err != nil {
				return "", err
			}
			if eng.GetPlayoffStatus() == "finished" {
				return models.TournamentStatusFinished, nil
			}
			return "", nil
		})

	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
