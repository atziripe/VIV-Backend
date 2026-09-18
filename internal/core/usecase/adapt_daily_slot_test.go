package usecase_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/content"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// seedDraft builds and saves (via the repo's own public SaveDraft, exactly
// like an already-generated VIV-106 week) a 7-day WeekDraft for weekStart
// (a midnight-UTC date), with each day's Assignment/IsRestDay set from
// days (indexed 0=weekStart..6). Returns the saved draft (with its
// assigned ID).
func seedDraft(t *testing.T, repo *fakeDraftRepo, userID string, weekStart time.Time, g goal.ID, days [7]usecase.DayPlan) usecase.WeekDraft {
	t.Helper()
	for i := range days {
		days[i].Date = weekStart.AddDate(0, 0, i)
	}
	draft := usecase.WeekDraft{
		UserID:    userID,
		StartDate: weekStart,
		EndDate:   weekStart.AddDate(0, 0, 6),
		Status:    "draft",
		GoalID:    g,
		Days:      days,
	}
	if err := repo.SaveDraft(context.Background(), &draft); err != nil {
		t.Fatalf("seedDraft: SaveDraft failed: %v", err)
	}
	return draft
}

func trainingDay(activityType activity.ID, intensity activity.IntensityLevel, impact activity.ImpactLevel, overrode bool) usecase.DayPlan {
	return usecase.DayPlan{
		Assignment: cascade.SlotAssignment{
			ActivityType: activityType,
			Intensity:    intensity,
			Impact:       impact,
			MuscleGroup:  activity.MuscleGroupFullBody,
			UserOverrode: overrode,
		},
	}
}

func restDay() usecase.DayPlan {
	return usecase.DayPlan{IsRestDay: true}
}

// badReadinessCheckin derives to RecoveryLow / BandwidthLow / BuildPullBack.
var badReadinessCheckin = checkin.DailyCheckin{
	Sleep:  checkin.SleepBarelySlept,        // -1
	Body:   checkin.BodySensitiveOrReactive, // -1 => recovery sum -2 => Low
	Demand: checkin.DemandPacked,            // => BandwidthLow
	Need:   checkin.NeedLetMeReset,          // => BuildPullBack
}

func newAdaptUsecase(phase domain.CyclePhase, drafts *fakeDraftRepo) *usecase.AdaptDailySlotUsecase {
	return usecase.NewAdaptDailySlotUsecase(fakePhaseLookup{phase: phase}, drafts)
}

// ============================================================================
// Still fits — no change
// ============================================================================

func TestAdaptDailySlot_StillFitsAfterBadReadiness_NoChange(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // a Monday

	// Bad-readiness target for strength_muscle is (L,L,2) — Strength@(L,L)
	// still fits it: L<=L, L<=L, and TrainingEffect[L]=[mobility], which is
	// one of strength_muscle's supporting priorities.
	existing := trainingDay(activity.Strength, activity.IntensityL, activity.ImpactL, false)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		existing, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	out, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    weekStart,
		Checkin: badReadinessCheckin,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.Changed {
		t.Error("expected Changed=false — the slot still fits")
	}
	if out.Reason != "" {
		t.Errorf("Reason = %q, want empty when nothing changed", out.Reason)
	}
	if out.Suggestion != nil {
		t.Errorf("expected no Suggestion, got %+v", out.Suggestion)
	}
	if out.Assignment != existing.Assignment {
		t.Errorf("Assignment = %+v, want the untouched existing assignment %+v", out.Assignment, existing.Assignment)
	}
}

// ============================================================================
// Non-overridden slot — full cascade
// ============================================================================

