package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := newRateLimiter(3, 60_000_000_000)
	key := "192.168.1.1"
	for i := 0; i < 3; i++ {
		if !rl.allow(key) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	if rl.allow(key) {
		t.Error("4th request should be denied")
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := newRateLimiter(1, 60_000_000_000)
	if !rl.allow("key1") {
		t.Error("first request for key1 should be allowed")
	}
	if !rl.allow("key2") {
		t.Error("first request for key2 should be allowed")
	}
	if rl.allow("key1") {
		t.Error("second request for key1 should be denied")
	}
}

func TestRateLimit_Middleware(t *testing.T) {
	handler := RateLimit(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimit_XForwardedFor(t *testing.T) {
	handler := RateLimit(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/v1/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", rec.Code)
	}
}
