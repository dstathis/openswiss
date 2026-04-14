package models

import (
	"testing"
)

func TestUser_HasRole(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		check    string
		expected bool
	}{
		{"has player role", []string{RolePlayer}, RolePlayer, true},
		{"has admin role", []string{RoleAdmin}, RoleAdmin, true},
		{"has organizer role", []string{RolePlayer, RoleOrganizer}, RoleOrganizer, true},
		{"missing role", []string{RolePlayer}, RoleAdmin, false},
		{"empty roles", []string{}, RolePlayer, false},
		{"nil roles", nil, RolePlayer, false},
		{"multiple roles check first", []string{RolePlayer, RoleOrganizer, RoleAdmin}, RolePlayer, true},
		{"multiple roles check last", []string{RolePlayer, RoleOrganizer, RoleAdmin}, RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{Roles: tt.roles}
			if got := u.HasRole(tt.check); got != tt.expected {
				t.Errorf("HasRole(%q) = %v, want %v", tt.check, got, tt.expected)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	if RolePlayer != "player" {
		t.Errorf("RolePlayer = %q, want %q", RolePlayer, "player")
	}
	if RoleOrganizer != "organizer" {
		t.Errorf("RoleOrganizer = %q, want %q", RoleOrganizer, "organizer")
	}
	if RoleAdmin != "admin" {
		t.Errorf("RoleAdmin = %q, want %q", RoleAdmin, "admin")
	}
	if TournamentStatusScheduled != "scheduled" {
		t.Errorf("TournamentStatusScheduled = %q", TournamentStatusScheduled)
	}
	if TournamentStatusRegistrationOpen != "registration_open" {
		t.Errorf("TournamentStatusRegistrationOpen = %q", TournamentStatusRegistrationOpen)
	}
	if TournamentStatusInProgress != "in_progress" {
		t.Errorf("TournamentStatusInProgress = %q", TournamentStatusInProgress)
	}
	if TournamentStatusPlayoff != "playoff" {
		t.Errorf("TournamentStatusPlayoff = %q", TournamentStatusPlayoff)
	}
	if TournamentStatusFinished != "finished" {
		t.Errorf("TournamentStatusFinished = %q", TournamentStatusFinished)
	}
	if RegistrationStatusPending != "pending" {
		t.Errorf("RegistrationStatusPending = %q", RegistrationStatusPending)
	}
	if RegistrationStatusConfirmed != "confirmed" {
		t.Errorf("RegistrationStatusConfirmed = %q", RegistrationStatusConfirmed)
	}
	if RegistrationStatusDropped != "dropped" {
		t.Errorf("RegistrationStatusDropped = %q", RegistrationStatusDropped)
	}
}
