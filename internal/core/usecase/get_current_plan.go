package usecase

import (
	"context"
	"strings"

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
	if strings.TrimSpace(in.UserID) == "" {
		return nil, ErrUserNotFound("")
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	// 1) Prefer explicit pointer to the active plan.
	if user.LastActivePlanID != nil {
		id := strings.TrimSpace(*user.LastActivePlanID)
		if id != "" {
			plan, err := uc.Plans.GetByID(ctx, user.ID, id)
			if err != nil {
				return nil, err
			}
			if plan != nil {
				return &GetCurrentPlanOutput{Plan: plan}, nil
			}
			// If the pointer is stale, fall through to latest.
		}
	}

	// 2) Fallback: return the most recently created plan for this user.
	latest, err := uc.Plans.GetLatest(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, ErrNoActivePlan(in.UserID)
	}

	// 3) Repair: persist pointer so future calls are fast and consistent.
	user.LastActivePlanID = &latest.ID
	_ = uc.Users.Save(ctx, user) // best-effort repair; do not fail the request

	return &GetCurrentPlanOutput{Plan: latest}, nil
}
