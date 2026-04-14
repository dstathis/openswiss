package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
	"github.com/go-chi/chi/v5"
)

type AdminHandler struct {
	DB   *sql.DB
	Tmpl TemplateRenderer
}

func (h *AdminHandler) UsersPage(w http.ResponseWriter, r *http.Request) {
	users, _ := db.ListUsers(r.Context(), h.DB, 1, 100)
	h.Tmpl.ExecuteTemplate(w, "admin_users.html", map[string]interface{}{
		"User":  middleware.GetUser(r.Context()),
		"Users": users,
	})
}

func (h *AdminHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	roles := r.Form["roles"]
	if len(roles) == 0 {
		roles = []string{models.RolePlayer}
	}
	db.UpdateUserRoles(r.Context(), h.DB, userID, roles)
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}
