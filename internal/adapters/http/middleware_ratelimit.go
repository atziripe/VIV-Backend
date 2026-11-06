package http

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// PerUserRateLimiter caps how often one authenticated user can hit a
// group of routes — an in-process token bucket per user ID (see
// golang.org/x/time/rate). Deliberately in-memory, not a distributed
// store (Redis/Firestore): this is a backstop against a runaway client
// or script hammering an endpoint — nothing stopped that before this —
// not a precise multi-instance quota. Good enough while the backend runs
// as a single instance; revisit with a shared store once it doesn't.
type PerUserRateLimiter struct {
	rate  rate.Limit
	burst int

	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewPerUserRateLimiter builds a limiter allowing eventsPerSecond
// sustained per user, bursting up to burst — same units as
// rate.NewLimiter.
func NewPerUserRateLimiter(eventsPerSecond float64, burst int) *PerUserRateLimiter {
	return &PerUserRateLimiter{
		rate:     rate.Limit(eventsPerSecond),
		burst:    burst,
		limiters: make(map[string]*rateLimiterEntry),
	}
}

func (l *PerUserRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.limiters[key]
	if !ok {
		entry = &rateLimiterEntry{limiter: rate.NewLimiter(l.rate, l.burst)}
		l.limiters[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter.Allow()
}

// evictStale drops limiter entries idle for longer than maxIdle, so a
// long-running process doesn't keep one entry per user ID forever.
func (l *PerUserRateLimiter) evictStale(maxIdle time.Duration) {
	cutoff := time.Now().Add(-maxIdle)
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range l.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(l.limiters, key)
		}
	}
}

// StartEvictionLoop runs evictStale on interval until ctx is cancelled —
// call once at boot, right after constructing the limiter.
func (l *PerUserRateLimiter) StartEvictionLoop(ctx context.Context, interval, maxIdle time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.evictStale(maxIdle)
			}
		}
	}()
}

// Middleware rejects a request with 429 once the calling user (from
// UserIDFromContext — this must be mounted after FirebaseAuthMiddleware)
// exceeds this limiter's rate. A request with no authenticated user ID
// passes through untouched here — on the `protected` router,
// EnsureUserMiddleware already rejects those with 401 before this ever
// runs, so this never needs an IP-based fallback for that case.
func (l *PerUserRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := UserIDFromContext(r.Context())
		if !ok || uid == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !l.allow(uid) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
