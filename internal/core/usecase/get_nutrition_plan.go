package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

type GetNutritionPlanInput struct {
	UserID string
	PlanID string // optional — if empty, uses current plan
}

type GetNutritionPlanOutput struct {
	Nutrition domain.NutritionWeekPlan
}

type GetNutritionPlanUseCase struct {
	users    UserRepository
	plans    PlanRepository
	checkins CheckinRepository
	cycle    CyclePhaseLookup
}

func NewGetNutritionPlanUseCase(
	users UserRepository,
	plans PlanRepository,
	checkins CheckinRepository,
	cycle CyclePhaseLookup,
) *GetNutritionPlanUseCase {
	return &GetNutritionPlanUseCase{
		users:    users,
		plans:    plans,
		checkins: checkins,
		cycle:    cycle,
	}
}

func (uc *GetNutritionPlanUseCase) Execute(ctx context.Context, in GetNutritionPlanInput) (*GetNutritionPlanOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}

	// Load user
	user, err := uc.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loading user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	// Load plan
	var plan *domain.Plan
	if strings.TrimSpace(in.PlanID) != "" {
		plan, err = uc.plans.GetByID(ctx, userID, in.PlanID)
	} else {
		plan, err = uc.plans.GetLatest(ctx, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("loading plan: %w", err)
	}
	if plan == nil {
		return nil, fmt.Errorf("no plan found for user: %s", userID)
	}

	// If NutritionJSON exists, use it (includes meals from LLM)
	if len(plan.NutritionJSON) > 0 {
		var nutritionPlan domain.NutritionWeekPlan
		if err := json.Unmarshal(plan.NutritionJSON, &nutritionPlan); err == nil {
			return &GetNutritionPlanOutput{Nutrition: nutritionPlan}, nil
		}
		// If unmarshal fails, fall through to recalculation
	}

	// Parse training plan from the stored JSON
	var trainingPlan domain.TrainingWeekPlan
	if err := json.Unmarshal(plan.TrainingJSON, &trainingPlan); err != nil {
		return nil, fmt.Errorf("parsing training plan: %w", err)
	}

	// Get current phase
	phase, err := uc.cycle.CurrentPhase(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("getting cycle phase: %w", err)
	}

	// Assemble nutrition plan
	nutrition := training.AssembleNutritionPlan(*user, trainingPlan, phase)

	return &GetNutritionPlanOutput{
		Nutrition: nutrition,
	}, nil
}
