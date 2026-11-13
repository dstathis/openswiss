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

func TestTournamentTier_AtLeast(t *testing.T) {
	tests := []struct {
		name string
		have TournamentTier
		min  TournamentTier
		want bool
	}{
		{"admin >= admin", TierAdmin, TierAdmin, true},
		{"admin >= co_org", TierAdmin, TierCoOrganizer, true},
		{"admin >= judge", TierAdmin, TierJudge, true},
		{"co_org >= judge", TierCoOrganizer, TierJudge, true},
		{"co_org not >= admin", TierCoOrganizer, TierAdmin, false},
		{"judge not >= co_org", TierJudge, TierCoOrganizer, false},
		{"judge not >= admin", TierJudge, TierAdmin, false},
		{"empty not >= judge", TournamentTier(""), TierJudge, false},
		{"empty not >= admin", TournamentTier(""), TierAdmin, false},
		{"unknown not >= judge", TournamentTier("garbage"), TierJudge, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.have.AtLeast(tt.min); got != tt.want {
				t.Errorf("%q.AtLeast(%q) = %v, want %v", tt.have, tt.min, got, tt.want)
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

func TestTournamentStarted(t *testing.T) {
	cases := map[string]bool{
		TournamentStatusScheduled:        false,
		TournamentStatusRegistrationOpen: false,
		TournamentStatusInProgress:       true,
		TournamentStatusPlayoff:          true,
		TournamentStatusFinished:         true,
	}
	for status, want := range cases {
		if got := TournamentStarted(status); got != want {
			t.Errorf("TournamentStarted(%q) = %v, want %v", status, got, want)
		}
	}
}

// The permission rule behind deleting a tournament. Before the start line a
// tournament admin decides on their own; past it, erasing the event rewrites
// other players' career statistics, so it takes the global admin role.
func TestCanDeleteTournament(t *testing.T) {
	organizer := &User{Roles: []string{RolePlayer, RoleOrganizer}}
	siteAdmin := &User{Roles: []string{RolePlayer, RoleAdmin}}

	cases := []struct {
		name   string
		user   *User
		status string
		want   bool
	}{
		{"organizer/scheduled", organizer, TournamentStatusScheduled, true},
		{"organizer/registration_open", organizer, TournamentStatusRegistrationOpen, true},
		{"organizer/in_progress", organizer, TournamentStatusInProgress, false},
		{"organizer/playoff", organizer, TournamentStatusPlayoff, false},
		{"organizer/finished", organizer, TournamentStatusFinished, false},

		{"site admin/scheduled", siteAdmin, TournamentStatusScheduled, true},
		{"site admin/registration_open", siteAdmin, TournamentStatusRegistrationOpen, true},
		{"site admin/in_progress", siteAdmin, TournamentStatusInProgress, true},
		{"site admin/playoff", siteAdmin, TournamentStatusPlayoff, true},
		{"site admin/finished", siteAdmin, TournamentStatusFinished, true},

		// Defence in depth: callers check tier first, but a nil user must
		// never clear the started-tournament bar on its own.
		{"anonymous/scheduled", nil, TournamentStatusScheduled, true},
		{"anonymous/finished", nil, TournamentStatusFinished, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanDeleteTournament(tc.user, tc.status); got != tc.want {
				t.Errorf("CanDeleteTournament(%v) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}
