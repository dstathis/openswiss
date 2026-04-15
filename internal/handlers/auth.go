package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/email"
	"github.com/dstathis/openswiss/internal/middleware"
)

var emailRegexp = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type AuthHandler struct {
	DB            *sql.DB
	Tmpl          TemplateRenderer
	Email         *email.Sender
	BaseURL       string
	SecureCookies bool
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.Tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
		"User":      middleware.GetUser(r.Context()),
		"CSRFToken": middleware.CSRFToken(r),
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := db.GetUserByEmail(r.Context(), h.DB, email)
	if err != nil || !auth.CheckPassword(user.PasswordHash, password) {
		h.Tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
			"Error":     "Invalid email or password.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	token, err := auth.GenerateSessionToken()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if err := db.CreateSession(r.Context(), h.DB, token, user.ID, expiresAt); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
		"User":      middleware.GetUser(r.Context()),
		"CSRFToken": middleware.CSRFToken(r),
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	displayName := r.FormValue("display_name")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if email == "" || displayName == "" || password == "" {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "All fields are required.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if !emailRegexp.MatchString(email) {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "Please enter a valid email address.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if len(email) > 254 {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "Email address is too long.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if len(displayName) > 100 {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "Display name must be 100 characters or fewer.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if len(password) < 8 {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "Password must be at least 8 characters.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if password != confirmPassword {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "Passwords do not match.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	user, err := db.CreateUser(r.Context(), h.DB, email, displayName, hash)
	if err != nil {
		h.Tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{
			"Error":     "Email or display name already taken.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	// Auto-login after registration
	token, err := auth.GenerateSessionToken()
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if err := db.CreateSession(r.Context(), h.DB, token, user.ID, expiresAt); err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		db.DeleteSession(r.Context(), h.DB, cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	h.Tmpl.ExecuteTemplate(w, "forgot_password.html", map[string]interface{}{
		"User":        middleware.GetUser(r.Context()),
		"SMTPEnabled": h.Email != nil && h.Email.Config.Enabled(),
		"CSRFToken":   middleware.CSRFToken(r),
	})
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if h.Email == nil || !h.Email.Config.Enabled() {
		http.Error(w, "Password reset is not available (SMTP not configured)", http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	addr := r.FormValue("email")

	// Always show success to avoid leaking whether the email exists
	successData := map[string]interface{}{
		"User":      middleware.GetUser(r.Context()),
		"Success":   "If an account with that email exists, a reset link has been sent.",
		"CSRFToken": middleware.CSRFToken(r),
	}

	user, err := db.GetUserByEmail(r.Context(), h.DB, addr)
	if err != nil {
		h.Tmpl.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	rawToken, tokenHash, err := auth.GenerateResetToken()
	if err != nil {
		log.Printf("ERROR generating reset token: %v", err)
		h.Tmpl.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	expiresAt := time.Now().Add(1 * time.Hour)
	if err := db.CreatePasswordReset(r.Context(), h.DB, user.ID, tokenHash, expiresAt); err != nil {
		log.Printf("ERROR creating password reset: %v", err)
		h.Tmpl.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", h.BaseURL, rawToken)
	if err := h.Email.SendPasswordReset(user.Email, resetURL); err != nil {
		log.Printf("ERROR sending password reset email: %v", err)
	}

	h.Tmpl.ExecuteTemplate(w, "forgot_password.html", successData)
}

func (h *AuthHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/forgot-password", http.StatusSeeOther)
		return
	}

	tokenHash := auth.HashResetToken(token)
	_, err := db.GetPasswordResetByTokenHash(r.Context(), h.DB, tokenHash)
	if err != nil {
		h.Tmpl.ExecuteTemplate(w, "reset_password.html", map[string]interface{}{
			"Error":     "Invalid or expired reset link. Please request a new one.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	h.Tmpl.ExecuteTemplate(w, "reset_password.html", map[string]interface{}{
		"Token":     token,
		"CSRFToken": middleware.CSRFToken(r),
	})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if token == "" || password == "" {
		h.Tmpl.ExecuteTemplate(w, "reset_password.html", map[string]interface{}{
			"Error":     "All fields are required.",
			"Token":     token,
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if len(password) < 8 {
		h.Tmpl.ExecuteTemplate(w, "reset_password.html", map[string]interface{}{
			"Error":     "Password must be at least 8 characters.",
			"Token":     token,
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	if password != confirmPassword {
		h.Tmpl.ExecuteTemplate(w, "reset_password.html", map[string]interface{}{
			"Error":     "Passwords do not match.",
			"Token":     token,
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	tokenHash := auth.HashResetToken(token)
	reset, err := db.GetPasswordResetByTokenHash(r.Context(), h.DB, tokenHash)
	if err != nil {
		h.Tmpl.ExecuteTemplate(w, "reset_password.html", map[string]interface{}{
			"Error":     "Invalid or expired reset link. Please request a new one.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := db.UpdateUserPassword(r.Context(), h.DB, reset.UserID, hash); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Consume the reset token
	db.DeletePasswordReset(r.Context(), h.DB, reset.ID)

	h.Tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
		"Success":   "Password reset successfully. Please log in.",
		"CSRFToken": middleware.CSRFToken(r),
	})
}
