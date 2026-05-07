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

func TestAdminAPI_ListUsers(t *testing.T) {
	database := testDB(t)
	api := &AdminAPI{DB: database}

	mustCreateUser(t, database, "a@example.com", "A")
	mustCreateUser(t, database, "b@example.com", "B")
	mustCreateUser(t, database, "c@example.com", "C")

	r := requestWithUser("GET", "/api/v1/admin/users", "", nil, nil)
	rec := httptest.NewRecorder()
	api.ListUsers(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []models.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 users, got %d", len(got))
	}
}

func TestAdminAPI_ListUsers_EmptyReturnsArray(t *testing.T) {
	database := testDB(t)
	api := &AdminAPI{DB: database}

	r := requestWithUser("GET", "/api/v1/admin/users", "", nil, nil)
	rec := httptest.NewRecorder()
	api.ListUsers(rec, r)

	body := rec.Body.String()
	if body != "[]\n" {
		t.Errorf("expected empty JSON array, got %q", body)
	}
}

func TestAdminAPI_ListUsers_Pagination(t *testing.T) {
	database := testDB(t)
	api := &AdminAPI{DB: database}
	for i := 0; i < 5; i++ {
		mustCreateUser(t, database, "p"+strconv.Itoa(i)+"@example.com", "P"+strconv.Itoa(i))
	}

	r := requestWithUser("GET", "/api/v1/admin/users?per_page=2&page=1", "", nil, nil)
	rec := httptest.NewRecorder()
	api.ListUsers(rec, r)

	var got []models.User
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got) != 2 {
		t.Errorf("expected 2 users on page 1 with per_page=2, got %d", len(got))
	}
}

func TestAdminAPI_UpdateUser_SetsRoles(t *testing.T) {
	database := testDB(t)
	api := &AdminAPI{DB: database}
	target := mustCreateUser(t, database, "target@example.com", "Target")

	body := `{"roles":["player","organizer"]}`
	r := requestWithUser("PATCH", "/api/v1/admin/users/"+strconv.FormatInt(target.ID, 10), body, nil,
		map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rec := httptest.NewRecorder()
	api.UpdateUser(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got, err := db.GetUserByID(context.Background(), database, target.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if len(got.Roles) != 2 {
		t.Errorf("expected 2 roles, got %v", got.Roles)
	}
}

func TestAdminAPI_UpdateUser_EmptyRolesDefaultsToPlayer(t *testing.T) {
	database := testDB(t)
	api := &AdminAPI{DB: database}
	target := mustCreateUser(t, database, "empty@example.com", "Empty", models.RoleAdmin)

	body := `{"roles":[]}`
	r := requestWithUser("PATCH", "/api/v1/admin/users/"+strconv.FormatInt(target.ID, 10), body, nil,
		map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rec := httptest.NewRecorder()
	api.UpdateUser(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got, _ := db.GetUserByID(context.Background(), database, target.ID)
	if len(got.Roles) != 1 || got.Roles[0] != models.RolePlayer {
		t.Errorf("expected [player] default, got %v", got.Roles)
	}
}

func TestAdminAPI_UpdateUser_InvalidJSON(t *testing.T) {
	database := testDB(t)
	api := &AdminAPI{DB: database}
	target := mustCreateUser(t, database, "bad@example.com", "Bad")

	r := requestWithUser("PATCH", "/api/v1/admin/users/"+strconv.FormatInt(target.ID, 10), "not-json", nil,
		map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rec := httptest.NewRecorder()
	api.UpdateUser(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", rec.Code)
	}
}
