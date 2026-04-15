package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dstathis/openswiss/internal/email"
	"github.com/dstathis/openswiss/internal/middleware"
	"github.com/dstathis/openswiss/internal/models"
)

type mockTemplate struct {
	calls []templateCall
}

type templateCall struct {
	Name string
	Data interface{}
}

func (m *mockTemplate) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	m.calls = append(m.calls, templateCall{Name: name, Data: data})
	return nil
}

func TestAuthHandler_LoginPage(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	h.LoginPage(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].Name != "login.html" {
		t.Errorf("expected template login.html, got %q", tmpl.calls[0].Name)
	}
}

func TestAuthHandler_LoginPage_WithUser(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	u := &models.User{ID: 1, DisplayName: "Test"}
	req := httptest.NewRequest("GET", "/login", nil)
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, u)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.LoginPage(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["User"] == nil {
		t.Error("expected User in template data")
	}
}

func TestAuthHandler_RegisterPage(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	req := httptest.NewRequest("GET", "/register", nil)
	rec := httptest.NewRecorder()
	h.RegisterPage(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	if tmpl.calls[0].Name != "register.html" {
		t.Errorf("expected template register.html, got %q", tmpl.calls[0].Name)
	}
}

func TestAuthHandler_Register_MissingFields(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("email", "")
	form.Set("display_name", "")
	form.Set("password", "")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "All fields are required." {
		t.Errorf("expected All fields are required, got %q", data["Error"])
	}
}

func TestAuthHandler_Register_PasswordMismatch(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("email", "test@example.com")
	form.Set("display_name", "Test User")
	form.Set("password", "password123")
	form.Set("confirm_password", "differentpassword")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Passwords do not match." {
		t.Errorf("expected Passwords do not match, got %q", data["Error"])
	}
}

func TestAuthHandler_Logout_NoCookie(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	req := httptest.NewRequest("POST", "/logout", nil)
	rec := httptest.NewRecorder()
	h.Logout(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "session" && c.MaxAge == -1 {
			found = true
		}
	}
	if !found {
		t.Error("expected session cookie to be cleared")
	}
}

func TestAuthHandler_ForgotPasswordPage_SMTPDisabled(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl, Email: nil}
	req := httptest.NewRequest("GET", "/forgot-password", nil)
	rec := httptest.NewRecorder()
	h.ForgotPasswordPage(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["SMTPEnabled"] != false {
		t.Errorf("expected SMTPEnabled=false, got %v", data["SMTPEnabled"])
	}
}

func TestAuthHandler_ForgotPasswordPage_SMTPEnabled(t *testing.T) {
	tmpl := &mockTemplate{}
	sender := &email.Sender{Config: email.Config{Host: "smtp.test.com", From: "noreply@test.com"}}
	h := &AuthHandler{Tmpl: tmpl, Email: sender}
	req := httptest.NewRequest("GET", "/forgot-password", nil)
	rec := httptest.NewRecorder()
	h.ForgotPasswordPage(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["SMTPEnabled"] != true {
		t.Errorf("expected SMTPEnabled=true, got %v", data["SMTPEnabled"])
	}
}

func TestAuthHandler_ForgotPassword_NoSMTP(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl, Email: nil}
	form := url.Values{}
	form.Set("email", "test@example.com")
	req := httptest.NewRequest("POST", "/forgot-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ForgotPassword(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when SMTP not configured, got %d", rec.Code)
	}
}

func TestAuthHandler_ResetPasswordPage_NoToken(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	req := httptest.NewRequest("GET", "/reset-password", nil)
	rec := httptest.NewRecorder()
	h.ResetPasswordPage(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/forgot-password" {
		t.Errorf("expected redirect to /forgot-password, got %q", loc)
	}
}

func TestAuthHandler_ResetPassword_MissingFields(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("token", "")
	form.Set("password", "")
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ResetPassword(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "All fields are required." {
		t.Errorf("expected All fields are required, got %q", data["Error"])
	}
}

func TestAuthHandler_ResetPassword_PasswordMismatch(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("token", "sometoken")
	form.Set("password", "newpass123")
	form.Set("confirm_password", "different")
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ResetPassword(rec, req)
	if len(tmpl.calls) != 1 {
		t.Fatalf("expected 1 template call, got %d", len(tmpl.calls))
	}
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Passwords do not match." {
		t.Errorf("expected Passwords do not match, got %q", data["Error"])
	}
	if data["Token"] != "sometoken" {
		t.Error("expected token to be preserved in form data")
	}
}

func TestTemplateRenderer_Interface(t *testing.T) {
	var _ TemplateRenderer = &mockTemplate{}
	m := &mockTemplate{}
	err := m.ExecuteTemplate(&bytes.Buffer{}, "test.html", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(m.calls) != 1 {
		t.Errorf("expected 1 call, got %d", len(m.calls))
	}
}

func TestAuthHandler_Register_InvalidEmail(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("email", "notanemail")
	form.Set("display_name", "Test User")
	form.Set("password", "password123")
	form.Set("confirm_password", "password123")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Please enter a valid email address." {
		t.Errorf("expected invalid email error, got %q", data["Error"])
	}
}

func TestAuthHandler_Register_EmailTooLong(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	longEmail := strings.Repeat("a", 250) + "@b.com"
	form := url.Values{}
	form.Set("email", longEmail)
	form.Set("display_name", "Test User")
	form.Set("password", "password123")
	form.Set("confirm_password", "password123")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Email address is too long." {
		t.Errorf("expected email too long error, got %q", data["Error"])
	}
}

func TestAuthHandler_Register_DisplayNameTooLong(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("email", "test@example.com")
	form.Set("display_name", strings.Repeat("x", 101))
	form.Set("password", "password123")
	form.Set("confirm_password", "password123")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Display name must be 100 characters or fewer." {
		t.Errorf("expected display name too long error, got %q", data["Error"])
	}
}

func TestAuthHandler_Register_PasswordTooShort(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("email", "test@example.com")
	form.Set("display_name", "Test User")
	form.Set("password", "short")
	form.Set("confirm_password", "short")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Password must be at least 8 characters." {
		t.Errorf("expected password too short error, got %q", data["Error"])
	}
}

func TestAuthHandler_ResetPassword_PasswordTooShort(t *testing.T) {
	tmpl := &mockTemplate{}
	h := &AuthHandler{Tmpl: tmpl}
	form := url.Values{}
	form.Set("token", "sometoken")
	form.Set("password", "short")
	form.Set("confirm_password", "short")
	req := httptest.NewRequest("POST", "/reset-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ResetPassword(rec, req)
	data := tmpl.calls[0].Data.(map[string]interface{})
	if data["Error"] != "Password must be at least 8 characters." {
		t.Errorf("expected password too short error, got %q", data["Error"])
	}
}