func TestAdaptDailySlot_NonOverriddenSlot_GetsFullCascade(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	// Strength@(H,M), not overridden. Bad-readiness target for
	// strength_muscle is (L,L,2). Full cascade: ProtectType (strength IS
	// protected) relaxes intensity H->L, still doesn't satisfy (impact
	// M>L); then RelaxImpact relaxes M->L, satisfies. LoadTier=Reduced
	// because ProtectType fired first.
	existing := trainingDay(activity.Strength, activity.IntensityH, activity.ImpactM, false)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		existing, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	out, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    weekStart,
		Checkin: badReadinessCheckin,
		Catalog: cascade.UserCatalog{Activities: []activity.ID{activity.Strength}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !out.Changed {
		t.Fatal("expected Changed=true")
	}
	if out.Reason == "" {
		t.Error("expected a non-empty Reason")
	}
	if out.Suggestion != nil {
		t.Errorf("expected no Suggestion on the non-overridden path, got %+v", out.Suggestion)
	}
	if out.Assignment.AdjustmentLever != cascade.AdjustmentRelaxImpact {
		t.Errorf("AdjustmentLever = %s, want %s", out.Assignment.AdjustmentLever, cascade.AdjustmentRelaxImpact)
	}
	if out.Assignment.LoadTier != cascade.LoadReduced {
		t.Errorf("LoadTier = %s, want %s (ProtectType should have fired first)", out.Assignment.LoadTier, cascade.LoadReduced)
	}
	if out.Assignment.Intensity != activity.IntensityL || out.Assignment.Impact != activity.ImpactL {
		t.Errorf("result = (%s,%s), want (L,L)", out.Assignment.Intensity, out.Assignment.Impact)
	}

	// And it must have actually been persisted.
	stored, err := drafts.GetByDate(context.Background(), "u1", weekStart)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if stored.Days[0].Assignment != out.Assignment {
		t.Errorf("persisted assignment = %+v, want %+v", stored.Days[0].Assignment, out.Assignment)
	}
}

// ============================================================================
// Overridden slot — only a safety suggestion, never a silent overwrite
// ============================================================================

func TestAdaptDailySlot_OverriddenSlot_OnlySuggestsRelaxLevers(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	// Same shape as the non-overridden test above, but UserOverrode=true.
	existing := trainingDay(activity.Strength, activity.IntensityH, activity.ImpactM, true)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		existing, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	out, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    weekStart,
		Checkin: badReadinessCheckin,
		Catalog: cascade.UserCatalog{Activities: []activity.ID{activity.Strength}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	// The stored assignment must be completely untouched.
	if out.Changed {
		t.Error("expected Changed=false — a user override is never silently overwritten")
	}
	if out.Assignment != existing.Assignment {
		t.Errorf("Assignment = %+v, want the untouched user-overridden assignment %+v", out.Assignment, existing.Assignment)
	}

	stored, err := drafts.GetByDate(context.Background(), "u1", weekStart)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if stored.Days[0].Assignment != existing.Assignment {
		t.Errorf("persisted assignment changed: %+v, want untouched %+v", stored.Days[0].Assignment, existing.Assignment)
	}

	// But a safety suggestion must be offered — RelaxImpact only (not
	// ProtectType, which would otherwise fire first and isn't safety-relevant).
	if out.Suggestion == nil {
		t.Fatal("expected a Suggestion")
	}
	if out.Suggestion.Reason == "" {
		t.Error("expected a non-empty Reason on the suggestion")
	}
	lever := out.Suggestion.Assignment.AdjustmentLever
	if lever != cascade.AdjustmentRelaxIntensity && lever != cascade.AdjustmentRelaxImpact {
		t.Errorf("Suggestion's AdjustmentLever = %s, want RelaxIntensity or RelaxImpact only", lever)
	}
	if lever == cascade.AdjustmentProtectType || lever == cascade.AdjustmentReduceDuration ||
		lever == cascade.AdjustmentSimplifyComplexity || lever == cascade.AdjustmentSubstituteType {
		t.Errorf("Suggestion's AdjustmentLever = %s — this lever must never fire over a user override", lever)
	}
	if out.Suggestion.Assignment.LoadTier != cascade.LoadStandard {
		t.Errorf("Suggestion's LoadTier = %s, want %s (ProtectType must not have run)", out.Suggestion.Assignment.LoadTier, cascade.LoadStandard)
	}
}

// TestAdaptDailySlot_OverriddenSlot_NoSuggestionWhenRelaxIsInsufficient
// proves the exclusion is real, not just "relax happened to be enough":
// HIIT's fixed intensity means relax alone can never resolve this
// conflict, even though the full (ungated) cascade WOULD resolve it via
// SubstituteType. An overridden slot must get no suggestion at all here,
// never one that reaches past RelaxIntensity/RelaxImpact.
func TestAdaptDailySlot_OverriddenSlot_NoSuggestionWhenRelaxIsInsufficient(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	existing := trainingDay(activity.HIIT, activity.IntensityH, activity.ImpactH, true)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		existing, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	out, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    weekStart,
		Checkin: badReadinessCheckin,
		Catalog: cascade.UserCatalog{Activities: []activity.ID{activity.HIIT, activity.Strength}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.Changed {
		t.Error("expected Changed=false")
	}
	if out.Suggestion != nil {
		t.Errorf("expected no Suggestion (relax alone can't resolve a fixed-intensity HIIT conflict), got %+v", out.Suggestion)
	}
	if out.Assignment != existing.Assignment {
		t.Errorf("Assignment = %+v, want untouched %+v", out.Assignment, existing.Assignment)
	}
}

// ============================================================================
// Rest day — no-op
// ============================================================================

func TestAdaptDailySlot_RestDay_NoOp(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	out, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    weekStart,
		Checkin: badReadinessCheckin,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Changed || out.Suggestion != nil {
		t.Errorf("expected a pure no-op for a rest day, got %+v", out)
	}
}

// ============================================================================
// Isolation — adapting one day never touches any other day
// ============================================================================

func TestAdaptDailySlot_OnlyTouchesGivenDate(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday

	monday := trainingDay(activity.Running, activity.IntensityM, activity.ImpactH, false)
	tuesday := trainingDay(activity.Strength, activity.IntensityH, activity.ImpactM, false) // will change
	wednesday := trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, true)    // user-overridden, untouched either way

	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		monday, tuesday, wednesday, restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	tuesdayDate := weekStart.AddDate(0, 0, 1)
	out, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    tuesdayDate,
		Checkin: badReadinessCheckin,
		Catalog: cascade.UserCatalog{Activities: []activity.ID{activity.Strength}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Changed {
		t.Fatal("expected Tuesday's slot to change (sanity check for this test)")
	}

	stored, err := drafts.GetByDate(context.Background(), "u1", weekStart)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}

	if !reflect.DeepEqual(stored.Days[0], usecase.DayPlan{Date: weekStart, Assignment: monday.Assignment}) {
		t.Errorf("Monday changed: %+v", stored.Days[0])
	}
	if !reflect.DeepEqual(stored.Days[2], usecase.DayPlan{Date: weekStart.AddDate(0, 0, 2), Assignment: wednesday.Assignment}) {
		t.Errorf("Wednesday changed: %+v", stored.Days[2])
	}
	for i := 3; i < 7; i++ {
		if !stored.Days[i].IsRestDay {
			t.Errorf("day %d changed: %+v", i, stored.Days[i])
		}
	}

	if stored.Days[1].Assignment == tuesday.Assignment {
		t.Error("expected Tuesday's assignment to actually have changed")
	}
	if stored.Days[1].Assignment != out.Assignment {
		t.Errorf("persisted Tuesday assignment = %+v, want %+v", stored.Days[1].Assignment, out.Assignment)
	}
}

// ============================================================================
// Errors
// ============================================================================

func TestAdaptDailySlot_NoGeneratedWeekErrors(t *testing.T) {
	uc := newAdaptUsecase(domain.PhaseFollicular, &fakeDraftRepo{})

	_, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "ghost",
		Date:    time.Now(),
		Checkin: badReadinessCheckin,
	})
	if err == nil {
		t.Fatal("expected an error when no week has been generated for this user")
	}
}

