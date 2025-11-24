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

package handlers

import (
	"html/template"
	"net/http"
	"openswiss/internal/auth"
)

type AuthHandlers struct {
	auth *auth.Auth
	tmpl *template.Template
}

func NewAuthHandlers(a *auth.Auth) *AuthHandlers {
	tmpl := template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	})
	tmpl = template.Must(tmpl.ParseFiles("templates/base.html"))
	// Parse all templates so base.html can access all content blocks
	tmpl = template.Must(tmpl.ParseGlob("templates/player/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("templates/admin/*.html"))
	tmpl = template.Must(tmpl.ParseGlob("templates/auth/*.html"))
	return &AuthHandlers{
		auth: a,
		tmpl: tmpl,
	}
}

func (h *AuthHandlers) LoginGet(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.GetSessionFromContext(r.Context())
	data := struct {
		Template   string
		IsAdmin    bool
		IsLoggedIn bool
		Error      string
	}{
		Template:   "login",
		IsAdmin:    session != nil && session.Role == auth.RoleAdmin,
		IsLoggedIn: session != nil,
	}

	if err := h.tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *AuthHandlers) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	password := r.FormValue("password")

	sessionID, err := h.auth.LoginAdmin(password)
	if err != nil {
		session, _ := auth.GetSessionFromContext(r.Context())
		h.tmpl.ExecuteTemplate(w, "base.html", struct {
			Template   string
			IsAdmin    bool
			IsLoggedIn bool
			Error      string
		}{
			Template:   "login",
			IsAdmin:    session != nil && session.Role == auth.RoleAdmin,
			IsLoggedIn: session != nil,
			Error:      "Invalid password",
		})
		return
	}

	h.auth.SetSessionCookie(w, sessionID)
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	session, _ := auth.GetSessionFromContext(r.Context())
	if session != nil {
		// Get session ID from cookie
		cookie, _ := r.Cookie("session")
		if cookie != nil {
			h.auth.ClearSession(w, cookie.Value)
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
