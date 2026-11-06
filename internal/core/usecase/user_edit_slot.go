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
		t, ok := activity.ByID(in.NewAssignment.ActivityType)
		if !ok {
			return DayPlan{}, fmt.Errorf("user edit slot: unknown activity id %q", in.NewAssignment.ActivityType)
		}
		if err := validateAssignmentAgainstTaxonomy(in.NewAssignment, t); err != nil {
			return DayPlan{}, fmt.Errorf("user edit slot: %w", err)
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

// validateAssignmentAgainstTaxonomy rejects a manual edit whose
// Intensity/Impact/MuscleGroup isn't a combination t's own taxonomy
// entry (activity.Type) actually allows — a syntactically valid-looking
// request (e.g. activity_type=hiit, intensity=L, when HIIT's
// IntensityRange is fixed to [H]) would otherwise sail through this
// usecase's only prior check (that the activity ID itself is known) and
// fail later, opaquely, inside content selection's library lookup —
// exactly the failure shape that surfaced the content.SessionLibrary
// gap for Running/Cycling/Swimming at LoadTier=Reduced.
//
// MuscleGroup is checked against DefaultMuscleGroup even for a
// UserSelectable type (HIIT, Functional): nothing in this codebase yet
// lets a user actually choose a non-default muscle group for either —
// GenerateWeeklyPlanUsecase always proposes DefaultMuscleGroup
// (generate_weekly_plan.go), and AdaptDailySlotUsecase only ever carries
// forward whatever was already resolved — and content.SessionLibrary has
// zero session content for HIIT/Functional at lower/upper/core. Relaxing
// this check to allow the other 3 groups is a real next-iteration task
// (building the actual muscle-group selection UI/flow plus its content),
// not something to allow silently ahead of that content existing.
func validateAssignmentAgainstTaxonomy(a cascade.SlotAssignment, t activity.Type) error {
	intensityOK := false
	for _, level := range t.IntensityRange {
		if level == a.Intensity {
			intensityOK = true
			break
		}
	}
	if !intensityOK {
		return fmt.Errorf("intensity %q is not valid for activity %q (allowed: %v)", a.Intensity, t.ID, t.IntensityRange)
	}

	impactOK := false
	for _, level := range t.ImpactRange {
		if level == a.Impact {
			impactOK = true
			break
		}
	}
	if !impactOK {
		return fmt.Errorf("impact %q is not valid for activity %q (allowed: %v)", a.Impact, t.ID, t.ImpactRange)
	}

	if a.MuscleGroup != t.DefaultMuscleGroup {
		return fmt.Errorf("muscle group %q is not valid for activity %q (only %q is supported today)", a.MuscleGroup, t.ID, t.DefaultMuscleGroup)
	}

	return nil
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
