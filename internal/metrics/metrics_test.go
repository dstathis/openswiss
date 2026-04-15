package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollector_Wrap_CountsRequests(t *testing.T) {
	c := New()
	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	snap := c.Snapshot()
	if snap.TotalRequests != 5 {
		t.Errorf("expected 5 total requests, got %d", snap.TotalRequests)
	}
}

func TestCollector_Wrap_TracksStatusCodes(t *testing.T) {
	c := New()
	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/ok", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/missing", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	snap := c.Snapshot()
	if snap.StatusCounts["200"] != 3 {
		t.Errorf("expected 3 2xx, got %d", snap.StatusCounts["200"])
	}
	if snap.StatusCounts["400"] != 2 {
		t.Errorf("expected 2 4xx, got %d", snap.StatusCounts["400"])
	}
}

func TestCollector_Wrap_TracksRoutes(t *testing.T) {
	c := New()
	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/tournaments", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
	req := httptest.NewRequest("POST", "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	snap := c.Snapshot()
	if len(snap.TopRoutes) < 2 {
		t.Fatalf("expected at least 2 routes, got %d", len(snap.TopRoutes))
	}
	// Top route should be GET /tournaments with 3 hits
	if snap.TopRoutes[0].Route != "GET /tournaments" {
		t.Errorf("expected top route GET /tournaments, got %q", snap.TopRoutes[0].Route)
	}
	if snap.TopRoutes[0].Count != 3 {
		t.Errorf("expected count 3, got %d", snap.TopRoutes[0].Count)
	}
}

func TestCollector_Wrap_TracksActiveRequests(t *testing.T) {
	c := New()
	insideHandler := make(chan struct{})
	proceed := make(chan struct{})

	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(insideHandler)
		<-proceed
		w.WriteHeader(http.StatusOK)
	}))

	go func() {
		req := httptest.NewRequest("GET", "/slow", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}()

	<-insideHandler
	if active := c.activeRequests.Load(); active != 1 {
		t.Errorf("expected 1 active request, got %d", active)
	}
	close(proceed)
}

func TestCollector_Wrap_TracksRequestSize(t *testing.T) {
	c := New()
	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader("hello world")
	req := httptest.NewRequest("POST", "/test", body)
	req.ContentLength = 11
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	snap := c.Snapshot()
	if snap.TotalRequestSize != 11 {
		t.Errorf("expected 11 request bytes, got %d", snap.TotalRequestSize)
	}
}

func TestCollector_Snapshot_Uptime(t *testing.T) {
	c := New()
	snap := c.Snapshot()
	if snap.UptimeSeconds < 0 {
		t.Error("uptime should not be negative")
	}
	if snap.Uptime == "" {
		t.Error("uptime string should not be empty")
	}
}

func TestCollector_Snapshot_GoMetrics(t *testing.T) {
	c := New()
	snap := c.Snapshot()
	if snap.Go.Goroutines == 0 {
		t.Error("expected at least 1 goroutine")
	}
	if snap.Go.HeapSys == 0 {
		t.Error("expected nonzero HeapSys")
	}
}

func TestCollector_Snapshot_AvgResponseMs(t *testing.T) {
	c := New()
	// No requests yet
	snap := c.Snapshot()
	if snap.AvgResponseMs != 0 {
		t.Errorf("expected 0 avg response ms with no requests, got %f", snap.AvgResponseMs)
	}

	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	snap = c.Snapshot()
	if snap.AvgResponseMs < 0 {
		t.Errorf("expected non-negative avg response, got %f", snap.AvgResponseMs)
	}
}

func TestCollector_Handler_ReturnsJSON(t *testing.T) {
	c := New()
	h := c.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var snap Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&snap); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if snap.Go.Goroutines == 0 {
		t.Error("expected goroutine count in response")
	}
}

func TestCollector_TopRoutes_LimitedTo20(t *testing.T) {
	c := New()
	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Generate 25 distinct routes
	for i := 0; i < 25; i++ {
		path := "/route-" + strings.Repeat("x", i+1)
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	snap := c.Snapshot()
	if len(snap.TopRoutes) > 20 {
		t.Errorf("expected at most 20 top routes, got %d", len(snap.TopRoutes))
	}
}

func TestStatusWriter_DefaultStatus(t *testing.T) {
	c := New()
	handler := c.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write body without explicit WriteHeader — should default to 200
		w.Write([]byte("hello"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	snap := c.Snapshot()
	if snap.StatusCounts["200"] != 1 {
		t.Errorf("expected 1 2xx request, got status counts: %v", snap.StatusCounts)
	}
}
