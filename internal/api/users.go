package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/go-chi/chi/v5"
)

type UsersAPI struct {
	DB *sql.DB
}

func (a *UsersAPI) GetMe(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r.Context())
	jsonResponse(w, http.StatusOK, user)
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
