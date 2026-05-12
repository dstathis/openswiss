//go:build integration

package db

import (
	"context"
	"errors"
	"testing"

	"github.com/dstathis/openswiss/internal/models"
)

// CreateTournament should atomically insert a tournament_staff row marking
// the creator as Admin — otherwise the tournament would be unmanageable
// since all permission checks route through tournament_staff.
func TestCreateTournament_InsertsCreatorAsAdmin(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	org := createTestOrganizer(t, database)

	tourn := &models.Tournament{
		Name:        "Creator-as-admin",
		Status:      models.TournamentStatusScheduled,
		OrganizerID: org.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("CreateTournament: %v", err)
	}

	tier, err := GetTournamentTier(ctx, database, tourn.ID, org.ID)
	if err != nil {
		t.Fatalf("GetTournamentTier: %v", err)
	}
	if tier != models.TierAdmin {
		t.Errorf("creator tier = %q, want %q", tier, models.TierAdmin)
	}
}

func TestEffectiveTournamentTier(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	creator := createTestOrganizer(t, database)
	other, err := CreateUser(ctx, database, "other-staff@example.com", "OtherStaff", "hash")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	tourn := &models.Tournament{
		Name:        "Tier-lookup",
		Status:      models.TournamentStatusScheduled,
		OrganizerID: creator.ID,
	}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("CreateTournament: %v", err)
	}

	// Nil user: no access.
	tier, err := EffectiveTournamentTier(ctx, database, tourn.ID, nil)
	if err != nil {
		t.Fatalf("nil user: %v", err)
	}
	if tier != "" {
		t.Errorf("nil user tier = %q, want empty", tier)
	}

	// Other user with no staff row and no global role: no access.
	tier, _ = EffectiveTournamentTier(ctx, database, tourn.ID, other)
	if tier != "" {
		t.Errorf("non-staff tier = %q, want empty", tier)
	}

	// Creator: admin.
	tier, _ = EffectiveTournamentTier(ctx, database, tourn.ID, creator)
	if tier != models.TierAdmin {
		t.Errorf("creator tier = %q, want %q", tier, models.TierAdmin)
	}

	// Promote other to judge: judge.
	if err := AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID,
		UserID:       other.ID,
		Tier:         models.TierJudge,
		GrantedBy:    &creator.ID,
	}); err != nil {
		t.Fatalf("AddTournamentStaff: %v", err)
	}
	tier, _ = EffectiveTournamentTier(ctx, database, tourn.ID, other)
	if tier != models.TierJudge {
		t.Errorf("granted-judge tier = %q, want %q", tier, models.TierJudge)
	}

	// A user holding the global admin role is upgraded to TierAdmin even
	// without a staff row on this tournament.
	globalAdmin, err := CreateUser(ctx, database, "global-admin@example.com", "GlobalAdmin", "hash")
	if err != nil {
		t.Fatalf("create global admin: %v", err)
	}
	globalAdmin.Roles = []string{models.RoleAdmin}
	tier, _ = EffectiveTournamentTier(ctx, database, tourn.ID, globalAdmin)
	if tier != models.TierAdmin {
		t.Errorf("global admin tier = %q, want %q", tier, models.TierAdmin)
	}
}

func TestRemoveTournamentStaff_RefusesLastAdmin(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	creator := createTestOrganizer(t, database)
	tourn := &models.Tournament{Name: "Last-admin-remove", Status: models.TournamentStatusScheduled, OrganizerID: creator.ID}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := RemoveTournamentStaff(ctx, database, tourn.ID, creator.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("removing sole admin: got %v, want ErrLastAdmin", err)
	}
}

func TestUpdateTournamentStaffTier_RefusesLastAdminDemotion(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	creator := createTestOrganizer(t, database)
	tourn := &models.Tournament{Name: "Last-admin-demote", Status: models.TournamentStatusScheduled, OrganizerID: creator.ID}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := UpdateTournamentStaffTier(ctx, database, tourn.ID, creator.ID, models.TierJudge); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demoting sole admin: got %v, want ErrLastAdmin", err)
	}
}

// With two admins, removing one is allowed (the other remains).
func TestRemoveTournamentStaff_AllowedWithBackupAdmin(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	creator := createTestOrganizer(t, database)
	tourn := &models.Tournament{Name: "Two-admins", Status: models.TournamentStatusScheduled, OrganizerID: creator.ID}
	if err := CreateTournament(ctx, database, tourn); err != nil {
		t.Fatalf("create: %v", err)
	}
	other, err := CreateUser(ctx, database, "two-admin@example.com", "TwoAdmin", "hash")
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if err := AddTournamentStaff(ctx, database, &models.TournamentStaff{
		TournamentID: tourn.ID, UserID: other.ID, Tier: models.TierAdmin,
	}); err != nil {
		t.Fatalf("add second admin: %v", err)
	}
	if err := RemoveTournamentStaff(ctx, database, tourn.ID, creator.ID); err != nil {
		t.Errorf("expected creator-removal to succeed when another admin exists: %v", err)
	}
	tier, _ := GetTournamentTier(ctx, database, tourn.ID, other.ID)
	if tier != models.TierAdmin {
		t.Errorf("remaining admin tier = %q, want admin", tier)
	}
}
