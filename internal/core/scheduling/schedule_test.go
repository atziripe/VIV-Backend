package scheduling_test

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/scheduling"
)

func slot(activityType activity.ID, intensity activity.IntensityLevel, impact activity.ImpactLevel, mg activity.MuscleGroup) cascade.SlotAssignment {
	return cascade.SlotAssignment{ActivityType: activityType, Intensity: intensity, Impact: impact, MuscleGroup: mg}
}

func placementFor(idx int, weekday string, a cascade.SlotAssignment) scheduling.Placement {
	return scheduling.Placement{
		SlotIndex: idx, Weekday: weekday,
		ActivityType: a.ActivityType, Intensity: a.Intensity, Impact: a.Impact, MuscleGroup: a.MuscleGroup,
	}
}

// ============================================================================
// ValidateAndApply — the mandatory hard backend check
// ============================================================================

func TestValidateAndApply_AcceptsValidReordering(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
		slot(activity.Running, activity.IntensityL, activity.ImpactH, activity.MuscleGroupLower),
		slot(activity.Yoga, activity.IntensityL, activity.ImpactL, activity.MuscleGroupFullBody),
	}
	proposed := []scheduling.Placement{
		placementFor(2, "monday", original[2]),
		placementFor(0, "wednesday", original[0]),
		placementFor(1, "friday", original[1]),
	}

	days, err := scheduling.ValidateAndApply(original, 4, proposed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if days[0].IsRestDay || days[0].Assignment != original[2] {
		t.Errorf("monday = %+v, want original[2] unchanged", days[0])
	}
	if days[2].IsRestDay || days[2].Assignment != original[0] {
		t.Errorf("wednesday = %+v, want original[0] unchanged", days[2])
	}
	if days[4].IsRestDay || days[4].Assignment != original[1] {
		t.Errorf("friday = %+v, want original[1] unchanged", days[4])
	}
	for _, i := range []int{1, 3, 5, 6} {
		if !days[i].IsRestDay {
			t.Errorf("day %d should be a rest day, got %+v", i, days[i])
		}
	}
}

func TestValidateAndApply_RejectsChangedActivityType(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	tampered := placementFor(0, "monday", original[0])
	tampered.ActivityType = activity.Running // attempted change

	_, err := scheduling.ValidateAndApply(original, 6, []scheduling.Placement{tampered})
	if err == nil {
		t.Fatal("expected an error for a placement that changes ActivityType")
	}
}

func TestValidateAndApply_RejectsChangedIntensity(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	tampered := placementFor(0, "monday", original[0])
	tampered.Intensity = activity.IntensityH // attempted change

	_, err := scheduling.ValidateAndApply(original, 6, []scheduling.Placement{tampered})
	if err == nil {
		t.Fatal("expected an error for a placement that changes Intensity")
	}
}

func TestValidateAndApply_RejectsChangedImpact(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	tampered := placementFor(0, "monday", original[0])
	tampered.Impact = activity.ImpactH

	_, err := scheduling.ValidateAndApply(original, 6, []scheduling.Placement{tampered})
	if err == nil {
		t.Fatal("expected an error for a placement that changes Impact")
	}
}

func TestValidateAndApply_RejectsChangedMuscleGroup(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	tampered := placementFor(0, "monday", original[0])
	tampered.MuscleGroup = activity.MuscleGroupUpper

	_, err := scheduling.ValidateAndApply(original, 6, []scheduling.Placement{tampered})
	if err == nil {
		t.Fatal("expected an error for a placement that changes MuscleGroup")
	}
}

func TestValidateAndApply_RejectsInventedSlotIndex(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	invented := scheduling.Placement{SlotIndex: 5, Weekday: "monday",
		ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM, MuscleGroup: activity.MuscleGroupLower}

	_, err := scheduling.ValidateAndApply(original, 6, []scheduling.Placement{invented})
	if err == nil {
		t.Fatal("expected an error for a slot index that doesn't exist")
	}
}

func TestValidateAndApply_RejectsDroppedSlot(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
		slot(activity.Running, activity.IntensityL, activity.ImpactH, activity.MuscleGroupLower),
	}
	// Only one placement for two resolved slots.
	proposed := []scheduling.Placement{placementFor(0, "monday", original[0])}

	_, err := scheduling.ValidateAndApply(original, 5, proposed)
	if err == nil {
		t.Fatal("expected an error when a resolved slot is dropped")
	}
}

func TestValidateAndApply_RejectsDuplicateSlotIndex(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
		slot(activity.Running, activity.IntensityL, activity.ImpactH, activity.MuscleGroupLower),
	}
	proposed := []scheduling.Placement{
		placementFor(0, "monday", original[0]),
		placementFor(0, "tuesday", original[0]), // same slot twice
	}

	_, err := scheduling.ValidateAndApply(original, 5, proposed)
	if err == nil {
		t.Fatal("expected an error for a slot index used twice")
	}
}

func TestValidateAndApply_RejectsDuplicateWeekday(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
		slot(activity.Running, activity.IntensityL, activity.ImpactH, activity.MuscleGroupLower),
	}
	proposed := []scheduling.Placement{
		placementFor(0, "monday", original[0]),
		placementFor(1, "monday", original[1]), // same weekday twice
	}

	_, err := scheduling.ValidateAndApply(original, 5, proposed)
	if err == nil {
		t.Fatal("expected an error for a weekday used twice")
	}
}

