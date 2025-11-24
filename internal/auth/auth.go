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

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	RoleAdmin  = "admin"
	RolePlayer = "player"
)

type sessionKey struct {
	role string
}

type Auth struct {
	adminPassword string
	sessions      map[string]*Session
	mu            sync.RWMutex
}

type Session struct {
	Role      string
	CreatedAt time.Time
}

func NewAuth(adminPassword string) *Auth {
	// Default admin password if not set
	if adminPassword == "" {
		adminPassword = "admin123" // Should be changed in production
	}

	return &Auth{
		adminPassword: adminPassword,
		sessions:    make(map[string]*Session),
	}
}

func (a *Auth) LoginAdmin(password string) (string, error) {
	if password != a.adminPassword {
		return "", fmt.Errorf("invalid password")
	}

	sessionID := a.generateSessionID()
	a.mu.Lock()
	a.sessions[sessionID] = &Session{
		Role:      RoleAdmin,
		CreatedAt: time.Now(),
	}
	a.mu.Unlock()

	return sessionID, nil
}

func (a *Auth) LoginPlayer() string {
	sessionID := a.generateSessionID()
	a.mu.Lock()
	a.sessions[sessionID] = &Session{
		Role:      RolePlayer,
		CreatedAt: time.Now(),
	}
	a.mu.Unlock()

	return sessionID
}

func (a *Auth) GetSession(sessionID string) (*Session, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, ok := a.sessions[sessionID]
	return session, ok
}

func (a *Auth) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := a.getSessionID(r)
		if sessionID == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session, ok := a.GetSession(sessionID)
		if !ok || session.Role != RoleAdmin {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), sessionKey{}, session)
		next(w, r.WithContext(ctx))
	}
}

func (a *Auth) RequirePlayer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := a.getSessionID(r)
		if sessionID == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		session, ok := a.GetSession(sessionID)
		if !ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), sessionKey{}, session)
		next(w, r.WithContext(ctx))
	}
}

func (a *Auth) OptionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := a.getSessionID(r)
		if sessionID != "" {
			if session, ok := a.GetSession(sessionID); ok {
				ctx := context.WithValue(r.Context(), sessionKey{}, session)
				r = r.WithContext(ctx)
			}
		}
		next(w, r)
	}
}

func (a *Auth) getSessionID(r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// GetSessionContext is a helper for tests to add session to context
func (a *Auth) GetSessionContext(ctx context.Context, sessionID string) context.Context {
	session, ok := a.GetSession(sessionID)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, session)
}

func (a *Auth) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *Auth) ClearSession(w http.ResponseWriter, sessionID string) {
	a.mu.Lock()
	delete(a.sessions, sessionID)
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func (a *Auth) generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func GetSessionFromContext(ctx context.Context) (*Session, bool) {
	session, ok := ctx.Value(sessionKey{}).(*Session)
	return session, ok
}

