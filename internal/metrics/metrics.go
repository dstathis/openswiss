package metrics

import (
	"encoding/json"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Collector gathers HTTP request metrics.
type Collector struct {
	startTime        time.Time
	totalRequests    atomic.Int64
	activeRequests   atomic.Int64
	totalResponseNs  atomic.Int64
	statusCounts     sync.Map // status code (int) -> *atomic.Int64
	routeCounts      sync.Map // method+path pattern (string) -> *atomic.Int64
	totalRequestSize atomic.Int64
}

// New creates a new metrics collector.
func New() *Collector {
	return &Collector{
		startTime: time.Now(),
	}
}

func (c *Collector) incStatusCount(code int) {
	bucket := code / 100 * 100 // 200, 300, 400, 500
	if val, ok := c.statusCounts.Load(bucket); ok {
		val.(*atomic.Int64).Add(1)
		return
	}
	v := &atomic.Int64{}
	v.Add(1)
	if actual, loaded := c.statusCounts.LoadOrStore(bucket, v); loaded {
		actual.(*atomic.Int64).Add(1)
	}
}

func (c *Collector) incRouteCount(key string) {
	if val, ok := c.routeCounts.Load(key); ok {
		val.(*atomic.Int64).Add(1)
		return
	}
	v := &atomic.Int64{}
	v.Add(1)
	if actual, loaded := c.routeCounts.LoadOrStore(key, v); loaded {
		actual.(*atomic.Int64).Add(1)
	}
}

// Wrap returns middleware that records request metrics.
func (c *Collector) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.totalRequests.Add(1)
		c.activeRequests.Add(1)
		defer c.activeRequests.Add(-1)

		if r.ContentLength > 0 {
			c.totalRequestSize.Add(r.ContentLength)
		}

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		duration := time.Since(start)

		c.totalResponseNs.Add(int64(duration))
		c.incStatusCount(sw.status)
		c.incRouteCount(r.Method + " " + r.URL.Path)
	})
}

// Snapshot holds a point-in-time view of the collected metrics.
type Snapshot struct {
	Uptime           string           `json:"uptime"`
	UptimeSeconds    float64          `json:"uptime_seconds"`
	TotalRequests    int64            `json:"total_requests"`
	ActiveRequests   int64            `json:"active_requests"`
	AvgResponseMs    float64          `json:"avg_response_ms"`
	StatusCounts     map[string]int64 `json:"status_counts"`
	TopRoutes        []RouteCount     `json:"top_routes"`
	TotalRequestSize int64            `json:"total_request_bytes"`
	Go               GoMetrics        `json:"go"`
}

// GoMetrics holds Go runtime metrics.
type GoMetrics struct {
	Goroutines int    `json:"goroutines"`
	HeapAlloc  uint64 `json:"heap_alloc_bytes"`
	HeapSys    uint64 `json:"heap_sys_bytes"`
	NumGC      uint32 `json:"num_gc"`
}

// RouteCount represents a request count for a single route.
type RouteCount struct {
	Route string `json:"route"`
	Count int64  `json:"count"`
}

// Snapshot returns a point-in-time snapshot of metrics.
func (c *Collector) Snapshot() Snapshot {
	total := c.totalRequests.Load()
	var avgMs float64
	if total > 0 {
		avgMs = float64(c.totalResponseNs.Load()) / float64(total) / 1e6
	}

	statusCounts := make(map[string]int64)
	c.statusCounts.Range(func(key, value any) bool {
		statusCounts[strconv.Itoa(key.(int))] = value.(*atomic.Int64).Load()
		return true
	})

	var routes []RouteCount
	c.routeCounts.Range(func(key, value any) bool {
		routes = append(routes, RouteCount{
			Route: key.(string),
			Count: value.(*atomic.Int64).Load(),
		})
		return true
	})
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Count > routes[j].Count
	})
	if len(routes) > 20 {
		routes = routes[:20]
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := time.Since(c.startTime)
	return Snapshot{
		Uptime:           uptime.Truncate(time.Second).String(),
		UptimeSeconds:    uptime.Seconds(),
		TotalRequests:    total,
		ActiveRequests:   c.activeRequests.Load(),
		AvgResponseMs:    avgMs,
		StatusCounts:     statusCounts,
		TopRoutes:        routes,
		TotalRequestSize: c.totalRequestSize.Load(),
		Go: GoMetrics{
			Goroutines: runtime.NumGoroutine(),
			HeapAlloc:  mem.HeapAlloc,
			HeapSys:    mem.HeapSys,
			NumGC:      mem.NumGC,
		},
	}
}

// Handler returns an http.HandlerFunc that serves the metrics as JSON.
func (c *Collector) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c.Snapshot())
	}
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.wroteHeader {
		sw.wroteHeader = true
	}
	return sw.ResponseWriter.Write(b)
}
