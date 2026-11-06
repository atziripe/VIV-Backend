package usecase_test

import (
	"context"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

func TestSaveNutritionMealSelection_HappyPath(t *testing.T) {
	plans := &fakeNutritionPlanRepo{saved: map[string]domain.NutritionWeekPlan{
		"u1": {Phase: domain.PhaseFollicular},
	}}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans, &fakePlanRepo{})

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "Monday", MealSlot: "Breakfast", OptionIndex: 1,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	saved := plans.saved["u1"]
	if saved.MealSelections["monday"]["breakfast"] != 1 {
		t.Errorf("MealSelections[monday][breakfast] = %d, want 1", saved.MealSelections["monday"]["breakfast"])
	}
}

func TestSaveNutritionMealSelection_AccumulatesAcrossCalls(t *testing.T) {
	plans := &fakeNutritionPlanRepo{saved: map[string]domain.NutritionWeekPlan{"u1": {}}}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans, &fakePlanRepo{})

	if err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 0}); err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{UserID: "u1", Weekday: "monday", MealSlot: "lunch", OptionIndex: 2}); err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}

	saved := plans.saved["u1"]
	if saved.MealSelections["monday"]["breakfast"] != 0 {
		t.Errorf("breakfast selection = %d, want 0 (should survive the second call)", saved.MealSelections["monday"]["breakfast"])
	}
	if saved.MealSelections["monday"]["lunch"] != 2 {
		t.Errorf("lunch selection = %d, want 2", saved.MealSelections["monday"]["lunch"])
	}
}

func TestSaveNutritionMealSelection_RejectsInvalidOptionIndex(t *testing.T) {
	plans := &fakeNutritionPlanRepo{saved: map[string]domain.NutritionWeekPlan{"u1": {}}}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans, &fakePlanRepo{})

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 3,
	})
	if err == nil {
		t.Fatal("expected an error for an out-of-range option_index")
	}
}

// ============================================================================
// A user with no new-pipeline nutrition plan falls back to her legacy
// domain.Plan — regression coverage for the bug where GET /nutrition/plan
// already fell back to the legacy plan (GetNutritionPlanUseCase), but this
// usecase didn't: a legacy-pipeline user could see her plan but never save
// a meal swap.
// ============================================================================

func TestSaveNutritionMealSelection_FallsBackToLegacyPlanWhenNoNewPipelinePlanExists(t *testing.T) {
	nutritionPlans := &fakeNutritionPlanRepo{} // no new-pipeline plan for u1
	legacyPlans := &fakePlanRepo{latest: map[string]*domain.Plan{
		"u1": {ID: "plan-1", UserID: "u1"},
	}}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(nutritionPlans, legacyPlans)

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 1,
	})
	if err != nil {
		t.Fatalf("expected the fallback to the legacy plan to succeed, got error: %v", err)
	}

	saved := legacyPlans.latest["u1"]
	if saved.MealSelections["monday"]["breakfast"] != 1 {
		t.Errorf("legacy plan MealSelections[monday][breakfast] = %d, want 1", saved.MealSelections["monday"]["breakfast"])
	}
	// The new-pipeline repository must stay untouched — this user never had
	// a plan there, and this call must not fabricate one.
	if nutritionPlans.saved != nil {
		t.Errorf("expected the new-pipeline nutrition plan repository to stay untouched, got %+v", nutritionPlans.saved)
	}
}

func TestSaveNutritionMealSelection_RequiresExistingPlanOnEitherPipeline(t *testing.T) {
	uc := usecase.NewSaveNutritionMealSelectionUseCase(&fakeNutritionPlanRepo{}, &fakePlanRepo{})

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 0,
	})
	if err == nil {
		t.Fatal("expected an error when the user has no nutrition plan on either pipeline")
	}
}

func TestSaveNutritionMealSelection_PrefersNewPipelinePlanWhenBothExist(t *testing.T) {
	nutritionPlans := &fakeNutritionPlanRepo{saved: map[string]domain.NutritionWeekPlan{"u1": {}}}
	legacyPlans := &fakePlanRepo{latest: map[string]*domain.Plan{
		"u1": {ID: "plan-1", UserID: "u1"},
	}}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(nutritionPlans, legacyPlans)

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 1,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := nutritionPlans.saved["u1"].MealSelections["monday"]["breakfast"]; got != 1 {
		t.Errorf("new-pipeline MealSelections[monday][breakfast] = %d, want 1", got)
	}
	if legacy := legacyPlans.latest["u1"]; legacy.MealSelections != nil {
		t.Errorf("expected the legacy plan to stay untouched when a new-pipeline plan exists, got %+v", legacy.MealSelections)
	}
}
