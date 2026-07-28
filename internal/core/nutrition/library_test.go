package nutrition_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/nutrition"
)

// TestSeedLibraries_LoadAndSolve is an end-to-end check against the actual
// seeded content in internal/content/nutrition: load both libraries, pick a
// template for each meal_type, resolve ingredient candidates under a
// realistic restriction, and solve a plate — proving the whole pipeline
// (not just the algebra in isolation) works against real authored content.
func TestSeedLibraries_LoadAndSolve(t *testing.T) {
	ingredients, err := nutrition.LoadIngredientLibrary("../../content/nutrition")
	if err != nil {
		t.Fatalf("LoadIngredientLibrary: %v", err)
	}
	templates, err := nutrition.LoadMealTemplateLibrary("../../content/nutrition")
	if err != nil {
		t.Fatalf("LoadMealTemplateLibrary: %v", err)
	}

	if got := ingredients.Count(); got < 40 {
		t.Errorf("ingredient library has %d ingredients, expected >= 40", got)
	}
	if got := templates.Count(); got < 10 {
		t.Errorf("template library has %d templates, expected >= 10", got)
	}

	mealTypes := []string{"breakfast", "lunch", "dinner", "pre_training"}
	targets := map[string]domain.DayMacros{
		"breakfast":    {Calories: 450, ProteinG: 30, CarbsG: 55, FatG: 12},
		"lunch":        {Calories: 550, ProteinG: 38, CarbsG: 60, FatG: 18},
		"dinner":       {Calories: 550, ProteinG: 40, CarbsG: 50, FatG: 20},
		"pre_training": {Calories: 220, ProteinG: 16, CarbsG: 30, FatG: 6},
	}

	// A realistic restriction combo: dairy + gluten avoided.
	avoidContains := []string{"dairy", "gluten"}
	var avoidDigestion []string

	for _, mt := range mealTypes {
		tpls := templates.ForMealType(mt)
		if len(tpls) == 0 {
			t.Fatalf("no templates for meal_type %q", mt)
		}
		tpl := tpls[0]

		proteinCands := ingredients.CandidatesForRole(domain.RoleProteinSource, avoidContains, avoidDigestion)
		carbCands := ingredients.CandidatesForRole(domain.RoleCarbSource, avoidContains, avoidDigestion)

		wantsFat := false
		for _, slot := range tpl.Slots {
			if slot.Role == domain.RoleFatSource {
				wantsFat = true
			}
		}

		var fixed []nutrition.FixedIngredient
		for _, slot := range tpl.Slots {
			if slot.Role == domain.RoleVegetable || slot.Role == domain.RoleFruit {
				cands := ingredients.CandidatesForRole(slot.Role, avoidContains, avoidDigestion)
				if len(cands) == 0 {
					t.Fatalf("%s: no %s candidates surviving avoid%v", mt, slot.Role, avoidContains)
				}
				fixed = append(fixed, nutrition.FixedIngredient{Ingredient: cands[0], Grams: slot.FixedGrams})
			}
		}

		var result nutrition.SolveResult
		var err error
		if wantsFat {
			fatCands := ingredients.CandidatesForRole(domain.RoleFatSource, avoidContains, avoidDigestion)
			if len(proteinCands) == 0 || len(carbCands) == 0 || len(fatCands) == 0 {
				t.Fatalf("%s: not enough candidates surviving avoid%v — protein=%d carb=%d fat=%d",
					mt, avoidContains, len(proteinCands), len(carbCands), len(fatCands))
			}
			result, err = nutrition.SelectBestPlate(targets[mt], proteinCands, carbCands, fatCands, fixed)
		} else {
			// pre_training: fat is a ceiling ("<= 8g"), not a solved target —
			// see solver_two_component.go.
			if len(proteinCands) == 0 || len(carbCands) == 0 {
				t.Fatalf("%s: not enough candidates surviving avoid%v — protein=%d carb=%d",
					mt, avoidContains, len(proteinCands), len(carbCands))
			}
			result, err = nutrition.SelectBestPlateTwoComponent(targets[mt], proteinCands, carbCands, fixed, 8)
		}
		if err != nil {
			t.Fatalf("%s: selection failed with template %q: %v", mt, tpl.ID, err)
		}
		if result.MaxDeviation > 0.20 {
			t.Errorf("%s: template %q deviated %.1f%% from target", mt, tpl.ID, result.MaxDeviation*100)
		}
		if result.FatCeilingExceeded {
			t.Errorf("%s: template %q exceeded the fat ceiling — achieved %.1fg", mt, tpl.ID, result.AchievedMacros.FatG)
		}

		t.Logf("%s [%s]: target=%+v achieved=%+v deviation=%.1f%% fatCeilingExceeded=%v",
			mt, tpl.ID, targets[mt], result.AchievedMacros, result.MaxDeviation*100, result.FatCeilingExceeded)
		for _, c := range result.Components {
			t.Logf("    %s: %s (%.0fg, clamped=%v)", c.Ingredient.ID, c.Approx, c.Grams, c.Clamped)
		}
	}
}
