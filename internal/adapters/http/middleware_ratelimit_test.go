package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func withTestUserID(req *http.Request, uid string) *http.Request {
	return req.WithContext(WithUserID(req.Context(), uid))
}

func TestPerUserRateLimiter_AllowsUpToBurstThenBlocks(t *testing.T) {
	limiter := NewPerUserRateLimiter(1, 2) // 1/sec sustained, burst 2
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++ })
	handler := limiter.Middleware(next)

	for i := 0; i < 2; i++ {
		req := withTestUserID(httptest.NewRequest(http.MethodPost, "/training/generate", nil), "u1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (within burst)", i, rec.Code)
		}
	}

	req := withTestUserID(httptest.NewRequest(http.MethodPost, "/training/generate", nil), "u1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("3rd request status = %d, want 429 (burst exhausted)", rec.Code)
	}
	if called != 2 {
		t.Errorf("next handler called %d times, want 2 (the 3rd must be rejected before reaching it)", called)
	}
}

func TestPerUserRateLimiter_TracksEachUserIndependently(t *testing.T) {
	limiter := NewPerUserRateLimiter(1, 1) // burst 1: a 2nd call from the SAME user blocks immediately
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := limiter.Middleware(next)

	req1 := withTestUserID(httptest.NewRequest(http.MethodPost, "/x", nil), "u1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("u1 first request: status = %d, want 200", rec1.Code)
	}

	// u1's burst is now exhausted...
	req1b := withTestUserID(httptest.NewRequest(http.MethodPost, "/x", nil), "u1")
	rec1b := httptest.NewRecorder()
	handler.ServeHTTP(rec1b, req1b)
	if rec1b.Code != http.StatusTooManyRequests {
		t.Fatalf("u1 second request: status = %d, want 429", rec1b.Code)
	}

	// ...but u2 has their own, untouched bucket.
	req2 := withTestUserID(httptest.NewRequest(http.MethodPost, "/x", nil), "u2")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("u2 request status = %d, want 200 — a different user must not share u1's bucket", rec2.Code)
	}
}

func TestPerUserRateLimiter_NoUserIDPassesThroughUnthrottled(t *testing.T) {
	limiter := NewPerUserRateLimiter(1, 1)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := limiter.Middleware(next)

	// No WithUserID at all — e.g. a request that somehow reaches this
	// middleware before auth, or in a context this middleware isn't
	// mounted behind. Must never panic or throttle by accident.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected the request to reach the next handler when there is no authenticated user ID")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestPerUserRateLimiter_EvictStaleRemovesIdleEntriesOnly(t *testing.T) {
	limiter := NewPerUserRateLimiter(1, 1)

	// u1: exhaust the burst, then mark it as seen long ago.
	limiter.allow("u1")
	limiter.mu.Lock()
	limiter.limiters["u1"].lastSeen = time.Now().Add(-time.Hour)
	limiter.mu.Unlock()

	// u2: fresh, exhaust its burst too.
	limiter.allow("u2")

	limiter.evictStale(time.Minute)

	limiter.mu.Lock()
	_, u1Still := limiter.limiters["u1"]
	_, u2Still := limiter.limiters["u2"]
	limiter.mu.Unlock()

	if u1Still {
		t.Error("expected u1's stale entry to be evicted")
	}
	if !u2Still {
		t.Error("expected u2's fresh entry to survive eviction")
	}

	// u1 gets a brand-new bucket after eviction — full burst again.
	if !limiter.allow("u1") {
		t.Error("expected u1 to get a fresh bucket (full burst) after its stale entry was evicted")
	}
}
