package handlers

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/email"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/validate"
	"github.com/go-chi/chi/v5"
)

// AccountHandler serves the self-service account settings page: display name,
// password, email address and API keys.
type AccountHandler struct {
	DB      *sql.DB
	Tmpl    TemplateRenderer
	Email   *email.Sender
	BaseURL string
}

// render draws the account page. It deliberately re-reads the user from the
// database rather than using middleware.GetUser: the context user was loaded
// when the request arrived and is stale the moment a rename or email change
// commits, which would show the user their old details right after a
// successful update.
func (h *AccountHandler) render(w http.ResponseWriter, r *http.Request, extra map[string]interface{}) {
	ctxUser := middleware.GetUser(r.Context())

	user := ctxUser
	if fresh, err := db.GetUserByID(r.Context(), h.DB, ctxUser.ID); err == nil {
		user = fresh
	} else {
		slog.ErrorContext(r.Context(), "account: reload user", "err", err, "user_id", ctxUser.ID)
	}

	keys, err := db.ListAPIKeysByUser(r.Context(), h.DB, user.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "account: list api keys", "err", err, "user_id", user.ID)
	}

	data := map[string]interface{}{
		"User":        user,
		"APIKeys":     keys,
		"SMTPEnabled": smtpEnabled(h.Email),
		"CSRFToken":   middleware.CSRFToken(r),
	}
	for k, v := range extra {
		data[k] = v
	}
	h.Tmpl.ExecuteTemplate(w, "account.html", data)
}

func (h *AccountHandler) renderError(w http.ResponseWriter, r *http.Request, msg string) {
	h.render(w, r, map[string]interface{}{"Error": msg})
}

func (h *AccountHandler) Page(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, nil)
}

func (h *AccountHandler) UpdateDisplayName(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	displayName := strings.TrimSpace(r.FormValue("display_name"))

	if msg, ok := validate.DisplayName(displayName); !ok {
		h.renderError(w, r, msg)
		return
	}
	if displayName == user.DisplayName {
		h.render(w, r, map[string]interface{}{"Success": "Display name unchanged."})
		return
	}

	err := db.UpdateUserDisplayName(r.Context(), h.DB, user.ID, displayName)
	if errors.Is(err, db.ErrDisplayNameTaken) {
		h.renderError(w, r, "That display name is already taken.")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "account: update display name", "err", err, "user_id", user.ID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	h.render(w, r, map[string]interface{}{
		"Success": "Display name updated. Tournaments that have already started keep the name you registered with.",
	})
}

func (h *AccountHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	currentPassword := r.FormValue("current_password")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// A wrong current password deliberately does NOT feed db.RecordFailedLogin:
	// that would let anyone holding a stolen session lock the real owner out of
	// their own account. The per-IP rate limit on this route is the
	// brute-force control instead.
	if !auth.CheckPassword(user.PasswordHash, currentPassword) {
		h.renderError(w, r, "Current password is incorrect.")
		return
	}
	if msg, ok := validate.NewPassword(password, confirmPassword); !ok {
		h.renderError(w, r, msg)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if err := db.UpdateUserPassword(r.Context(), h.DB, user.ID, hash); err != nil {
		slog.ErrorContext(r.Context(), "account: update password", "err", err, "user_id", user.ID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Evict every other session so a password change actually kicks out
	// anyone else who is already signed in as this user.
	if err := db.DeleteUserSessionsExcept(r.Context(), h.DB, user.ID, currentSessionID(r)); err != nil {
		slog.ErrorContext(r.Context(), "account: revoke other sessions", "err", err, "user_id", user.ID)
	}

	h.render(w, r, map[string]interface{}{
		"Success": "Password changed. Any other devices have been signed out.",
	})
}

func (h *AccountHandler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	currentPassword := r.FormValue("current_password")
	addr := strings.TrimSpace(r.FormValue("email"))

	// Changing the login address is an account-takeover move, so it costs the
	// current password just like a password change does.
	if !auth.CheckPassword(user.PasswordHash, currentPassword) {
		h.renderError(w, r, "Current password is incorrect.")
		return
	}
	if msg, ok := validate.Email(addr); !ok {
		h.renderError(w, r, msg)
		return
	}
	if strings.EqualFold(addr, user.Email) {
		h.render(w, r, map[string]interface{}{"Success": "Email address unchanged."})
		return
	}

	// Without SMTP there is no link to send, so the change applies straight
	// away — mirroring how registration auto-verifies in that configuration.
	if !smtpEnabled(h.Email) {
		err := db.UpdateUserEmail(r.Context(), h.DB, user.ID, addr, true)
		if errors.Is(err, db.ErrEmailTaken) {
			h.renderError(w, r, "That email address is already in use.")
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "account: update email", "err", err, "user_id", user.ID)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		h.render(w, r, map[string]interface{}{"Success": "Email address updated."})
		return
	}

	// Reject an address that's already taken before emailing it — otherwise we
	// send a confirmation link that is guaranteed to fail, and we'd be mailing
	// an address on behalf of someone who didn't ask for it.
	if _, err := db.GetUserByEmail(r.Context(), h.DB, addr); err == nil {
		h.renderError(w, r, "That email address is already in use.")
		return
	}

	if err := sendVerificationEmail(r.Context(), h.DB, h.Email, h.BaseURL, user.ID, addr, &addr); err != nil {
		slog.ErrorContext(r.Context(), "account: send email change verification", "err", err, "user_id", user.ID)
		h.renderError(w, r, "Could not send the confirmation email. Please try again later.")
		return
	}

	h.render(w, r, map[string]interface{}{
		"Success": "Confirmation link sent to " + addr + ". Your address changes once you click it.",
	})
}

func (h *AccountHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	user := middleware.GetUser(r.Context())
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderError(w, r, "API key name is required.")
		return
	}

	fullKey, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	hash, err := auth.HashAPIKey(fullKey)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if _, err := db.CreateAPIKey(r.Context(), h.DB, user.ID, hash, prefix, name, nil); err != nil {
		slog.ErrorContext(r.Context(), "account: create api key", "err", err, "user_id", user.ID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// The full key is shown here and never again — only its hash is stored.
	h.render(w, r, map[string]interface{}{
		"NewAPIKey":     fullKey,
		"NewAPIKeyName": name,
	})
}

func (h *AccountHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, "Invalid API key.")
		return
	}

	// DeleteAPIKey is scoped to the owner, so another user's key id is a
	// no-op rather than a deletion.
	if err := db.DeleteAPIKey(r.Context(), h.DB, id, user.ID); err != nil {
		h.renderError(w, r, "API key not found.")
		return
	}

	h.render(w, r, map[string]interface{}{"Success": "API key revoked."})
}

// currentSessionID returns the session the request is authenticated with, or
// "" when it arrived with an API key instead.
func currentSessionID(r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return cookie.Value
}
