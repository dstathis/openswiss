package storage

import (
	"os"
	"testing"
	st "github.com/dstathis/swisstools"
)

func setupTestStorage(t *testing.T) *TournamentStorage {
	// Each test gets its own isolated storage by clearing files
	// This ensures tests don't interfere with each other
	os.Remove("data/tournament.json")
	os.Remove("data/pending_players.json")
	
	t.Cleanup(func() {
		os.Remove("data/tournament.json")
		os.Remove("data/pending_players.json")
	})
	
	// Create fresh storage
	ts, err := NewTournamentStorage()
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}
	
	return ts
}

func TestAddPendingPlayer(t *testing.T) {
	ts := setupTestStorage(t)
	
	tests := []struct {
		name    string
		player  string
		wantErr bool
	}{
		{"valid player", "Alice", false},
		{"another player", "Bob", false},
		{"duplicate pending", "Alice", true}, // Already pending
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ts.AddPendingPlayer(tt.player)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddPendingPlayer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetPendingPlayers(t *testing.T) {
	ts := setupTestStorage(t)
	
	// Add some pending players
	ts.AddPendingPlayer("Alice")
	ts.AddPendingPlayer("Bob")
	ts.AddPendingPlayer("Charlie")
	
	pending := ts.GetPendingPlayers()
	if len(pending) != 3 {
		t.Errorf("GetPendingPlayers() returned %d players, want 3", len(pending))
	}
	
	// Accept one
	ts.AcceptPlayer("Alice")
	
	// Should now only show 2 pending
	pending = ts.GetPendingPlayers()
	if len(pending) != 2 {
		t.Errorf("GetPendingPlayers() returned %d players after accept, want 2", len(pending))
	}
}

func TestAcceptPlayer(t *testing.T) {
	ts := setupTestStorage(t)
	
	// Add pending player
	if err := ts.AddPendingPlayer("TestPlayer"); err != nil {
		t.Fatalf("Failed to add pending player: %v", err)
	}
	
	// Accept player
	if err := ts.AcceptPlayer("TestPlayer"); err != nil {
		t.Errorf("AcceptPlayer() error = %v", err)
	}
	
	// Verify player is in tournament
	tournament := ts.GetTournament()
	if _, found := tournament.GetPlayerByName("TestPlayer"); !found {
		t.Error("Accepted player not found in tournament")
	}
	
	// Verify player no longer pending
	pending := ts.GetPendingPlayers()
	for _, p := range pending {
		if p.Name == "TestPlayer" {
			t.Error("Accepted player still in pending list")
		}
	}
}

func TestRejectPlayer(t *testing.T) {
	ts := setupTestStorage(t)
	
	// Add pending player
	ts.AddPendingPlayer("RejectMe")
	
	// Reject player
	if err := ts.RejectPlayer("RejectMe"); err != nil {
		t.Errorf("RejectPlayer() error = %v", err)
	}
	
	// Verify player not in tournament
	tournament := ts.GetTournament()
	if _, found := tournament.GetPlayerByName("RejectMe"); found {
		t.Error("Rejected player found in tournament")
	}
	
	// Verify player not in pending
	pending := ts.GetPendingPlayers()
	for _, p := range pending {
		if p.Name == "RejectMe" {
			t.Error("Rejected player still in pending list")
		}
	}
}

func TestTournamentWorkflow(t *testing.T) {
	ts := setupTestStorage(t)
	
	// Add all players first
	players := []string{"Alice", "Bob", "Charlie"}
	for _, p := range players {
		if err := ts.AddPendingPlayer(p); err != nil {
			t.Fatalf("Failed to add pending player %s: %v", p, err)
		}
	}
	
	// Then accept them one by one
	for _, p := range players {
		if err := ts.AcceptPlayer(p); err != nil {
			t.Fatalf("Failed to accept player %s: %v", p, err)
		}
	}
	
	// Verify all players in tournament
	tournament := ts.GetTournament()
	if count := tournament.GetPlayerCount(); count != 3 {
		t.Errorf("Tournament has %d players, want 3", count)
	}
	
	// Start tournament
	err := ts.UpdateTournament(func(t *st.Tournament) error {
		return t.StartTournament()
	})
	if err != nil {
		t.Errorf("StartTournament() error = %v", err)
	}
	
	// Verify tournament started
	tournament = ts.GetTournament()
	if status := tournament.GetStatus(); status != "in_progress" {
		t.Errorf("Tournament status = %s, want in_progress", status)
	}
	
	// StartTournament automatically creates pairings for round 1
	// Verify pairings exist
	tournament = ts.GetTournament()
	round := tournament.GetRound()
	if len(round) == 0 {
		t.Error("No pairings created after StartTournament")
	}
}

func TestAddResult(t *testing.T) {
	ts := setupTestStorage(t)
	
	// Setup tournament with 2 players
	ts.AddPendingPlayer("Player1")
	ts.AddPendingPlayer("Player2")
	ts.AcceptPlayer("Player1")
	ts.AcceptPlayer("Player2")
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.StartTournament()
	})
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.Pair(false)
	})
	
	// Get player ID
	tournament := ts.GetTournament()
	playerID, found := tournament.GetPlayerID("Player1")
	if !found {
		t.Fatal("Player1 not found")
	}
	
	// Add result
	err := ts.UpdateTournament(func(t *st.Tournament) error {
		return t.AddResult(playerID, 2, 1, 0)
	})
	if err != nil {
		t.Errorf("AddResult() error = %v", err)
	}
}

func TestUpdateStandings(t *testing.T) {
	ts := setupTestStorage(t)
	
	// Setup tournament
	players := []string{"A", "B"}
	for _, p := range players {
		ts.AddPendingPlayer(p)
		ts.AcceptPlayer(p)
	}
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.StartTournament()
	})
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.Pair(false)
	})
	
	// Add results
	tournament := ts.GetTournament()
	round := tournament.GetRound()
	if len(round) < 1 {
		t.Fatal("No pairings")
	}
	
	playerA := round[0].PlayerA()
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.AddResult(playerA, 2, 1, 0)
	})
	
	// Update standings
	err := ts.UpdateTournament(func(t *st.Tournament) error {
		return t.UpdatePlayerStandings()
	})
	if err != nil {
		t.Errorf("UpdatePlayerStandings() error = %v", err)
	}
	
	// Verify standings exist
	tournament = ts.GetTournament()
	standings := tournament.GetStandings()
	if len(standings) == 0 {
		t.Error("No standings after update")
	}
}

