//go:build integration

package handlers

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestStaffHandler_StaffPage_AdminOK(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &StaffHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner-staff-page@example.com", "OwnerStaffPage")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("GET", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.StaffPage(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if len(tmpl.calls) != 1 || tmpl.calls[0].Name != "tournament_staff.html" {
		t.Errorf("expected one render of tournament_staff.html, got %+v", tmpl.calls)
	}
}

func TestStaffHandler_StaffPage_NonAdminForbidden(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &StaffHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner-staff-403@example.com", "OwnerStaff403")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	// Give 'other' Co-organizer tier — still not Admin, so /staff is 403.
	other := mustCreateUser(t, database, "other-staff-403@example.com", "OtherStaff403")
	if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: other.ID, Tier: models.TierCoOrganizer,
	}); err != nil {
		t.Fatalf("seed co-organizer: %v", err)
	}

	req := requestWithUser("GET", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.StaffPage(rec, req)
	if rec.Code != 403 {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
