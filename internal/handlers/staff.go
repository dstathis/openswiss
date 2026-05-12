package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/email"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"
)

type StaffHandler struct {
	DB      *sql.DB
	Tmpl    TemplateRenderer
	Email   *email.Sender
	BaseURL string
}

// StaffPage renders the staff management UI for a single tournament. Admin
// tier required — the page is the entrypoint to grant/update/revoke, which
// are Admin-only operations.
func (h *StaffHandler) StaffPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !middleware.AuthorizeTournament(w, r, h.DB, id, models.TierAdmin) {
		return
	}
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	staff, err := db.ListTournamentStaff(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	h.Tmpl.ExecuteTemplate(w, "tournament_staff.html", map[string]interface{}{
		"User":       middleware.GetUser(r.Context()),
		"Tournament": t,
		"Staff":      staff,
	})
}

// GrantStaff adds a user (looked up by display_name) to the tournament's
// staff at the given tier. Best-effort emails the new staff member.
// Min tier: Admin.
func (h *StaffHandler) GrantStaff(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !middleware.AuthorizeTournament(w, r, h.DB, id, models.TierAdmin) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	tier := models.TournamentTier(r.FormValue("tier"))
	if displayName == "" || !validTier(tier) {
		http.Error(w, "display_name and a valid tier are required", http.StatusBadRequest)
		return
	}
	target, err := db.GetUserByDisplayName(r.Context(), h.DB, displayName)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	t, err := db.GetTournament(r.Context(), h.DB, id)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	granter := middleware.GetUser(r.Context())
	if err := db.AddTournamentStaff(r.Context(), h.DB, &models.TournamentStaff{
		TournamentID: id,
		UserID:       target.ID,
		Tier:         tier,
		GrantedBy:    &granter.ID,
	}); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			http.Error(w, "User is already on staff; change their tier instead", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to grant staff", http.StatusInternalServerError)
		return
	}
	h.sendGrantEmail(target, granter, t, tier)
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

// UpdateStaffTier changes a staff member's tier. Min tier: Admin.
// Last-admin demotions are refused by the DB layer (409).
func (h *StaffHandler) UpdateStaffTier(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !middleware.AuthorizeTournament(w, r, h.DB, id, models.TierAdmin) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	tier := models.TournamentTier(r.FormValue("tier"))
	if !validTier(tier) {
		http.Error(w, "tier must be admin, co_organizer, or judge", http.StatusBadRequest)
		return
	}
	if err := db.UpdateTournamentStaffTier(r.Context(), h.DB, id, userID, tier); err != nil {
		switch {
		case errors.Is(err, db.ErrStaffNotFound):
			http.Error(w, "User is not on staff", http.StatusNotFound)
		case errors.Is(err, db.ErrLastAdmin):
			http.Error(w, "Cannot demote the last admin; promote someone else first", http.StatusConflict)
		default:
			http.Error(w, "Failed to update tier", http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

// RemoveStaff revokes access. A staff member can remove themselves; only
// Admins can remove others.
func (h *StaffHandler) RemoveStaff(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	requester := middleware.GetUser(r.Context())
	if requester == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if requester.ID != userID && !middleware.AuthorizeTournament(w, r, h.DB, id, models.TierAdmin) {
		return
	}
	if err := db.RemoveTournamentStaff(r.Context(), h.DB, id, userID); err != nil {
		switch {
		case errors.Is(err, db.ErrStaffNotFound):
			http.Error(w, "User is not on staff", http.StatusNotFound)
		case errors.Is(err, db.ErrLastAdmin):
			http.Error(w, "Cannot remove the last admin; promote someone else first", http.StatusConflict)
		default:
			http.Error(w, "Failed to remove staff", http.StatusInternalServerError)
		}
		return
	}
	// Self-removal: send the user back to their dashboard (they no longer
	// have access to /manage). Anyone else: back to manage.
	if requester.ID == userID {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tournaments/%d/manage", id), http.StatusSeeOther)
}

func (h *StaffHandler) sendGrantEmail(target, granter *models.User, t *models.Tournament, tier models.TournamentTier) {
	if h.Email == nil || !h.Email.Config.Enabled() {
		return
	}
	url := fmt.Sprintf("%s/tournaments/%d", h.BaseURL, t.ID)
	if err := h.Email.SendStaffGranted(target.Email, granter.DisplayName, string(tier), t.Name, url); err != nil {
		log.Printf("staff grant notification email failed: %v", err)
	}
}

func validTier(t models.TournamentTier) bool {
	switch t {
	case models.TierAdmin, models.TierCoOrganizer, models.TierJudge:
		return true
	}
	return false
}
