//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

// deleteReq builds the POST that the Delete Tournament form issues, with the
// confirmation phrase filled in correctly.
func deleteReq(user *models.User, tournamentID int64) *http.Request {
	return deleteReqWithConfirm(user, tournamentID, deleteConfirmPhrase)
}

// deleteReqWithConfirm is deleteReq with an arbitrary confirmation phrase, for
// exercising the server-side check.
func deleteReqWithConfirm(user *models.User, tournamentID int64, confirm string) *http.Request {
	body := url.Values{"confirm": {confirm}}.Encode()
	return requestWithUser("POST", "/", body, user, map[string]string{"id": strconv.FormatInt(tournamentID, 10)})
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

func TestTournamentHandler_Delete_Scheduled(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(owner, tourn.ID))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/tournaments" {
		t.Errorf("Location = %q, want /tournaments", loc)
	}
	assertGone(t, database, tourn.ID)
}

func TestTournamentHandler_Delete_RegistrationOpen(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(owner, tourn.ID))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, tourn.ID)
}

// assertRejectedWithFormBack checks the response to a bad confirmation phrase:
// a 400 that re-renders the manage page carrying an error, with the delete
// control still available so the user can simply retype. A bare status page
// would be a dead end behind the browser's Back button.
func assertRejectedWithFormBack(t *testing.T, rec *httptest.ResponseRecorder, tmpl *mockTemplate) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if len(tmpl.calls) != 1 {
		t.Fatalf("rendered %d templates, want 1 (the manage page back with an error)", len(tmpl.calls))
	}
	call := tmpl.calls[0]
	if call.Name != "tournament_manage.html" {
		t.Fatalf("rendered %q, want tournament_manage.html", call.Name)
	}
	data, ok := call.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("template data is %T, want map[string]interface{}", call.Data)
	}
	msg, _ := data["DeleteError"].(string)
	if msg == "" {
		t.Error("re-render carries no DeleteError message")
	}
	// The message has to quote the phrase; "that did not match" alone leaves
	// the user guessing at what they got wrong.
	if !strings.Contains(msg, deleteConfirmPhrase) {
		t.Errorf("DeleteError does not include the required phrase: %q", msg)
	}
	if canDelete, _ := data["CanDelete"].(bool); !canDelete {
		t.Error("CanDelete is false on the re-render, so the form the user needs is gone")
	}
	if data["Tournament"] == nil {
		t.Error("re-render has no Tournament, so the page cannot draw")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

// The confirmation phrase is enforced server-side, not by the browser: a POST
// that skips the form must not delete anything. A near miss is an ordinary
// typo, so the response is the form back with an error, not a dead end.
func TestTournamentHandler_Delete_WrongConfirmPhrase(t *testing.T) {
	for name, confirm := range map[string]string{
		"empty":            "",
		"close":            "I want to permanently delete the tournament",  // no trailing period
		"wrong case":       "i want to permanently delete the tournament.", // lowercase I
		"leading space":    " I want to permanently delete the tournament.",
		"tournament name":  "Test Tournament",
		"something else":   "yes",
		"phrase with typo": "I want to permanently delete the tournamet.",
	} {
		t.Run(name, func(t *testing.T) {
			database := testDB(t)
			tmpl := &mockTemplate{}
			h := &TournamentHandler{DB: database, Tmpl: tmpl}
			owner := mustCreateUser(t, database, "owner@example.com", "Owner")
			tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

			rec := httptest.NewRecorder()
			h.Delete(rec, deleteReqWithConfirm(owner, tourn.ID, confirm))

			assertRejectedWithFormBack(t, rec, tmpl)
			assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
		})
	}
}

// A request with no form body at all (no confirm field) is refused the same way.
func TestTournamentHandler_Delete_MissingConfirmPhrase(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	assertRejectedWithFormBack(t, rec, tmpl)
	assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
}

// A started tournament keeps the higher bar on the re-render too: a site admin
// who fumbles the phrase still gets the form back, and the control stays
// visible because they are allowed to use it.
func TestTournamentHandler_Delete_WrongConfirmPhraseOnStarted(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusFinished)
	siteAdmin := mustCreateUser(t, database, "admin@example.com", "SiteAdmin", models.RoleAdmin)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReqWithConfirm(siteAdmin, tourn.ID, "nope"))

	assertRejectedWithFormBack(t, rec, tmpl)
	assertPresent(t, database, tourn.ID, models.TournamentStatusFinished)
}

