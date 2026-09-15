package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/cascade"
)

// UserEditSlotUsecase applies a user's manual edit to a single day's slot
// in an already-generated week (VIV-106). This is the upstream entry
// point AdaptDailySlotUsecase (VIV-107) depends on: it's what sets
// SlotAssignment.UserOverrode, the flag that tells daily adaptation "a
// human changed this by hand — only offer a safety suggestion here, never
// silently overwrite it" (design doc §9).
type UserEditSlotUsecase struct {
	drafts WeeklyPlanDraftRepository
}

func NewUserEditSlotUsecase(drafts WeeklyPlanDraftRepository) *UserEditSlotUsecase {
	return &UserEditSlotUsecase{drafts: drafts}
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
// This only ever touches the single day for Date — every other day in the
// week is left exactly as it was, both in memory and in storage (see
// WeeklyPlanDraftRepository.UpdateDaySlot).
func (uc *UserEditSlotUsecase) Execute(ctx context.Context, in UserEditSlotInput) (DayPlan, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return DayPlan{}, fmt.Errorf("user edit slot: userID is required")
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

	day := draft.Days[idx]
	day.Assignment = newAssignment
	day.IsRestDay = newAssignment.ActivityType == ""

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
