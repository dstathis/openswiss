// This file is part of OpenSwiss.
//
// OpenSwiss is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// OpenSwiss is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with OpenSwiss. If not, see <https://www.gnu.org/licenses/>.

package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	st "github.com/dstathis/swisstools"
)

const tournamentFile = "data/tournament.json"
const pendingPlayersFile = "data/pending_players.json"

type TournamentStorage struct {
	mu           sync.RWMutex
	tournament   st.Tournament
	pendingPlayers []PendingPlayer
}

type PendingPlayer struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pending", "accepted", "rejected"
}

func NewTournamentStorage() (*TournamentStorage, error) {
	ts := &TournamentStorage{
		pendingPlayers: make([]PendingPlayer, 0),
		tournament:     st.NewTournament(), // Initialize empty tournament
	}

	// Load tournament if it exists
	if err := ts.loadTournament(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load tournament: %w", err)
	}

	// Load pending players if they exist
	if err := ts.loadPendingPlayers(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load pending players: %w", err)
	}

	return ts, nil
}

func (ts *TournamentStorage) loadTournament() error {
	data, err := os.ReadFile(tournamentFile)
	if err != nil {
		return err
	}

	tournament, err := st.LoadTournament(data)
	if err != nil {
		return fmt.Errorf("failed to parse tournament: %w", err)
	}

	ts.mu.Lock()
	ts.tournament = tournament
	ts.mu.Unlock()

	return nil
}

func (ts *TournamentStorage) loadPendingPlayers() error {
	data, err := os.ReadFile(pendingPlayersFile)
	if err != nil {
		return err
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	return json.Unmarshal(data, &ts.pendingPlayers)
}

func (ts *TournamentStorage) saveTournament() error {
	// Note: This function assumes the caller already holds the lock
	// Do NOT acquire another lock here to avoid deadlock
	data, err := ts.tournament.DumpTournament()
	if err != nil {
		return fmt.Errorf("failed to dump tournament: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(tournamentFile), 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	if err := os.WriteFile(tournamentFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tournament file: %w", err)
	}

	return nil
}

func (ts *TournamentStorage) savePendingPlayers() error {
	// Note: This function assumes the caller already holds the lock
	// Do NOT acquire another lock here to avoid deadlock
	data, err := json.Marshal(ts.pendingPlayers)
	if err != nil {
		return fmt.Errorf("failed to marshal pending players: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(pendingPlayersFile), 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	if err := os.WriteFile(pendingPlayersFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write pending players file: %w", err)
	}

	return nil
}

func (ts *TournamentStorage) GetTournament() st.Tournament {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.tournament
}

func (ts *TournamentStorage) UpdateTournament(fn func(*st.Tournament) error) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if err := fn(&ts.tournament); err != nil {
		return err
	}

	return ts.saveTournament()
}

func (ts *TournamentStorage) AddPendingPlayer(name string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check if already pending or accepted
	for _, pp := range ts.pendingPlayers {
		if pp.Name == name && pp.Status == "pending" {
			return fmt.Errorf("player %s is already pending", name)
		}
	}

	// Check if player is already in tournament
	tournament := ts.tournament
	if _, found := tournament.GetPlayerByName(name); found {
		return fmt.Errorf("player %s is already in the tournament", name)
	}

	ts.pendingPlayers = append(ts.pendingPlayers, PendingPlayer{
		Name:   name,
		Status: "pending",
	})

	return ts.savePendingPlayers()
}

func (ts *TournamentStorage) GetPendingPlayers() []PendingPlayer {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	pending := make([]PendingPlayer, 0)
	for _, pp := range ts.pendingPlayers {
		if pp.Status == "pending" {
			pending = append(pending, pp)
		}
	}
	return pending
}

func (ts *TournamentStorage) AcceptPlayer(name string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Find player (case-insensitive match to be safe) - but DON'T change status yet
	found := false
	var actualName string
	var playerIndex int
	nameLower := strings.ToLower(strings.TrimSpace(name))
	for i := range ts.pendingPlayers {
		pendingNameLower := strings.ToLower(strings.TrimSpace(ts.pendingPlayers[i].Name))
		if pendingNameLower == nameLower && ts.pendingPlayers[i].Status == "pending" {
			actualName = ts.pendingPlayers[i].Name // Use the stored name exactly as it was registered
			playerIndex = i
			found = true
			break
		}
	}

	if !found {
		// Return helpful error with available pending players (check BEFORE any modifications)
		pendingNames := make([]string, 0)
		for _, pp := range ts.pendingPlayers {
			if pp.Status == "pending" {
				pendingNames = append(pendingNames, fmt.Sprintf("%q", pp.Name))
			}
		}
		if len(pendingNames) > 0 {
			return fmt.Errorf("player %q not found in pending list. Available: %s", name, strings.Join(pendingNames, ", "))
		}
		return fmt.Errorf("player %q not found in pending list (no pending players)", name)
	}

	// Try to add to tournament FIRST before changing status
	if err := ts.tournament.AddPlayer(actualName); err != nil {
		// Don't change status since we never changed it
		return fmt.Errorf("failed to add player to tournament: %w", err)
	}

	// Only mark as accepted after successful addition
	ts.pendingPlayers[playerIndex].Status = "accepted"

	if err := ts.saveTournament(); err != nil {
		// Rollback status on save failure
		ts.pendingPlayers[playerIndex].Status = "pending"
		return err
	}

	if err := ts.savePendingPlayers(); err != nil {
		// Rollback status on save failure
		ts.pendingPlayers[playerIndex].Status = "pending"
		// Note: tournament was already saved, so player is in tournament but status shows as pending
		// This is a partial failure state
		return err
	}

	return nil
}

func (ts *TournamentStorage) RejectPlayer(name string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	found := false
	for i := range ts.pendingPlayers {
		if ts.pendingPlayers[i].Name == name && ts.pendingPlayers[i].Status == "pending" {
			ts.pendingPlayers[i].Status = "rejected"
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("player %s not found in pending list", name)
	}

	return ts.savePendingPlayers()
}

