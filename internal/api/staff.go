package api

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

type StaffAPI struct {
	DB      *sql.DB
	Email   *email.Sender
	BaseURL string
}

// List returns the staff list for a tournament. Public — the staff list is
// shown on each tournament's detail page (per the design decision).
func (a *StaffAPI) List(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	staff, err := db.ListTournamentStaff(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list staff")
		return
	}
	if staff == nil {
		staff = []db.StaffMember{}
	}
	jsonResponse(w, http.StatusOK, staff)
}

// Search returns DisplayName matches for the typeahead on the staff
// management page. Gated to Admin tier on the tournament so it isn't an
// open-to-the-world enumeration vector. Returns at most 10 results; empty
// q returns an empty list.
func (a *StaffAPI) Search(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if !middleware.AuthorizeTournament(w, r, a.DB, id, models.TierAdmin) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		jsonResponse(w, http.StatusOK, []db.UserSearchResult{})
		return
	}
	results, err := db.SearchUsersByDisplayName(r.Context(), a.DB, q, 10)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "search failed")
		return
	}
	if results == nil {
		results = []db.UserSearchResult{}
	}
	jsonResponse(w, http.StatusOK, results)
}

// Grant adds a user as staff at the given tier. Resolves the target by
// `user_id` if provided, otherwise by `display_name`. Sends a best-effort
// email notification on success.
func (a *StaffAPI) Grant(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if !middleware.AuthorizeTournament(w, r, a.DB, id, models.TierAdmin) {
		return
	}
	var body struct {
		UserID      int64                 `json:"user_id"`
		DisplayName string                `json:"display_name"`
		Tier        models.TournamentTier `json:"tier"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target, ok := a.resolveTarget(w, r, body.UserID, body.DisplayName)
	if !ok {
		return
	}
	if !validTier(body.Tier) {
		jsonError(w, http.StatusBadRequest, "tier must be admin, co_organizer, or judge")
		return
	}
	t, err := db.GetTournament(r.Context(), a.DB, id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}

	granter := middleware.GetUser(r.Context())
	staff := &models.TournamentStaff{
		TournamentID: id,
		UserID:       target.ID,
		Tier:         body.Tier,
		GrantedBy:    &granter.ID,
	}
	if err := db.AddTournamentStaff(r.Context(), a.DB, staff); err != nil {
		// Unique-constraint hit => the user is already staff. Direct the
		// caller to use PATCH instead of trying to re-grant.
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			jsonError(w, http.StatusConflict, "user is already on staff; use PATCH to change tier")
			return
		}
		jsonError(w, http.StatusInternalServerError, "failed to grant staff")
		return
	}
	a.sendGrantEmail(target, granter, t, body.Tier)
	jsonResponse(w, http.StatusCreated, db.StaffMember{
		UserID:      target.ID,
		DisplayName: target.DisplayName,
		Tier:        body.Tier,
		GrantedBy:   &granter.ID,
		GrantedAt:   staff.GrantedAt,
	})
}

// UpdateTier changes a staff member's tier. The last-admin safeguard lives
// inside db.UpdateTournamentStaffTier — demoting the only remaining admin
// returns ErrLastAdmin, which we surface as 409.
func (a *StaffAPI) UpdateTier(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if !middleware.AuthorizeTournament(w, r, a.DB, id, models.TierAdmin) {
		return
	}
	var body struct {
		Tier models.TournamentTier `json:"tier"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validTier(body.Tier) {
		jsonError(w, http.StatusBadRequest, "tier must be admin, co_organizer, or judge")
		return
	}
	if err := db.UpdateTournamentStaffTier(r.Context(), a.DB, id, userID, body.Tier); err != nil {
		switch {
		case errors.Is(err, db.ErrStaffNotFound):
			jsonError(w, http.StatusNotFound, "user is not on staff")
		case errors.Is(err, db.ErrLastAdmin):
			jsonError(w, http.StatusConflict, "cannot demote the last admin; promote someone else first")
		default:
			jsonError(w, http.StatusInternalServerError, "failed to update tier")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Remove revokes a user's staff access. Any authenticated user may remove
// themselves; only Admins can remove others. Refuses (409) when the target
// is the last admin.
func (a *StaffAPI) Remove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	// Self-removal is always allowed for an authenticated user; removing
	// someone else requires per-tournament Admin. The DB layer still
	// blocks self-removal of the last admin.
	requester := middleware.GetUser(r.Context())
	if requester == nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if requester.ID != userID && !middleware.AuthorizeTournament(w, r, a.DB, id, models.TierAdmin) {
		return
	}
	if err := db.RemoveTournamentStaff(r.Context(), a.DB, id, userID); err != nil {
		switch {
		case errors.Is(err, db.ErrStaffNotFound):
			jsonError(w, http.StatusNotFound, "user is not on staff")
		case errors.Is(err, db.ErrLastAdmin):
			jsonError(w, http.StatusConflict, "cannot remove the last admin; promote someone else first")
		default:
			jsonError(w, http.StatusInternalServerError, "failed to remove staff")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveTarget looks up the user named in a grant request. If both
// user_id and display_name are present, user_id wins. Writes a 400/404 on
// failure and returns ok=false; in that case the caller should return.
func (a *StaffAPI) resolveTarget(w http.ResponseWriter, r *http.Request, userID int64, displayName string) (*models.User, bool) {
	if userID > 0 {
		u, err := db.GetUserByID(r.Context(), a.DB, userID)
		if err != nil {
			jsonError(w, http.StatusNotFound, "user not found")
			return nil, false
		}
		return u, true
	}
	if dn := strings.TrimSpace(displayName); dn != "" {
		u, err := db.GetUserByDisplayName(r.Context(), a.DB, dn)
		if err != nil {
			jsonError(w, http.StatusNotFound, "user not found")
			return nil, false
		}
		return u, true
	}
	jsonError(w, http.StatusBadRequest, "user_id or display_name is required")
	return nil, false
}

func (a *StaffAPI) sendGrantEmail(target, granter *models.User, t *models.Tournament, tier models.TournamentTier) {
	if a.Email == nil || !a.Email.Config.Enabled() {
		return
	}
	url := fmt.Sprintf("%s/tournaments/%d", a.BaseURL, t.ID)
	if err := a.Email.SendStaffGranted(target.Email, granter.DisplayName, string(tier), t.Name, url); err != nil {
		// Best-effort: log and move on. The grant has already succeeded.
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
