package usecase

import (
	"context"
	"fmt"
	"strings"
)

type SaveMealSelectionInput struct {
	UserID      string
	PlanID      string
	Weekday     string
	MealSlot    string
	OptionIndex int
}

type SaveMealSelectionUseCase struct {
	plans PlanRepository
}

func NewSaveMealSelectionUseCase(plans PlanRepository) *SaveMealSelectionUseCase {
	return &SaveMealSelectionUseCase{plans: plans}
}

func (uc *SaveMealSelectionUseCase) Execute(ctx context.Context, in SaveMealSelectionInput) error {
	weekday := strings.ToLower(strings.TrimSpace(in.Weekday))
	slot := strings.ToLower(strings.TrimSpace(in.MealSlot))

	if weekday == "" || slot == "" {
		return fmt.Errorf("weekday and meal_slot are required")
	}
	if in.OptionIndex < 0 || in.OptionIndex > 2 {
		return fmt.Errorf("option_index must be 0, 1, or 2")
	}

	plan, err := uc.plans.GetByID(ctx, in.UserID, in.PlanID)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan not found: %s", in.PlanID)
	}

	if plan.MealSelections == nil {
		plan.MealSelections = make(map[string]map[string]int)
	}
	if plan.MealSelections[weekday] == nil {
		plan.MealSelections[weekday] = make(map[string]int)
	}

	plan.MealSelections[weekday][slot] = in.OptionIndex

	return uc.plans.UpdateMealSelections(ctx, in.UserID, in.PlanID, plan.MealSelections)
}
