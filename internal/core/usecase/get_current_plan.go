package usecase

import (
	"context"

	"viv/internal/core/domain"
)

type GetCurrentPlanInput struct {
	UserID string
}

type GetCurrentPlanOutput struct {
	Plan *domain.Plan
}

type GetCurrentPlanUseCase struct {
	Users UserRepository
	Plans PlanRepository
}

func NewGetCurrentPlanUseCase(users UserRepository, plans PlanRepository) *GetCurrentPlanUseCase {
	return &GetCurrentPlanUseCase{
		Users: users,
		Plans: plans,
	}
}

func (uc *GetCurrentPlanUseCase) Execute(ctx context.Context, in GetCurrentPlanInput) (*GetCurrentPlanOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("")
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	if user.LastActivePlanID == nil || *user.LastActivePlanID == "" {
		return nil, ErrNoActivePlan(in.UserID)
	}

	plan, err := uc.Plans.GetByID(ctx, user.ID, *user.LastActivePlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound(*user.LastActivePlanID)
	}

	return &GetCurrentPlanOutput{Plan: plan}, nil
}
