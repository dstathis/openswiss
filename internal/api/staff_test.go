//go:build integration

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestStaffAPI_List_Public(t *testing.T) {
	database := testDB(t)
	owner, tourn := mkStaffTourn(t, database, "list")
	api := &StaffAPI{DB: database}

	// No auth context — list is public.
	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.List(rec, r)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []db.StaffMember
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].UserID != owner.ID || got[0].Tier != models.TierAdmin {
		t.Errorf("got %+v, want [{owner, admin}]", got)
	}
}

func TestStaffAPI_Grant_ByDisplayName(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := mkStaffTourn(t, database, "grant-dn")
	target := mustCreateUser(t, database, "target-grant-dn@example.com", "TargetGrantDN")
	api := &StaffAPI{DB: database}

	body := fmt.Sprintf(`{"display_name":%q,"tier":"judge"}`, target.DisplayName)
	r := requestWithUser("POST", "/api/v1/tournaments/x/staff", body, owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Grant(rec, r)
	if rec.Code != 201 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	tier, _ := db.GetTournamentTier(ctx, database, tourn.ID, target.ID)
	if tier != models.TierJudge {
		t.Errorf("granted tier = %q, want judge", tier)
	}
}

func TestStaffAPI_Grant_DuplicateReturnsConflict(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := mkStaffTourn(t, database, "grant-dup")
	target := mustCreateUser(t, database, "target-grant-dup@example.com", "TargetGrantDup")
	api := &StaffAPI{DB: database}

	if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: target.ID, Tier: models.TierJudge,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := fmt.Sprintf(`{"user_id":%d,"tier":"co_organizer"}`, target.ID)
	r := requestWithUser("POST", "/api/v1/tournaments/x/staff", body, owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Grant(rec, r)
	if rec.Code != 409 {
		t.Errorf("expected 409 for duplicate, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestStaffAPI_Grant_NonAdminForbidden(t *testing.T) {
	database := testDB(t)
	_, tourn := mkStaffTourn(t, database, "grant-403")
	other := mustCreateUser(t, database, "other-grant-403@example.com", "OtherGrant403")
	api := &StaffAPI{DB: database}

	body := fmt.Sprintf(`{"display_name":%q,"tier":"judge"}`, other.DisplayName)
	r := requestWithUser("POST", "/api/v1/tournaments/x/staff", body, other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Grant(rec, r)
	if rec.Code != 403 {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestStaffAPI_UpdateTier(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := mkStaffTourn(t, database, "update")
	target := mustCreateUser(t, database, "target-update@example.com", "TargetUpdate")
	if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: target.ID, Tier: models.TierJudge,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	api := &StaffAPI{DB: database}

	r := requestWithUser("PATCH", "/", `{"tier":"co_organizer"}`, owner, map[string]string{
		"id":     strconv.FormatInt(tourn.ID, 10),
		"userID": strconv.FormatInt(target.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.UpdateTier(rec, r)
	if rec.Code != 204 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	tier, _ := db.GetTournamentTier(ctx, database, tourn.ID, target.ID)
	if tier != models.TierCoOrganizer {
		t.Errorf("updated tier = %q, want co_organizer", tier)
	}
}

// The creator (sole admin) can't demote themselves; doing so would leave
// the tournament with no admin.
func TestStaffAPI_UpdateTier_LastAdminRefused(t *testing.T) {
	database := testDB(t)
	owner, tourn := mkStaffTourn(t, database, "update-last")
	api := &StaffAPI{DB: database}

	r := requestWithUser("PATCH", "/", `{"tier":"co_organizer"}`, owner, map[string]string{
		"id":     strconv.FormatInt(tourn.ID, 10),
		"userID": strconv.FormatInt(owner.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.UpdateTier(rec, r)
	if rec.Code != 409 {
		t.Errorf("expected 409 for last-admin demote, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestStaffAPI_Remove(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	owner, tourn := mkStaffTourn(t, database, "remove")
	target := mustCreateUser(t, database, "target-remove@example.com", "TargetRemove")
	if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: target.ID, Tier: models.TierJudge,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	api := &StaffAPI{DB: database}

	r := requestWithUser("DELETE", "/", "", owner, map[string]string{
		"id":     strconv.FormatInt(tourn.ID, 10),
		"userID": strconv.FormatInt(target.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.Remove(rec, r)
	if rec.Code != 204 {
		t.Fatalf("status = %d", rec.Code)
	}
	tier, _ := db.GetTournamentTier(ctx, database, tourn.ID, target.ID)
	if tier != "" {
		t.Errorf("after remove tier = %q, want empty", tier)
	}
}

func TestStaffAPI_Remove_SelfAllowed(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	_, tourn := mkStaffTourn(t, database, "remove-self")
	target := mustCreateUser(t, database, "target-remove-self@example.com", "TargetRemoveSelf")
	if err := db.AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: target.ID, Tier: models.TierJudge,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	api := &StaffAPI{DB: database}

	// Target removes themselves — they're not admin, but self-removal is
	// always permitted (and they aren't the last admin).
	r := requestWithUser("DELETE", "/", "", target, map[string]string{
		"id":     strconv.FormatInt(tourn.ID, 10),
		"userID": strconv.FormatInt(target.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.Remove(rec, r)
	if rec.Code != 204 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	tier, _ := db.GetTournamentTier(ctx, database, tourn.ID, target.ID)
	if tier != "" {
		t.Errorf("after self-remove tier = %q, want empty", tier)
	}
}

func TestStaffAPI_Remove_LastAdminRefused(t *testing.T) {
	database := testDB(t)
	owner, tourn := mkStaffTourn(t, database, "remove-last")
	api := &StaffAPI{DB: database}

	r := requestWithUser("DELETE", "/", "", owner, map[string]string{
		"id":     strconv.FormatInt(tourn.ID, 10),
		"userID": strconv.FormatInt(owner.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.Remove(rec, r)
	if rec.Code != 409 {
		t.Errorf("expected 409 for last-admin removal, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestStaffAPI_Search(t *testing.T) {
	database := testDB(t)
	owner, tourn := mkStaffTourn(t, database, "search")
	// Seed three users matching the same prefix to verify ordering + limit.
	mustCreateUser(t, database, "alice@example.com", "AliceSearchable")
	mustCreateUser(t, database, "albert@example.com", "AlbertSearchable")
	mustCreateUser(t, database, "bob@example.com", "BobUnrelated")
	api := &StaffAPI{DB: database}

	r := requestWithUser("GET", "/?q=searchable", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Search(rec, r)
	if rec.Code != 200 {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got []db.UserSearchResult
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d results, want 2 (%+v)", len(got), got)
	}
}

func TestStaffAPI_Search_NonAdminForbidden(t *testing.T) {
	database := testDB(t)
	_, tourn := mkStaffTourn(t, database, "search-403")
	other := mustCreateUser(t, database, "other-search-403@example.com", "OtherSearch403")
	api := &StaffAPI{DB: database}

	r := requestWithUser("GET", "/?q=anything", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Search(rec, r)
	if rec.Code != 403 {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// mkStaffTourn creates a tournament for staff tests. nameTag distinguishes
// the email/display_name across tests so they don't collide.
func mkStaffTourn(t *testing.T, database *sql.DB, nameTag string) (*models.User, *models.Tournament) {
	t.Helper()
	owner := mustCreateUser(t, database, "owner-staff-"+nameTag+"@example.com", "OwnerStaff-"+nameTag)
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	return owner, tourn
}
