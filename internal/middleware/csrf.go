package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

const (
	csrfCookieName = "csrf_token"
	csrfFieldName  = "csrf_token"
	csrfTokenLen   = 32
)

type csrfContextKey struct{}

func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CSRFProtect is middleware that sets a CSRF cookie and validates it on
// state-changing requests (POST, PUT, PATCH, DELETE) for non-API routes.
// API routes (starting with /api/) are excluded since they use Bearer tokens.
func CSRFProtect(secureCookie bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF for API routes (they use Bearer token auth)
			if strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// Ensure a CSRF cookie exists
			cookie, err := r.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				token, err := generateCSRFToken()
				if err != nil {
					http.Error(w, "Internal error", http.StatusInternalServerError)
					return
				}
				http.SetCookie(w, &http.Cookie{
					Name:     csrfCookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: false, // JS needs to read it for htmx
					Secure:   secureCookie,
					SameSite: http.SameSiteLaxMode,
				})
				cookie = &http.Cookie{Value: token}
			}

			// Store token in context for templates
			ctx := context.WithValue(r.Context(), csrfContextKey{}, cookie.Value)
			r = r.WithContext(ctx)

			// Validate CSRF token on state-changing methods
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				formToken := r.FormValue(csrfFieldName)
				if formToken == "" {
					formToken = r.Header.Get("X-CSRF-Token")
				}
				if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(formToken)) != 1 {
					http.Error(w, "Forbidden — invalid CSRF token", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CSRFToken extracts the current CSRF token from the request context.
func CSRFToken(r *http.Request) string {
	tok, _ := r.Context().Value(csrfContextKey{}).(string)
	return tok
}
