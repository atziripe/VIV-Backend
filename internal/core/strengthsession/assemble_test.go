package strengthsession

import (
	"strings"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
)

func fixtureLibrary(t *testing.T) *strengthlibrary.Library {
	t.Helper()
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"lower_squat_a","activity_type":"strength","name":"Squat A","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"lower_hipthrust_a","activity_type":"strength","name":"Hip Thrust A","zone":"lower","movement_group":"Hip thrust / bridge","review_status":"approved"},
		{"id":"lower_hipthrust_b","activity_type":"strength","name":"Hip Thrust B","zone":"lower","movement_group":"Hip thrust / bridge","review_status":"approved"},
		{"id":"lower_calf_a","activity_type":"strength","name":"Calf A","zone":"lower","movement_group":"Calves","review_status":"approved"},
		{"id":"lower_unregistered","activity_type":"strength","name":"Unregistered Exercise","zone":"lower","movement_group":"Squat","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("fixtureLibrary: %v", err)
	}
	return lib
}

func fixtureConfig(t *testing.T) *strengthlibrary.BlockConfig {
	t.Helper()
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "draft_not_reviewed_by_sport_scientist",
		"zones": {
			"Lower body": [
				{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]},
				{"slot": "Hip Thrust", "priority": "Accessory", "source_groups": ["Hip thrust / bridge"]},
				{"slot": "Calves", "priority": "Optional", "source_groups": ["Calves"]}
			]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {
			"Squat A": {"type": "C", "per_side": false},
			"Hip Thrust A": {"type": "C", "per_side": false},
			"Hip Thrust B": {"type": "C", "per_side": true},
			"Calf A": {"type": "A", "per_side": false}
		},
		"weekly_prescription_by_type": {
			"C": ["3 × 8–10 @ RIR 3", "3 × 8–10 @ RIR 2–3", "3 × 6–8 @ RIR 2 · week3", "3 × 6–8 @ RIR 2 · week4"],
			"A": ["2 × 12–15 @ RIR 3", "2 × 12–15 @ RIR 2–3", "3 × 12–15 @ RIR 2", "3 × 12–15 @ RIR 2"]
		},
		"variant_rules": {
			"short": {"drop_order": ["Optional", "Accessory"], "never_drop": ["Main"], "min_exercises": 2},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {
				"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2},
				"Medium": "as_written",
				"High": "treated_as_Medium_until_reviewed"
			}
		},
		"warmup_cooldown": {
			"_status": "test_fixture",
			"applies_to_all_tiers": true,
			"ramp_up_note": "Before the first Main exercise, do 1-2 light sets.",
			"zones": {
				"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["Leg swings"], "cooldown": ["Quad stretch"]}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("fixtureConfig: %v", err)
	}
	return cfg
}

func baseAssignment() cascade.SlotAssignment {
	return cascade.SlotAssignment{
		ActivityType:   activity.Strength,
		MuscleGroup:    activity.MuscleGroupLower,
		Intensity:      activity.IntensityM,
		DurationTier:   cascade.DurationStandard,
		LoadTier:       cascade.LoadStandard,
		ComplexityTier: cascade.ComplexityStandard,
	}
}

func pinForBlock(block int, activatedOn time.Time) mesocycle.PinnedExerciseSet {
	return mesocycle.PinnedExerciseSet{
		ActivityType: activity.Strength,
		MuscleGroup:  activity.MuscleGroupLower,
		BlockNumber:  block,
		ActivatedOn:  activatedOn,
		RotatesOn:    activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays),
	}
}

func findExercise(t *testing.T, exercises []AssembledExercise, slot string) AssembledExercise {
	t.Helper()
	for _, e := range exercises {
		if e.Slot == slot {
			return e
		}
	}
	t.Fatalf("no exercise found for slot %q in %+v", slot, exercises)
	return AssembledExercise{}
}

// ============================================================================
// Week 1 vs week 3 give different prescription text
// ============================================================================

func TestAssemble_Week1AndWeek3GiveDifferentPrescriptionText(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	week1, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week1: unexpected error: %v", err)
	}
	if week1.Week != 1 {
		t.Fatalf("week1.Week = %d, want 1", week1.Week)
	}

	week3Today := activatedOn.AddDate(0, 0, 14) // day 14 = week 3
	week3, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), week3Today, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week3: unexpected error: %v", err)
	}
	if week3.Week != 3 {
		t.Fatalf("week3.Week = %d, want 3", week3.Week)
	}

	sq1 := findExercise(t, week1.Exercises, "Squat")
	sq3 := findExercise(t, week3.Exercises, "Squat")
	if sq1.PrescriptionText == sq3.PrescriptionText {
		t.Errorf("expected different prescription text for week 1 vs week 3, both were %q", sq1.PrescriptionText)
	}
}

// ============================================================================
// Main slots hold across blocks; Accessory rotates
// ============================================================================

