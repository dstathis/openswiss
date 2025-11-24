// This file is part of OpenSwiss.
//
// OpenSwiss is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// OpenSwiss is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with OpenSwiss. If not, see <https://www.gnu.org/licenses/>.

package handlers

import (
	"html/template"
	"net/http"
	"openswiss/internal/auth"
	"openswiss/internal/storage"
	st "github.com/dstathis/swisstools"
)

type PlayerHandlers struct {
	storage *storage.TournamentStorage
	auth    *auth.Auth
	tmpl    *template.Template
}

func NewPlayerHandlers(storage *storage.TournamentStorage, auth *auth.Auth) *PlayerHandlers {
	tmpl := template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	})
	tmpl = template.Must(tmpl.ParseFiles("templates/base.html"))
	// Parse all templates so base.html can access all content blocks
	tmpl = template.Must(tmpl.ParseGlob("templates/player/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("templates/admin/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("templates/auth/*.html"))
	return &PlayerHandlers{
		storage: storage,
		auth:    auth,
		tmpl:    tmpl,
	}
}

func (h *PlayerHandlers) RegisterGet(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Template string
		Error    string
		Success  string
		IsAdmin  bool
		IsLoggedIn bool
	}{
		Template: "register",
	}
	session, _ := auth.GetSessionFromContext(r.Context())
	if session != nil {
		data.IsAdmin = session.Role == auth.RoleAdmin
		data.IsLoggedIn = true
	}

	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *PlayerHandlers) RegisterPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	session, _ := auth.GetSessionFromContext(r.Context())
	data := struct {
		Template  string
		Error     string
		Success   string
		IsAdmin   bool
		IsLoggedIn bool
	}{
		Template: "register",
	}
	if session != nil {
		data.IsAdmin = session.Role == auth.RoleAdmin
		data.IsLoggedIn = true
	}
	
	if name == "" {
		data.Error = "Name is required"
		if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := h.storage.AddPendingPlayer(name); err != nil {
		data.Error = err.Error()
		if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Auto-login as player after registration
	sessionID := h.auth.LoginPlayer()
	h.auth.SetSessionCookie(w, sessionID)
	data.IsLoggedIn = true

	data.Success = "Registration submitted! Waiting for admin approval."
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *PlayerHandlers) Standings(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.GetSessionFromContext(r.Context())
	tournament := h.storage.GetTournament()
	standings := tournament.GetStandings()

	data := struct {
		Template   string
		Standings  []interface{}
		Round      int
		Status     string
		IsAdmin    bool
		IsLoggedIn bool
	}{
		Template:   "standings",
		Standings:   make([]interface{}, len(standings)),
		Round:       tournament.GetCurrentRound(),
		Status:      tournament.GetStatus(),
		IsAdmin:     session != nil && session.Role == auth.RoleAdmin,
		IsLoggedIn:  session != nil,
	}

	for i, s := range standings {
		data.Standings[i] = s
	}

	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *PlayerHandlers) Pairings(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.GetSessionFromContext(r.Context())
	tournament := h.storage.GetTournament()
	round := tournament.GetRound()

	// Get all players for lookup (build from standings since Player fields are unexported)
	standingsForLookup := tournament.GetStandings()
	players := make(map[int]string)
	for _, s := range standingsForLookup {
		id, ok := tournament.GetPlayerID(s.Name)
		if ok {
			players[id] = s.Name
		}
	}
	
	// Also get from pending players that were accepted
	pending := h.storage.GetPendingPlayers()
	for _, pp := range pending {
		if pp.Status == "accepted" {
			if id, ok := tournament.GetPlayerID(pp.Name); ok {
				if _, exists := players[id]; !exists {
					players[id] = pp.Name
				}
			}
		}
	}

	type PairingDisplay struct {
		PlayerA     string
		PlayerB     string
		PlayerAID   int
		PlayerBID   int
		IsBye       bool
	}

	pairings := make([]PairingDisplay, len(round))
	for i, p := range round {
		playerBID := p.PlayerB()
		isBye := playerBID == st.BYE_OPPONENT_ID
		
		pairings[i] = PairingDisplay{
			PlayerA:   players[p.PlayerA()],
			PlayerB:   func() string {
				if isBye {
					return "Bye"
				}
				return players[playerBID]
			}(),
			PlayerAID: p.PlayerA(),
			PlayerBID: playerBID,
			IsBye:     isBye,
		}
	}

	data := struct {
		Template   string
		Pairings   []PairingDisplay
		Round      int
		Status     string
		IsAdmin    bool
		IsLoggedIn bool
	}{
		Template:   "pairings",
		Pairings:   pairings,
		Round:      tournament.GetCurrentRound(),
		Status:     tournament.GetStatus(),
		IsAdmin:    session != nil && session.Role == auth.RoleAdmin,
		IsLoggedIn: session != nil,
	}

	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *PlayerHandlers) Home(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.GetSessionFromContext(r.Context())
	tournament := h.storage.GetTournament()

	data := struct {
		Template    string
		IsPlayer    bool
		IsAdmin     bool
		IsLoggedIn  bool
		Round       int
		Status      string
		PlayerCount int
	}{
		Template:    "home",
		IsPlayer:    session != nil && session.Role == auth.RolePlayer,
		IsAdmin:     session != nil && session.Role == auth.RoleAdmin,
		IsLoggedIn:  session != nil,
		Round:       tournament.GetCurrentRound(),
		Status:      tournament.GetStatus(),
		PlayerCount: tournament.GetPlayerCount(),
	}

	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

