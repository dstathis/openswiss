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

type RoundsAPI struct {
	DB *sql.DB
}

func (a *RoundsAPI) ListRounds(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if t.EngineState == nil {
		jsonResponse(w, http.StatusOK, []interface{}{})
		return
	}
	eng, err := swisstools.LoadTournament(t.EngineState)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to load engine state")
		return
	}

	type roundData struct {
		RoundNumber int               `json:"round_number"`
		Pairings    []pairingResponse `json:"pairings"`
	}
	var rounds []roundData
	for i := 1; i <= eng.GetCurrentRound(); i++ {
		pairings, err := eng.GetRoundByNumber(i)
		if err != nil {
			continue
		}
		rounds = append(rounds, roundData{
			RoundNumber: i,
			Pairings:    formatPairings(pairings),
		})
	}
	if rounds == nil {
		rounds = []roundData{}
	}
	jsonResponse(w, http.StatusOK, rounds)
}

func (a *RoundsAPI) GetCurrentRound(w http.ResponseWriter, r *http.Request) {
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
	pairings := eng.GetRound()
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"round_number": eng.GetCurrentRound(),
		"pairings":     formatPairings(pairings),
	})
}

func (a *RoundsAPI) GetRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	roundNum, _ := strconv.Atoi(chi.URLParam(r, "round"))
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
	pairings, err := eng.GetRoundByNumber(roundNum)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"round_number": roundNum,
		"pairings":     formatPairings(pairings),
	})
}

type resultBatch struct {
	Results []resultEntry `json:"results"`
}

type resultEntry struct {
	PlayerID int `json:"player_id"`
	Wins     int `json:"wins"`
	Losses   int `json:"losses"`
	Draws    int `json:"draws"`
}

func (a *RoundsAPI) SubmitResults(w http.ResponseWriter, r *http.Request) {
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
				if err := eng.AddResult(res.PlayerID, res.Wins, res.Losses, res.Draws); err != nil {
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

func (a *RoundsAPI) NextRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), a.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if err := eng.NextRound(); err != nil {
				return "", err
			}
			if eng.GetStatus() == "finished" {
				return models.TournamentStatusFinished, nil
			}
			if err := eng.Pair(false); err != nil {
				return "", err
			}
			return "", nil
		})

	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *RoundsAPI) GetStandings(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if t.EngineState == nil {
		jsonResponse(w, http.StatusOK, []interface{}{})
		return
	}
	eng, err := swisstools.LoadTournament(t.EngineState)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to load engine state")
		return
	}
	jsonResponse(w, http.StatusOK, eng.GetStandings())
}

// Helpers

type pairingResponse struct {
	PlayerA     int  `json:"player_a"`
	PlayerB     int  `json:"player_b"`
	PlayerAWins int  `json:"player_a_wins"`
	PlayerBWins int  `json:"player_b_wins"`
	Draws       int  `json:"draws"`
	IsBye       bool `json:"is_bye"`
}

func formatPairings(pairings []swisstools.Pairing) []pairingResponse {
	result := make([]pairingResponse, 0, len(pairings))
	for _, p := range pairings {
		result = append(result, pairingResponse{
			PlayerA:     p.PlayerA(),
			PlayerB:     p.PlayerB(),
			PlayerAWins: p.PlayerAWins(),
			PlayerBWins: p.PlayerBWins(),
			Draws:       p.Draws(),
			IsBye:       p.PlayerB() == swisstools.BYE_OPPONENT_ID,
		})
	}
	return result
}
