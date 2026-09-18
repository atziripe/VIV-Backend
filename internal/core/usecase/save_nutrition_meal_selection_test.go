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
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans)

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
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans)

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

func TestSaveNutritionMealSelection_RequiresExistingPlan(t *testing.T) {
	plans := &fakeNutritionPlanRepo{}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans)

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 0,
	})
	if err == nil {
		t.Fatal("expected an error when the user has no nutrition plan yet")
	}
}

func TestSaveNutritionMealSelection_RejectsInvalidOptionIndex(t *testing.T) {
	plans := &fakeNutritionPlanRepo{saved: map[string]domain.NutritionWeekPlan{"u1": {}}}
	uc := usecase.NewSaveNutritionMealSelectionUseCase(plans)

	err := uc.Execute(context.Background(), usecase.SaveNutritionMealSelectionInput{
		UserID: "u1", Weekday: "monday", MealSlot: "breakfast", OptionIndex: 3,
	})
	if err == nil {
		t.Fatal("expected an error for an out-of-range option_index")
	}
}
