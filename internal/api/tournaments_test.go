//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestTournamentAPI_List_Empty(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}

	r := requestWithUser("GET", "/api/v1/tournaments", "", nil, nil)
	rec := httptest.NewRecorder()
	api.List(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Errorf("expected empty array, got %q", rec.Body.String())
	}
}

func TestTournamentAPI_List_FilterByStatus(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	org := mustCreateUser(t, database, "org@example.com", "Org")
	mustCreateTournament(t, database, org.ID, models.TournamentStatusScheduled)
	mustCreateTournament(t, database, org.ID, models.TournamentStatusFinished)

	r := requestWithUser("GET", "/api/v1/tournaments?status=scheduled", "", nil, nil)
	rec := httptest.NewRecorder()
	api.List(rec, r)

	var got []models.Tournament
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got) != 1 {
		t.Errorf("expected 1 scheduled tournament, got %d", len(got))
	}
}

func TestTournamentAPI_Get(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	org := mustCreateUser(t, database, "org@example.com", "Org")
	tourn := mustCreateTournament(t, database, org.ID, models.TournamentStatusScheduled)

	r := requestWithUser("GET", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), "", nil,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Get(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got models.Tournament
	json.NewDecoder(rec.Body).Decode(&got)
	if got.ID != tourn.ID {
		t.Errorf("got id %d, want %d", got.ID, tourn.ID)
	}
}

func TestTournamentAPI_Get_NotFound(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}

	r := requestWithUser("GET", "/api/v1/tournaments/999", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.Get(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentAPI_Get_InvalidID(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}

	r := requestWithUser("GET", "/api/v1/tournaments/not-a-number", "", nil, map[string]string{"id": "not-a-number"})
	rec := httptest.NewRecorder()
	api.Get(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-integer id, got %d", rec.Code)
	}
}

func TestTournamentAPI_Create(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	user := mustCreateUser(t, database, "creator@example.com", "Creator", models.RoleOrganizer)

	body := `{"name":"Spring Open","max_players":32,"top_cut":8}`
	r := requestWithUser("POST", "/api/v1/tournaments", body, user, nil)
	rec := httptest.NewRecorder()
	api.Create(rec, r)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var got models.Tournament
	json.NewDecoder(rec.Body).Decode(&got)
	if got.Name != "Spring Open" {
		t.Errorf("name = %q", got.Name)
	}
	if got.OrganizerID != user.ID {
		t.Errorf("organizer = %d, want %d", got.OrganizerID, user.ID)
	}
	if got.Status != models.TournamentStatusScheduled {
		t.Errorf("status default = %q, want scheduled", got.Status)
	}
	if got.PointsWin != 3 {
		t.Errorf("points_win default = %d, want 3", got.PointsWin)
	}
	if got.PointsDraw != 1 {
		t.Errorf("points_draw default = %d, want 1", got.PointsDraw)
	}
}

func TestTournamentAPI_Create_InvalidJSON(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	user := mustCreateUser(t, database, "u@example.com", "U")

	r := requestWithUser("POST", "/api/v1/tournaments", "not-json", user, nil)
	rec := httptest.NewRecorder()
	api.Create(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentAPI_Update(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	user := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, user.ID, models.TournamentStatusScheduled)

	body := `{"name":"Updated Name","max_players":64}`
	r := requestWithUser("PATCH", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), body, user,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Update(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Name != "Updated Name" {
		t.Errorf("name = %q, want Updated Name", got.Name)
	}
	if got.MaxPlayers != 64 {
		t.Errorf("max_players = %d, want 64", got.MaxPlayers)
	}
}

func TestTournamentAPI_Update_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("PATCH", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), `{"name":"Hijack"}`, other,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Update(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentAPI_Update_AdminOverride(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	admin := mustCreateUser(t, database, "admin@example.com", "Admin", models.RoleAdmin)
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("PATCH", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), `{"name":"AdminEdit"}`, admin,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Update(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("admin should be able to edit, got %d", rec.Code)
	}
}

func TestTournamentAPI_Update_WrongState(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusInProgress)

	r := requestWithUser("PATCH", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), `{"name":"x"}`, owner,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Update(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 in non-scheduled state, got %d", rec.Code)
	}
}

func TestTournamentAPI_Update_NotFound(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	user := mustCreateUser(t, database, "u@example.com", "U")

	r := requestWithUser("PATCH", "/api/v1/tournaments/999", `{}`, user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.Update(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentAPI_Delete(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("DELETE", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), "", owner,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Delete(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
	_, err := db.GetTournament(context.Background(), database, tourn.ID)
	if err == nil {
		t.Error("tournament should be deleted")
	}
}

func TestTournamentAPI_Delete_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("DELETE", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), "", other,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Delete(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentAPI_Delete_WrongState(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusFinished)

	r := requestWithUser("DELETE", "/api/v1/tournaments/"+strconv.FormatInt(tourn.ID, 10), "", owner,
		map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Delete(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 deleting finished tournament, got %d", rec.Code)
	}
}

func TestTournamentAPI_Delete_NotFound(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	user := mustCreateUser(t, database, "u@example.com", "U")

	r := requestWithUser("DELETE", "/api/v1/tournaments/999", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.Delete(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentAPI_OpenRegistration(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("POST", "", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.OpenRegistration(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Status != models.TournamentStatusRegistrationOpen {
		t.Errorf("status = %q, want registration_open", got.Status)
	}
}

func TestTournamentAPI_OpenRegistration_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	r := requestWithUser("POST", "", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.OpenRegistration(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentAPI_OpenRegistration_WrongState(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("POST", "", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.OpenRegistration(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 opening already-open registration, got %d", rec.Code)
	}
}

func TestTournamentAPI_OpenRegistration_NotFound(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	user := mustCreateUser(t, database, "u@example.com", "U")

	r := requestWithUser("POST", "", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.OpenRegistration(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentAPI_Start(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		u := mustCreateUser(t, database, "p"+strconv.Itoa(i)+"@example.com", "P"+strconv.Itoa(i))
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	r := requestWithUser("POST", "", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Start(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("Start status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(ctx, database, tourn.ID)
	if got.Status != models.TournamentStatusInProgress {
		t.Errorf("after Start, status = %q, want in_progress", got.Status)
	}
}

func TestTournamentAPI_Finish(t *testing.T) {
	database := testDB(t)
	owner, tourn := startedTournament(t, database)
	api := &TournamentAPI{DB: database}

	r := requestWithUser("POST", "", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Finish(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("Finish status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Status != models.TournamentStatusFinished {
		t.Errorf("after Finish, status = %q, want finished", got.Status)
	}
}

func TestTournamentAPI_Start_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &TournamentAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("POST", "", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Start(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
