package nutrition

import (
	"math"
	"testing"

	"viv/internal/core/domain"
)

func chickenBreast() domain.Ingredient {
	return domain.Ingredient{
		ID:            "chicken_breast",
		Role:          domain.RoleProteinSource,
		Name:          "Chicken breast",
		MacrosPer100g: domain.MacrosPer100g{Calories: 165, ProteinG: 31, CarbsG: 0, FatG: 3.6},
		MinGrams:      60,
		MaxGrams:      220,
	}
}

func cookedRice() domain.Ingredient {
	return domain.Ingredient{
		ID:            "white_rice_cooked",
		Role:          domain.RoleCarbSource,
		Name:          "White rice, cooked",
		MacrosPer100g: domain.MacrosPer100g{Calories: 130, ProteinG: 2.7, CarbsG: 28, FatG: 0.3},
		MinGrams:      50,
		MaxGrams:      300,
	}
}

func oliveOil() domain.Ingredient {
	return domain.Ingredient{
		ID:            "olive_oil",
		Role:          domain.RoleFatSource,
		Name:          "Olive oil",
		MacrosPer100g: domain.MacrosPer100g{Calories: 884, ProteinG: 0, CarbsG: 0, FatG: 100},
		MinGrams:      5,
		MaxGrams:      25,
	}
}

// TestSolvePlate_MatchesHandComputedExample verifies the solver against the
// system solved by hand in the design discussion: target 30g protein/55g
// carbs/12g fat from chicken+rice+olive oil should yield ~80g chicken,
// ~196g rice, ~8.5g oil (before rounding).
func TestSolvePlate_MatchesHandComputedExample(t *testing.T) {
	target := domain.DayMacros{Calories: 450, ProteinG: 30, CarbsG: 55, FatG: 12}

	result, err := SolvePlate(target, chickenBreast(), cookedRice(), oliveOil(), nil)
	if err != nil {
		t.Fatalf("SolvePlate returned error: %v", err)
	}

	raw := map[string]float64{}
	for _, c := range result.Components {
		raw[c.Ingredient.ID] = c.RawGrams
	}

	wantApprox := map[string]float64{
		"chicken_breast":    79.67,
		"white_rice_cooked": 196.43,
		"olive_oil":         8.54,
	}
	for id, want := range wantApprox {
		got, ok := raw[id]
		if !ok {
			t.Fatalf("missing component %q in result", id)
		}
		if diff := math.Abs(got - want); diff > 0.5 {
			t.Errorf("%s: raw grams = %.2f, want ~%.2f (diff %.2f)", id, got, want, diff)
		}
	}

	// The solved system should reproduce the target macros almost exactly
	// before any clamping/rounding — verify by reconstructing macros from
	// raw (unclamped, unrounded) grams directly.
	var p, c, f float64
	for _, comp := range result.Components {
		factor := comp.RawGrams / 100
		p += factor * comp.Ingredient.MacrosPer100g.ProteinG
		c += factor * comp.Ingredient.MacrosPer100g.CarbsG
		f += factor * comp.Ingredient.MacrosPer100g.FatG
	}
	if math.Abs(p-30) > 0.01 {
		t.Errorf("reconstructed protein = %.4f, want 30", p)
	}
	if math.Abs(c-55) > 0.01 {
		t.Errorf("reconstructed carbs = %.4f, want 55", c)
	}
	if math.Abs(f-12) > 0.01 {
		t.Errorf("reconstructed fat = %.4f, want 12", f)
	}
}

func TestSolvePlate_ClampsUnrealisticAmounts(t *testing.T) {
	// An extreme protein target should clamp chicken to its max_grams
	// rather than solving for an absurd portion.
	target := domain.DayMacros{Calories: 900, ProteinG: 150, CarbsG: 20, FatG: 10}

	protein := chickenBreast()
	protein.MaxGrams = 220 // 150g protein / 31g per 100g ~= 484g raw, well above max

	result, err := SolvePlate(target, protein, cookedRice(), oliveOil(), nil)
	if err != nil {
		t.Fatalf("SolvePlate returned error: %v", err)
	}

	var proteinComp *SolvedComponent
	for i := range result.Components {
		if result.Components[i].Ingredient.ID == protein.ID {
			proteinComp = &result.Components[i]
		}
	}
	if proteinComp == nil {
		t.Fatal("protein component not found")
	}
	if !proteinComp.Clamped {
		t.Errorf("expected protein component to be clamped, RawGrams=%.1f MaxGrams=%.1f", proteinComp.RawGrams, protein.MaxGrams)
	}
	if proteinComp.Grams > protein.MaxGrams {
		t.Errorf("clamped grams %.1f exceeds MaxGrams %.1f", proteinComp.Grams, protein.MaxGrams)
	}
	// Achieved protein should now fall short of target — that's expected
	// and exactly what MaxDeviation exists to surface.
	if result.AchievedMacros.ProteinG >= target.ProteinG {
		t.Errorf("expected achieved protein (%.1f) to fall short of target (%.1f) after clamping",
			result.AchievedMacros.ProteinG, target.ProteinG)
	}
	if result.MaxDeviation <= 0 {
		t.Errorf("expected non-zero MaxDeviation after clamping, got %.4f", result.MaxDeviation)
	}
}

func TestSolvePlate_FixedSlotsSubtractedFromTarget(t *testing.T) {
	target := domain.DayMacros{Calories: 470, ProteinG: 30, CarbsG: 60, FatG: 12}

	spinach := domain.Ingredient{
		ID:            "spinach",
		Role:          domain.RoleVegetable,
		MacrosPer100g: domain.MacrosPer100g{Calories: 23, ProteinG: 2.9, CarbsG: 3.6, FatG: 0.4},
	}
	fixed := []FixedIngredient{{Ingredient: spinach, Grams: 80}}

	result, err := SolvePlate(target, chickenBreast(), cookedRice(), oliveOil(), fixed)
	if err != nil {
		t.Fatalf("SolvePlate returned error: %v", err)
	}

	// 15% matches the tolerance the current LLM-based generator already
	// targets (meal_generator.go: "macros must approximately match... within
	// ±15%") — rounding 3 components to realistic serving increments can't
	// do better than that without a unit_hint per ingredient.
	if result.MaxDeviation > 0.15 {
		t.Errorf("expected solved plate (with fixed veg slot subtracted) to land within 15%% of target, got %.4f deviation", result.MaxDeviation)
	}

	found := false
	for _, c := range result.Components {
		if c.Ingredient.ID == "spinach" {
			found = true
			if c.Grams != 80 {
				t.Errorf("fixed spinach slot grams = %.1f, want 80 (unsolved)", c.Grams)
			}
		}
	}
	if !found {
		t.Error("fixed vegetable slot missing from result components")
	}
}

func TestSolvePlate_RejectsWrongRole(t *testing.T) {
	target := domain.DayMacros{Calories: 400, ProteinG: 30, CarbsG: 40, FatG: 10}
	_, err := SolvePlate(target, cookedRice() /* wrong role */, cookedRice(), oliveOil(), nil)
	if err == nil {
		t.Fatal("expected error when protein slot is filled by a carb_source ingredient, got nil")
	}
}
