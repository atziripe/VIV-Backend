package usecase

import (
	"context"

	"viv/internal/core/domain"
)

type GetPlanByIDInput struct {
	UserID string
	PlanID string
}

type GetPlanByIDOutput struct {
	Plan *domain.Plan
}

type GetPlanByIDUseCase struct {
	Users UserRepository
	Plans PlanRepository
}

func NewGetPlanByIDUseCase(users UserRepository, plans PlanRepository) *GetPlanByIDUseCase {
	return &GetPlanByIDUseCase{
		Users: users,
		Plans: plans,
	}
}

func (uc *GetPlanByIDUseCase) Execute(ctx context.Context, in GetPlanByIDInput) (*GetPlanByIDOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("")
	}

	if in.PlanID == "" {
		return nil, ErrPlanNotFound("")
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	plan, err := uc.Plans.GetByID(ctx, user.ID, in.PlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound(*user.LastActivePlanID)
	}

	return &GetPlanByIDOutput{Plan: plan}, nil
}
