//go:build integration

package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestPlayerHandler_Dashboard(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &PlayerHandler{DB: database, Tmpl: tmpl}

	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	user := mustCreateUser(t, database, "u@example.com", "U")

	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	req := requestWithUser("GET", "/", "", user, nil)
	rec := httptest.NewRecorder()
	h.Dashboard(rec, req)

	if tmpl.calls[0].Name != "dashboard.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
}

func TestPlayerHandler_Dashboard_Empty(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &PlayerHandler{DB: database, Tmpl: tmpl}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("GET", "/", "", user, nil)
	rec := httptest.NewRecorder()
	h.Dashboard(rec, req)

	if tmpl.calls[0].Name != "dashboard.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
}
