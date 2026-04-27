package usecase

import (
	"context"
	"fmt"
	"strings"
)

var validWeekdays = map[string]bool{
	"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
	"friday": true, "saturday": true, "sunday": true,
}

type CompleteTrainingDayInput struct {
	UserID  string
	PlanID  string
	Weekday string
}

type CompleteTrainingDayUseCase struct {
	plans PlanRepository
}

func NewCompleteTrainingDayUseCase(plans PlanRepository) *CompleteTrainingDayUseCase {
	return &CompleteTrainingDayUseCase{plans: plans}
}

func (uc *CompleteTrainingDayUseCase) Execute(ctx context.Context, in CompleteTrainingDayInput) error {
	weekday := strings.ToLower(strings.TrimSpace(in.Weekday))
	if !validWeekdays[weekday] {
		return fmt.Errorf("invalid weekday: %q", in.Weekday)
	}

	plan, err := uc.plans.GetByID(ctx, in.UserID, in.PlanID)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan not found: %s", in.PlanID)
	}

	if plan.TrainingCompleted == nil {
		plan.TrainingCompleted = make(map[string]bool)
	}
	plan.TrainingCompleted[weekday] = true

	return uc.plans.UpdateTrainingCompleted(ctx, in.UserID, in.PlanID, plan.TrainingCompleted)
}
