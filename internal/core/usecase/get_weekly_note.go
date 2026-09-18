package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// GET WEEKLY NOTE — the HTTP-facing read that resolves "today's weekly
// note" (VIV-113) for a user: loads the already-generated week covering
// Date, then runs WeeklyNoteUsecase over it. Kept separate from
// WeeklyNoteUsecase itself (which only ever knows how to summarize a
// WeekDraft it's handed) so that usecase's narrow, plan-state-free input
// contract — see weekly_note.go's doc comments — isn't diluted with a
// repository dependency.
// ============================================================================

type GetWeeklyNoteInput struct {
	UserID string
	Date   time.Time // local calendar date, client-sent — same convention as every other date-scoped endpoint here
}

type GetWeeklyNoteOutput struct {
	// Found is false when no generated week covers Date.
	Found bool
	Note  string
}

type GetWeeklyNoteUseCase struct {
	Drafts WeeklyPlanDraftRepository
	Notes  *WeeklyNoteUsecase
}

func NewGetWeeklyNoteUseCase(drafts WeeklyPlanDraftRepository, notes *WeeklyNoteUsecase) *GetWeeklyNoteUseCase {
	return &GetWeeklyNoteUseCase{Drafts: drafts, Notes: notes}
}

func (uc *GetWeeklyNoteUseCase) Execute(ctx context.Context, in GetWeeklyNoteInput) (GetWeeklyNoteOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return GetWeeklyNoteOutput{}, fmt.Errorf("get weekly note: userID is required")
	}
	date := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)

	draft, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return GetWeeklyNoteOutput{}, fmt.Errorf("get weekly note: %w", err)
	}
	if draft == nil {
		return GetWeeklyNoteOutput{}, nil
	}

	note, err := uc.Notes.Execute(ctx, *draft, date)
	if err != nil {
		return GetWeeklyNoteOutput{}, fmt.Errorf("get weekly note: %w", err)
	}

	return GetWeeklyNoteOutput{Found: true, Note: note}, nil
}
