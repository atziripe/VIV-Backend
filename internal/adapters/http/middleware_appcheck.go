package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const appCheckJWKSURL = "https://firebaseappcheck.googleapis.com/v1/jwks"

// appCheckJWKSCache fetches and caches Firebase App Check's public signing
// keys. Google rotates them roughly every 6 hours, so re-fetching on every
// request would be both slow and unnecessary; a stale cached set is kept
// and reused if a refresh fails, rather than failing every request open
// during a transient network blip. Safe for concurrent use.
type appCheckJWKSCache struct {
	httpClient *http.Client
	url        string
	ttl        time.Duration

	mu        sync.RWMutex
	keys      jose.JSONWebKeySet
	fetchedAt time.Time
}

func newAppCheckJWKSCache() *appCheckJWKSCache {
	return &appCheckJWKSCache{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		url:        appCheckJWKSURL,
		ttl:        6 * time.Hour,
	}
}

func (c *appCheckJWKSCache) get(ctx context.Context) (jose.JSONWebKeySet, error) {
	c.mu.RLock()
	fresh := time.Since(c.fetchedAt) < c.ttl && len(c.keys.Keys) > 0
	cached := c.keys
	c.mu.RUnlock()
	if fresh {
		return cached, nil
	}

	fetched, err := c.fetch(ctx)
	if err != nil {
		if len(cached.Keys) > 0 {
			log.Printf("[appcheck] jwks refresh failed, reusing stale keys: %v", err)
			return cached, nil
		}
		return jose.JSONWebKeySet{}, err
	}

	c.mu.Lock()
	c.keys = fetched
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	return fetched, nil
}

func (c *appCheckJWKSCache) fetch(ctx context.Context) (jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return jose.JSONWebKeySet{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var keys jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return jose.JSONWebKeySet{}, err
	}
	return keys, nil
}

// AppCheckMiddleware verifies the Firebase App Check token on the
// X-Firebase-AppCheck header — layered in front of FirebaseAuthMiddleware,
// it answers "is this request coming from our real app" (device/app
// attestation), a separate question from FirebaseAuthMiddleware's "which
// user is this" (identity via the Authorization header). Verification is
// done by hand (fetch Firebase's published JWKS, check the RS256
// signature, then iss/aud/exp) rather than through the Admin SDK: the
// firebase.google.com/go v3 version this repo pins has no AppCheck client
// (that landed in the v4 admin-go line) — this is the same verification
// Firebase's own docs describe for backends without SDK support.
//
// projectNumber is the Firebase project's numeric project number (Firebase
// console → Project settings → General), not FirebaseProjectID — App
// Check's iss/aud claims are keyed by project number. Leaving it empty
// disables App Check verification entirely (a no-op passthrough) — that's
// the default until FIREBASE_PROJECT_NUMBER is configured.
//
// enforce controls whether a missing/invalid token actually blocks the
// request (401) or is only logged. Start with enforce=false (monitor
// mode) while the mobile client ships its App Check SDK integration
// (Play Integrity/DeviceCheck) — enforcing before any client sends the
// header would lock every user out of the app. Flip APP_CHECK_ENFORCE=true
// once the App Check console's request metrics show real traffic carrying
// valid tokens.
func AppCheckMiddleware(projectNumber string, enforce bool) func(http.Handler) http.Handler {
	projectNumber = strings.TrimSpace(projectNumber)
	if projectNumber == "" {
		log.Printf("[appcheck] FIREBASE_PROJECT_NUMBER not set — App Check verification disabled")
		return func(next http.Handler) http.Handler { return next }
	}

	return newAppCheckMiddleware(projectNumber, enforce, newAppCheckJWKSCache())
}

// newAppCheckMiddleware is AppCheckMiddleware's testable core — takes an
// already-constructed cache so tests can point it at a fake JWKS server
// instead of Firebase's real endpoint, without a global mutable seam.
func newAppCheckMiddleware(projectNumber string, enforce bool, cache *appCheckJWKSCache) func(http.Handler) http.Handler {
	issuer := "https://firebaseappcheck.googleapis.com/" + projectNumber
	audience := jwt.Audience{"projects/" + projectNumber}

	mode := "monitor"
	if enforce {
		mode = "enforce"
	}
	log.Printf("[appcheck] verification enabled, mode=%s", mode)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := strings.TrimSpace(r.Header.Get("X-Firebase-AppCheck"))

			var failReason string
			if token == "" {
				failReason = "missing X-Firebase-AppCheck header"
			} else if err := verifyAppCheckToken(r.Context(), cache, issuer, audience, token); err != nil {
				failReason = err.Error()
			}

			if failReason == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !enforce {
				log.Printf("[appcheck] would reject (monitor mode) path=%s reason=%s", r.URL.Path, failReason)
				next.ServeHTTP(w, r)
				return
			}

			log.Printf("[appcheck] rejected path=%s reason=%s", r.URL.Path, failReason)
			http.Error(w, "invalid or missing App Check token", http.StatusUnauthorized)
		})
	}
}

func verifyAppCheckToken(ctx context.Context, cache *appCheckJWKSCache, issuer string, audience jwt.Audience, rawToken string) error {
	parsed, err := jwt.ParseSigned(rawToken, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return fmt.Errorf("parsing token: %w", err)
	}

	keys, err := cache.get(ctx)
	if err != nil {
		return fmt.Errorf("fetching signing keys: %w", err)
	}

	var claims jwt.Claims
	if err := parsed.Claims(&keys, &claims); err != nil {
		return fmt.Errorf("verifying signature: %w", err)
	}

	if err := claims.Validate(jwt.Expected{Issuer: issuer, AnyAudience: audience}); err != nil {
		return fmt.Errorf("validating claims: %w", err)
	}

	return nil
}
