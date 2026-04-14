package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/engine"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/dstathis/swisstools"
	"github.com/go-chi/chi/v5"
)

type TournamentHandler struct {
	DB   *sql.DB
	Tmpl TemplateRenderer
}

type resolvedPairing struct {
	PlayerAID   int
	PlayerBID   int
	PlayerAName string
	PlayerBName string
	PlayerAWins int
	PlayerBWins int
	Draws       int
	IsBye       bool
}

func resolvePairings(eng *swisstools.Tournament, pairings []swisstools.Pairing) []resolvedPairing {
	resolved := make([]resolvedPairing, len(pairings))
	for i, p := range pairings {
		rp := resolvedPairing{
			PlayerAID:   p.PlayerA(),
			PlayerBID:   p.PlayerB(),
			PlayerAWins: max(p.PlayerAWins(), 0),
			PlayerBWins: max(p.PlayerBWins(), 0),
			Draws:       max(p.Draws(), 0),
			IsBye:       p.PlayerB() == swisstools.BYE_OPPONENT_ID,
		}
		if player, ok := eng.GetPlayerById(p.PlayerA()); ok {
			rp.PlayerAName = player.Name
		}
		if player, ok := eng.GetPlayerById(p.PlayerB()); ok {
			rp.PlayerBName = player.Name
		}
		resolved[i] = rp
	}
	return resolved
}

func (h *TournamentHandler) Home(w http.ResponseWriter, r *http.Request) {
	tournaments, _ := db.ListUpcomingTournaments(r.Context(), h.DB, 20)
	h.Tmpl.ExecuteTemplate(w, "home.html", map[string]interface{}{
		"User":        middleware.GetUser(r.Context()),
		"Tournaments": tournaments,
	})
}

func (h *TournamentHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	tournaments, _ := db.ListTournaments(r.Context(), h.DB, status, 1, 50)
	h.Tmpl.ExecuteTemplate(w, "tournaments.html", map[string]interface{}{
		"User":        middleware.GetUser(r.Context()),
		"Tournaments": tournaments,
		"Status":      status,
	})
}

func (h *TournamentHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	regs, _ := db.ListRegistrations(r.Context(), h.DB, id)

	user := middleware.GetUser(r.Context())
	var myReg *models.Registration
	if user != nil {
		for i := range regs {
			if regs[i].UserID == user.ID {
				myReg = &regs[i]
				break
			}
		}
	}

	// Load engine for standings/pairings if in progress
	var standings []swisstools.PlayerStanding
	var pairings []swisstools.Pairing
	var currentRound int
	if t.EngineState != nil && len(t.EngineState) > 0 {
		eng, err := swisstools.LoadTournament(t.EngineState)
		if err == nil {
			standings = eng.GetStandings()
			pairings = eng.GetRound()
			currentRound = eng.GetCurrentRound()
		}
	}

	canManage := user != nil && (t.OrganizerID == user.ID || user.HasRole(models.RoleAdmin))
	h.Tmpl.ExecuteTemplate(w, "tournament_detail.html", map[string]interface{}{
		"User":           user,
		"Tournament":     t,
		"Registrations":  regs,
		"MyRegistration": myReg,
		"Standings":      standings,
		"Pairings":       pairings,
		"CurrentRound":   currentRound,
		"CanManage":      canManage,
	})
}

func (h *TournamentHandler) NewPage(w http.ResponseWriter, r *http.Request) {
	h.Tmpl.ExecuteTemplate(w, "tournament_new.html", map[string]interface{}{
		"User": middleware.GetUser(r.Context()),
	})
}

