package usecase

import (
	"context"
	"fmt"
	"time"

	"viv/internal/core/goal"
)

// ============================================================================
// WEEKLY NOTE — VIV-113, design doc's §15
// ============================================================================
//
// Explored VIV-102 (goal.GoalProfile.CopyTone) and VIV-106 (WeekDraft, the
// finalized week this reads from) before writing this, plus the existing
// note/copy-generation code (internal/adapters/llm/openai/copy_generator.go
// — mealgen.CopyGenerator): same architectural pattern already established
// there — the LLM call's input type is a small, plain-value DTO
// (mealgen.CopyRequest), never the live solver/plan objects, specifically
// so there's no code path for the LLM's output to feed back into what it
// was given. WeeklyNoteInput below is this task's equivalent of
// CopyRequest.
//
// WeeklyNoteDaySummary and WeeklyNoteInput are deliberately their own
// types, built ONLY of plain values (string/bool) — not WeekDraft,
// DayPlan, or cascade.SlotAssignment, and not pointers to any of them.
// SummarizeWeekForNote is the one and only place a WeekDraft is ever
// converted into this shape; nothing downstream of it (the
// WeeklyNoteGenerator interface, any real LLM implementation) ever sees
// a WeekDraft at all — enforced by the Go type system, not by prompt
// wording. See TestWeeklyNoteGenerator_InputTypeCannotCarryMutablePlanState.
type WeeklyNoteDaySummary struct {
	Weekday   string
	IsRestDay bool

	// Empty/false for a rest day.
	ActivityType    string
	Intensity       string
	Impact          string
	MuscleGroup     string
	Substituted     bool
	AdjustmentLever string
}

// WeeklyNoteInput is the weekly note generator's ENTIRE input — a
// read-only summary of the finalized week plus the goal's CopyTone. This
// is genuinely all a WeeklyNoteGenerator implementation can ever receive;
// it has no way to ask for more, hold a reference to the real plan, or
// write anything back.
type WeeklyNoteInput struct {
	CopyTone     string
	TodayWeekday string // lowercase, e.g. "monday" — which of the 7 Days is "today"
	Days         [7]WeeklyNoteDaySummary
}

// SummarizeWeekForNote builds a WeeklyNoteInput from an already-finalized
// WeekDraft (VIV-106's output, run through Layers 1-3) and today's date.
// Pure extraction: copies values out, never hands out a reference to
// draft itself.
func SummarizeWeekForNote(draft WeekDraft, today time.Time) (WeeklyNoteInput, error) {
	g, ok := goal.ByID(draft.GoalID)
	if !ok {
		return WeeklyNoteInput{}, fmt.Errorf("weekly note: unknown goal id %q", draft.GoalID)
	}

	input := WeeklyNoteInput{CopyTone: g.CopyTone}

	todayFound := false
	for i, d := range draft.Days {
		summary := WeeklyNoteDaySummary{Weekday: d.Weekday, IsRestDay: d.IsRestDay}
		if !d.IsRestDay {
			summary.ActivityType = string(d.Assignment.ActivityType)
			summary.Intensity = string(d.Assignment.Intensity)
			summary.Impact = string(d.Assignment.Impact)
			summary.MuscleGroup = string(d.Assignment.MuscleGroup)
			summary.Substituted = d.Assignment.Substituted
			summary.AdjustmentLever = string(d.Assignment.AdjustmentLever)
		}
		input.Days[i] = summary

		if sameDay(d.Date, today) {
			input.TodayWeekday = d.Weekday
			todayFound = true
		}
	}
	if !todayFound {
		return WeeklyNoteInput{}, fmt.Errorf("weekly note: %s is not part of this week", today.Format("2006-01-02"))
	}

	return input, nil
}

// WeeklyNoteGenerator is the LLM call itself — an interface so a real
// implementation (internal/adapters/llm/openai) can be swapped without
// this package depending on any LLM client. Its signature is the
// structural enforcement the task asks for: it is IMPOSSIBLE to pass a
// WeekDraft, DayPlan, or WeeklyPlanDraftRepository to this method, because
// the type system doesn't allow it — not a matter of an implementation
// choosing not to.
type WeeklyNoteGenerator interface {
	GenerateNote(ctx context.Context, input WeeklyNoteInput) (string, error)
}

// WeeklyNoteUsecase generates today's note for an already-finalized week.
// Deliberately separate from GenerateWeeklyPlanUsecase: the note doesn't
// exist until the week is fully resolved (VIV-105 through VIV-112 have
// all run), and generating it can never influence any of that — see the
// WeeklyNoteGenerator doc comment above.
type WeeklyNoteUsecase struct {
	generator WeeklyNoteGenerator
}

func NewWeeklyNoteUsecase(generator WeeklyNoteGenerator) *WeeklyNoteUsecase {
	return &WeeklyNoteUsecase{generator: generator}
}

func (uc *WeeklyNoteUsecase) Execute(ctx context.Context, draft WeekDraft, today time.Time) (string, error) {
	input, err := SummarizeWeekForNote(draft, today)
	if err != nil {
		return "", err
	}

	note, err := uc.generator.GenerateNote(ctx, input)
	if err != nil {
		return "", fmt.Errorf("weekly note: generating: %w", err)
	}
	return note, nil
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
