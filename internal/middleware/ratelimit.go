package middleware

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiterMaxKeys caps the per-IP map so an attacker spraying random
// X-Forwarded-For values (in deployments where the header is trusted) or
// hitting from many real IPs can't grow the map without bound. When the cap
// is reached, new IPs share the next eviction cycle's headroom.
const rateLimiterMaxKeys = 10_000

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.gcLoop()
	return rl
}

func (rl *rateLimiter) gcLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.gc()
	}
}

func (rl *rateLimiter) gc() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-rl.window)
	for key, reqs := range rl.requests {
		valid := reqs[:0]
		for _, t := range reqs {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.requests, key)
		} else {
			rl.requests[key] = valid
		}
	}
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	reqs := rl.requests[key]
	valid := reqs[:0]
	for _, t := range reqs {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}

	// Refuse to grow the map past the cap — fail open for the unknown key
	// rather than crash the process under a memory-exhaustion attack.
	if _, exists := rl.requests[key]; !exists && len(rl.requests) >= rateLimiterMaxKeys {
		return true
	}

	rl.requests[key] = append(valid, now)
	return true
}

// RateLimit applies per-IP rate limiting. The IP is taken from the request
// context (populated by RealIP middleware) and falls back to RemoteAddr when
// RealIP isn't installed. Crucially, X-Forwarded-For is NEVER trusted here
// directly — that determination belongs in RealIP, which checks the trusted-
// proxy list.
func RateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	rl := newRateLimiter(requestsPerMinute, time.Minute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(ClientIP(r)) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
