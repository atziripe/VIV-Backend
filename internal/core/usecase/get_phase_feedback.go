package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
)

type SavePhaseFeedbackInput struct {
	UserID                    string
	PlanID                    string
	Phase                     string
	HormonalBriefingResonates bool
}

type SavePhaseFeedbackUseCase struct {
	plans PlanRepository
}

func NewSavePhaseFeedbackUseCase(plans PlanRepository) *SavePhaseFeedbackUseCase {
	return &SavePhaseFeedbackUseCase{plans: plans}
}

func (uc *SavePhaseFeedbackUseCase) Execute(ctx context.Context, in SavePhaseFeedbackInput) error {
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.PlanID) == "" {
		return fmt.Errorf("user_id and plan_id are required")
	}

	plan, err := uc.plans.GetByID(ctx, in.UserID, in.PlanID)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan not found: %s", in.PlanID)
	}

	if plan.PhaseFeedback == nil {
		plan.PhaseFeedback = make(map[string]domain.PhaseFeedbackEntry)
	}

	resonates := in.HormonalBriefingResonates
	plan.PhaseFeedback[in.Phase] = domain.PhaseFeedbackEntry{
		HormonalBriefingResonates: &resonates,
		RespondedAt:               time.Now().UTC().Format(time.RFC3339),
	}

	return uc.plans.UpdatePhaseFeedback(ctx, in.UserID, in.PlanID, plan.PhaseFeedback)
}
