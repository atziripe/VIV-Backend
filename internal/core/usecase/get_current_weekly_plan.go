package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// GET CURRENT WEEKLY PLAN — reads the already-generated week (VIV-106) a
// date falls in, for a "This week" list view. Read-only: never generates
// or adapts anything itself (see GenerateWeeklyPlanUsecase/
// AdaptDailySlotUsecase for that) — it just answers "what does the
// already-resolved week look like".
// ============================================================================

type GetCurrentWeeklyPlanInput struct {
	UserID string
	Date   time.Time // any date within the week to fetch — same "client sends local date" convention as SubmitDailyCheckinInput
}

type GetCurrentWeeklyPlanOutput struct {
	// Draft is nil when no generated week covers Date — e.g. between a
	// week rolling over and the next check-in generating a new one.
	Draft *WeekDraft
}

type GetCurrentWeeklyPlanUseCase struct {
	Drafts WeeklyPlanDraftRepository
}

func NewGetCurrentWeeklyPlanUseCase(drafts WeeklyPlanDraftRepository) *GetCurrentWeeklyPlanUseCase {
	return &GetCurrentWeeklyPlanUseCase{Drafts: drafts}
}

func (uc *GetCurrentWeeklyPlanUseCase) Execute(ctx context.Context, in GetCurrentWeeklyPlanInput) (GetCurrentWeeklyPlanOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return GetCurrentWeeklyPlanOutput{}, fmt.Errorf("get current weekly plan: userID is required")
	}

	draft, err := uc.Drafts.GetByDate(ctx, userID, in.Date)
	if err != nil {
		return GetCurrentWeeklyPlanOutput{}, fmt.Errorf("get current weekly plan: %w", err)
	}
	return GetCurrentWeeklyPlanOutput{Draft: draft}, nil
}
