package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/go-chi/chi/v5"
)

// StatsAPI serves player statistics. It reads through the same
// internal/db/stats.go queries as the web page, so the two can never disagree.
type StatsAPI struct {
	DB *sql.DB
}

type statsPlayer struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
}

type statsResponse struct {
	Player      statsPlayer               `json:"player"`
	Career      *models.CareerStats       `json:"career"`
	Tournaments []models.TournamentResult `json:"tournaments"`
}

type matchOpponent struct {
	UserID *int64 `json:"user_id"`
	// DisplayName is the opponent's current name, for linking. NameAtEvent is
	// the name they played under. A rename makes them differ, and neither can
	// be derived from the other, so both are sent.
	DisplayName string `json:"display_name"`
	NameAtEvent string `json:"name_at_event"`
}

type gameRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Draws  int `json:"draws"`
}

type matchResponse struct {
	TournamentID   int64          `json:"tournament_id"`
	TournamentName string         `json:"tournament_name"`
	Round          int            `json:"round"`
	Playoff        bool           `json:"playoff"`
	RoundLabel     string         `json:"round_label"`
	Outcome        string         `json:"outcome"`
	GameRecord     gameRecord     `json:"game_record"`
	Opponent       *matchOpponent `json:"opponent"` // null for a bye
}

type matchesResponse struct {
	Matches []matchResponse `json:"matches"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
	Total   int             `json:"total"`
}

type headToHeadEntry struct {
	Opponent statsPlayer `json:"opponent"`
	Wins     int         `json:"wins"`
	Losses   int         `json:"losses"`
	Draws    int         `json:"draws"`
	Matches  int         `json:"matches"`
}

type headToHeadResponse struct {
	HeadToHead []headToHeadEntry `json:"head_to_head"`
}

// resolvePlayer looks up the {name} path parameter, writing a 404 and
// returning nil when there is no such player.
func (a *StatsAPI) resolvePlayer(w http.ResponseWriter, r *http.Request) *models.User {
	name := chi.URLParam(r, "name")
	user, err := db.GetUserByDisplayName(r.Context(), a.DB, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, http.StatusNotFound, "player not found")
		} else {
			jsonError(w, http.StatusInternalServerError, "internal error")
		}
		return nil
	}
	return user
}

// GetStats returns a player's career totals and per-tournament finishes.
// Only finished tournaments are represented, and decklists never appear.
func (a *StatsAPI) GetStats(w http.ResponseWriter, r *http.Request) {
	user := a.resolvePlayer(w, r)
	if user == nil {
		return
	}
	a.writeStats(w, r, user)
}

// GetMyStats is the same payload for the authenticated caller. It is a
// separate path from /players/{name} so that a player whose display name is
// literally "me" cannot shadow it.
func (a *StatsAPI) GetMyStats(w http.ResponseWriter, r *http.Request) {
	a.writeStats(w, r, middleware.GetUser(r.Context()))
}

func (a *StatsAPI) writeStats(w http.ResponseWriter, r *http.Request, user *models.User) {
	career, err := db.GetCareerStats(r.Context(), a.DB, user.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tournaments, err := db.ListTournamentResultsForUser(r.Context(), a.DB, user.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tournaments == nil {
		tournaments = []models.TournamentResult{}
	}

	jsonResponse(w, http.StatusOK, statsResponse{
		Player:      statsPlayer{ID: user.ID, DisplayName: user.DisplayName},
		Career:      career,
		Tournaments: tournaments,
	})
}

// ListMatches returns a page of a player's match history, newest first.
func (a *StatsAPI) ListMatches(w http.ResponseWriter, r *http.Request) {
	user := a.resolvePlayer(w, r)
	if user == nil {
		return
	}

	page, perPage := paginationParams(r)
	total, err := db.CountMatchesForUser(r.Context(), a.DB, user.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	matches, err := db.ListMatchesForUser(r.Context(), a.DB, user.ID, perPage, (page-1)*perPage)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]matchResponse, 0, len(matches))
	for _, m := range matches {
		mr := matchResponse{
			TournamentID:   m.TournamentID,
			TournamentName: m.TournamentName,
			Round:          m.Round,
			Playoff:        m.Playoff,
			RoundLabel:     m.RoundLabel,
			Outcome:        m.Outcome,
			GameRecord:     gameRecord{Wins: m.GameWins, Losses: m.GameLosses, Draws: m.GameDraws},
		}
		if !m.IsBye() {
			// A guest opponent has no account: UserID stays null and
			// DisplayName falls back to the name they played under.
			display := m.OpponentDisplayName
			if display == "" {
				display = m.OpponentName
			}
			mr.Opponent = &matchOpponent{
				UserID:      m.OpponentUserID,
				DisplayName: display,
				NameAtEvent: m.OpponentName,
			}
		}
		out = append(out, mr)
	}

	jsonResponse(w, http.StatusOK, matchesResponse{
		Matches: out, Page: page, PerPage: perPage, Total: total,
	})
}

// ListHeadToHead returns a player's record against each opponent who has an
// account. Byes and guest opponents are excluded, so these counts
// intentionally do not sum to the career match total.
func (a *StatsAPI) ListHeadToHead(w http.ResponseWriter, r *http.Request) {
	user := a.resolvePlayer(w, r)
	if user == nil {
		return
	}

	h2h, err := db.ListHeadToHeadForUser(r.Context(), a.DB, user.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]headToHeadEntry, 0, len(h2h))
	for _, h := range h2h {
		out = append(out, headToHeadEntry{
			Opponent: statsPlayer{ID: h.OpponentUserID, DisplayName: h.OpponentDisplayName},
			Wins:     h.Wins, Losses: h.Losses, Draws: h.Draws, Matches: h.Matches,
		})
	}

	jsonResponse(w, http.StatusOK, headToHeadResponse{HeadToHead: out})
}
