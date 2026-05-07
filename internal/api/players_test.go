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

func TestPlayersAPI_List_Empty(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.List(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Errorf("expected empty list, got %q", rec.Body.String())
	}
}

func TestPlayersAPI_List_WithPlayers(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	u := mustCreateUser(t, database, "alice@example.com", "Alice")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := db.CreateGuestRegistration(ctx, database, tourn.ID, "Guest1"); err != nil {
		t.Fatalf("guest: %v", err)
	}

	r := requestWithUser("GET", "/", "", nil, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.List(rec, r)

	var got []map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 players, got %d", len(got))
	}
	var sawGuest bool
	for _, p := range got {
		if p["is_guest"].(bool) {
			sawGuest = true
		}
	}
	if !sawGuest {
		t.Error("expected to see at least one guest")
	}
}

func TestPlayersAPI_Register(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p1@example.com", "P1")

	r := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Register(rec, r)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	if len(regs) != 1 {
		t.Errorf("expected 1 registration, got %d", len(regs))
	}
}

func TestPlayersAPI_Register_RegistrationClosed(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusScheduled)
	user := mustCreateUser(t, database, "p1@example.com", "P1")

	r := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Register(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayersAPI_Register_NotFound(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	user := mustCreateUser(t, database, "p1@example.com", "P1")

	r := requestWithUser("POST", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.Register(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayersAPI_Register_TournamentFull(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	tourn.MaxPlayers = 1
	if err := db.UpdateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("update tournament: %v", err)
	}

	first := mustCreateUser(t, database, "first@example.com", "First")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, first.ID, first.DisplayName); err != nil {
		t.Fatalf("first registration: %v", err)
	}

	user := mustCreateUser(t, database, "second@example.com", "Second")
	r := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Register(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for full tournament, got %d", rec.Code)
	}
}

func TestPlayersAPI_Register_RequiresDecklist_CreatesPending(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	tourn.RequireDecklist = true
	if err := db.UpdateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("update tournament: %v", err)
	}

	user := mustCreateUser(t, database, "p@example.com", "P")
	r := requestWithUser("POST", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Register(rec, r)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	reg, err := db.GetRegistration(ctx, database, tourn.ID, user.ID)
	if err != nil {
		t.Fatalf("get reg: %v", err)
	}
	if reg.Status != models.RegistrationStatusPending {
		t.Errorf("status = %q, want pending", reg.Status)
	}
}

func TestPlayersAPI_Unregister(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	r := requestWithUser("DELETE", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Unregister(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if len(regs) != 0 {
		t.Errorf("expected 0 regs after unregister, got %d", len(regs))
	}
}

func TestPlayersAPI_Unregister_AfterStart(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusInProgress)
	user := mustCreateUser(t, database, "p@example.com", "P")

	r := requestWithUser("DELETE", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.Unregister(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unregister after start, got %d", rec.Code)
	}
}

func TestPlayersAPI_Unregister_NotFound(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	user := mustCreateUser(t, database, "p@example.com", "P")

	r := requestWithUser("DELETE", "/", "", user, map[string]string{"id": "999"})
	rec := httptest.NewRecorder()
	api.Unregister(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayersAPI_AddPlayer(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("POST", "/", `{"player_name":"Bob"}`, owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.AddPlayer(rec, r)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	if len(regs) != 1 || regs[0].DisplayName != "Bob" {
		t.Errorf("expected one guest 'Bob', got %+v", regs)
	}
}

func TestPlayersAPI_AddPlayer_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("POST", "/", `{"player_name":"Bob"}`, other, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.AddPlayer(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestPlayersAPI_AddPlayer_WrongState(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusFinished)

	r := requestWithUser("POST", "/", `{"player_name":"Bob"}`, owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.AddPlayer(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayersAPI_AddPlayer_BlankName(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("POST", "/", `{"player_name":"   "}`, owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.AddPlayer(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayersAPI_AddPlayer_MidTournament(t *testing.T) {
	database := testDB(t)
	owner, tourn := startedTournament(t, database)
	api := &PlayersAPI{DB: database}

	r := requestWithUser("POST", "/", `{"player_name":"LatePlayer"}`, owner, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.AddPlayer(rec, r)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	var found *models.Registration
	for i := range regs {
		if regs[i].DisplayName == "LatePlayer" {
			found = &regs[i]
		}
	}
	if found == nil {
		t.Fatal("expected LatePlayer registration")
	}
	if found.EnginePlayerID == nil {
		t.Error("expected engine_player_id to be set for mid-tournament add")
	}
}

func TestPlayersAPI_DropPlayer_PreTournament(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	guest, err := db.CreateGuestRegistration(ctx, database, tourn.ID, "Guest")
	if err != nil {
		t.Fatalf("guest: %v", err)
	}

	r := requestWithUser("DELETE", "/", "", owner, map[string]string{
		"id":  strconv.FormatInt(tourn.ID, 10),
		"pid": strconv.FormatInt(guest.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.DropPlayer(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	if len(regs) != 0 {
		t.Errorf("expected 0 regs after drop, got %d", len(regs))
	}
}

func TestPlayersAPI_DropPlayer_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("DELETE", "/", "", other, map[string]string{
		"id":  strconv.FormatInt(tourn.ID, 10),
		"pid": "1",
	})
	rec := httptest.NewRecorder()
	api.DropPlayer(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestPlayersAPI_DropPlayer_NotFound(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	user := mustCreateUser(t, database, "u@example.com", "U")

	r := requestWithUser("DELETE", "/", "", user, map[string]string{"id": "999", "pid": "1"})
	rec := httptest.NewRecorder()
	api.DropPlayer(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayersAPI_DropPlayer_MidTournament(t *testing.T) {
	database := testDB(t)
	owner, tourn := startedTournament(t, database)
	api := &PlayersAPI{DB: database}

	regs, _ := db.ListRegistrations(context.Background(), database, tourn.ID)
	var enginePlayerID int
	var dropReg models.Registration
	for _, r := range regs {
		if r.EnginePlayerID != nil {
			enginePlayerID = *r.EnginePlayerID
			dropReg = r
			break
		}
	}
	if enginePlayerID == 0 {
		t.Fatal("expected at least one player with engine_player_id")
	}

	r := requestWithUser("DELETE", "/", "", owner, map[string]string{
		"id":  strconv.FormatInt(tourn.ID, 10),
		"pid": strconv.Itoa(enginePlayerID),
	})
	rec := httptest.NewRecorder()
	api.DropPlayer(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got, err := db.GetRegistrationByID(context.Background(), database, dropReg.ID)
	if err != nil {
		t.Fatalf("get reg: %v", err)
	}
	if got.Status != models.RegistrationStatusDropped {
		t.Errorf("status = %q, want dropped", got.Status)
	}
}

func TestPlayersAPI_GetDecklist_NoDeck(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	r := requestWithUser("GET", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetDecklist(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var dl map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&dl)
	if dl["main"] == nil {
		t.Error("expected main key in response")
	}
}

func TestPlayersAPI_GetDecklist_NotRegistered(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")

	r := requestWithUser("GET", "/", "", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.GetDecklist(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayersAPI_SubmitDecklist(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	body := `{"main":{"Brainstorm":4},"sideboard":{"Pyroblast":2}}`
	r := requestWithUser("PUT", "/", body, user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitDecklist(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	got, _ := db.GetRegistration(ctx, database, tourn.ID, user.ID)
	if got.Decklist == nil {
		t.Error("expected decklist to be saved")
	}
	if got.Status != models.RegistrationStatusConfirmed {
		t.Errorf("status = %q, want confirmed", got.Status)
	}
}

func TestPlayersAPI_SubmitDecklist_BadJSON(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")

	r := requestWithUser("PUT", "/", "not-json", user, map[string]string{"id": strconv.FormatInt(tourn.ID, 10)})
	rec := httptest.NewRecorder()
	api.SubmitDecklist(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayersAPI_GetRegistrationDecklist(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	reg, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	r := requestWithUser("GET", "/", "", owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.GetRegistrationDecklist(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestPlayersAPI_GetRegistrationDecklist_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	reg, _ := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)

	r := requestWithUser("GET", "/", "", other, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.GetRegistrationDecklist(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestPlayersAPI_GetRegistrationDecklist_NotFound(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("GET", "/", "", owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": "999",
	})
	rec := httptest.NewRecorder()
	api.GetRegistrationDecklist(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestPlayersAPI_SetRegistrationDecklist(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	reg, _ := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)

	body := `{"main":{"Brainstorm":4},"sideboard":{}}`
	r := requestWithUser("PUT", "/", body, owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.SetRegistrationDecklist(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	got, _ := db.GetRegistrationByID(ctx, database, reg.ID)
	if got.Decklist == nil {
		t.Error("expected decklist saved")
	}
}

func TestPlayersAPI_SetRegistrationDecklist_BadJSON(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	reg, _ := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName)

	r := requestWithUser("PUT", "/", "not-json", owner, map[string]string{
		"id":    strconv.FormatInt(tourn.ID, 10),
		"regID": strconv.FormatInt(reg.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.SetRegistrationDecklist(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestPlayersAPI_GetPlayerDecklist_AsOrganizer(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	ctx := context.Background()
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)
	user := mustCreateUser(t, database, "p@example.com", "P")
	if _, err := db.CreateRegistration(ctx, database, tourn.ID, user.ID, user.DisplayName); err != nil {
		t.Fatalf("register: %v", err)
	}

	r := requestWithUser("GET", "/", "", owner, map[string]string{
		"id":  strconv.FormatInt(tourn.ID, 10),
		"pid": strconv.FormatInt(user.ID, 10),
	})
	rec := httptest.NewRecorder()
	api.GetPlayerDecklist(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestPlayersAPI_GetPlayerDecklist_Forbidden(t *testing.T) {
	database := testDB(t)
	api := &PlayersAPI{DB: database}
	owner := mustCreateUser(t, database, "owner@example.com", "Owner")
	other := mustCreateUser(t, database, "other@example.com", "Other")
	tourn := mustCreateTournament(t, database, owner.ID, models.TournamentStatusRegistrationOpen)

	r := requestWithUser("GET", "/", "", other, map[string]string{
		"id":  strconv.FormatInt(tourn.ID, 10),
		"pid": "1",
	})
	rec := httptest.NewRecorder()
	api.GetPlayerDecklist(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