func TestValidateAndApply_RejectsInvalidWeekday(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	bad := placementFor(0, "funday", original[0])

	_, err := scheduling.ValidateAndApply(original, 6, []scheduling.Placement{bad})
	if err == nil {
		t.Fatal("expected an error for an invalid weekday")
	}
}

func TestValidateAndApply_RejectsWrongTotalCount(t *testing.T) {
	original := []cascade.SlotAssignment{
		slot(activity.Strength, activity.IntensityM, activity.ImpactM, activity.MuscleGroupLower),
	}
	proposed := []scheduling.Placement{placementFor(0, "monday", original[0])}

	_, err := scheduling.ValidateAndApply(original, 2, proposed) // 1 + 2 != 7
	if err == nil {
		t.Fatal("expected an error when resolved slots + rest days don't add up to 7")
	}
}

// ============================================================================
// RestDaysRequiredFor / the lookup table
// ============================================================================

func TestRestDaysRequiredFor(t *testing.T) {
	tests := []struct {
		cost int
		want int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{3, 2},
	}
	for _, tt := range tests {
		got, err := scheduling.RestDaysRequiredFor(tt.cost)
		if err != nil {
			t.Errorf("cost=%d: unexpected error: %v", tt.cost, err)
		}
		if got != tt.want {
			t.Errorf("cost=%d: got %d, want %d", tt.cost, got, tt.want)
		}
	}
}

func TestRestDaysRequiredFor_UnknownValueErrors(t *testing.T) {
	if _, err := scheduling.RestDaysRequiredFor(4); err == nil {
		t.Error("expected an error for an unrecognized RecoveryCost value")
	}
	if _, err := scheduling.RestDaysRequiredFor(-1); err == nil {
		t.Error("expected an error for a negative RecoveryCost value")
	}
}

// ============================================================================
// CheckSpacing
// ============================================================================

func day(assignment cascade.SlotAssignment) scheduling.DayPlacement {
	return scheduling.DayPlacement{Assignment: assignment}
}
func rest() scheduling.DayPlacement { return scheduling.DayPlacement{IsRestDay: true} }

func TestCheckSpacing_NoViolationsForGoodSchedule(t *testing.T) {
	// Strength@H has RecoveryCost 3 (needs 2 rest days between repeats of
	// the same muscle group) — Lower on Monday, next Lower session on
	// Thursday (2 full rest days between: Tue, Wed).
	days := [7]scheduling.DayPlacement{
		day(slot(activity.Strength, activity.IntensityH, activity.ImpactM, activity.MuscleGroupLower)), // Mon
		rest(), // Tue
		rest(), // Wed
		day(slot(activity.Strength, activity.IntensityH, activity.ImpactM, activity.MuscleGroupUpper)), // Thu
		rest(), // Fri
		day(slot(activity.Yoga, activity.IntensityL, activity.ImpactL, activity.MuscleGroupFullBody)), // Sat
		rest(), // Sun
	}
	if v := scheduling.CheckSpacing(days); len(v) != 0 {
		t.Errorf("expected no violations, got %v", v)
	}
}

func TestCheckSpacing_FlagsConsecutiveHighImpactDays(t *testing.T) {
	days := [7]scheduling.DayPlacement{
		day(slot(activity.Running, activity.IntensityH, activity.ImpactH, activity.MuscleGroupLower)),
		day(slot(activity.HIIT, activity.IntensityH, activity.ImpactH, activity.MuscleGroupFullBody)),
		rest(), rest(), rest(), rest(), rest(),
	}
	v := scheduling.CheckSpacing(days)
	if len(v) == 0 {
		t.Fatal("expected a violation for two consecutive High-impact days")
	}
}

func TestCheckSpacing_FlagsInsufficientMuscleGroupGap(t *testing.T) {
	// Strength@H = RecoveryCost 3 (needs 2 rest days). Only 1 rest day
	// between two Lower sessions here (Monday, Wednesday) — a violation.
	days := [7]scheduling.DayPlacement{
		day(slot(activity.Strength, activity.IntensityH, activity.ImpactM, activity.MuscleGroupLower)), // Mon
		rest(), // Tue
		day(slot(activity.Strength, activity.IntensityH, activity.ImpactM, activity.MuscleGroupLower)), // Wed
		rest(), rest(), rest(), rest(),
	}
	v := scheduling.CheckSpacing(days)
	if len(v) == 0 {
		t.Fatal("expected a violation for an insufficient muscle-group gap")
	}
}

func TestCheckSpacing_AllowsRecoveryCost0Or1WithNoGap(t *testing.T) {
	// Mobility@AR has RecoveryCost 0 — back-to-back days targeting the
	// same muscle group are fine.
	days := [7]scheduling.DayPlacement{
		day(slot(activity.Mobility, activity.IntensityAR, activity.ImpactL, activity.MuscleGroupFullBody)),
		day(slot(activity.Mobility, activity.IntensityAR, activity.ImpactL, activity.MuscleGroupFullBody)),
		rest(), rest(), rest(), rest(), rest(),
	}
	if v := scheduling.CheckSpacing(days); len(v) != 0 {
		t.Errorf("expected no violations for RecoveryCost 0 back-to-back, got %v", v)
	}
}
