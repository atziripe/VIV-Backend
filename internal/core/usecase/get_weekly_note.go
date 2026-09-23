package usecase

import (
	"context"
	"fmt"
	"log"
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

	dateKey := date.Format("2006-01-02")
	if cached, ok := draft.Notes[dateKey]; ok {
		return GetWeeklyNoteOutput{Found: true, Note: cached}, nil
	}

	note, err := uc.Notes.Execute(ctx, *draft, date)
	if err != nil {
		return GetWeeklyNoteOutput{}, fmt.Errorf("get weekly note: %w", err)
	}

	// Best-effort: the note was already generated successfully, so a
	// failure to cache it must never turn into a failed response — it
	// just means the next request pays for another LLM call.
	if err := uc.Drafts.SetNote(ctx, userID, draft.ID, dateKey, note); err != nil {
		log.Printf("[weeklynote] failed to cache note user=%s draft=%s date=%s: %v", userID, draft.ID, dateKey, err)
	}

	return GetWeeklyNoteOutput{Found: true, Note: note}, nil
}
