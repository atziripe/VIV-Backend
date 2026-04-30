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
	Plan               *domain.Plan
	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int
}

type GetPlanByWeekStartUseCase struct {
	planRepo PlanRepository
	userRepo UserRepository
}

func NewGetPlanByWeekStartUseCase(planRepo PlanRepository) *GetPlanByWeekStartUseCase {
	return &GetPlanByWeekStartUseCase{planRepo: planRepo}
}

func (uc *GetPlanByWeekStartUseCase) Execute(ctx context.Context, in GetPlanByWeekStartInput) (*GetPlanByWeekStartOutput, error) {
	plan, err := uc.planRepo.GetLatestByWeekStart(ctx, in.UserID, in.WeekStart)
	if err != nil {
		return nil, err
	}

	user, err := uc.userRepo.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	phase := mapCyclePhase(user.CyclePhase)
	nextPhase := NextPhaseName(phase)
	daysRemaining := DaysUntilNextPhase(user)

	return &GetPlanByWeekStartOutput{
		Plan:               plan,
		CurrentPhase:       string(phase),
		NextPhase:          nextPhase,
		DaysUntilNextPhase: daysRemaining,
	}, nil
}
