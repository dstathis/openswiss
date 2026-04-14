package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJsonResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}
	jsonResponse(rec, http.StatusOK, data)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["hello"] != "world" {
		t.Errorf("expected hello=world, got %q", result["hello"])
	}
}

func TestJsonResponse_CustomStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	jsonResponse(rec, http.StatusCreated, map[string]int{"id": 1})
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
}

func TestJsonError(t *testing.T) {
	rec := httptest.NewRecorder()
	jsonError(rec, http.StatusNotFound, "not found")
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["error"] != "not found" {
		t.Errorf("expected error=not found, got %q", result["error"])
	}
}

func TestPaginationParams_Defaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments", nil)
	page, perPage := paginationParams(req)
	if page != 1 {
		t.Errorf("default page should be 1, got %d", page)
	}
	if perPage != 50 {
		t.Errorf("default perPage should be 50, got %d", perPage)
	}
}

func TestPaginationParams_Custom(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments?page=3&per_page=25", nil)
	page, perPage := paginationParams(req)
	if page != 3 {
		t.Errorf("expected page 3, got %d", page)
	}
	if perPage != 25 {
		t.Errorf("expected perPage 25, got %d", perPage)
	}
}

func TestPaginationParams_InvalidValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments?page=abc&per_page=xyz", nil)
	page, perPage := paginationParams(req)
	if page != 1 {
		t.Errorf("expected default page 1 for invalid input, got %d", page)
	}
	if perPage != 50 {
		t.Errorf("expected default perPage 50 for invalid input, got %d", perPage)
	}
}

func TestPaginationParams_NegativeValues(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments?page=-1&per_page=-10", nil)
	page, perPage := paginationParams(req)
	if page != 1 {
		t.Errorf("expected default page 1 for negative input, got %d", page)
	}
	if perPage != 50 {
		t.Errorf("expected default perPage 50 for negative input, got %d", perPage)
	}
}

func TestPaginationParams_ZeroPage(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments?page=0", nil)
	page, _ := paginationParams(req)
	if page != 1 {
		t.Errorf("expected default page 1 for zero input, got %d", page)
	}
}

func TestPaginationParams_PerPageExceedsMax(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments?per_page=200", nil)
	_, perPage := paginationParams(req)
	if perPage != 50 {
		t.Errorf("expected perPage capped at default 50 for >100 input, got %d", perPage)
	}
}

func TestPaginationParams_PerPageAtMax(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/tournaments?per_page=100", nil)
	_, perPage := paginationParams(req)
	if perPage != 100 {
		t.Errorf("expected perPage 100, got %d", perPage)
	}
}

func TestDecodeJSON_NilBody(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Body = nil
	var v struct{}
	err := decodeJSON(req, &v)
	if err == nil {
		t.Error("expected error for nil body")
	}
}
