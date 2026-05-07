//go:build integration

package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
	st "github.com/dstathis/swisstools"
)

func TestInitTournamentEngine_SkipsNonConfirmed(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	org, _ := db.CreateUser(ctx, database, "org-skip@example.com", "OrgSkip", "hash")
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "Skip",
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: org.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}

	// 2 confirmed, 1 pending (should be skipped)
	for i, status := range []string{"confirmed", "confirmed", "pending"} {
		u, err := db.CreateUser(ctx, database, "u"+string(rune('a'+i))+"@example.com", "U"+string(rune('A'+i)), "hash")
		if err != nil {
			t.Fatalf("user: %v", err)
		}
		if status == "pending" {
			if _, err := db.CreatePendingRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
				t.Fatalf("pending reg: %v", err)
			}
		} else {
			if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
				t.Fatalf("confirmed reg: %v", err)
			}
		}
	}

	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	state, err := InitTournamentEngine(ctx, tx, tourn, regs)
	if err != nil {
		t.Fatalf("InitTournamentEngine: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	eng, err := st.LoadTournament(state)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := eng.GetPlayerCount(); got != 2 {
		t.Errorf("player count = %d, want 2 (pending should be skipped)", got)
	}
}

func TestInitTournamentEngine_LoadsDecklist(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	org, _ := db.CreateUser(ctx, database, "org-dl@example.com", "OrgDL", "hash")
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "DL",
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: org.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	for i := 0; i < 2; i++ {
		u, _ := db.CreateUser(ctx, database, "dl"+string(rune('a'+i))+"@example.com", "DL"+string(rune('A'+i)), "hash")
		if _, err := db.CreateRegistration(ctx, database, tourn.ID, u.ID, u.DisplayName); err != nil {
			t.Fatalf("reg: %v", err)
		}
	}
	// Attach a decklist to the first registration
	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	dl := st.Decklist{
		Main:      map[string]int{"Brainstorm": 4},
		Sideboard: map[string]int{"Pyroblast": 2},
	}
	dlJSON, _ := json.Marshal(dl)
	if err := db.UpdateRegistrationDecklistByID(ctx, database, regs[0].ID, dlJSON); err != nil {
		t.Fatalf("update decklist: %v", err)
	}
	regs, _ = db.ListRegistrations(ctx, database, tourn.ID)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	state, err := InitTournamentEngine(ctx, tx, tourn, regs)
	if err != nil {
		t.Fatalf("InitTournamentEngine: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	eng, _ := st.LoadTournament(state)
	playerID, _ := eng.GetPlayerID(regs[0].DisplayName)
	player, ok := eng.GetPlayerById(playerID)
	if !ok {
		t.Fatal("expected to find player by id")
	}
	if player.Decklist == nil {
		t.Fatal("expected decklist to be loaded onto player")
	}
	if player.Decklist.Main["Brainstorm"] != 4 {
		t.Errorf("Brainstorm = %d, want 4", player.Decklist.Main["Brainstorm"])
	}
}

func TestInitTournamentEngine_GuestNoExternalID(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	org, _ := db.CreateUser(ctx, database, "org-guest@example.com", "OrgGuest", "hash")
	numRounds := 2
	tourn := &models.Tournament{
		Name:        "Guest",
		MaxPlayers:  16,
		NumRounds:   &numRounds,
		PointsWin:   3,
		PointsDraw:  1,
		PointsLoss:  0,
		Status:      models.TournamentStatusRegistrationOpen,
		OrganizerID: org.ID,
	}
	if err := db.CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create tournament: %v", err)
	}
	if _, err := db.CreateGuestRegistration(ctx, database, tourn.ID, "Guest1"); err != nil {
		t.Fatalf("guest1: %v", err)
	}
	if _, err := db.CreateGuestRegistration(ctx, database, tourn.ID, "Guest2"); err != nil {
		t.Fatalf("guest2: %v", err)
	}

	regs, _ := db.ListRegistrations(ctx, database, tourn.ID)
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	state, err := InitTournamentEngine(ctx, tx, tourn, regs)
	if err != nil {
		t.Fatalf("InitTournamentEngine: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	eng, _ := st.LoadTournament(state)
	if got := eng.GetPlayerCount(); got != 2 {
		t.Errorf("player count = %d, want 2", got)
	}
	for _, p := range eng.GetPlayers() {
		if p.ExternalID != nil {
			t.Errorf("guest player should have nil ExternalID, got %d", *p.ExternalID)
		}
	}
}
