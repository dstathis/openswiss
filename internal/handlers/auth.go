package handlers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/dstathis/openswiss/internal/auth"
	"github.com/dstathis/openswiss/internal/db"
	"github.com/dstathis/openswiss/internal/email"
	"github.com/dstathis/openswiss/internal/middleware"
)

var emailRegexp = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const verificationTokenTTL = 24 * time.Hour

type AuthHandler struct {
	DB            *sql.DB
	Tmpl          TemplateRenderer
	Email         *email.Sender
	BaseURL       string
	SecureCookies bool
}

// smtpEnabled is true when email delivery is configured. When false, the
// registration flow auto-verifies new accounts (since there's no way to send
// a verification link) and the login gate on email verification is bypassed.
func (h *AuthHandler) smtpEnabled() bool {
	return h.Email != nil && h.Email.Config.Enabled()
}

// sendVerificationEmail mints a fresh verification token, persists its hash,
// and emails the link. It overwrites any pending verification for the user so
// the most recently issued link is the only one that works.
func (h *AuthHandler) sendVerificationEmail(r *http.Request, userID int64, addr string) error {
	rawToken, tokenHash, err := auth.GenerateResetToken()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	expiresAt := time.Now().Add(verificationTokenTTL)
	if err := db.CreateEmailVerification(r.Context(), h.DB, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("persist token: %w", err)
	}
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", h.BaseURL, rawToken)
	if err := h.Email.SendEmailVerification(addr, verifyURL); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
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

	addr := r.FormValue("email")
	password := r.FormValue("password")

	genericFail := func() {
		h.Tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
			"Error":     "Invalid email or password.",
			"CSRFToken": middleware.CSRFToken(r),
		})
	}

	user, err := db.GetUserByEmail(r.Context(), h.DB, addr)
	if err != nil {
		genericFail()
		return
	}

	// If the account is currently locked, refuse without revealing it.
	// Logging is the alerting hook — wire alerts to "auth.account_locked"
	// when you stand up log-based monitoring.
	if user.IsLocked(time.Now()) {
		slog.WarnContext(r.Context(), "auth.login_attempt_on_locked_account",
			"user_id", user.ID, "client_ip", middleware.ClientIP(r))
		genericFail()
		return
	}

	if !auth.CheckPassword(user.PasswordHash, password) {
		locked, attempts, ferr := db.RecordFailedLogin(r.Context(), h.DB, user.ID)
		if ferr != nil {
			slog.ErrorContext(r.Context(), "record failed login", "err", ferr, "user_id", user.ID)
		}
		if locked {
			slog.WarnContext(r.Context(), "auth.account_locked",
				"user_id", user.ID,
				"attempts", attempts,
				"client_ip", middleware.ClientIP(r))
		}
		genericFail()
		return
	}

	// Refuse login until the email is verified, but only when SMTP is
	// configured — otherwise verification is impossible and we'd lock out
	// every user permanently.
	if h.smtpEnabled() && !user.EmailVerified() {
		h.Tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{
			"Error":           "Please verify your email address before logging in.",
			"UnverifiedEmail": addr,
			"CSRFToken":       middleware.CSRFToken(r),
		})
		return
	}

	// Successful login: clear the failure counter so we don't carry old
	// attempts into the next session lifetime.
	if err := db.ResetFailedLogins(r.Context(), h.DB, user.ID); err != nil {
		slog.ErrorContext(r.Context(), "reset failed logins", "err", err, "user_id", user.ID)
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

	// When SMTP is configured, send a verification email and stop short of
	// auto-login — the user must click the link first.
	if h.smtpEnabled() {
		if err := h.sendVerificationEmail(r, user.ID, user.Email); err != nil {
			slog.ErrorContext(r.Context(), "send verification email", "err", err, "user_id", user.ID)
		}
		h.Tmpl.ExecuteTemplate(w, "check_email.html", map[string]interface{}{
			"Email":     user.Email,
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	// SMTP disabled: there's no way to verify, so auto-mark verified and
	// fall through to the legacy auto-login flow. Mirrors the existing
	// behavior before email verification was added.
	if err := db.MarkUserEmailVerified(r.Context(), h.DB, user.ID); err != nil {
		slog.ErrorContext(r.Context(), "auto-verify (no SMTP)", "err", err, "user_id", user.ID)
	}

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

// VerifyEmail consumes a verification token. The token hash is looked up; on
// success the user's email_verified_at is set and the token deleted.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		h.Tmpl.ExecuteTemplate(w, "verify_email.html", map[string]interface{}{
			"Error":     "Missing verification token.",
			"CSRFToken": middleware.CSRFToken(r),
		})
		return
	}

	tokenHash := auth.HashResetToken(token)
	verification, err := db.GetEmailVerificationByTokenHash(r.Context(), h.DB, tokenHash)
	if err != nil {
		h.Tmpl.ExecuteTemplate(w, "verify_email.html", map[string]interface{}{
			"Error":      "Invalid or expired verification link. Request a new one below.",
			"ShowResend": true,
			"CSRFToken":  middleware.CSRFToken(r),
		})
		return
	}

	if err := db.MarkUserEmailVerified(r.Context(), h.DB, verification.UserID); err != nil {
		slog.ErrorContext(r.Context(), "mark email verified", "err", err, "user_id", verification.UserID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	_ = db.DeleteEmailVerification(r.Context(), h.DB, verification.ID)

	h.Tmpl.ExecuteTemplate(w, "verify_email.html", map[string]interface{}{
		"Success":   "Email verified. You can now log in.",
		"CSRFToken": middleware.CSRFToken(r),
	})
}

// ResendVerification accepts an email and, if it matches an unverified
// account, sends a new verification link. Always renders the same success
// message to avoid leaking which addresses are registered.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	if !h.smtpEnabled() {
		http.Error(w, "Email verification is not available (SMTP not configured)", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	addr := r.FormValue("email")

	successData := map[string]interface{}{
		"Success":   "If an unverified account exists for that email, a new link has been sent.",
		"CSRFToken": middleware.CSRFToken(r),
	}

	user, err := db.GetUserByEmail(r.Context(), h.DB, addr)
	if err != nil || user.EmailVerified() {
		h.Tmpl.ExecuteTemplate(w, "verify_email.html", successData)
		return
	}

	if err := h.sendVerificationEmail(r, user.ID, user.Email); err != nil {
		slog.ErrorContext(r.Context(), "resend verification email", "err", err, "user_id", user.ID)
	}

	h.Tmpl.ExecuteTemplate(w, "verify_email.html", successData)
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
		slog.ErrorContext(r.Context(), "generate reset token", "err", err)
		h.Tmpl.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	expiresAt := time.Now().Add(1 * time.Hour)
	if err := db.CreatePasswordReset(r.Context(), h.DB, user.ID, tokenHash, expiresAt); err != nil {
		slog.ErrorContext(r.Context(), "create password reset", "err", err)
		h.Tmpl.ExecuteTemplate(w, "forgot_password.html", successData)
		return
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", h.BaseURL, rawToken)
	if err := h.Email.SendPasswordReset(user.Email, resetURL); err != nil {
		slog.ErrorContext(r.Context(), "send password reset email", "err", err)
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
