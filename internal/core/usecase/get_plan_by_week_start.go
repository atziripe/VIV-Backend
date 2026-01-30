package usecase

import (
	"context"
	"time"

	"viv/internal/core/domain"
)

type GetPlanByWeekStartInput struct {
	UserID    string
	WeekStart time.Time
}

type GetPlanByWeekStartOutput struct {
	Plan *domain.Plan
}

type GetPlanByWeekStartUseCase struct {
	planRepo PlanRepository
}

func NewGetPlanByWeekStartUseCase(planRepo PlanRepository) *GetPlanByWeekStartUseCase {
	return &GetPlanByWeekStartUseCase{planRepo: planRepo}
}

func (uc *GetPlanByWeekStartUseCase) Execute(ctx context.Context, in GetPlanByWeekStartInput) (*GetPlanByWeekStartOutput, error) {
	plan, err := uc.planRepo.GetLatestByWeekStart(ctx, in.UserID, in.WeekStart)
	if err != nil {
		return nil, err
	}
	return &GetPlanByWeekStartOutput{Plan: plan}, nil
}
