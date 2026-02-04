package usecase

import (
	"context"
	"fmt"
	"log"

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
	if uc.Users == nil || uc.Plans == nil {
		return nil, fmt.Errorf("GetPlanByIDUseCase: missing dependencies")
	}
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

	log.Printf("[plans.getByID] userID=%s planID=%s", user.ID, in.PlanID)
	plan, err := uc.Plans.GetByID(ctx, user.ID, in.PlanID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound(in.PlanID)
	}

	return &GetPlanByIDOutput{Plan: plan}, nil
}
