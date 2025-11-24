package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"openswiss/internal/auth"
	"openswiss/internal/storage"
	st "github.com/dstathis/swisstools"
)

func setupTestHandlers(t *testing.T) (*PlayerHandlers, *AdminHandlers, *AuthHandlers, *storage.TournamentStorage, *auth.Auth) {
	// Ensure we're in the project root directory
	// Find project root by looking for go.mod
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	
	// Walk up to find go.mod
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			os.Chdir(dir)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, give up
			break
		}
		dir = parent
	}
	
	// Clean up test files for isolation
	os.Remove("data/tournament.json")
	os.Remove("data/pending_players.json")
	
	t.Cleanup(func() {
		os.Remove("data/tournament.json")
		os.Remove("data/pending_players.json")
	})
	
	ts, err := storage.NewTournamentStorage()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	
	authService := auth.NewAuth("testpass")
	playerHandlers := NewPlayerHandlers(ts, authService)
	adminHandlers := NewAdminHandlers(ts, authService)
	authHandlers := NewAuthHandlers(authService)
	
	return playerHandlers, adminHandlers, authHandlers, ts, authService
}

func TestPlayerRegistration(t *testing.T) {
	ph, _, _, _, _ := setupTestHandlers(t)
	
	tests := []struct {
		name    string
		method  string
		form    map[string]string
		wantCode int
	}{
		{"GET registration", "GET", nil, http.StatusOK},
		{"POST valid registration", "POST", map[string]string{"name": "Alice"}, http.StatusOK},
		{"POST empty name", "POST", map[string]string{"name": ""}, http.StatusOK}, // Shows error
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.method == "GET" {
				req = httptest.NewRequest("GET", "/register", nil)
			} else {
				form := url.Values{}
				for k, v := range tt.form {
					form.Set(k, v)
				}
				req = httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			
			w := httptest.NewRecorder()
			if tt.method == "GET" {
				ph.RegisterGet(w, req)
			} else {
				ph.RegisterPost(w, req)
			}
			
			if w.Code != tt.wantCode {
				t.Errorf("Register() status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestAdminAcceptPlayer(t *testing.T) {
	_, ah, _, ts, authService := setupTestHandlers(t)
	
	// Add a pending player
	ts.AddPendingPlayer("TestPlayer")
	
	// Login as admin
	sessionID, _ := authService.LoginAdmin("testpass")
	
	// Accept player
	form := url.Values{"name": {"TestPlayer"}}
	req := httptest.NewRequest("POST", "/admin/accept", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
	req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
	
	w := httptest.NewRecorder()
	ah.AcceptPlayer(w, req)
	
	if w.Code != http.StatusSeeOther {
		t.Errorf("AcceptPlayer() status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	
	// Verify player accepted
	tournament := ts.GetTournament()
	if _, found := tournament.GetPlayerByName("TestPlayer"); !found {
		t.Error("Player not found in tournament after accept")
	}
}

func TestStartTournament(t *testing.T) {
	_, ah, _, ts, authService := setupTestHandlers(t)
	
	// Add and accept players
	players := []string{"Alice", "Bob"}
	for _, p := range players {
		ts.AddPendingPlayer(p)
		ts.AcceptPlayer(p)
	}
	
	// Login as admin
	sessionID, _ := authService.LoginAdmin("testpass")
	
	// Start tournament
	req := httptest.NewRequest("POST", "/admin/start", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
	req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
	
	w := httptest.NewRecorder()
	ah.StartTournament(w, req)
	
	if w.Code != http.StatusSeeOther {
		t.Errorf("StartTournament() status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	
	// Verify tournament started
	tournament := ts.GetTournament()
	if status := tournament.GetStatus(); status != "in_progress" {
		t.Errorf("Tournament status = %s, want in_progress", status)
	}
}

func TestPairings(t *testing.T) {
	ph, _, _, ts, _ := setupTestHandlers(t)
	
	// Setup tournament with players
	players := []string{"Alice", "Bob", "Charlie"}
	for _, p := range players {
		ts.AddPendingPlayer(p)
		ts.AcceptPlayer(p)
	}
	
	// Start tournament
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.StartTournament()
	})
	
	// Create pairings
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.Pair(false)
	})
	
	// Test pairings page
	req := httptest.NewRequest("GET", "/pairings", nil)
	w := httptest.NewRecorder()
	ph.Pairings(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Pairings() status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestStandings(t *testing.T) {
	ph, _, _, ts, _ := setupTestHandlers(t)
	
	// Setup tournament with results
	players := []string{"Alice", "Bob"}
	for _, p := range players {
		ts.AddPendingPlayer(p)
		ts.AcceptPlayer(p)
	}
	
	// Start, pair, add result, update standings
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.StartTournament()
	})
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.Pair(false)
	})
	
	// Get player ID and add result
	tournament := ts.GetTournament()
	playerID, found := tournament.GetPlayerID("Alice")
	if !found {
		t.Fatal("Alice not found in tournament")
	}
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.AddResult(playerID, 2, 1, 0)
	})
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		return t.UpdatePlayerStandings()
	})
	
	req := httptest.NewRequest("GET", "/standings", nil)
	w := httptest.NewRecorder()
	ph.Standings(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Standings() status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAddResult(t *testing.T) {
	_, ah, _, ts, authService := setupTestHandlers(t)
	
	// Setup tournament
	players := []string{"PlayerA", "PlayerB"}
	for _, p := range players {
		ts.AddPendingPlayer(p)
		ts.AcceptPlayer(p)
	}
	
	ts.UpdateTournament(func(t *st.Tournament) error {
		if err := t.StartTournament(); err != nil {
			return err
		}
		return t.Pair(false)
	})
	
	// Get player ID
	tournament := ts.GetTournament()
	playerID, found := tournament.GetPlayerID("PlayerA")
	if !found {
		t.Fatal("PlayerA not found")
	}
	
	// Login
	sessionID, _ := authService.LoginAdmin("testpass")
	
	// Add result
	form := url.Values{
		"player_id": {strconv.Itoa(playerID)},
		"wins":      {"2"},
		"losses":    {"1"},
		"draws":     {"0"},
	}
	req := httptest.NewRequest("POST", "/admin/add-result", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
	req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
	
	w := httptest.NewRecorder()
	ah.AddResult(w, req)
	
	if w.Code != http.StatusSeeOther {
		t.Errorf("AddResult() status = %d, want %d", w.Code, http.StatusSeeOther)
	}
}

