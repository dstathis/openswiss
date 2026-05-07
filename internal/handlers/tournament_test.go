//go:build integration

package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

func TestTournamentHandler_Home(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("GET", "/", "", nil, nil)
	rec := httptest.NewRecorder()
	h.Home(rec, req)

	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].Name != "home.html" {
		t.Errorf("template = %q, want home.html", tmpl.calls[0].Name)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Tournaments"] == nil {
		t.Error("expected Tournaments in template data")
	}
}

func TestTournamentHandler_List(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("GET", "/tournaments?status=scheduled", "", nil, nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if tmpl.calls[0].Name != "tournaments.html" {
		t.Errorf("template = %q, want tournaments.html", tmpl.calls[0].Name)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Status"] != "scheduled" {
		t.Errorf("Status = %v, want scheduled", data["Status"])
	}
}

func TestTournamentHandler_Detail(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("GET", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if tmpl.calls[0].Name != "tournament_detail.html" {
		t.Errorf("template = %q, want tournament_detail.html", tmpl.calls[0].Name)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["CanManage"] != true {
		t.Error("organizer should be able to manage")
	}
}

func TestTournamentHandler_Detail_AsRegisteredPlayer(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	req := requestWithUser("GET", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["MyRegistration"] == nil {
		t.Error("expected MyRegistration to be set for registered player")
	}
	if data["CanManage"] != false {
		t.Error("non-organizer should not manage")
	}
}

func TestTournamentHandler_Detail_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	req := requestWithUser("GET", "/", "", nil, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.Detail(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_Detail_BadID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	req := requestWithUser("GET", "/", "", nil, map[string]string{"id": "not-an-int"})
	rec := httptest.NewRecorder()
	h.Detail(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_NewPage(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("GET", "/", "", user, nil)
	rec := httptest.NewRecorder()
	h.NewPage(rec, req)
	if tmpl.calls[0].Name != "tournament_new.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
}

func TestTournamentHandler_Create(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	form := url.Values{}
	form.Set("name", "Spring Open")
	form.Set("description", "A test tournament")
	form.Set("location", "Online")
	form.Set("max_players", "32")
	form.Set("num_rounds", "4")
	form.Set("top_cut", "8")
	form.Set("points_win", "3")
	form.Set("points_draw", "1")
	form.Set("points_loss", "0")
	form.Set("require_decklist", "on")
	form.Set("decklist_public", "on")
	form.Set("scheduled_at", "2026-06-15T10:00")
	req := requestWithUser("POST", "/tournaments", form.Encode(), user, nil)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/tournaments/") {
		t.Errorf("expected redirect to /tournaments/<id>, got %q", loc)
	}
}

func TestTournamentHandler_Create_DBError(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}

	// User with non-existent ID will fail the organizer_id foreign key
	bogus := &models.User{ID: 99999, Email: "ghost@example.com"}
	form := url.Values{}
	form.Set("name", "Bad")
	req := requestWithUser("POST", "/tournaments", form.Encode(), bogus, nil)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	// Should re-render the form with error
	if len(tmpl.calls) == 0 {
		t.Fatalf("expected template render on DB error, got status %d", rec.Code)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] == nil {
		t.Error("expected Error in template data")
	}
}

func TestTournamentHandler_EditTournament(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	form := url.Values{}
	form.Set("name", "Updated")
	form.Set("max_players", "64")
	form.Set("num_rounds", "5")
	form.Set("description", "")
	form.Set("location", "")
	form.Set("scheduled_at", "")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.EditTournament(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Name != "Updated" {
		t.Errorf("name = %q, want Updated", got.Name)
	}
	if got.MaxPlayers != 64 {
		t.Errorf("max_players = %d, want 64", got.MaxPlayers)
	}
	if got.Description != nil {
		t.Errorf("description = %v, want nil", got.Description)
	}
}

func TestTournamentHandler_EditTournament_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("POST", "/", "name=x", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.EditTournament(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_EditTournament_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.EditTournament(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_EditTournament_WrongState(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusInProgress)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.EditTournament(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_ManagePage(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("GET", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.ManagePage(rec, req)
	if tmpl.calls[0].Name != "tournament_manage.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
}

func TestTournamentHandler_ManagePage_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("GET", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.ManagePage(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_ManagePage_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("GET", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.ManagePage(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_ManagePage_Started(t *testing.T) {
	database := testDB(t)
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner, tourn := startedTournament(t, database)

	req := requestWithUser("GET", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.ManagePage(rec, req)
	if tmpl.calls[0].Name != "tournament_manage.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["CurrentRound"] == 0 {
		t.Error("expected non-zero current round")
	}
}

func TestTournamentHandler_OpenRegistration(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.OpenRegistration(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Status != models.TournamentStatusRegistrationOpen {
		t.Errorf("status = %q", got.Status)
	}
}

func TestTournamentHandler_OpenRegistration_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.OpenRegistration(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_OpenRegistration_WrongState(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.OpenRegistration(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_OpenRegistration_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.OpenRegistration(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_Register(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTournamentHandler_Register_RequiresDecklist(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	tourn.RequireDecklist = true
	if err := db.UpdateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("update: %v", err)
	}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	reg, _ := db.GetRegistration(ctx, database, tourn.ID, user.ID)
	if reg.Status != models.RegistrationStatusPending {
		t.Errorf("status = %q, want pending", reg.Status)
	}
}

func TestTournamentHandler_Register_Closed(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_Register_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_Register_Full(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	tourn.MaxPlayers = 1
	if err := db.UpdateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("update: %v", err)
	}
	first := mustCreateUser(t, database, "first@example.com", "First")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, first.ID, first.DisplayName); err != nil {
		t.Fatalf("first: %v", err)
	}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 full, got %d", rec.Code)
	}
}

func TestTournamentHandler_Unregister(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Unregister(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if len(regs) != 0 {
		t.Errorf("expected 0 regs, got %d", len(regs))
	}
}

func TestTournamentHandler_Unregister_AfterStart(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusInProgress)
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Unregister(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_Unregister_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.Unregister(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_DecklistPage(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	req := requestWithUser("GET", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DecklistPage(rec, req)
	if tmpl.calls[0].Name != "decklist.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
}

func TestTournamentHandler_DecklistPage_NotRegistered(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("GET", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DecklistPage(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_SubmitDecklist(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	form := url.Values{}
	form.Set("decklist", "4 Brainstorm\n\nSideboard\n2 Pyroblast\n")
	req := requestWithUser("POST", "/", form.Encode(), user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.SubmitDecklist(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	got, _ := db.GetRegistration(ctx, database, tourn.ID, user.ID)
	if got.Decklist == nil {
		t.Error("expected decklist saved")
	}
}

func TestTournamentHandler_Start(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	for i := 0; i < 4; i++ {
		u := mustCreateUser(t, database, "p"+strconv.Itoa(i)+"@example.com", "P"+strconv.Itoa(i))
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(ctx, database, tourn.ID)
	if got.Status != models.TournamentStatusInProgress {
		t.Errorf("status = %q", got.Status)
	}
}

func TestTournamentHandler_Start_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Start(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_SubmitResults(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}

	owner := mustCreateUser(t, database, "owner-sr@example.com", "OwnerSR")
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "SR " + t.Name(),
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: owner.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	for i := 0; i < 4; i++ {
		u := mustCreateUser(t, database, "p"+strconv.Itoa(i)+"@example.com", "P"+strconv.Itoa(i))
		db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName)
	}

	// Start it via the handler so we can read pairings out of the engine
	startReq := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	h.Start(httptest.NewRecorder(), startReq)

	got, _ := db.GetTournament(ctx, database, tourn.ID)
	form := url.Values{}
	// Submit results for each pairing
	for _, reg := range mustListRegs(t, database, tourn.ID) {
		if reg.EnginePlayerID != nil {
			pid := strconv.Itoa(*reg.EnginePlayerID)
			form.Set("wins_a_"+pid, "2")
			form.Set("wins_b_"+pid, "0")
			form.Set("draws_"+pid, "0")
		}
	}
	_ = got
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.SubmitResults(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTournamentHandler_SubmitResults_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-sr@example.com", "OtherSR")
	_ = owner

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.SubmitResults(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_NextRound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.NextRound(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTournamentHandler_NextRound_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-nr@example.com", "OtherNR")

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.NextRound(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_RepairRound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.RepairRound(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTournamentHandler_RepairRound_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-rr@example.com", "OtherRR")

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.RepairRound(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_Finish(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Finish(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, _ := db.GetTournament(context.Background(), database, tourn.ID)
	if got.Status != models.TournamentStatusFinished {
		t.Errorf("status = %q, want finished", got.Status)
	}
}

func TestTournamentHandler_Finish_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-fn@example.com", "OtherFN")

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.Finish(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_AddPlayer(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	form := url.Values{}
	form.Set("player_name", "Bob")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.AddPlayer(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	if len(regs) != 1 || regs[0].DisplayName != "Bob" {
		t.Errorf("expected guest 'Bob', got %+v", regs)
	}
}

func TestTournamentHandler_AddPlayer_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	form := url.Values{}
	form.Set("player_name", "Bob")
	req := requestWithUser("POST", "/", form.Encode(), other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.AddPlayer(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_AddPlayer_BlankName(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	form := url.Values{}
	form.Set("player_name", "   ")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.AddPlayer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_AddPlayer_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")

	form := url.Values{}
	form.Set("player_name", "Bob")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.AddPlayer(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_AddPlayer_WrongState(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusFinished)

	form := url.Values{}
	form.Set("player_name", "Bob")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.AddPlayer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_AddPlayer_MidTournament(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)

	form := url.Values{}
	form.Set("player_name", "LatePlayer")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.AddPlayer(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTournamentHandler_DropPlayer_PreTournament(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	guest, err := db.CreateGuestRegistration(ctx, database, tourn.ID, "Guest")
	if err != nil {
		t.Fatalf("guest: %v", err)
	}

	form := url.Values{}
	form.Set("registration_id", strconv.FormatInt(guest.ID, 10))
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if len(regs) != 0 {
		t.Errorf("expected 0 regs, got %d", len(regs))
	}
}

func TestTournamentHandler_DropPlayer_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	form := url.Values{}
	form.Set("player_id", "1")
	req := requestWithUser("POST", "/", form.Encode(), other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_DropPlayer_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "player_id=1", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_DropPlayer_BadPlayerID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusInProgress)

	form := url.Values{}
	form.Set("player_id", "not-an-int")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_DropPlayer_PreTournament_BadRegID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	form := url.Values{}
	form.Set("registration_id", "not-an-int")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_DropPlayer_RegistrationIDAfterStart(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)

	form := url.Values{}
	form.Set("registration_id", "1")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (need player_id when in_progress), got %d", rec.Code)
	}
}

func TestTournamentHandler_DropPlayer_MidTournament(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database)
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	var enginePlayerID int
	for _, r := range regs {
		if r.EnginePlayerID != nil {
			enginePlayerID = *r.EnginePlayerID
			break
		}
	}
	if enginePlayerID == 0 {
		t.Fatal("expected engine_player_id")
	}

	form := url.Values{}
	form.Set("player_id", strconv.Itoa(enginePlayerID))
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.DropPlayer(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTournamentHandler_StartPlayoff_NoTopCut(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner, tourn := startedTournament(t, database) // top_cut=0

	req := requestWithUser("POST", "/", "", owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.StartPlayoff(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_StartPlayoff_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-pf@example.com", "OtherPF")

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.StartPlayoff(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (forbidden), got %d", rec.Code)
	}
}

func TestTournamentHandler_PlayoffResults_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-pr@example.com", "OtherPR")

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.PlayoffResults(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_NextPlayoffRound_Forbidden(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	_, tourn := startedTournament(t, database)
	other := mustCreateUser(t, database, "other-npr@example.com", "OtherNPR")

	req := requestWithUser("POST", "/", "", other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.NextPlayoffRound(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestTournamentHandler_RequestDrop(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	h.RequestDrop(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	reg, _ := db.GetRegistration(ctx, database, tourn.ID, user.ID)
	if reg.Status != models.RegistrationStatusDropped {
		t.Errorf("status = %q, want dropped", reg.Status)
	}
}

func TestTournamentHandler_OrganizerDecklistPage(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	tmpl := &mockTemplate{}
	h := &TournamentHandler{DB: database, Tmpl: tmpl}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	reg, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	req := requestWithUser("GET", "/", "", owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	h.OrganizerDecklistPage(rec, req)
	if tmpl.calls[0].Name != "organizer_decklist.html" {
		t.Errorf("template = %q", tmpl.calls[0].Name)
	}
}

func TestTournamentHandler_OrganizerDecklistPage_Forbidden(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	reg, _ := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)

	req := requestWithUser("GET", "/", "", other, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	h.OrganizerDecklistPage(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_OrganizerDecklistPage_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("GET", "/", "", user, map[string]string{"id": "999", "regID": "1"})
	rec := httptest.NewRecorder()
	h.OrganizerDecklistPage(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_OrganizerDecklistPage_BadRegID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	req := requestWithUser("GET", "/", "", owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": "not-an-int",
	})
	rec := httptest.NewRecorder()
	h.OrganizerDecklistPage(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_OrganizerSubmitDecklist(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	reg, _ := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)

	form := url.Values{}
	form.Set("decklist", "4 Brainstorm")
	req := requestWithUser("POST", "/", form.Encode(), owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	h.OrganizerSubmitDecklist(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	got, _ := db.GetRegistrationByID(ctx, database, reg.ID)
	if got.Decklist == nil {
		t.Error("expected decklist saved")
	}
}

func TestTournamentHandler_OrganizerSubmitDecklist_Forbidden(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "u@example.com", "U")
	reg, _ := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)

	req := requestWithUser("POST", "/", "decklist=4 Brainstorm", other, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	h.OrganizerSubmitDecklist(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTournamentHandler_OrganizerSubmitDecklist_NotFound(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	user := mustCreateUser(t, database, "u@example.com", "U")

	req := requestWithUser("POST", "/", "", user, map[string]string{"id": "999", "regID": "1"})
	rec := httptest.NewRecorder()
	h.OrganizerSubmitDecklist(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestTournamentHandler_OrganizerSubmitDecklist_BadRegID(t *testing.T) {
	database := testDB(t)
	h := &TournamentHandler{DB: database, Tmpl: &mockTemplate{}}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	req := requestWithUser("POST", "/", "", owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": "not-an-int",
	})
	rec := httptest.NewRecorder()
	h.OrganizerSubmitDecklist(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// mustListRegs is a small helper to fetch the registrations for a tournament.
func mustListRegs(t *testing.T, database *sql.DB, tournamentID int64) []models.Registration {
	t.Helper()
	regs, err := db.ListRegistrations(context.Background(), database, tournamentID)
	if err != nil {
		t.Fatalf("list regs: %v", err)
	}
	return regs
}
