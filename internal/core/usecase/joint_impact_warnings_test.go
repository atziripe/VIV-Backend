package usecase_test

import (
	"context"
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/usecase"
)

// ============================================================================
// Joint-impact warning
// ============================================================================

func TestJointImpactWarningsLayer_HighImpactGetsWarning(t *testing.T) {
	layer := usecase.NewJointImpactWarningsLayer()
	draft := usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			{Assignment: cascade.SlotAssignment{ActivityType: activity.Running, Intensity: activity.IntensityM, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupLower}},
		},
	}

	got, err := layer.Apply(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Days[0].Warning == "" {
		t.Error("expected a non-empty Warning for a High-impact slot")
	}
}

func TestJointImpactWarningsLayer_LowImpactGetsNoWarning(t *testing.T) {
	layer := usecase.NewJointImpactWarningsLayer()
	draft := usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			{Assignment: cascade.SlotAssignment{ActivityType: activity.Cycling, Intensity: activity.IntensityM, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupLower}},
		},
	}

	got, err := layer.Apply(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Days[0].Warning != "" {
		t.Errorf("expected no Warning for a Low-impact slot, got %q", got.Days[0].Warning)
	}
}

func TestJointImpactWarningsLayer_MediumImpactMatchesCurrentThreshold(t *testing.T) {
	// Documents the current, TODO-flagged threshold: Medium impact does
	// NOT trigger a warning today. If JointImpactWarningThreshold[Medium]
	// is ever flipped to true (once product/clinical confirms it), this
	// test should be updated alongside that change.
	layer := usecase.NewJointImpactWarningsLayer()
	draft := usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			{Assignment: cascade.SlotAssignment{ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM, MuscleGroup: activity.MuscleGroupLower}},
		},
	}

	got, err := layer.Apply(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantWarning := usecase.JointImpactWarningThreshold[activity.ImpactM]
	gotWarning := got.Days[0].Warning != ""
	if gotWarning != wantWarning {
		t.Errorf("Medium-impact Warning present = %v, want %v (per JointImpactWarningThreshold)", gotWarning, wantWarning)
	}
}

func TestJointImpactWarningsLayer_RestDaysNeverGetAWarning(t *testing.T) {
	layer := usecase.NewJointImpactWarningsLayer()
	draft := usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			{IsRestDay: true},
		},
	}

	got, err := layer.Apply(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Days[0].Warning != "" {
		t.Errorf("expected no Warning for a rest day, got %q", got.Days[0].Warning)
	}
}

// ============================================================================
// Load override survives unchanged (LoadTier), and nothing else about the
// slot is ever touched — the "verify and test" half of this task.
// ============================================================================

func TestJointImpactWarningsLayer_LoadTierSurvivesUnchanged(t *testing.T) {
	layer := usecase.NewJointImpactWarningsLayer()

	// Simulates VIV-105's cascade output when ProtectType fired.
	original := cascade.SlotAssignment{
		ActivityType:    activity.Strength,
		Intensity:       activity.IntensityL,
		Impact:          activity.ImpactH,
		MuscleGroup:     activity.MuscleGroupLower,
		LoadTier:        cascade.LoadReduced,
		AdjustmentLever: cascade.AdjustmentProtectType,
	}
	draft := usecase.WeekDraft{
		Days: [7]usecase.DayPlan{
			{Weekday: "monday", Assignment: original},
		},
	}

	got, err := layer.Apply(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Days[0].Assignment != original {
		t.Errorf("Assignment changed: got %+v, want unchanged %+v", got.Days[0].Assignment, original)
	}
	if got.Days[0].Assignment.LoadTier != cascade.LoadReduced {
		t.Errorf("LoadTier = %s, want %s to survive from VIV-105's output", got.Days[0].Assignment.LoadTier, cascade.LoadReduced)
	}
}

// TestJointImpactWarningsLayer_NeverChangesResolvedAttributesOrDayPlacement
// is the broader "purely additive" guarantee: across a full week with a
// mix of impacts, the only field this layer is ever allowed to touch is
// DayPlan.Warning.
func TestJointImpactWarningsLayer_NeverChangesResolvedAttributesOrDayPlacement(t *testing.T) {
	layer := usecase.NewJointImpactWarningsLayer()

	before := usecase.WeekDraft{
		UserID: "u1",
		Days: [7]usecase.DayPlan{
			{Weekday: "monday", Assignment: cascade.SlotAssignment{ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupLower, LoadTier: cascade.LoadReduced}},
			{Weekday: "tuesday", IsRestDay: true},
			{Weekday: "wednesday", Assignment: cascade.SlotAssignment{ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody}},
			{Weekday: "thursday", IsRestDay: true},
			{Weekday: "friday", Assignment: cascade.SlotAssignment{ActivityType: activity.Running, Intensity: activity.IntensityM, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupLower, Substituted: true, OriginalType: activity.HIIT}},
			{Weekday: "saturday", IsRestDay: true},
			{Weekday: "sunday", IsRestDay: true},
		},
	}

	got, err := layer.Apply(context.Background(), before)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := range before.Days {
		wantDay := before.Days[i]
		gotDay := got.Days[i]

		if gotDay.Date != wantDay.Date || gotDay.Weekday != wantDay.Weekday || gotDay.IsRestDay != wantDay.IsRestDay {
			t.Errorf("day %d: day placement changed: got %+v, want %+v", i, gotDay, wantDay)
		}
		if gotDay.Assignment != wantDay.Assignment {
			t.Errorf("day %d: Assignment changed: got %+v, want %+v", i, gotDay.Assignment, wantDay.Assignment)
		}
	}

	if got.UserID != before.UserID {
		t.Errorf("UserID changed: got %q, want %q", got.UserID, before.UserID)
	}

	// Sanity: the High-impact days DID get a warning — proves the layer
	// actually ran, not that Apply is a no-op.
	if got.Days[0].Warning == "" {
		t.Error("expected monday (High impact) to get a Warning")
	}
	if got.Days[4].Warning == "" {
		t.Error("expected friday (High impact) to get a Warning")
	}
	if got.Days[2].Warning != "" {
		t.Error("expected wednesday (Low impact) to get no Warning")
	}
}
