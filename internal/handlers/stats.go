package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/go-chi/chi/v5"
)

// maxMatchHistory caps the match list rendered on the page. The API paginates
// properly; the page shows one screenful of history per player, which is
// plenty for a hobby-scale event history and keeps the response bounded.
const maxMatchHistory = 500

type StatsHandler struct {
	DB   *sql.DB
	Tmpl TemplateRenderer
}

// MyStats sends a signed-in player to their own public page, so there is one
// canonical URL for a player's statistics rather than two.
func (h *StatsHandler) MyStats(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	http.Redirect(w, r, "/players/"+url.PathEscape(user.DisplayName), http.StatusFound)
}

// tournamentMatches groups a player's matches under the event they were played
// in, which is how the history reads on the page.
type tournamentMatches struct {
	TournamentID   int64
	TournamentName string
	Matches        []models.PlayerMatch
}

// PlayerStats renders a player's public statistics page.
//
// Public because it re-indexes data that is already public: tournament
// standings, pairings and results are all served unauthenticated. Decklists
// are deliberately not included, so the per-tournament decklist_public setting
// is never in play here.
func (h *StatsHandler) PlayerStats(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	profile, err := db.GetUserByDisplayName(r.Context(), h.DB, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	career, err := db.GetCareerStats(r.Context(), h.DB, profile.ID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	tournaments, err := db.ListTournamentResultsForUser(r.Context(), h.DB, profile.ID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	matches, err := db.ListMatchesForUser(r.Context(), h.DB, profile.ID, maxMatchHistory, 0)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	headToHead, err := db.ListHeadToHeadForUser(r.Context(), h.DB, profile.ID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	user := middleware.GetUser(r.Context())
	h.Tmpl.ExecuteTemplate(w, "player_stats.html", map[string]interface{}{
		// User is the viewer (the layout nav reads it); Profile is the player
		// being looked at. On a public view they are different people.
		"User":        user,
		"Profile":     profile,
		"IsSelf":      user != nil && user.ID == profile.ID,
		"Career":      career,
		"Tournaments": tournaments,
		"History":     groupByTournament(matches),
		"HeadToHead":  headToHead,
	})
}

// groupByTournament preserves the order the matches arrive in, which is newest
// event first and then round order within an event.
func groupByTournament(matches []models.PlayerMatch) []tournamentMatches {
	var out []tournamentMatches
	for _, m := range matches {
		if n := len(out); n > 0 && out[n-1].TournamentID == m.TournamentID {
			out[n-1].Matches = append(out[n-1].Matches, m)
			continue
		}
		out = append(out, tournamentMatches{
			TournamentID:   m.TournamentID,
			TournamentName: m.TournamentName,
			Matches:        []models.PlayerMatch{m},
		})
	}
	return out
}
