package middleware

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
)

type contextKey string

const UserContextKey contextKey = "user"

func GetUser(ctx context.Context) *models.User {
	u, _ := ctx.Value(UserContextKey).(*models.User)
	return u
}

// SessionAuth populates the user from session cookie (for web UI).
func SessionAuth(database *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			sess, err := db.GetSession(r.Context(), database, cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			user, err := db.GetUserByID(r.Context(), database, sess.UserID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyAuth populates the user from Bearer token (for REST API).
func APIKeyAuth(database *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If user already set from session, skip
			if GetUser(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}
			token := strings.TrimPrefix(header, "Bearer ")
			if len(token) < 11 {
				next.ServeHTTP(w, r)
				return
			}
			prefix := token[:11]
			keys, err := db.GetAPIKeysByPrefix(r.Context(), database, prefix)
			if err != nil || len(keys) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			for _, k := range keys {
				if auth.CheckAPIKey(k.KeyHash, token) {
					user, err := db.GetUserByID(r.Context(), database, k.UserID)
					// Update last used (fire and forget)
					go db.UpdateAPIKeyLastUsed(context.Background(), database, k.ID)
					if err != nil {
						break
					}
					ctx := context.WithValue(r.Context(), UserContextKey, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth returns 401/redirect if no user is authenticated.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUser(r.Context()) == nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuthorizeTournament looks up the requester's effective tier on the given
// tournament and writes a 403 (or 500 on lookup error) if they don't hold at
// least `min`. Returns true when the request should proceed. Designed as a
// one-line gate at the top of tournament-management handlers, replacing the
// older inline "OrganizerID != user.ID && !HasRole(admin)" pattern.
func AuthorizeTournament(w http.ResponseWriter, r *http.Request, database *sql.DB, tournamentID int64, min models.TournamentTier) bool {
	user := GetUser(r.Context())
	tier, err := db.EffectiveTournamentTier(r.Context(), database, tournamentID, user)
	if err != nil {
		writeTournamentAuthError(w, r, http.StatusInternalServerError, "internal error")
		return false
	}
	if !tier.AtLeast(min) {
		writeTournamentAuthError(w, r, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

func writeTournamentAuthError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error":%q}`, msg)
		return
	}
	// Web UI: title-case the message to match the existing http.Error style.
	if len(msg) > 0 {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
	}
	http.Error(w, msg, status)
}

// RequireRole checks that the authenticated user has the specified role.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUser(r.Context())
			if user == nil || (!user.HasRole(role) && !user.HasRole(models.RoleAdmin)) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				} else {
					http.Error(w, "Forbidden", http.StatusForbidden)
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
