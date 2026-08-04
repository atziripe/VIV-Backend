package mealgen_test

import (
	"context"
	"testing"

	"viv/internal/adapters/mealgen"
	"viv/internal/core/domain"
	"viv/internal/core/nutrition"
)

// fakeCopyGenerator returns canned copy and counts how many times it was
// called, so tests can assert the cache actually avoids repeat calls.
type fakeCopyGenerator struct {
	calls int
}

func (f *fakeCopyGenerator) GenerateCopy(_ context.Context, requests []mealgen.CopyRequest) ([]mealgen.CopyResult, error) {
	f.calls++
	results := make([]mealgen.CopyResult, len(requests))
	for i, r := range requests {
		results[i] = mealgen.CopyResult{
			Key:     r.Key,
			Name:    "Polished: " + r.TemplateName,
			Summary: "A delicious plate.",
		}
	}
	return results, nil
}

func samplePlan() domain.NutritionWeekPlan {
	opt := domain.MealOption{
		Name: "Mediterranean lunch bowl", // the pre-enrichment placeholder is the template name
		Ingredients: []domain.MealIngredient{
			{Name: "Chicken breast", AmountG: 150, Approx: "~1 breast"},
			{Name: "White rice, cooked", AmountG: 200, Approx: "~1.25 cups"},
		},
		Macros: domain.DayMacros{Calories: 500, ProteinG: 40, CarbsG: 50, FatG: 15},
	}
	return domain.NutritionWeekPlan{
		Days: [7]domain.NutritionDayPlan{
			{Weekday: "monday", Meals: []domain.MealSlot{{MealName: "Lunch", Options: []domain.MealOption{opt}}}},
		},
	}
}

func TestEnrichPlanCopy_CacheMissCallsGeneratorAndPopulatesCache(t *testing.T) {
	gen := &fakeCopyGenerator{}
	cache := mealgen.NewInMemoryCopyCache()

	enriched, changed, err := mealgen.EnrichPlanCopy(context.Background(), samplePlan(), gen, cache)
	if err != nil {
		t.Fatalf("EnrichPlanCopy returned error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on a cache miss that got resolved")
	}
	if gen.calls != 1 {
		t.Errorf("expected 1 generator call, got %d", gen.calls)
	}

	got := enriched.Days[0].Meals[0].Options[0]
	if got.Name != "Polished: Mediterranean lunch bowl" {
		t.Errorf("Name = %q, want polished copy", got.Name)
	}
	if got.Summary != "A delicious plate." {
		t.Errorf("Summary = %q, want polished copy", got.Summary)
	}
	// Ingredients/macros must be untouched — copy polish never changes them.
	if len(got.Ingredients) != 2 || got.Macros.Calories != 500 {
		t.Errorf("ingredients/macros were modified by copy enrichment: %+v", got)
	}
}

func TestEnrichPlanCopy_CacheHitSkipsGenerator(t *testing.T) {
	gen := &fakeCopyGenerator{}
	cache := mealgen.NewInMemoryCopyCache()

	// Prime the cache with a first call.
	_, _, err := mealgen.EnrichPlanCopy(context.Background(), samplePlan(), gen, cache)
	if err != nil {
		t.Fatalf("priming call failed: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("expected 1 generator call after priming, got %d", gen.calls)
	}

	// A second, independent plan with the exact same composition should hit
	// the cache and never call the generator again.
	enriched, changed, err := mealgen.EnrichPlanCopy(context.Background(), samplePlan(), gen, cache)
	if err != nil {
		t.Fatalf("EnrichPlanCopy returned error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on a cache hit that updated placeholder text")
	}
	if gen.calls != 1 {
		t.Errorf("expected generator to still have been called only once (cache hit), got %d calls", gen.calls)
	}

	got := enriched.Days[0].Meals[0].Options[0]
	if got.Name != "Polished: Mediterranean lunch bowl" {
		t.Errorf("Name = %q, want cached polished copy", got.Name)
	}
}

func TestEnrichPlanCopy_GeneratorErrorPreservesCacheHits(t *testing.T) {
	cache := mealgen.NewInMemoryCopyCache()
	plan := samplePlan()
	primedOpt := plan.Days[0].Meals[0].Options[0]
	primedKey := nutrition.CompositionKey(primedOpt.Name, primedOpt.Ingredients)
	cache.Set(primedKey, "Cached Name", "Cached summary")
	// Add a second option with a different composition that will miss the
	// cache and hit the failing generator.
	plan.Days[0].Meals[0].Options = append(plan.Days[0].Meals[0].Options, domain.MealOption{
		Name: "Latin lunch bowl",
		Ingredients: []domain.MealIngredient{
			{Name: "Tempeh", AmountG: 100, Approx: "~1 block"},
		},
	})

	failing := &erroringGenerator{}
	enriched, changed, err := mealgen.EnrichPlanCopy(context.Background(), plan, failing, cache)
	if err == nil {
		t.Fatal("expected an error from the failing generator")
	}
	if !changed {
		t.Error("expected changed=true because the cache-hit option was still applied despite the miss failing")
	}
	if enriched.Days[0].Meals[0].Options[0].Name != "Cached Name" {
		t.Errorf("cache-hit option should keep its cached copy even though the batch call failed, got %q",
			enriched.Days[0].Meals[0].Options[0].Name)
	}
}

type erroringGenerator struct{}

func (erroringGenerator) GenerateCopy(context.Context, []mealgen.CopyRequest) ([]mealgen.CopyResult, error) {
	return nil, context.DeadlineExceeded
}