func (h *TournamentHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	t := &models.Tournament{
		Name:            r.FormValue("name"),
		OrganizerID:     user.ID,
		RequireDecklist: r.FormValue("require_decklist") == "on",
		DecklistPublic:  r.FormValue("decklist_public") == "on",
		PointsWin:       3,
		PointsDraw:      1,
		PointsLoss:      0,
		Status:          models.TournamentStatusScheduled,
	}
	if desc := r.FormValue("description"); desc != "" {
		t.Description = &desc
	}
	if loc := r.FormValue("location"); loc != "" {
		t.Location = &loc
	}
	if sa := r.FormValue("scheduled_at"); sa != "" {
		if parsed, err := time.Parse("2006-01-02T15:04", sa); err == nil {
			t.ScheduledAt = &parsed
		}
	}
	if mp := r.FormValue("max_players"); mp != "" {
		if v, err := strconv.Atoi(mp); err == nil {
			t.MaxPlayers = v
		}
	}
	if nr := r.FormValue("num_rounds"); nr != "" {
		if v, err := strconv.Atoi(nr); err == nil {
			t.NumRounds = &v
		}
	}
	if tc := r.FormValue("top_cut"); tc != "" {
		if v, err := strconv.Atoi(tc); err == nil {
			t.TopCut = v
		}
	}
	if pw := r.FormValue("points_win"); pw != "" {
		if v, err := strconv.Atoi(pw); err == nil {
			t.PointsWin = v
		}
	}
	if pd := r.FormValue("points_draw"); pd != "" {
		if v, err := strconv.Atoi(pd); err == nil {
			t.PointsDraw = v
		}
	}
	if pl := r.FormValue("points_loss"); pl != "" {
		if v, err := strconv.Atoi(pl); err == nil {
			t.PointsLoss = v
		}
	}

	if err := db.CreateTournament(r.Context(), h.DB, t); err != nil {
		h.Tmpl.ExecuteTemplate(w, "tournament_new.html", map[string]interface{}{
			"User":  user,
			"Error": "Failed to create tournament.",
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d", t.ID), http.StatusSeeOther)
}

func (h *TournamentHandler) EditTournament(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	user := middleware.GetUser(r.Context())
	if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if t.Status != models.TournamentStatusScheduled && t.Status != models.TournamentStatusRegistrationOpen {
		http.Error(w, "Cannot edit a tournament that has already started", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	t.Name = r.FormValue("name")
	t.RequireDecklist = r.FormValue("require_decklist") == "on"
	t.DecklistPublic = r.FormValue("decklist_public") == "on"
	if desc := r.FormValue("description"); desc != "" {
		t.Description = &desc
	} else {
		t.Description = nil
	}
	if loc := r.FormValue("location"); loc != "" {
		t.Location = &loc
	} else {
		t.Location = nil
	}
	if sa := r.FormValue("scheduled_at"); sa != "" {
		if parsed, err := time.Parse("2006-01-02T15:04", sa); err == nil {
			t.ScheduledAt = &parsed
		}
	} else {
		t.ScheduledAt = nil
	}
	if mp := r.FormValue("max_players"); mp != "" {
		if v, err := strconv.Atoi(mp); err == nil {
			t.MaxPlayers = v
		}
	}
	if nr := r.FormValue("num_rounds"); nr != "" {
		if v, err := strconv.Atoi(nr); err == nil {
			t.NumRounds = &v
		}
	} else {
		t.NumRounds = nil
	}
	if tc := r.FormValue("top_cut"); tc != "" {
		if v, err := strconv.Atoi(tc); err == nil {
			t.TopCut = v
		}
	}
	if pw := r.FormValue("points_win"); pw != "" {
		if v, err := strconv.Atoi(pw); err == nil {
			t.PointsWin = v
		}
	}
	if pd := r.FormValue("points_draw"); pd != "" {
		if v, err := strconv.Atoi(pd); err == nil {
			t.PointsDraw = v
		}
	}
	if pl := r.FormValue("points_loss"); pl != "" {
		if v, err := strconv.Atoi(pl); err == nil {
			t.PointsLoss = v
		}
	}

	if err := db.UpdateTournament(r.Context(), h.DB, t); err != nil {
		http.Error(w, "Failed to update tournament", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) ManagePage(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	user := middleware.GetUser(r.Context())
	if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	regs, _ := db.ListRegistrations(r.Context(), h.DB, id)

	var standings []swisstools.PlayerStanding
	var pairings []resolvedPairing
	var currentRound int
	var playoffStatus string
	var playoffPairings []resolvedPairing
	if t.EngineState != nil && len(t.EngineState) > 0 {
		eng, err := swisstools.LoadTournament(t.EngineState)
		if err == nil {
			standings = eng.GetStandings()
			pairings = resolvePairings(&eng, eng.GetRound())
			currentRound = eng.GetCurrentRound()
			playoffStatus = eng.GetPlayoffStatus()
			playoffPairings = resolvePairings(&eng, eng.GetPlayoffRound())
		}
	}

	h.Tmpl.ExecuteTemplate(w, "tournament_manage.html", map[string]interface{}{
		"User":            user,
		"Tournament":      t,
		"Registrations":   regs,
		"Standings":       standings,
		"Pairings":        pairings,
		"CurrentRound":    currentRound,
		"PlayoffStatus":   playoffStatus,
		"PlayoffPairings": playoffPairings,
	})
}

func (h *TournamentHandler) OpenRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	user := middleware.GetUser(r.Context())
	if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if t.Status != models.TournamentStatusScheduled {
		http.Error(w, "Tournament is not in scheduled state", http.StatusBadRequest)
		return
	}
	db.UpdateTournamentStatus(r.Context(), h.DB, id, models.TournamentStatusRegistrationOpen)
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) Register(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if t.Status != models.TournamentStatusRegistrationOpen {
		http.Error(w, "Registration is not open", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())

	if t.MaxPlayers > 0 {
		count, _ := db.CountRegistrations(r.Context(), h.DB, id)
		if count >= t.MaxPlayers {
			http.Error(w, "Tournament is full", http.StatusBadRequest)
			return
		}
	}

	if t.RequireDecklist {
		db.CreatePendingRegistration(r.Context(), h.DB, id, user.ID)
	} else {
		db.CreateRegistration(r.Context(), h.DB, id, user.ID)
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d", id), http.StatusSeeOther)
}

func (h *TournamentHandler) Unregister(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if t.Status != models.TournamentStatusRegistrationOpen {
		http.Error(w, "Cannot unregister after tournament started", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	db.DeleteRegistration(r.Context(), h.DB, id, user.ID)
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d", id), http.StatusSeeOther)
}

func (h *TournamentHandler) DecklistPage(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	user := middleware.GetUser(r.Context())
	reg, err := db.GetRegistration(r.Context(), h.DB, id, user.ID)
	if err != nil {
		http.Error(w, "Not registered", http.StatusNotFound)
		return
	}
	t, _ := db.GetTournament(r.Context(), h.DB, id)

	var deckText string
	if reg.Decklist != nil {
		var dl swisstools.Decklist
		if json.Unmarshal(reg.Decklist, &dl) == nil {
			deckText = formatDecklist(dl)
		}
	}

	h.Tmpl.ExecuteTemplate(w, "decklist.html", map[string]interface{}{
		"User":       user,
		"Tournament": t,
		"DeckText":   deckText,
	})
}

func (h *TournamentHandler) SubmitDecklist(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	deckText := r.FormValue("decklist")
	dl := parseDecklist(deckText)
	data, _ := json.Marshal(dl)
	db.UpdateRegistrationDecklist(r.Context(), h.DB, id, user.ID, data)
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d", id), http.StatusSeeOther)
}

func (h *TournamentHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	regs, _ := db.ListRegistrations(r.Context(), h.DB, id)

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if t.Status != models.TournamentStatusRegistrationOpen && t.Status != models.TournamentStatusScheduled {
				return "", fmt.Errorf("tournament cannot be started from state %s", t.Status)
			}

			// Initialize the engine: add all confirmed players, start tournament
			state, err := engine.InitTournamentEngine(r.Context(), tx, t, regs)
			if err != nil {
				return "", err
			}

			// We need to load the initialized state into eng
			newEng, err := swisstools.LoadTournament(state)
			if err != nil {
				return "", err
			}
			*eng = newEng
			return models.TournamentStatusInProgress, nil
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) SubmitResults(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}

			// Parse results from form: wins_a_<playerID>, wins_b_<playerID>, draws_<playerID>
			for key := range r.Form {
				if !strings.HasPrefix(key, "wins_a_") {
					continue
				}
				playerIDStr := strings.TrimPrefix(key, "wins_a_")
				playerID, err := strconv.Atoi(playerIDStr)
				if err != nil {
					continue
				}
				wins, _ := strconv.Atoi(r.FormValue("wins_a_" + playerIDStr))
				losses, _ := strconv.Atoi(r.FormValue("wins_b_" + playerIDStr))
				draws, _ := strconv.Atoi(r.FormValue("draws_" + playerIDStr))
				if err := eng.AddResult(playerID, wins, losses, draws); err != nil {
					return "", fmt.Errorf("adding result for player %d: %w", playerID, err)
				}
			}
			return "", nil
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) NextRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if err := eng.NextRound(); err != nil {
				return "", err
			}
			// Check if tournament auto-finished (max rounds reached)
			if eng.GetStatus() == "finished" {
				return models.TournamentStatusFinished, nil
			}
			if err := eng.Pair(false); err != nil {
				return "", err
			}
			return "", nil
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) RepairRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if err := eng.Pair(true); err != nil {
				return "", err
			}
			return "", nil
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) Finish(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			if err := eng.FinishTournament(); err != nil {
				return "", err
			}
			if t.TopCut > 0 {
				return models.TournamentStatusFinished, nil
			}
			return models.TournamentStatusFinished, nil
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) AddPlayer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	playerName := r.FormValue("player_name")

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			return "", eng.AddPlayer(playerName)
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) DropPlayer(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	playerIDStr := r.FormValue("player_id")
	playerID, err := strconv.Atoi(playerIDStr)
	if err != nil {
		http.Error(w, "Invalid player ID", http.StatusBadRequest)
		return
	}

	err = engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			return "", eng.RemovePlayerById(playerID)
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) StartPlayoff(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) PlayoffResults(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
		func(tx *sql.Tx, t *models.Tournament, eng *swisstools.Tournament) (string, error) {
			user := middleware.GetUser(r.Context())
			if t.OrganizerID != user.ID && !user.HasRole(models.RoleAdmin) {
				return "", fmt.Errorf("forbidden")
			}
			for key := range r.Form {
				if !strings.HasPrefix(key, "wins_a_") {
					continue
				}
				playerIDStr := strings.TrimPrefix(key, "wins_a_")
				playerID, err := strconv.Atoi(playerIDStr)
				if err != nil {
					continue
				}
				wins, _ := strconv.Atoi(r.FormValue("wins_a_" + playerIDStr))
				losses, _ := strconv.Atoi(r.FormValue("wins_b_" + playerIDStr))
				draws, _ := strconv.Atoi(r.FormValue("draws_" + playerIDStr))
				if err := eng.AddPlayoffResult(playerID, wins, losses, draws); err != nil {
					return "", fmt.Errorf("adding playoff result for player %d: %w", playerID, err)
				}
			}
			return "", nil
		})

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) NextPlayoffRound(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	err := engine.WithTournamentEngine(r.Context(), h.DB, id,
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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *TournamentHandler) RequestDrop(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	user := middleware.GetUser(r.Context())
	db.UpdateRegistrationStatus(r.Context(), h.DB, id, user.ID, models.RegistrationStatusDropped)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Helpers

func parseDecklist(text string) swisstools.Decklist {
	dl := swisstools.Decklist{
		Main:      map[string]int{},
		Sideboard: map[string]int{},
	}
	inSideboard := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "sideboard") {
			if strings.EqualFold(line, "sideboard") {
				inSideboard = true
			}
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		qty, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if inSideboard {
			dl.Sideboard[name] = qty
		} else {
			dl.Main[name] = qty
		}
	}
	return dl
}

func formatDecklist(dl swisstools.Decklist) string {
	var sb strings.Builder
	for name, qty := range dl.Main {
		fmt.Fprintf(&sb, "%d %s\n", qty, name)
	}
	if len(dl.Sideboard) > 0 {
		sb.WriteString("\nSideboard\n")
		for name, qty := range dl.Sideboard {
			fmt.Fprintf(&sb, "%d %s\n", qty, name)
		}
	}
	return sb.String()
}