func TestAdaptDailySlot_DateOutsideWeekErrors(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newAdaptUsecase(domain.PhaseFollicular, drafts)
	_, err := uc.Execute(context.Background(), usecase.AdaptDailySlotInput{
		UserID:  "u1",
		Date:    weekStart.AddDate(0, 0, 30),
		Checkin: badReadinessCheckin,
	})
	if err == nil {
		t.Fatal("expected an error for a date outside the loaded week")
	}
}

// ============================================================================
// UserEditSlotUsecase
// ============================================================================

func TestUserEditSlot_SetsUserOverrodeTrue(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		trainingDay(activity.Strength, activity.IntensityM, activity.ImpactM, false),
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	newAssignment := cascade.SlotAssignment{
		ActivityType: activity.Yoga,
		Intensity:    activity.IntensityL,
		Impact:       activity.ImpactL,
		MuscleGroup:  activity.MuscleGroupFullBody,
		UserOverrode: false, // deliberately false — Execute must force it to true anyway
	}

	got, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID:        "u1",
		Date:          weekStart,
		NewAssignment: newAssignment,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !got.Assignment.UserOverrode {
		t.Error("expected UserOverrode=true regardless of the input value")
	}
	if got.Assignment.ActivityType != activity.Yoga {
		t.Errorf("ActivityType = %s, want %s", got.Assignment.ActivityType, activity.Yoga)
	}

	stored, err := drafts.GetByDate(context.Background(), "u1", weekStart)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if !stored.Days[0].Assignment.UserOverrode {
		t.Error("persisted assignment does not have UserOverrode set")
	}
}

