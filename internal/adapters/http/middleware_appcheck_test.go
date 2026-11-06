package http

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// testAppCheckKey is a throwaway RSA key generated once per test run — App
// Check tokens are RS256, same as everything else in this file.
func testAppCheckKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test RSA key: %v", err)
	}
	return key
}

// startTestJWKSServer serves a single JWK (the given key's public half,
// under kid) as a JWKS — standing in for
// https://firebaseappcheck.googleapis.com/v1/jwks in tests.
func startTestJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &key.PublicKey, KeyID: kid, Algorithm: string(jose.RS256), Use: "sig"},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// signTestAppCheckToken builds a real RS256-signed JWT with the given
// claims, the same shape a real App Check token has.
func signTestAppCheckToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.Claims) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{"kid": kid},
	})
	if err != nil {
		t.Fatalf("building signer: %v", err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return token
}

func newTestCache(url string) *appCheckJWKSCache {
	return &appCheckJWKSCache{httpClient: http.DefaultClient, url: url, ttl: time.Hour}
}

func TestAppCheckMiddleware_EmptyProjectNumberIsNoop(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	handler := AppCheckMiddleware("", true)(next)
	req := httptest.NewRequest(http.MethodGet, "/me", nil) // no header at all
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected the request to reach the next handler — App Check is disabled with no project number")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (next handler never set a status)", rec.Code)
	}
}

func TestAppCheckMiddleware_MonitorModeLetsMissingTokenThrough(t *testing.T) {
	key := testAppCheckKey(t)
	srv := startTestJWKSServer(t, key, "kid-1")
	cache := newTestCache(srv.URL)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := newAppCheckMiddleware("123456", false, cache)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil) // no X-Firebase-AppCheck header
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected the request to reach the next handler in monitor mode, even with no token")
	}
}

func TestAppCheckMiddleware_EnforceModeRejectsMissingToken(t *testing.T) {
	key := testAppCheckKey(t)
	srv := startTestJWKSServer(t, key, "kid-1")
	cache := newTestCache(srv.URL)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := newAppCheckMiddleware("123456", true, cache)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Error("expected the request to be rejected before reaching the next handler")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAppCheckMiddleware_EnforceModeRejectsTokenSignedByWrongKey(t *testing.T) {
	realKey := testAppCheckKey(t)
	srv := startTestJWKSServer(t, realKey, "kid-1")
	cache := newTestCache(srv.URL)

	// Signed by a different key than the one published in the JWKS — same
	// kid, forged signature.
	attackerKey := testAppCheckKey(t)
	now := time.Now()
	token := signTestAppCheckToken(t, attackerKey, "kid-1", jwt.Claims{
		Issuer:   "https://firebaseappcheck.googleapis.com/123456",
		Audience: jwt.Audience{"projects/123456"},
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt: jwt.NewNumericDate(now),
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("forged token must never reach the next handler")
	})
	handler := newAppCheckMiddleware("123456", true, cache)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("X-Firebase-AppCheck", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAppCheckMiddleware_EnforceModeRejectsExpiredToken(t *testing.T) {
	key := testAppCheckKey(t)
	srv := startTestJWKSServer(t, key, "kid-1")
	cache := newTestCache(srv.URL)

	past := time.Now().Add(-2 * time.Hour)
	token := signTestAppCheckToken(t, key, "kid-1", jwt.Claims{
		Issuer:   "https://firebaseappcheck.googleapis.com/123456",
		Audience: jwt.Audience{"projects/123456"},
		Expiry:   jwt.NewNumericDate(past.Add(time.Minute)), // expired well past the 1-minute leeway
		IssuedAt: jwt.NewNumericDate(past),
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("expired token must never reach the next handler")
	})
	handler := newAppCheckMiddleware("123456", true, cache)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("X-Firebase-AppCheck", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAppCheckMiddleware_EnforceModeRejectsWrongAudience(t *testing.T) {
	key := testAppCheckKey(t)
	srv := startTestJWKSServer(t, key, "kid-1")
	cache := newTestCache(srv.URL)

	now := time.Now()
	// Valid signature, valid issuer, but for a different Firebase project.
	token := signTestAppCheckToken(t, key, "kid-1", jwt.Claims{
		Issuer:   "https://firebaseappcheck.googleapis.com/123456",
		Audience: jwt.Audience{"projects/999999"},
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt: jwt.NewNumericDate(now),
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("token for a different project must never reach the next handler")
	})
	handler := newAppCheckMiddleware("123456", true, cache)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("X-Firebase-AppCheck", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAppCheckMiddleware_EnforceModeAcceptsValidToken(t *testing.T) {
	key := testAppCheckKey(t)
	srv := startTestJWKSServer(t, key, "kid-1")
	cache := newTestCache(srv.URL)

	now := time.Now()
	token := signTestAppCheckToken(t, key, "kid-1", jwt.Claims{
		Issuer:   "https://firebaseappcheck.googleapis.com/123456",
		Subject:  "1:123456:android:abcdef",
		Audience: jwt.Audience{"projects/123456"},
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt: jwt.NewNumericDate(now),
	})

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	handler := newAppCheckMiddleware("123456", true, cache)(next)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("X-Firebase-AppCheck", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected a genuinely valid token to reach the next handler")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAppCheckJWKSCache_ReusesStaleKeysOnFetchFailure(t *testing.T) {
	key := testAppCheckKey(t)
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "kid-1", Algorithm: string(jose.RS256), Use: "sig"},
		}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)

	cache := &appCheckJWKSCache{httpClient: http.DefaultClient, url: srv.URL, ttl: 0} // ttl 0: always "stale", forces a fetch on every get

	if _, err := cache.get(t.Context()); err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	up = false
	keys, err := cache.get(t.Context())
	if err != nil {
		t.Fatalf("expected the cache to fall back to stale keys instead of erroring, got: %v", err)
	}
	if len(keys.Keys) != 1 {
		t.Errorf("keys = %v, want the previously cached key reused", keys)
	}
}
