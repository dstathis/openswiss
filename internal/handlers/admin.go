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
	"strconv"
	"strings"
	st "github.com/dstathis/swisstools"
)

type AdminHandlers struct {
	storage *storage.TournamentStorage
	auth    *auth.Auth
	tmpl    *template.Template
}

func NewAdminHandlers(storage *storage.TournamentStorage, auth *auth.Auth) *AdminHandlers {
	tmpl := template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	})
	tmpl = template.Must(tmpl.ParseFiles("templates/base.html"))
	// Parse all templates so base.html can access all content blocks
	tmpl = template.Must(tmpl.ParseGlob("templates/player/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("templates/admin/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("templates/auth/*.html"))
	return &AdminHandlers{
		storage: storage,
		auth:    auth,
		tmpl:    tmpl,
	}
}

func (h *AdminHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	tournament := h.storage.GetTournament()
	pending := h.storage.GetPendingPlayers()

	// Build player map from standings (which have names) and pending accepted players
	standingsForLookup := tournament.GetStandings()
	playerMap := make(map[int]string)
	for _, s := range standingsForLookup {
		id, ok := tournament.GetPlayerID(s.Name)
		if ok {
			playerMap[id] = s.Name
		}
	}
	
	// Also include pending players that were accepted (they're in tournament but may not have standings yet)
	for _, pp := range pending {
		if pp.Status == "accepted" {
			if id, ok := tournament.GetPlayerID(pp.Name); ok {
				if _, exists := playerMap[id]; !exists {
					playerMap[id] = pp.Name
				}
			}
		}
	}

	// Get all players
	type PlayerInfo struct {
		ID   int
		Name string
	}
	players := make([]PlayerInfo, 0)
	for id, name := range playerMap {
		players = append(players, PlayerInfo{
			ID:   id,
			Name: name,
		})
	}

	// Prepare pairings for display
	type PairingDisplay struct {
		PlayerA   string
		PlayerB   string
		PlayerAID int
		PlayerBID int
		IsBye     bool
	}
	round := tournament.GetRound()
	pairings := make([]PairingDisplay, len(round))
	for i, p := range round {
		playerBID := p.PlayerB()
		isBye := playerBID == st.BYE_OPPONENT_ID
		pairings[i] = PairingDisplay{
			PlayerA:   playerMap[p.PlayerA()],
			PlayerB:   func() string {
				if isBye {
					return "Bye"
				}
				return playerMap[playerBID]
			}(),
			PlayerAID: p.PlayerA(),
			PlayerBID: playerBID,
			IsBye:     isBye,
		}
	}

	// Prepare standings with player IDs
	type StandingWithID struct {
		st.PlayerStanding
		ID int
		// Add aliases for template compatibility
		MatchWins   int
		MatchLosses int
		MatchDraws  int
	}
	rawStandings := tournament.GetStandings()
	standings := make([]StandingWithID, len(rawStandings))
	for i, s := range rawStandings {
		id, _ := tournament.GetPlayerID(s.Name)
		// The swisstools PlayerStanding has Wins, Losses, Draws which are match-level
		standings[i] = StandingWithID{
			PlayerStanding: s,
			ID:             id,
			MatchWins:      s.Wins,
			MatchLosses:    s.Losses,
			MatchDraws:     s.Draws,
		}
	}

	session, _ := auth.GetSessionFromContext(r.Context())
	data := struct {
		Template   string
		IsAdmin     bool
		IsLoggedIn  bool
		Players     []PlayerInfo
		Pending     []storage.PendingPlayer
		Round       int
		Status      string
		Standings   []StandingWithID
		Pairings    []PairingDisplay
	}{
		Template:   "dashboard",
		IsAdmin:    session != nil && session.Role == auth.RoleAdmin,
		IsLoggedIn: session != nil,
		Players:    players,
		Pending:    pending,
		Round:      tournament.GetCurrentRound(),
		Status:     tournament.GetStatus(),
		Standings:  standings,
		Pairings:   pairings,
	}

	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *AdminHandlers) AcceptPlayer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if err := h.storage.AcceptPlayer(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) RejectPlayer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if err := h.storage.RejectPlayer(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) StartTournament(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = h.storage.UpdateTournament(func(t *st.Tournament) error {
		return t.StartTournament()
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) Pair(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	allowRepair := r.FormValue("allow_repair") == "true"

	var err error
	if err = h.storage.UpdateTournament(func(t *st.Tournament) error {
		return t.Pair(allowRepair)
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) NextRound(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = h.storage.UpdateTournament(func(t *st.Tournament) error {
		return t.NextRound()
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) AddResult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	playerIDStr := r.FormValue("player_id")
	winsStr := r.FormValue("wins")
	lossesStr := r.FormValue("losses")
	drawsStr := r.FormValue("draws")

	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	wins, _ := strconv.Atoi(winsStr)
	losses, _ := strconv.Atoi(lossesStr)
	draws, _ := strconv.Atoi(drawsStr)

	var updateErr error
	if updateErr = h.storage.UpdateTournament(func(t *st.Tournament) error {
		return t.AddResult(playerID, wins, losses, draws)
	}); updateErr != nil {
		http.Error(w, updateErr.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) UpdateStandings(w http.ResponseWriter, r *http.Request) {
	var err error
	if err = h.storage.UpdateTournament(func(t *st.Tournament) error {
		return t.UpdatePlayerStandings()
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandlers) RemovePlayer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	playerIDStr := r.FormValue("player_id")
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	var removeErr error
	if removeErr = h.storage.UpdateTournament(func(t *st.Tournament) error {
		return t.RemovePlayerById(playerID)
	}); removeErr != nil {
		http.Error(w, removeErr.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

