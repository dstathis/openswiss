package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodySize_AllowsSmallBody(t *testing.T) {
	handler := MaxBodySize(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("unexpected error reading body: %v", err)
		}
		if string(body) != "hello" {
			t.Errorf("expected body 'hello', got %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestMaxBodySize_RejectsLargeBody(t *testing.T) {
	handler := MaxBodySize(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err == nil {
			t.Error("expected error reading oversized body")
		}
	}))
	body := strings.Repeat("x", 100)
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}
