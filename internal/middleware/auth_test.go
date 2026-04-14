package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dstathis/openswiss/internal/models"
)

func TestGetUser_NoUser(t *testing.T) {
	ctx := context.Background()
	user := GetUser(ctx)
	if user != nil {
		t.Error("expected nil user from empty context")
	}
}

func TestGetUser_WithUser(t *testing.T) {
	u := &models.User{ID: 42, Email: "test@example.com"}
	ctx := context.WithValue(context.Background(), UserContextKey, u)
	got := GetUser(ctx)
	if got == nil {
		t.Fatal("expected non-nil user")
	}
	if got.ID != 42 {
		t.Errorf("expected user ID 42, got %d", got.ID)
	}
}

func TestGetUser_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserContextKey, "not a user")
	user := GetUser(ctx)
	if user != nil {
		t.Error("expected nil for wrong type")
	}
}

func TestRequireAuth_NoUser(t *testing.T) {
	handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuth_NoUser_APIPath(t *testing.T) {
	handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/api/v1/tournaments", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_WithUser(t *testing.T) {
	called := false
	handler := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/dashboard", nil)
	u := &models.User{ID: 1}
	ctx := context.WithValue(req.Context(), UserContextKey, u)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should have been called")
	}
}

func TestRequireRole_HasRole(t *testing.T) {
	called := false
	handler := RequireRole(models.RoleOrganizer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/tournaments/new", nil)
	u := &models.User{ID: 1, Roles: []string{models.RoleOrganizer}}
	ctx := context.WithValue(req.Context(), UserContextKey, u)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("handler should have been called")
	}
}

func TestRequireRole_AdminBypass(t *testing.T) {
	called := false
	handler := RequireRole(models.RoleOrganizer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/tournaments/new", nil)
	u := &models.User{ID: 1, Roles: []string{models.RoleAdmin}}
	ctx := context.WithValue(req.Context(), UserContextKey, u)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !called {
		t.Error("admin should bypass role check")
	}
}

func TestRequireRole_MissingRole(t *testing.T) {
	handler := RequireRole(models.RoleOrganizer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/tournaments/new", nil)
	u := &models.User{ID: 1, Roles: []string{models.RolePlayer}}
	ctx := context.WithValue(req.Context(), UserContextKey, u)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireRole_MissingRole_APIPath(t *testing.T) {
	handler := RequireRole(models.RoleOrganizer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	u := &models.User{ID: 1, Roles: []string{models.RolePlayer}}
	ctx := context.WithValue(req.Context(), UserContextKey, u)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireRole_NoUser(t *testing.T) {
	handler := RequireRole(models.RoleOrganizer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/tournaments/new", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