// Registrations hang off the tournament by FK; deleting must take them with
// it (ON DELETE CASCADE) without touching the underlying user accounts.
func TestTournamentHandler_Delete_DeletesRegistrationsKeepsUsers(t *testing.T) {
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
	h.Delete(rec, deleteReq(owner, tourn.ID))
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
		t.Errorf("player account was destroyed by delete: %v", err)
	}
}

// Staff rows are the permission table; they must cascade too, or a re-used
// tournament id would inherit stale grants.
func TestTournamentHandler_Delete_DeletesStaffRows(t *testing.T) {
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
	h.Delete(rec, deleteReq(owner, tourn.ID))
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

// Delete must remove only its target.
func TestTournamentHandler_Delete_LeavesOtherTournaments(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	victim := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	bystander := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(owner, victim.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, victim.ID)
	assertPresent(t, database, bystander.ID, models.TournamentStatusScheduled)
}

// A global site admin outranks the staff table and may delete anyone's
// not-yet-started tournament.
func TestTournamentHandler_Delete_SiteAdmin(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	siteAdmin := mustCreateUser(t, database, "admin@example.com", "SiteAdmin", models.RoleAdmin)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(siteAdmin, tourn.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, tourn.ID)
}

// Past the start line the tournament holds played rounds whose results feed
// every participant's career stats, so tier admin on the tournament is no
// longer enough -- deleting it rewrites records belonging to other people.
func TestTournamentHandler_Delete_StartedStatesRequireSiteAdmin(t *testing.T) {
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

			// The tournament's own admin is refused.
			rec := httptest.NewRecorder()
			h.Delete(rec, deleteReq(owner, tourn.ID))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("tournament admin: status = %d, want 403, body=%s", rec.Code, rec.Body.String())
			}
			assertPresent(t, database, tourn.ID, status)

			// A site admin goes through.
			siteAdmin := mustCreateUser(t, database, "admin@example.com", "SiteAdmin", models.RoleAdmin)
			rec = httptest.NewRecorder()
			h.Delete(rec, deleteReq(siteAdmin, tourn.ID))
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("site admin: status = %d, want 303, body=%s", rec.Code, rec.Body.String())
			}
			assertGone(t, database, tourn.ID)
		})
	}
}

