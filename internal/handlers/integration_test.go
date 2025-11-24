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
	st "github.com/dstathis/swisstools"
)

func init() {
	// Ensure we're in the project root directory for template loading
	wd, _ := os.Getwd()
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			os.Chdir(dir)
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
}

func TestFullTournamentFlow(t *testing.T) {
	ph, ah, _, ts, authService := setupTestHandlers(t)
	
	// Step 1: Register players
	t.Run("RegisterPlayers", func(t *testing.T) {
		players := []string{"Alice", "Bob", "Charlie", "David"}
		for _, name := range players {
			form := url.Values{"name": {name}}
			req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			ph.RegisterPost(w, req)
			
			if w.Code != http.StatusOK {
				t.Errorf("Register %s failed with status %d", name, w.Code)
			}
		}
		
		// Verify pending
		pending := ts.GetPendingPlayers()
		if len(pending) != 4 {
			t.Errorf("Expected 4 pending players, got %d", len(pending))
		}
	})
	
	// Step 2: Login as admin
	t.Run("AdminLogin", func(t *testing.T) {
		sessionID, err := authService.LoginAdmin("testpass")
		if err != nil {
			t.Fatalf("Admin login failed: %v", err)
		}
		if sessionID == "" {
			t.Fatal("Admin login returned empty session")
		}
	})
	
	// Step 3: Accept all players
	t.Run("AcceptAllPlayers", func(t *testing.T) {
		sessionID, _ := authService.LoginAdmin("testpass")
		players := []string{"Alice", "Bob", "Charlie", "David"}
		
		for _, name := range players {
			form := url.Values{"name": {name}}
			req := httptest.NewRequest("POST", "/admin/accept", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
			req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
			w := httptest.NewRecorder()
			ah.AcceptPlayer(w, req)
			
			if w.Code != http.StatusSeeOther {
				t.Errorf("Accept %s failed with status %d", name, w.Code)
			}
		}
		
		// Verify all in tournament
		tournament := ts.GetTournament()
		if count := tournament.GetPlayerCount(); count != 4 {
			t.Errorf("Expected 4 players in tournament, got %d", count)
		}
	})
	
	// Step 4: Start tournament
	t.Run("StartTournament", func(t *testing.T) {
		sessionID, _ := authService.LoginAdmin("testpass")
		req := httptest.NewRequest("POST", "/admin/start", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
		req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
		w := httptest.NewRecorder()
		ah.StartTournament(w, req)
		
		if w.Code != http.StatusSeeOther {
			t.Errorf("Start tournament failed with status %d", w.Code)
		}
		
		tournament := ts.GetTournament()
		if status := tournament.GetStatus(); status != "in_progress" {
			t.Errorf("Tournament status = %s, want in_progress", status)
		}
	})
	
	// Step 5: Verify pairings were created (StartTournament automatically creates them)
	t.Run("VerifyPairings", func(t *testing.T) {
		tournament := ts.GetTournament()
		round := tournament.GetRound()
		if len(round) == 0 {
			t.Error("No pairings created after StartTournament")
		}
	})
	
	// Step 6: Record results for all matches
	t.Run("RecordResults", func(t *testing.T) {
		sessionID, _ := authService.LoginAdmin("testpass")
		tournament := ts.GetTournament()
		round := tournament.GetRound()
		
		for _, pairing := range round {
			if pairing.PlayerB() == st.BYE_OPPONENT_ID {
				continue // Skip byes
			}
			
			// Record result for Player A: 2-1
			form := url.Values{
				"player_id": {strconv.Itoa(pairing.PlayerA())},
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
				t.Errorf("Add result failed with status %d", w.Code)
			}
		}
	})
	
	// Step 7: Update standings
	t.Run("UpdateStandings", func(t *testing.T) {
		sessionID, _ := authService.LoginAdmin("testpass")
		req := httptest.NewRequest("POST", "/admin/update-standings", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
		req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
		w := httptest.NewRecorder()
		ah.UpdateStandings(w, req)
		
		if w.Code != http.StatusSeeOther {
			t.Errorf("Update standings failed with status %d", w.Code)
		}
		
		tournament := ts.GetTournament()
		standings := tournament.GetStandings()
		if len(standings) == 0 {
			t.Error("No standings after update")
		}
	})
	
	// Step 8: Verify standings and pairings are visible
	t.Run("ViewStandings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/standings", nil)
		w := httptest.NewRecorder()
		ph.Standings(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("View standings failed with status %d", w.Code)
		}
	})
	
	t.Run("ViewPairings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/pairings", nil)
		w := httptest.NewRecorder()
		ph.Pairings(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("View pairings failed with status %d", w.Code)
		}
	})
	
	// Step 9: Next round
	t.Run("NextRound", func(t *testing.T) {
		sessionID, _ := authService.LoginAdmin("testpass")
		req := httptest.NewRequest("POST", "/admin/next-round", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
		req = req.WithContext(authService.GetSessionContext(req.Context(), sessionID))
		w := httptest.NewRecorder()
		ah.NextRound(w, req)
		
		if w.Code != http.StatusSeeOther {
			t.Errorf("Next round failed with status %d", w.Code)
		}
		
		tournament := ts.GetTournament()
		if round := tournament.GetCurrentRound(); round != 2 {
			t.Errorf("Expected round 2, got %d", round)
		}
	})
}