func TestAssemble_MainHoldsAccessoryRotatesAcrossBlocks(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	block1, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("block1: unexpected error: %v", err)
	}
	block2, err := Assemble(baseAssignment(), pinForBlock(2, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("block2: unexpected error: %v", err)
	}

	sq1, sq2 := findExercise(t, block1.Exercises, "Squat"), findExercise(t, block2.Exercises, "Squat")
	if sq1.ExerciseName != sq2.ExerciseName {
		t.Errorf("Main slot Squat changed between blocks: block1=%q block2=%q", sq1.ExerciseName, sq2.ExerciseName)
	}

	ht1, ht2 := findExercise(t, block1.Exercises, "Hip Thrust"), findExercise(t, block2.Exercises, "Hip Thrust")
	if ht1.ExerciseName == ht2.ExerciseName {
		t.Errorf("Accessory slot Hip Thrust should change between blocks, both were %q", ht1.ExerciseName)
	}
}

// ============================================================================
// Short drops Optional before Accessory, never Main
// ============================================================================

func TestAssemble_ShortDropsOptionalBeforeAccessoryNeverMain(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	a := baseAssignment()
	a.DurationTier = cascade.DurationShort
	got, err := Assemble(a, pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// min_exercises=2 in the fixture, 3 slots total (Main, Accessory,
	// Optional) -> Optional dropped first (3->2), floor reached, Main
	// and the one remaining (Accessory) survive.
	if len(got.Exercises) != 2 {
		t.Fatalf("expected 2 exercises after Short, got %d: %+v", len(got.Exercises), got.Exercises)
	}
	var sawMain, sawOptional bool
	for _, e := range got.Exercises {
		if e.Slot == "Squat" {
			sawMain = true
		}
		if e.Slot == "Calves" {
			sawOptional = true
		}
	}
	if !sawMain {
		t.Error("Main slot (Squat) must never be dropped by Short")
	}
	if sawOptional {
		t.Error("Optional slot (Calves) should have been dropped before Accessory")
	}

	foundTierChange := false
	for _, tc := range got.TierChanges {
		if tc.Lever == LeverDurationShort {
			foundTierChange = true
		}
	}
	if !foundTierChange {
		t.Error("expected a duration_short TierChange to be recorded")
	}
}

func TestAssemble_ShortNeverDropsBelowMinExercisesEvenIfOnlyMainLeft(t *testing.T) {
	// Upper-body-style fixture: 1 Main + 1 Accessory, min_exercises=2 —
	// dropping the lone Accessory would go to 1, below the floor, so it
	// must survive even though Short wants to shorten further.
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"lower_squat_a","activity_type":"strength","name":"Squat A","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"lower_hipthrust_a","activity_type":"strength","name":"Hip Thrust A","zone":"lower","movement_group":"Hip thrust / bridge","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "x",
		"zones": {
			"Lower body": [
				{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]},
				{"slot": "Hip Thrust", "priority": "Accessory", "source_groups": ["Hip thrust / bridge"]}
			]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Squat A": {"type": "C", "per_side": false}, "Hip Thrust A": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["3 × 8", "3 × 8", "3 × 8", "3 × 8"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional", "Accessory"], "never_drop": ["Main"], "min_exercises": 2},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "test_fixture",
			"applies_to_all_tiers": true,
			"ramp_up_note": "Before the first Main exercise, do 1-2 light sets.",
			"zones": {
				"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["Leg swings"], "cooldown": ["Quad stretch"]}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	a := baseAssignment()
	a.DurationTier = cascade.DurationShort
	got, err := Assemble(a, pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Exercises) != 2 {
		t.Fatalf("expected both exercises to survive (floor=2), got %d", len(got.Exercises))
	}
}

// ============================================================================
// Reduced uses week-1 text in week 3
// ============================================================================

func TestAssemble_ReducedUsesWeek1TextInWeek3(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	week1Plain, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week1: unexpected error: %v", err)
	}

	a := baseAssignment()
	a.LoadTier = cascade.LoadReduced
	week3Today := activatedOn.AddDate(0, 0, 14)
	week3Reduced, err := Assemble(a, pinForBlock(1, activatedOn), week3Today, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week3 reduced: unexpected error: %v", err)
	}

	if week3Reduced.Week != 3 {
		t.Errorf("reported Week = %d, want 3 (Reduced changes the TEXT, not the reported week)", week3Reduced.Week)
	}

	sq1 := findExercise(t, week1Plain.Exercises, "Squat")
	sq3reduced := findExercise(t, week3Reduced.Exercises, "Squat")
	if sq1.PrescriptionText != sq3reduced.PrescriptionText {
		t.Errorf("Reduced in week 3 should match week 1's text: week1=%q week3reduced=%q", sq1.PrescriptionText, sq3reduced.PrescriptionText)
	}

	foundTierChange := false
	for _, tc := range week3Reduced.TierChanges {
		if tc.Lever == LeverLoadReduced {
			foundTierChange = true
		}
	}
	if !foundTierChange {
		t.Error("expected a load_reduced TierChange to be recorded")
	}
}