// Same rule, but against a tournament genuinely driven through the engine
// rather than one with a hand-set status column.
func TestTournamentHandler_Delete_ReallyStartedTournament(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := startedTournament(t, database)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(owner, tourn.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tournament admin: status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	assertPresent(t, database, tourn.ID, models.TournamentStatusInProgress)

	// Registrations must survive a refused delete.
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if len(regs) != 4 {
		t.Fatalf("registrations = %d, want 4", len(regs))
	}

	siteAdmin := mustCreateUser(t, database, "admin@example.com", "SiteAdmin", models.RoleAdmin)
	rec = httptest.NewRecorder()
	h.Delete(rec, deleteReq(siteAdmin, tourn.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("site admin: status = %d, want 303, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, tourn.ID)

	if regs, _ := db.ListRegistrations(ctx, database, tourn.ID); len(regs) != 0 {
		t.Errorf("registrations left behind: %d", len(regs))
	}
}

// The end-to-end guarantee behind this feature: once a finished tournament is
// deleted, it stops counting toward every participant's career stats. Career
// totals are not stored anywhere -- they are recomputed per request from the
// tournament_results / matches projection, which the FK cascade clears -- so
// this asserts the whole chain through the real handler.
func TestTournamentHandler_Delete_FinishedClearsPlayerStats(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := startedTournament(t, database)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	finishReq := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Finish(rec, finishReq)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("finish: status = %d, body=%s", rec.Code, rec.Body.String())
	}

	regs, err := db.ListRegistrations(ctx, database, tourn.ID)
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) != 4 {
		t.Fatalf("registrations = %d, want 4", len(regs))
	}

	// Every player must have picked up exactly this one event.
	for _, reg := range regs {
		if reg.UserID == nil {
			t.Fatalf("registration %d has no user", reg.ID)
		}
		stats, err := db.GetCareerStats(ctx, database, *reg.UserID)
		if err != nil {
			t.Fatalf("career stats for %s: %v", reg.DisplayName, err)
		}
		if stats.Events != 1 {
			t.Fatalf("setup: %s has %d events, want 1 (projection did not land)", reg.DisplayName, stats.Events)
		}
	}

	siteAdmin := mustCreateUser(t, database, "admin@example.com", "SiteAdmin", models.RoleAdmin)
	rec = httptest.NewRecorder()
	h.Delete(rec, deleteReq(siteAdmin, tourn.ID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete: status = %d, body=%s", rec.Code, rec.Body.String())
	}
	assertGone(t, database, tourn.ID)

	for _, reg := range regs {
		stats, err := db.GetCareerStats(ctx, database, *reg.UserID)
		if err != nil {
			t.Fatalf("career stats for %s after delete: %v", reg.DisplayName, err)
		}
		if stats.Events != 0 {
			t.Errorf("%s: Events = %d after delete, want 0", reg.DisplayName, stats.Events)
		}
		if stats.Wins != 0 || stats.Losses != 0 || stats.Draws != 0 {
			t.Errorf("%s: record = %d-%d-%d after delete, want 0-0-0",
				reg.DisplayName, stats.Wins, stats.Losses, stats.Draws)
		}
		if stats.Byes != 0 {
			t.Errorf("%s: Byes = %d after delete, want 0", reg.DisplayName, stats.Byes)
		}

		matches, err := db.ListMatchesForUser(ctx, database, *reg.UserID, 50, 0)
		if err != nil {
			t.Fatalf("match history for %s: %v", reg.DisplayName, err)
		}
		if len(matches) != 0 {
			t.Errorf("%s: %d matches left in history, want 0", reg.DisplayName, len(matches))
		}

		h2h, err := db.ListHeadToHeadForUser(ctx, database, *reg.UserID)
		if err != nil {
			t.Fatalf("head-to-head for %s: %v", reg.DisplayName, err)
		}
		if len(h2h) != 0 {
			t.Errorf("%s: %d head-to-head rows left, want 0", reg.DisplayName, len(h2h))
		}
	}
}

// Everything below Admin tier is refused, including tiers that may run the
// tournament day-to-day.
func TestTournamentHandler_Delete_BelowAdminTierForbidden(t *testing.T) {
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
			h.Delete(rec, deleteReq(staff, tourn.ID))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
			}
			assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
		})
	}
}

func TestTournamentHandler_Delete_StrangerForbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(other, tourn.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
}

// Defence in depth: the route sits behind auth middleware, but the handler
// itself must not delete anything for an anonymous request.
func TestTournamentHandler_Delete_AnonymousForbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(nil, tourn.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
	assertPresent(t, database, tourn.ID, models.TournamentStatusScheduled)
}

// The authorization check must not be reachable with an id the caller made
// up: an unknown id is a 404 before any tier lookup.
func TestTournamentHandler_Delete_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	rec := httptest.NewRecorder()
	h.Delete(rec, deleteReq(user, 999999))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestTournamentHandler_Delete_BadID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	body := url.Values{"confirm": {deleteConfirmPhrase}}.Encode()
	req := requestWithUser("POST", "/", body, user, map[string]string{"id": "not-a-number"})
	rec := httptest.NewRecorder()
	h.Delete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// A double submit (or a stale tab) must not 500.
func TestTournamentHandler_Delete_Twice(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	first := httptest.NewRecorder()
	h.Delete(first, deleteReq(owner, tourn.ID))
	if first.Code != http.StatusSeeOther {
		t.Fatalf("first delete: status = %d, body=%s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	h.Delete(second, deleteReq(owner, tourn.ID))
	if second.Code != http.StatusNotFound {
		t.Errorf("second delete: status = %d, want 404", second.Code)
	}
}
