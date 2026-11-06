package usecase

import (
	"context"
	"fmt"
	"strings"
)

// SaveNutritionMealSelectionUseCase is SaveMealSelectionUseCase's
// counterpart for the new pipeline's standalone nutrition plan (see
// submit_nutrition_onboarding.go) — same weekday/meal_slot/option_index
// semantics, without a plan_id since NutritionPlanRepository holds one
// plan per user rather than one per generation.
type SaveNutritionMealSelectionInput struct {
	UserID      string
	Weekday     string
	MealSlot    string
	OptionIndex int
}

type SaveNutritionMealSelectionUseCase struct {
	NutritionPlans NutritionPlanRepository
}

func NewSaveNutritionMealSelectionUseCase(nutritionPlans NutritionPlanRepository) *SaveNutritionMealSelectionUseCase {
	return &SaveNutritionMealSelectionUseCase{NutritionPlans: nutritionPlans}
}

func (uc *SaveNutritionMealSelectionUseCase) Execute(ctx context.Context, in SaveNutritionMealSelectionInput) error {
	userID := strings.TrimSpace(in.UserID)
	weekday := strings.ToLower(strings.TrimSpace(in.Weekday))
	slot := strings.ToLower(strings.TrimSpace(in.MealSlot))

	if userID == "" {
		return fmt.Errorf("save nutrition meal selection: userID is required")
	}
	if weekday == "" || slot == "" {
		return fmt.Errorf("weekday and meal_slot are required")
	}
	if in.OptionIndex < 0 || in.OptionIndex > 2 {
		return fmt.Errorf("option_index must be 0, 1, or 2")
	}

	existing, err := uc.NutritionPlans.GetByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading nutrition plan: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("no nutrition plan found for user: %s", userID)
	}

	plan := existing.Plan
	if plan.MealSelections == nil {
		plan.MealSelections = make(map[string]map[string]int)
	}
	if plan.MealSelections[weekday] == nil {
		plan.MealSelections[weekday] = make(map[string]int)
	}
	plan.MealSelections[weekday][slot] = in.OptionIndex

	return uc.NutritionPlans.Save(ctx, userID, plan)
}
