package handlers

import (
	"database/sql"
	"net/http"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
)

type PlayerHandler struct {
	DB   *sql.DB
	Tmpl TemplateRenderer
}

func (h *PlayerHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	regs, _ := db.ListUserRegistrations(r.Context(), h.DB, user.ID)

	// Load tournament details for each registration
	type RegWithTournament struct {
		Registration models.Registration
		Tournament   *models.Tournament
	}
	var regList []RegWithTournament
	for _, reg := range regs {
		t, _ := db.GetTournament(r.Context(), h.DB, reg.TournamentID)
		regList = append(regList, RegWithTournament{Registration: reg, Tournament: t})
	}

	h.Tmpl.ExecuteTemplate(w, "dashboard.html", map[string]interface{}{
		"User":          user,
		"Registrations": regList,
	})
}