// ============================================================================
// High intensity downgraded, flagged for the caller to log
// ============================================================================

func TestAssemble_HighIntensityDowngradedToMedium(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	a := baseAssignment()
	a.Intensity = activity.IntensityH
	high, err := Assemble(a, pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !high.HighIntensityDowngraded {
		t.Error("expected HighIntensityDowngraded = true")
	}

	a.Intensity = activity.IntensityM
	medium, err := Assemble(a, pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sqHigh := findExercise(t, high.Exercises, "Squat")
	sqMedium := findExercise(t, medium.Exercises, "Squat")
	if sqHigh.PrescriptionText != sqMedium.PrescriptionText {
		t.Errorf("High (downgraded) should produce the same text as Medium: high=%q medium=%q", sqHigh.PrescriptionText, sqMedium.PrescriptionText)
	}
}

// ============================================================================
// Intensity Low rewrites RIR/sets; per-side appends "per side"
// ============================================================================

func TestAssemble_LowIntensityAdjustsSetsAndRIR(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	a := baseAssignment()
	a.Intensity = activity.IntensityL
	got, err := Assemble(a, pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sq := findExercise(t, got.Exercises, "Squat")
	// Base week-1 C text: "3 × 8–10 @ RIR 3" -> sets 3-1=2, RIR 3+2=5.
	if !strings.HasPrefix(sq.PrescriptionText, "2 ×") {
		t.Errorf("expected sets reduced to 2, got %q", sq.PrescriptionText)
	}
	if !strings.Contains(sq.PrescriptionText, "RIR 5") {
		t.Errorf("expected RIR increased to 5, got %q", sq.PrescriptionText)
	}
	if sq.Sets != 2 {
		t.Errorf("Sets = %d, want 2 (parsed from the adjusted text, not the base 3)", sq.Sets)
	}
}

func TestAssemble_PopulatesExerciseIDAndSets(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	got, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sq := findExercise(t, got.Exercises, "Squat")
	if sq.ExerciseID == "" {
		t.Error("expected a non-empty ExerciseID so a client can log sets against it")
	}
	if sq.Sets != 3 {
		t.Errorf("Sets = %d, want 3 (parsed from base week-1 text \"3 × 8–10 @ RIR 3\")", sq.Sets)
	}
}

func TestAssemble_PerSideExerciseTextSaysPerSide(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	// Block 2 rotates Hip Thrust to "Hip Thrust B", which is per_side in
	// the fixture config.
	got, err := Assemble(baseAssignment(), pinForBlock(2, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ht := findExercise(t, got.Exercises, "Hip Thrust")
	if ht.ExerciseName != "Hip Thrust B" {
		t.Fatalf("expected block 2 to pick Hip Thrust B, got %q", ht.ExerciseName)
	}
	if !strings.HasSuffix(ht.PrescriptionText, "per side") {
		t.Errorf("expected prescription text to end with \"per side\", got %q", ht.PrescriptionText)
	}
}

// ============================================================================
// An exercise with no type configured errors loudly
// ============================================================================

func TestAssemble_ExerciseWithNoTypeErrors(t *testing.T) {
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"lower_unregistered","activity_type":"strength","name":"Unregistered Exercise","zone":"lower","movement_group":"Squat","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Some Other Exercise": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "test_fixture",
			"applies_to_all_tiers": true,
			"ramp_up_note": "Before the first Main exercise, do 1-2 light sets.",
			"zones": {
				"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["Leg swings"], "cooldown": ["Quad stretch"]}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, err = Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err == nil {
		t.Fatal("expected an error for an exercise with no configured type, got nil")
	}
}

// ============================================================================
// Non-Strength activity type is rejected outright by Assemble — the
// dispatch decision ("is this Strength") belongs to the caller
// (usecase.SessionContentSelector), but Assemble itself refuses to
// silently do something for a type it was never designed for.
// ============================================================================

func TestAssemble_NonStrengthActivityErrors(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	a := baseAssignment()
	a.ActivityType = activity.Pilates
	_, err := Assemble(a, pinForBlock(1, time.Now()), time.Now(), lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err == nil {
		t.Fatal("expected an error for a non-Strength activity type, got nil")
	}
}

// ============================================================================
// Week outside 1-4 fails loudly
// ============================================================================

func TestAssemble_WeekOutsideRangeErrors(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	// 35 days since block start -> week 6, outside 1-4.
	tooLate := activatedOn.AddDate(0, 0, 35)
	_, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), tooLate, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err == nil {
		t.Fatal("expected an error for a week outside 1-4, got nil")
	}

	// today before ActivatedOn -> negative days, also an error.
	tooEarly := activatedOn.AddDate(0, 0, -1)
	_, err = Assemble(baseAssignment(), pinForBlock(1, activatedOn), tooEarly, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err == nil {
		t.Fatal("expected an error for today before ActivatedOn, got nil")
	}
}
