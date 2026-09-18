package mealgen

import (
	"context"
	"log"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// AsyncNutritionPlanCopyEnricher is AsyncCopyEnricher's counterpart for the
// new pipeline's NutritionPlanRepository (one document per user, no
// planID) — same fire-and-forget, patch-after-save behavior.
type AsyncNutritionPlanCopyEnricher struct {
	generator CopyGenerator
	cache     CopyCache
	plans     usecase.NutritionPlanRepository
	timeout   time.Duration
}

func NewAsyncNutritionPlanCopyEnricher(generator CopyGenerator, cache CopyCache, plans usecase.NutritionPlanRepository) *AsyncNutritionPlanCopyEnricher {
	return &AsyncNutritionPlanCopyEnricher{
		generator: generator,
		cache:     cache,
		plans:     plans,
		timeout:   60 * time.Second,
	}
}

func (e *AsyncNutritionPlanCopyEnricher) EnrichAsync(userID string, plan domain.NutritionWeekPlan) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
		defer cancel()

		enriched, changed, err := EnrichPlanCopy(ctx, plan, e.generator, e.cache)
		if err != nil {
			log.Printf("[mealgen.copy] nutrition-plan enrichment failed user=%s err=%v", userID, err)
		}
		if !changed {
			return
		}

		if err := e.plans.Save(ctx, userID, enriched); err != nil {
			log.Printf("[mealgen.copy] nutrition-plan update failed user=%s err=%v", userID, err)
			return
		}
		log.Printf("[mealgen.copy] enriched copy saved user=%s (nutrition plan)", userID)
	}()
}
