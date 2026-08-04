package mealgen

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// AsyncCopyEnricher runs EnrichPlanCopy in a detached goroutine and patches
// the already-saved plan when done. It must never block plan generation or
// job completion — decoupling LLM latency from the user-facing "plan ready"
// signal is the entire point of this design (see design discussion: the
// original LLM-in-the-critical-path approach was the thing being fixed).
type AsyncCopyEnricher struct {
	generator CopyGenerator
	cache     CopyCache
	planRepo  usecase.PlanRepository
	timeout   time.Duration
}

func NewAsyncCopyEnricher(generator CopyGenerator, cache CopyCache, planRepo usecase.PlanRepository) *AsyncCopyEnricher {
	return &AsyncCopyEnricher{
		generator: generator,
		cache:     cache,
		planRepo:  planRepo,
		timeout:   60 * time.Second,
	}
}

// EnrichAsync kicks off enrichment in the background and returns
// immediately — callers never wait on this.
func (e *AsyncCopyEnricher) EnrichAsync(userID, planID string, plan domain.NutritionWeekPlan) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()

		enriched, changed, err := EnrichPlanCopy(ctx, plan, e.generator, e.cache)
		if err != nil {
			log.Printf("[mealgen.copy] enrichment failed user=%s plan=%s err=%v", userID, planID, err)
		}
		if !changed {
			return
		}

		data, err := json.Marshal(enriched)
		if err != nil {
			log.Printf("[mealgen.copy] marshal failed user=%s plan=%s err=%v", userID, planID, err)
			return
		}
		if err := e.planRepo.UpdateNutritionJSON(ctx, userID, planID, data); err != nil {
			log.Printf("[mealgen.copy] update failed user=%s plan=%s err=%v", userID, planID, err)
			return
		}
		log.Printf("[mealgen.copy] enriched copy saved user=%s plan=%s", userID, planID)
	}()
}
