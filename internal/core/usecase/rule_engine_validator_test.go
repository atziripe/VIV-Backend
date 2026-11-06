package usecase_test

import (
	"context"
	"strings"
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/usecase"
)

// fakeScheduler is a fake usecase.SchedulingLayer that records how many
// times it was called and with what feedback, and returns a pre-scripted
// response.
type fakeScheduler struct {
	calls        int
	lastFeedback string
	returnDraft  usecase.WeekDraft
	err          error
}

func (f *fakeScheduler) Schedule(_ context.Context, _ usecase.WeekDraft, feedback string) (usecase.WeekDraft, error) {
	f.calls++
	f.lastFeedback = feedback
	if f.err != nil {
		return usecase.WeekDraft{}, f.err
	}
	return f.returnDraft, nil
}

func draftDay(weekday string, isRest bool, activityType activity.ID, intensity activity.IntensityLevel, impact activity.ImpactLevel, mg activity.MuscleGroup) usecase.DayPlan {
	if isRest {
		return usecase.DayPlan{Weekday: weekday, IsRestDay: true}
	}
	return usecase.DayPlan{
		Weekday: weekday,
		Assignment: cascade.SlotAssignment{
			ActivityType: activityType, Intensity: intensity, Impact: impact, MuscleGroup: mg,
		},
	}
}

// validWeekDraft has no spacing violations: Strength@H (RecoveryCost 3,
// needs 2 rest days) on Monday and Thursday (Tue+Wed rest between them),
// no consecutive High-impact days.
func validWeekDraft() usecase.WeekDraft {
	return usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			draftDay("monday", false, activity.Strength, activity.IntensityH, activity.ImpactM, activity.MuscleGroupLower),
			draftDay("tuesday", true, "", "", "", ""),
			draftDay("wednesday", true, "", "", "", ""),
			draftDay("thursday", false, activity.Strength, activity.IntensityH, activity.ImpactM, activity.MuscleGroupUpper),
			draftDay("friday", true, "", "", "", ""),
			draftDay("saturday", false, activity.Yoga, activity.IntensityL, activity.ImpactL, activity.MuscleGroupFullBody),
			draftDay("sunday", true, "", "", "", ""),
		},
	}
}

// invalidWeekDraft has two consecutive High-impact days (Monday, Tuesday).
func invalidWeekDraft() usecase.WeekDraft {
	return usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			draftDay("monday", false, activity.Running, activity.IntensityH, activity.ImpactH, activity.MuscleGroupLower),
			draftDay("tuesday", false, activity.HIIT, activity.IntensityH, activity.ImpactH, activity.MuscleGroupFullBody),
			draftDay("wednesday", true, "", "", "", ""),
			draftDay("thursday", true, "", "", "", ""),
			draftDay("friday", true, "", "", "", ""),
			draftDay("saturday", true, "", "", "", ""),
			draftDay("sunday", true, "", "", "", ""),
		},
	}
}

// ============================================================================
// Valid plan — passes on the first check, no retry
// ============================================================================

func TestRuleEngineValidator_ValidPlanPassesWithNoRetry(t *testing.T) {
	scheduler := &fakeScheduler{}
	v := usecase.NewRuleEngineValidator(scheduler)

	draft := validWeekDraft()
	got, err := v.Validate(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Days != draft.Days {
		t.Errorf("Validate changed a valid draft's Days: %+v", got.Days)
	}
	if scheduler.calls != 0 {
		t.Errorf("scheduler.calls = %d, want 0 — a valid plan must not trigger scheduling at all", scheduler.calls)
	}
}

// ============================================================================
// Invalid plan — exactly one retry, with correctly-constructed feedback
// ============================================================================

func TestRuleEngineValidator_InvalidPlanTriggersExactlyOneRetryWithFeedback(t *testing.T) {
	fixed := validWeekDraft()
	scheduler := &fakeScheduler{returnDraft: fixed}
	v := usecase.NewRuleEngineValidator(scheduler)

	got, err := v.Validate(context.Background(), invalidWeekDraft())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scheduler.calls != 1 {
		t.Fatalf("scheduler.calls = %d, want exactly 1", scheduler.calls)
	}
	if got.Days != fixed.Days {
		t.Errorf("Validate did not return the scheduler's fixed draft: %+v", got.Days)
	}

	feedback := scheduler.lastFeedback
	if feedback == "" {
		t.Fatal("expected non-empty feedback describing the violation")
	}
	if !strings.Contains(feedback, "Monday") || !strings.Contains(feedback, "Tuesday") {
		t.Errorf("feedback should name the specific days involved (Monday, Tuesday), got: %s", feedback)
	}
	if !strings.Contains(feedback, "Impact=High") {
		t.Errorf("feedback should describe the specific rule violated (Impact=High), got: %s", feedback)
	}
}

// ============================================================================
// Still invalid after retry — a hard error, never a silently-accepted bad plan
// ============================================================================

func TestRuleEngineValidator_StillInvalidAfterRetrySurfacesError(t *testing.T) {
	scheduler := &fakeScheduler{returnDraft: invalidWeekDraft()} // "fixed" attempt is still invalid
	v := usecase.NewRuleEngineValidator(scheduler)

	got, err := v.Validate(context.Background(), invalidWeekDraft())
	if err == nil {
		t.Fatal("expected an error when the retried schedule still violates spacing rules")
	}
	if got.Days != (usecase.WeekDraft{}).Days {
		t.Errorf("expected a zero-value WeekDraft on error, got %+v", got)
	}
	if scheduler.calls != 1 {
		t.Errorf("scheduler.calls = %d, want exactly 1 — must not retry more than once", scheduler.calls)
	}
	if !strings.Contains(err.Error(), "Design Overview") {
		t.Errorf("expected the error to reference the undecided fallback policy (Design Overview §11), got: %v", err)
	}
}

// ============================================================================
// Scheduler error propagates
// ============================================================================

func TestRuleEngineValidator_SchedulerErrorPropagates(t *testing.T) {
	scheduler := &fakeScheduler{err: context.DeadlineExceeded}
	v := usecase.NewRuleEngineValidator(scheduler)

	_, err := v.Validate(context.Background(), invalidWeekDraft())
	if err == nil {
		t.Fatal("expected the scheduler's error to propagate")
	}
	if scheduler.calls != 1 {
		t.Errorf("scheduler.calls = %d, want exactly 1", scheduler.calls)
	}
}
