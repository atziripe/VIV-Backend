package mealgen

import (
	"context"
	"sync"

	"viv/internal/core/domain"
)

// ============================================================================
// COPY POLISH — enriches the plain, deterministic Name/Summary that
// Generator.GenerateWeekMeals fills in synchronously with LLM-written copy,
// entirely off the request path. See copy_enricher.go for the orchestration
// and async_enricher.go for how it's wired to run after the plan is already
// saved and the job marked done.
// ============================================================================

// CopyRequest asks for polished name+summary for one already-solved plate.
// Ingredients/amounts are fixed inputs, never something the generator may
// change — the macros are already correct by the time this runs.
type CopyRequest struct {
	Key          string
	TemplateName string
	Ingredients  []domain.MealIngredient
}

// CopyResult is the polished copy for one CopyRequest, matched back by Key.
type CopyResult struct {
	Key     string
	Name    string
	Summary string
}

// CopyGenerator writes copy for a batch of solved plates in one call.
// Defined here (the consumer), not in the LLM adapter, so implementations
// stay swappable/mockable — same convention as usecase.MealContentGenerator.
type CopyGenerator interface {
	GenerateCopy(ctx context.Context, requests []CopyRequest) ([]CopyResult, error)
}

// CopyCache avoids repeat LLM calls for a composition already seen — the
// ingredient/template space is bounded (58 ingredients x 20 templates at
// last count), so hit rate climbs fast as more users are served.
type CopyCache interface {
	Get(key string) (name, summary string, ok bool)
	Set(key, name, summary string)
}

// InMemoryCopyCache is a process-local CopyCache — resets on restart, but
// needs no infra and already captures most of the benefit given the bounded
// composition space. A persisted (Firestore/Neon) implementation can satisfy
// the same interface later without touching callers.
type InMemoryCopyCache struct {
	mu    sync.RWMutex
	items map[string][2]string // key -> [name, summary]
}

func NewInMemoryCopyCache() *InMemoryCopyCache {
	return &InMemoryCopyCache{items: make(map[string][2]string)}
}

func (c *InMemoryCopyCache) Get(key string) (string, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[key]
	if !ok {
		return "", "", false
	}
	return v[0], v[1], true
}

func (c *InMemoryCopyCache) Set(key, name, summary string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = [2]string{name, summary}
}
