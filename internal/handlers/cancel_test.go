//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

// cancelReq builds the POST that the Cancel Tournament button issues.
func cancelReq(user *models.User, tournamentID int64) *http.Request {
	return requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tournamentID, 10)})
}

// assertGone fails unless the tournament row is really deleted.
func assertGone(t *testing.T, database *sql.DB, id int64) {
	t.Helper()
	if _, err := db.GetTournament(context.Background(), database, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tournament %d still exists (GetTournament err = %v)", id, err)
	}
}

// assertPresent fails unless the tournament row survived, with its status intact.
func assertPresent(t *testing.T, database *sql.DB, id int64, wantStatus string) {
	t.Helper()
	got, err := db.GetTournament(context.Background(), database, id)
	if err != nil {
		t.Fatalf("tournament %d was deleted, want it kept: %v", id, err)
	}
	if got.Status != wantStatus {
		t.Errorf("status = %q, want %q", got.Status, wantStatus)
	}
}

func TestTournamentHandler_Cancel_Scheduled(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(owner, tourn.ID))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/tournaments" {
		t.Errorf("Location = %q, want /tournaments", loc)
	}
	assertGone(t, database, tourn.ID)
}

func TestTournamentHandler_Cancel_RegistrationOpen(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(owner, tourn.ID))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, tourn.ID)
}

// Registrations hang off the tournament by FK; cancelling must take them with
// it (ON DELETE CASCADE) without touching the underlying user accounts.
func TestTournamentHandler_Cancel_DeletesRegistrationsKeepsUsers(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	player := mustCreateUser(t, database, "player@example.com", "Player")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, player.ID, player.DisplayName); err != nil {
		t.Fatalf("register player: %v", err)
	}
	if regs, _ := db.ListRegistrations(ctx, database, tourn.ID); len(regs) != 1 {
		t.Fatalf("setup: expected 1 registration, got %d", len(regs))
	}

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(owner, tourn.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	assertGone(t, database, tourn.ID)
	var regCount int
	if err := database.QueryRow(`SELECT count(*) FROM registrations WHERE tournament_id = $1`, tourn.ID).Scan(&regCount); err != nil {
		t.Fatalf("count registrations: %v", err)
	}
	if regCount != 0 {
		t.Errorf("registrations left behind: %d", regCount)
	}
	if _, err := db.GetUserByID(ctx, database, player.ID); err != nil {
		t.Errorf("player account was destroyed by cancel: %v", err)
	}
}

// Staff rows are the permission table; they must cascade too, or a re-used
// tournament id would inherit stale grants.
func TestTournamentHandler_Cancel_DeletesStaffRows(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	judge := mustCreateUser(t, database, "judge@example.com", "Judge")
	if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: judge.ID, Tier: models.TierJudge,
	}); err != nil {
		t.Fatalf("seed judge: %v", err)
	}

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(owner, tourn.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var staffCount int
	if err := database.QueryRow(`SELECT count(*) FROM tournament_staff WHERE tournament_id = $1`, tourn.ID).Scan(&staffCount); err != nil {
		t.Fatalf("count staff: %v", err)
	}
	if staffCount != 0 {
		t.Errorf("staff rows left behind: %d", staffCount)
	}
}

// Cancel must delete only its target.
func TestTournamentHandler_Cancel_LeavesOtherTournaments(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	victim := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	bystander := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(owner, victim.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, victim.ID)
	assertPresent(t, database, bystander.ID, models.TournamentStatusScheduled)
}

// A global site admin outranks the staff table and may cancel anyone's
// not-yet-started tournament.
func TestTournamentHandler_Cancel_SiteAdmin(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	siteAdmin := mustCreateUser(t, database, "admin@example.com", "SiteAdmin", models.RoleAdmin)

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(siteAdmin, tourn.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, tourn.ID)
}

// Anything at or past the start line is off limits: deleting it would destroy
// played rounds and standings.
func TestTournamentHandler_Cancel_StartedStates(t *testing.T) {
	for _, status := range []string{
		models.TournamentStatusInProgress,
		models.TournamentStatusPlayoff,
		models.TournamentStatusFinished,
	} {
		t.Run(status, func(t *testing.T) {
			database := testDB(t)
			h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
			owner := mustCreateUser(t, database, "owner@example.com", "Owner")
			tourn := mustCreateTournament(t, database, owner.ID, status)

			rec := httptest.NewRecorder()
			h.Cancel(rec, cancelReq(owner, tourn.ID))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
			}
			assertPresent(t, database, tourn.ID, status)
		})
	}
}

// Same rule, but against a tournament genuinely driven through the engine
// rather than one with a hand-set status column.
func TestTournamentHandler_Cancel_ReallyStartedTournament(t *testing.T) {
	database := testDB(t)
	owner, tourn := startedTournament(t, database)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(owner, tourn.ID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	assertPresent(t, database, tourn.ID, models.TournamentStatusInProgress)

	// Registrations must survive a rejected cancel.
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	if len(regs) != 4 {
		t.Errorf("registrations = %d, want 4", len(regs))
	}
}

// Everything below Admin tier is refused, including tiers that may run the
// tournament day-to-day.
func TestTournamentHandler_Cancel_BelowAdminTierForbidden(t *testing.T) {
	for _, tier := range []models.TournamentTier{models.TierCoOrganizer, models.TierJudge} {
		t.Run(string(tier), func(t *testing.T) {
			database := testDB(t)
			ctx := context.Background()
			h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
			owner := mustCreateUser(t, database, "owner@example.com", "Owner")
			tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
			staff := mustCreateUser(t, database, "staff@example.com", "Staff")
			if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
				TournamentID: tourn.ID, UserID: staff.ID, Tier: tier,
			}); err != nil {
				t.Fatalf("seed %s: %v", tier, err)
			}

			rec := httptest.NewRecorder()
			h.Cancel(rec, cancelReq(staff, tourn.ID))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
			}
			assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
		})
	}
}

func TestTournamentHandler_Cancel_StrangerForbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(other, tourn.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
}

// Defence in depth: the route sits behind auth middleware, but the handler
// itself must not delete anything for an anonymous request.
func TestTournamentHandler_Cancel_AnonymousForbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(nil, tourn.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
}

// The authorization check must not be reachable with an id the caller made
// up: an unknown id is a 404 before any tier lookup.
func TestTournamentHandler_Cancel_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	rec := httptest.NewRecorder()
	h.Cancel(rec, cancelReq(user, 999999))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestTournamentHandler_Cancel_BadID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": "not-a-number"})
	rec := httptest.NewRecorder()
	h.Cancel(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// A double submit (or a stale tab) must not 500.
func TestTournamentHandler_Cancel_Twice(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	first := httptest.NewRecorder()
	h.Cancel(first, cancelReq(owner, tourn.ID))
	if first.Code != http.StatusSeeOther {
		t.Fatalf("first cancel: status = %d, body=%s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	h.Cancel(second, cancelReq(owner, tourn.ID))
	if second.Code != http.StatusNotFound {
		t.Errorf("second cancel: status = %d, want 404", second.Code)
	}
}
