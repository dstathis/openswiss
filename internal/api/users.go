package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/validate"
	"github.com/go-chi/chi/v5"
)

type UsersAPI struct {
	DB *sql.DB
}

func (a *UsersAPI) GetMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	jsonResponse(w, http.StatusOK, user)
}

// UpdateMe is the API twin of the account settings page. Every field is
// optional; only the ones present in the body are touched. Changing the
// password or the email address costs current_password, because an API key
// alone must not be enough to take over the account it belongs to.
func (a *UsersAPI) UpdateMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())

	var body struct {
		DisplayName     *string `json:"display_name"`
		Email           *string `json:"email"`
		Password        *string `json:"password"`
		CurrentPassword string  `json:"current_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.DisplayName == nil && body.Email == nil && body.Password == nil {
		jsonError(w, http.StatusBadRequest, "nothing to update")
		return
	}

	if body.Password != nil || body.Email != nil {
		if body.CurrentPassword == "" {
			jsonError(w, http.StatusBadRequest, "current_password is required to change your password or email")
			return
		}
		if !auth.CheckPassword(user.PasswordHash, body.CurrentPassword) {
			jsonError(w, http.StatusForbidden, "current password is incorrect")
			return
		}
	}

	// Validate everything before writing anything, so a bad email can't land
	// after a display name has already been committed.
	var displayName, addr, password string
	if body.DisplayName != nil {
		displayName = strings.TrimSpace(*body.DisplayName)
		if msg, ok := validate.DisplayName(displayName); !ok {
			jsonError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if body.Email != nil {
		addr = strings.TrimSpace(*body.Email)
		if msg, ok := validate.Email(addr); !ok {
			jsonError(w, http.StatusBadRequest, msg)
			return
		}
	}
	if body.Password != nil {
		password = *body.Password
		// No confirmation field over JSON — the caller sent exactly one value.
		if msg, ok := validate.NewPassword(password, password); !ok {
			jsonError(w, http.StatusBadRequest, msg)
			return
		}
	}

	if body.DisplayName != nil && displayName != user.DisplayName {
		err := db.UpdateUserDisplayName(r.Context(), a.DB, user.ID, displayName)
		if errors.Is(err, db.ErrDisplayNameTaken) {
			jsonError(w, http.StatusConflict, "display name already taken")
			return
		}
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to update display name")
			return
		}
	}

	// The API has no way to click a confirmation link, so an email change here
	// applies immediately and drops the address's verified status — the user
	// can re-verify from /account or via POST /resend-verification.
	if body.Email != nil && !strings.EqualFold(addr, user.Email) {
		err := db.UpdateUserEmail(r.Context(), a.DB, user.ID, addr, false)
		if errors.Is(err, db.ErrEmailTaken) {
			jsonError(w, http.StatusConflict, "email already in use")
			return
		}
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to update email")
			return
		}
	}

	if body.Password != nil {
		hash, err := auth.HashPassword(password)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		if err := db.UpdateUserPassword(r.Context(), a.DB, user.ID, hash); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to update password")
			return
		}
		// Revoke other sessions. An API-key caller has no session cookie, so
		// exceptID is empty and all of them go.
		exceptID := ""
		if cookie, err := r.Cookie("session"); err == nil {
			exceptID = cookie.Value
		}
		if err := db.DeleteUserSessionsExcept(r.Context(), a.DB, user.ID, exceptID); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to revoke other sessions")
			return
		}
	}

	updated, err := db.GetUserByID(r.Context(), a.DB, user.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	jsonResponse(w, http.StatusOK, updated)
}

func (a *UsersAPI) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}

	fullKey, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	hash, err := auth.HashAPIKey(fullKey)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to hash key")
		return
	}

	key, err := db.CreateAPIKey(r.Context(), a.DB, user.ID, hash, prefix, body.Name, nil)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to create key")
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":         key.ID,
		"key":        fullKey,
		"prefix":     prefix,
		"name":       key.Name,
		"created_at": key.CreatedAt,
	})
}

func (a *UsersAPI) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	keys, err := db.ListAPIKeysByUser(r.Context(), a.DB, user.ID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}
	jsonResponse(w, http.StatusOK, keys)
}

func (a *UsersAPI) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	keyID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := db.DeleteAPIKey(r.Context(), a.DB, keyID, user.ID); err != nil {
		jsonError(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
