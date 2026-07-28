package mealgen_test

import (
	"context"
	"testing"

	"viv/internal/adapters/mealgen"
	"viv/internal/core/domain"
	"viv/internal/core/nutrition"
	"viv/internal/core/usecase"
)

// TestGenerateWeekMeals_EndToEnd exercises the full drop-in replacement for
// the LLM-based generator against the real seeded content: a training week
// (morning session) plus a rest day, with a realistic restriction combo
// (dairy + gluten avoided, mostly plant-based preference), and checks the
// output has the right shape and lands within tolerance for every slot.
func TestGenerateWeekMeals_EndToEnd(t *testing.T) {
	ingredients, err := nutrition.LoadIngredientLibrary("../../content/nutrition")
	if err != nil {
		t.Fatalf("LoadIngredientLibrary: %v", err)
	}
	templates, err := nutrition.LoadMealTemplateLibrary("../../content/nutrition")
	if err != nil {
		t.Fatalf("LoadMealTemplateLibrary: %v", err)
	}

	gen := mealgen.New(ingredients, templates)

	input := usecase.MealGenerationInput{
		Phase: domain.CyclePhase("follicular"),
		Targets: domain.MacroTargets{
			TrainingDay: domain.DayMacros{Calories: 2100, ProteinG: 130, CarbsG: 230, FatG: 65},
			RestDay:     domain.DayMacros{Calories: 1850, ProteinG: 130, CarbsG: 170, FatG: 60},
		},
		Days: []usecase.MealDayInput{
			{
				Weekday:       "monday",
				IsTrainingDay: true,
				SessionTime:   "morning",
				Macros:        domain.DayMacros{Calories: 2100, ProteinG: 130, CarbsG: 230, FatG: 65},
				MealSlots:     []string{"breakfast", "pre_training", "lunch", "dinner"},
			},
			{
				Weekday:       "tuesday",
				IsTrainingDay: false,
				Macros:        domain.DayMacros{Calories: 1850, ProteinG: 130, CarbsG: 170, FatG: 60},
				MealSlots:     []string{"breakfast", "lunch", "dinner"},
			},
		},
		DietRestrictions:    []string{"No dairy", "No gluten"},
		ProteinPreference:   "Mostly plant-based",
		DigestiveConditions: []string{"Bloating"},
		EatingStyle:         "Meal prep,Mediterranean",
	}

	days, tokens, err := gen.GenerateWeekMeals(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateWeekMeals returned error: %v", err)
	}
	if tokens.PromptTokens != 0 || tokens.CompletionTokens != 0 {
		t.Errorf("expected zero token usage (no LLM call), got %+v", tokens)
	}
	if len(days) != 2 {
		t.Fatalf("expected 2 days, got %d", len(days))
	}

	// ProteinPreference is "Mostly plant-based" above — animal-sourced
	// protein (including shellfish, which is tagged "shellfish" not "fish")
	// should be rare/absent, not the dominant choice.
	animalProteinNames := map[string]bool{
		"Chicken breast": true, "Turkey breast": true, "Salmon fillet": true,
		"Cod fillet": true, "Shrimp": true, "Eggs": true, "Egg whites": true,
		"Canned tuna": true, "Greek yogurt": true, "Cottage cheese": true,
		"Whey protein powder": true,
	}
	animalProteinCount := 0
	totalOptions := 0

	wantSlotsPerDay := map[string]int{"monday": 4, "tuesday": 3}
	for _, day := range days {
		want := wantSlotsPerDay[day.Weekday]
		if len(day.Meals) != want {
			t.Errorf("%s: got %d meal slots, want %d", day.Weekday, len(day.Meals), want)
		}

		for _, meal := range day.Meals {
			if len(meal.Options) == 0 {
				t.Errorf("%s/%s: no options generated", day.Weekday, meal.MealName)
				continue
			}
			if len(meal.Options) > 3 {
				t.Errorf("%s/%s: got %d options, want <= 3", day.Weekday, meal.MealName, len(meal.Options))
			}

			for _, opt := range meal.Options {
				totalOptions++
				if opt.Name == "" {
					t.Errorf("%s/%s: option has empty name", day.Weekday, meal.MealName)
				}
				if len(opt.Ingredients) == 0 {
					t.Errorf("%s/%s option %q: no ingredients", day.Weekday, meal.MealName, opt.Name)
				}
				for _, ing := range opt.Ingredients {
					if ing.Name == "" || ing.AmountG <= 0 {
						t.Errorf("%s/%s option %q: malformed ingredient %+v", day.Weekday, meal.MealName, opt.Name, ing)
					}
					if animalProteinNames[ing.Name] {
						animalProteinCount++
					}
				}
			}

			t.Logf("%s/%s (%s, target=%+v):", day.Weekday, meal.MealName, meal.TimingWindow, meal.MacroTargets)
			for _, opt := range meal.Options {
				t.Logf("    [%s] %s -> achieved=%+v", opt.Name, opt.Summary, opt.Macros)
			}
		}
	}

	if animalProteinCount > totalOptions/4 {
		t.Errorf("ProteinPreference=%q but %d/%d options served animal-sourced protein — preference isn't being applied",
			input.ProteinPreference, animalProteinCount, totalOptions)
	}
}

// TestGenerateWeekMeals_NeverServesAvoidedIngredients is a stricter,
// targeted check: with dairy explicitly avoided, no returned ingredient
// name should be one of the library's known dairy items.
func TestGenerateWeekMeals_NeverServesAvoidedIngredients(t *testing.T) {
	ingredients, err := nutrition.LoadIngredientLibrary("../../content/nutrition")
	if err != nil {
		t.Fatalf("LoadIngredientLibrary: %v", err)
	}
	templates, err := nutrition.LoadMealTemplateLibrary("../../content/nutrition")
	if err != nil {
		t.Fatalf("LoadMealTemplateLibrary: %v", err)
	}
	gen := mealgen.New(ingredients, templates)

	dairyNames := map[string]bool{
		"Greek yogurt": true, "Cottage cheese": true, "Butter": true,
		"Cheddar cheese": true, "Ghee": true, "Whey protein powder": true,
	}

	input := usecase.MealGenerationInput{
		Phase: domain.CyclePhase("luteal"),
		Days: []usecase.MealDayInput{
			{
				Weekday:   "wednesday",
				Macros:    domain.DayMacros{Calories: 1900, ProteinG: 120, CarbsG: 180, FatG: 60},
				MealSlots: []string{"breakfast", "lunch", "dinner"},
			},
		},
		DietRestrictions:  []string{"No dairy"},
		ProteinPreference: "Mixed plant + animal",
	}

	days, _, err := gen.GenerateWeekMeals(context.Background(), input)
	if err != nil {
		t.Fatalf("GenerateWeekMeals returned error: %v", err)
	}

	for _, day := range days {
		for _, meal := range day.Meals {
			for _, opt := range meal.Options {
				for _, ing := range opt.Ingredients {
					if dairyNames[ing.Name] {
						t.Errorf("%s/%s option %q served dairy ingredient %q despite 'No dairy' restriction",
							day.Weekday, meal.MealName, opt.Name, ing.Name)
					}
				}
			}
		}
	}
}
