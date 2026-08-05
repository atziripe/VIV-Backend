package mealgen

import (
	"context"

	"viv/internal/core/domain"
	"viv/internal/core/nutrition"
)

// EnrichPlanCopy replaces each meal option's placeholder Name/Summary (the
// literal ingredient list Generator.GenerateWeekMeals fills in synchronously)
// with polished copy — cache hits first, then one batched LLM call for
// everything still missing. Pure enough to unit test without a goroutine or
// a real LLM; the async/fire-and-forget wiring lives in AsyncCopyEnricher.
//
// changed reports whether anything in plan was actually modified — false
// means the caller can skip writing the plan back to storage.
func EnrichPlanCopy(
	ctx context.Context,
	plan domain.NutritionWeekPlan,
	gen CopyGenerator,
	cache CopyCache,
) (enriched domain.NutritionWeekPlan, changed bool, err error) {
	type ref struct{ day, meal, option int }

	var misses []CopyRequest
	var missRefs []ref

	for di := range plan.Days {
		for mi := range plan.Days[di].Meals {
			for oi := range plan.Days[di].Meals[mi].Options {
				opt := plan.Days[di].Meals[mi].Options[oi]
				key := nutrition.CompositionKey(opt.Name, opt.Ingredients)

				if name, summary, ok := cache.Get(key); ok {
					plan.Days[di].Meals[mi].Options[oi].Name = name
					plan.Days[di].Meals[mi].Options[oi].Summary = summary
					changed = true
					continue
				}

				misses = append(misses, CopyRequest{
					Key:          key,
					TemplateName: opt.Name,
					Ingredients:  opt.Ingredients,
				})
				missRefs = append(missRefs, ref{di, mi, oi})
			}
		}
	}

	if len(misses) == 0 {
		// Every option resolved from cache (or there was nothing to do) —
		// the plan is fully polished.
		plan.CopyEnriched = true
		return plan, changed, nil
	}

	results, genErr := gen.GenerateCopy(ctx, misses)
	if genErr != nil {
		// Cache hits already applied above are still a real improvement —
		// return them rather than discarding partial progress. CopyEnriched
		// stays false: some options are still unresolved, so the client
		// should keep polling.
		return plan, changed, genErr
	}

	byKey := make(map[string]CopyResult, len(results))
	for _, r := range results {
		byKey[r.Key] = r
	}

	for i, r := range missRefs {
		res, ok := byKey[misses[i].Key]
		if !ok {
			continue
		}
		plan.Days[r.day].Meals[r.meal].Options[r.option].Name = res.Name
		plan.Days[r.day].Meals[r.meal].Options[r.option].Summary = res.Summary
		cache.Set(misses[i].Key, res.Name, res.Summary)
		changed = true
	}

	plan.CopyEnriched = true
	return plan, changed, nil
}
