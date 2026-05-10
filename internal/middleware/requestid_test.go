package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_GeneratesAndEchoes(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDOf(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if seen == "" {
		t.Fatal("expected a request ID in context")
	}
	if got := rec.Header().Get("X-Request-Id"); got != seen {
		t.Errorf("X-Request-Id header %q != context %q", got, seen)
	}
}

func TestRequestID_HonorsValidIncomingHeader(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDOf(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc-123_DEF")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen != "abc-123_DEF" {
		t.Errorf("expected incoming ID, got %q", seen)
	}
}

func TestRequestID_RejectsUnsafeIncomingHeader(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDOf(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "a b\ninjected")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if seen == "a b\ninjected" {
		t.Error("unsafe ID should be discarded and replaced with a generated one")
	}
	if seen == "" {
		t.Error("a fresh ID should have been generated")
	}
}

func TestContextLogHandler_AddsRequestID(t *testing.T) {
	var buf bytes.Buffer
	h := ContextLogHandler{Handler: slog.NewJSONHandler(&buf, nil)}
	logger := slog.New(h)

	ctx := context.WithValue(context.Background(), requestIDKey{}, "test-id-99")
	logger.InfoContext(ctx, "hello")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if got["request_id"] != "test-id-99" {
		t.Errorf("expected request_id=test-id-99, got %v", got["request_id"])
	}
}
