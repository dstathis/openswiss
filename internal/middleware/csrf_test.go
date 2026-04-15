package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFProtect_SetsCookie(t *testing.T) {
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName {
			found = true
			if c.Value == "" {
				t.Error("expected non-empty CSRF token")
			}
			if c.HttpOnly {
				t.Error("CSRF cookie should not be HttpOnly (JS needs to read it)")
			}
			if c.Secure {
				t.Error("expected Secure=false when secureCookie=false")
			}
		}
	}
	if !found {
		t.Error("expected csrf_token cookie to be set")
	}
}

func TestCSRFProtect_SecureCookie(t *testing.T) {
	handler := CSRFProtect(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName && !c.Secure {
			t.Error("expected Secure=true when secureCookie=true")
		}
	}
}

func TestCSRFProtect_SkipsAPIRoutes(t *testing.T) {
	var called bool
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	// POST to /api/ without any CSRF token should still succeed
	req := httptest.NewRequest("POST", "/api/v1/tournaments", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called for API route")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for API route, got %d", rec.Code)
	}
}

func TestCSRFProtect_BlocksPostWithoutToken(t *testing.T) {
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("POST", "/register", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "validtoken123"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCSRFProtect_BlocksPostWithWrongToken(t *testing.T) {
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	form := url.Values{}
	form.Set(csrfFieldName, "wrongtoken")
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "correcttoken"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestCSRFProtect_AllowsPostWithValidToken(t *testing.T) {
	var called bool
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	token := "somevalidtoken123"
	form := url.Values{}
	form.Set(csrfFieldName, token)
	req := httptest.NewRequest("POST", "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called with valid CSRF token")
	}
}

func TestCSRFProtect_AllowsHeaderToken(t *testing.T) {
	var called bool
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	token := "somevalidtoken123"
	req := httptest.NewRequest("POST", "/register", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should have been called with X-CSRF-Token header")
	}
}

func TestCSRFProtect_AllowsGetWithoutToken(t *testing.T) {
	var called bool
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "sometoken"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("GET request should be allowed without CSRF validation")
	}
}

func TestCSRFToken_FromContext(t *testing.T) {
	var gotToken string
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = CSRFToken(r)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "mytoken123"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotToken != "mytoken123" {
		t.Errorf("expected token mytoken123, got %q", gotToken)
	}
}

func TestCSRFToken_EmptyWithoutMiddleware(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if tok := CSRFToken(req); tok != "" {
		t.Errorf("expected empty token without middleware, got %q", tok)
	}
}

func TestCSRFProtect_ValidatesDeleteMethod(t *testing.T) {
	handler := CSRFProtect(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("DELETE", "/tournaments/1", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "validtoken"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for DELETE without token, got %d", rec.Code)
	}
}
