//go:build integration

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestAdminHandler_UsersPage(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &AdminHandler{DB: database, Tmpl: tmpl}

	mustCreateUser(t, database, "a@example.com", "A")
	mustCreateUser(t, database, "b@example.com", "B")

	req := requestWithUser("GET", "/", "", nil, nil)
	rec := httptest.NewRecorder()
	h.UsersPage(rec, req)

	if tmpl.calls[0].Name != "admin_users.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	users := data["Users"].([]models.User)
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestAdminHandler_UpdateRole_SetsRoles(t *testing.T) {
	database := testDB(t)
	h := &AdminHandler{DB: database, Tmpl: &mockTemplate{}}
	target := mustCreateUser(t, database, "target@example.com", "Target")

	form := url.Values{}
	form.Add("roles", "player")
	form.Add("roles", "organizer")
	req := requestWithUser("POST", "/", form.Encode(), nil, map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	got, _ := db.GetUserByID(context.Background(), database, target.ID)
	if len(got.Roles) != 2 {
		t.Errorf("expected 2 roles, got %v", got.Roles)
	}
}

func TestAdminHandler_UpdateRole_EmptyDefaultsToPlayer(t *testing.T) {
	database := testDB(t)
	h := &AdminHandler{DB: database, Tmpl: &mockTemplate{}}
	target := mustCreateUser(t, database, "target@example.com", "Target", models.RoleAdmin)

	req := requestWithUser("POST", "/", "", nil, map[string]string{"id": strconv.FormatInt(target.ID, 10)})
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	got, _ := db.GetUserByID(context.Background(), database, target.ID)
	if len(got.Roles) != 1 || got.Roles[0] != models.RolePlayer {
		t.Errorf("expected [player] default, got %v", got.Roles)
	}
}
