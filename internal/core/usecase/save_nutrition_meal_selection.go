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

	// Plans is the OLD pipeline's repository — the fallback for a user who
	// was onboarded before the new standalone nutrition pipeline existed
	// and has never generated a new-pipeline NutritionPlan.
	// GetNutritionPlanUseCase already falls back to this same legacy
	// domain.Plan for reads (GET /nutrition/plan), so a user like that
	// sees a real plan in the app; without the identical fallback here,
	// she could see it but never save a meal swap, since this usecase
	// only knew about the new pipeline. See Execute.
	Plans PlanRepository
}

func NewSaveNutritionMealSelectionUseCase(nutritionPlans NutritionPlanRepository, plans PlanRepository) *SaveNutritionMealSelectionUseCase {
	return &SaveNutritionMealSelectionUseCase{NutritionPlans: nutritionPlans, Plans: plans}
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
	if existing != nil {
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

	// No new-pipeline plan for this user — fall back to the legacy
	// domain.Plan, mirroring GetNutritionPlanUseCase's own read-side
	// fallback exactly.
	legacy, err := uc.Plans.GetLatest(ctx, userID)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	if legacy == nil {
		return fmt.Errorf("no nutrition plan found for user: %s", userID)
	}

	if legacy.MealSelections == nil {
		legacy.MealSelections = make(map[string]map[string]int)
	}
	if legacy.MealSelections[weekday] == nil {
		legacy.MealSelections[weekday] = make(map[string]int)
	}
	legacy.MealSelections[weekday][slot] = in.OptionIndex

	return uc.Plans.UpdateMealSelections(ctx, userID, legacy.ID, legacy.MealSelections)
}
