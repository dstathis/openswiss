package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/go-chi/chi/v5"
)

type AdminAPI struct {
	DB *sql.DB
}

func (a *AdminAPI) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, perPage := paginationParams(r)
	users, err := db.ListUsers(r.Context(), a.DB, page, perPage)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	if users == nil {
		users = []models.User{}
	}
	jsonResponse(w, http.StatusOK, users)
}

func (a *AdminAPI) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var body struct {
		Roles []string `json:"roles"`
	}
	if err := decodeJSON(r, &body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Roles) == 0 {
		body.Roles = []string{models.RolePlayer}
	}
	if err := db.UpdateUserRoles(r.Context(), a.DB, userID, body.Roles); err != nil {
		jsonError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	user, _ := db.GetUserByID(r.Context(), a.DB, userID)
	jsonResponse(w, http.StatusOK, user)
}