func TestUserEditSlot_DoesNotTouchOtherDays(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	monday := trainingDay(activity.Running, activity.IntensityM, activity.ImpactH, false)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		monday, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	_, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   weekStart.AddDate(0, 0, 3), // Thursday
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.Pilates, Intensity: activity.IntensityL, Impact: activity.ImpactL,
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	stored, err := drafts.GetByDate(context.Background(), "u1", weekStart)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if !reflect.DeepEqual(stored.Days[0], usecase.DayPlan{Date: weekStart, Assignment: monday.Assignment}) {
		t.Errorf("Monday changed: %+v", stored.Days[0])
	}
	if stored.Days[3].Assignment.ActivityType != activity.Pilates {
		t.Errorf("Thursday's edit didn't apply: %+v", stored.Days[3])
	}
	for _, i := range []int{1, 2, 4, 5, 6} {
		if !stored.Days[i].IsRestDay {
			t.Errorf("day %d changed: %+v", i, stored.Days[i])
		}
	}
}

func TestUserEditSlot_ClearingActivityTypeMarksRestDay(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		trainingDay(activity.Strength, activity.IntensityM, activity.ImpactM, false),
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	got, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID:        "u1",
		Date:          weekStart,
		NewAssignment: cascade.SlotAssignment{}, // empty ActivityType
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !got.IsRestDay {
		t.Error("expected clearing the activity type to mark the day as a rest day")
	}
}

// fakeContentSelector is a usecase.ContentSelectionLayer fake that stamps a
// fixed, recognizable SelectedContent onto every non-rest day it sees —
// used to verify UserEditSlotUsecase actually re-selects content for the
// edited day (rather than leaving whatever content the day's PREVIOUS
// assignment had), and clears it for a day edited into a rest day.
type fakeContentSelector struct {
	content usecase.SelectedContent
	calls   int
}

func (f *fakeContentSelector) SelectContent(_ context.Context, draft usecase.WeekDraft) (usecase.WeekDraft, error) {
	f.calls++
	for i := range draft.Days {
		if draft.Days[i].IsRestDay {
			continue
		}
		c := f.content
		draft.Days[i].Content = &c
	}
	return draft, nil
}

func TestUserEditSlot_ReselectsContentForTheEditedDay(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	monday := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactM, false)
	staleContent := usecase.SelectedContent{Session: &content.Session{DurationMinutes: 99}}
	monday.Content = &staleContent
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		monday, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	selector := &fakeContentSelector{content: usecase.SelectedContent{Session: &content.Session{DurationMinutes: 20}}}
	uc := usecase.NewUserEditSlotUsecase(drafts, selector)

	got, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   weekStart,
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL,
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if selector.calls != 1 {
		t.Fatalf("content selector calls = %d, want 1", selector.calls)
	}
	if got.Content == nil || got.Content.Session == nil || got.Content.Session.DurationMinutes != 20 {
		t.Errorf("Content = %+v, want the re-selected content (duration 20), not the stale Strength content (duration 99)", got.Content)
	}
}

func TestUserEditSlot_ClearingToRestDayClearsContentWithoutReselecting(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	monday := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactM, false)
	staleContent := usecase.SelectedContent{Session: &content.Session{DurationMinutes: 99}}
	monday.Content = &staleContent
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle, [7]usecase.DayPlan{
		monday, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	selector := &fakeContentSelector{}
	uc := usecase.NewUserEditSlotUsecase(drafts, selector)

	got, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID:        "u1",
		Date:          weekStart,
		NewAssignment: cascade.SlotAssignment{}, // empty ActivityType -> rest day
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if selector.calls != 0 {
		t.Errorf("content selector calls = %d, want 0 (a rest day never needs content)", selector.calls)
	}
	if got.Content != nil {
		t.Errorf("Content = %+v, want nil for a day cleared to rest", got.Content)
	}
}
