package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
)

// UserEditSlotUsecase applies a user's manual edit to a single day's slot
// in an already-generated week (VIV-106). This is the upstream entry
// point AdaptDailySlotUsecase (VIV-107) depends on: it's what sets
// SlotAssignment.UserOverrode, the flag that tells daily adaptation "a
// human changed this by hand — only offer a safety suggestion here, never
// silently overwrite it" (design doc §9).
type UserEditSlotUsecase struct {
	drafts  WeeklyPlanDraftRepository
	content ContentSelectionLayer
}

func NewUserEditSlotUsecase(drafts WeeklyPlanDraftRepository, content ContentSelectionLayer) *UserEditSlotUsecase {
	return &UserEditSlotUsecase{drafts: drafts, content: content}
}

type UserEditSlotInput struct {
	UserID        string
	Date          time.Time
	NewAssignment cascade.SlotAssignment
}

// Execute stores NewAssignment as the given date's slot, forcing
// UserOverrode = true regardless of what the caller set it to — the
// entire point of this entry point is "a person just edited this slot by
// hand", so that flag isn't the caller's to opt out of.
//
// Whether the resulting day is a rest day is inferred from
// NewAssignment.ActivityType being empty — an edit that clears the
// activity type is read as "make this day a rest day."
//
// The edited day's Content (VIV-112's hydrated session/exercises) is
// re-selected against the new Assignment before saving — otherwise a day
// edited from, say, Strength to Yoga would keep showing Strength's
// exercises. Content selection runs over the whole in-memory draft
// (ContentSelectionLayer's only shape) but only the edited day is ever
// persisted, via WeeklyPlanDraftRepository.UpdateDaySlot — every other
// day in the week is left exactly as it was, both in memory and in
// storage. Warning (VIV-111's joint-impact annotation) is cleared rather
// than recomputed: it was computed for the old assignment and would be
// misleading left as-is, but recomputing it needs the warnings/overrides
// layer this usecase doesn't otherwise depend on.
func (uc *UserEditSlotUsecase) Execute(ctx context.Context, in UserEditSlotInput) (DayPlan, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return DayPlan{}, fmt.Errorf("user edit slot: userID is required")
	}
	if in.NewAssignment.ActivityType != "" {
		if _, ok := activity.ByID(in.NewAssignment.ActivityType); !ok {
			return DayPlan{}, fmt.Errorf("user edit slot: unknown activity id %q", in.NewAssignment.ActivityType)
		}
	}

	draft, err := uc.drafts.GetByDate(ctx, userID, in.Date)
	if err != nil {
		return DayPlan{}, fmt.Errorf("user edit slot: loading week: %w", err)
	}
	if draft == nil {
		return DayPlan{}, fmt.Errorf("user edit slot: no generated week covers %s", in.Date.Format("2006-01-02"))
	}

	idx, ok := dayIndexForDate(*draft, in.Date)
	if !ok {
		return DayPlan{}, fmt.Errorf("user edit slot: %s is not part of the loaded week", in.Date.Format("2006-01-02"))
	}

	newAssignment := in.NewAssignment
	newAssignment.UserOverrode = true

	draft.Days[idx].Assignment = newAssignment
	draft.Days[idx].IsRestDay = newAssignment.ActivityType == ""
	draft.Days[idx].Warning = ""

	var day DayPlan
	if draft.Days[idx].IsRestDay {
		draft.Days[idx].Content = nil
		day = draft.Days[idx]
	} else {
		reselected, err := uc.content.SelectContent(ctx, *draft)
		if err != nil {
			return DayPlan{}, fmt.Errorf("user edit slot: selecting content: %w", err)
		}
		day = reselected.Days[idx]
	}

	if err := uc.drafts.UpdateDaySlot(ctx, userID, draft.ID, idx, day); err != nil {
		return DayPlan{}, fmt.Errorf("user edit slot: saving edit: %w", err)
	}

	return day, nil
}

// dayIndexForDate finds which of a WeekDraft's 7 days matches date (by
// calendar day, ignoring time-of-day) — shared by UserEditSlotUsecase and
// AdaptDailySlotUsecase.
func dayIndexForDate(draft WeekDraft, date time.Time) (int, bool) {
	for i, day := range draft.Days {
		if sameDay(day.Date, date) {
			return i, true
		}
	}
	return -1, false
}
