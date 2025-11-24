package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginAdmin(t *testing.T) {
	a := NewAuth("testpass")
	
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"correct password", "testpass", false},
		{"wrong password", "wrong", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, err := a.LoginAdmin(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoginAdmin() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && sessionID == "" {
				t.Error("LoginAdmin() returned empty session ID")
			}
		})
	}
}

func TestLoginPlayer(t *testing.T) {
	a := NewAuth("testpass")
	
	sessionID := a.LoginPlayer()
	if sessionID == "" {
		t.Error("LoginPlayer() returned empty session ID")
	}
	
	// Verify session
	session, ok := a.GetSession(sessionID)
	if !ok {
		t.Error("GetSession() returned false for valid session")
	}
	if session.Role != RolePlayer {
		t.Errorf("Session role = %s, want %s", session.Role, RolePlayer)
	}
}

func TestRequireAdmin(t *testing.T) {
	a := NewAuth("testpass")
	
	handler := a.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	
	tests := []struct {
		name    string
		session *Session
		wantCode int
	}{
		{"no session", nil, http.StatusSeeOther},
		{"player session", &Session{Role: RolePlayer}, http.StatusUnauthorized},
		{"admin session", &Session{Role: RoleAdmin}, http.StatusOK},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			var sessionID string
			if tt.session != nil {
				if tt.session.Role == RoleAdmin {
					sessionID, _ = a.LoginAdmin("testpass")
				} else {
					sessionID = a.LoginPlayer()
				}
				req.AddCookie(&http.Cookie{Name: "session", Value: sessionID})
				req = req.WithContext(a.GetSessionContext(req.Context(), sessionID))
			}
			w := httptest.NewRecorder()
			handler(w, req)
			
			if w.Code != tt.wantCode {
				t.Errorf("RequireAdmin() status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestSessionManagement(t *testing.T) {
	a := NewAuth("testpass")
	
	// Create session
	sessionID := a.LoginPlayer()
	
	// Verify it exists
	if _, ok := a.GetSession(sessionID); !ok {
		t.Error("Session not found after creation")
	}
	
	// Clear session
	w := httptest.NewRecorder()
	a.ClearSession(w, sessionID)
	
	// Verify it's gone
	if _, ok := a.GetSession(sessionID); ok {
		t.Error("Session still exists after clear")
	}
}

