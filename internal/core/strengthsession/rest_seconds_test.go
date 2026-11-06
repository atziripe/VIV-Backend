package strengthsession

import (
	"testing"
	"time"

	"viv/internal/core/cascade"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
)

// ============================================================================
// A Main compound shows 90s rest in week 1 and 120s in week 3
// ============================================================================

func TestAssemble_MainCompoundRestSecondsWeek1And3(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	week1, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week1: unexpected error: %v", err)
	}
	sq1 := findExercise(t, week1.Exercises, "Squat")
	if sq1.RestSeconds != 90 {
		t.Errorf("week1 Squat RestSeconds = %d, want 90", sq1.RestSeconds)
	}

	week3Today := activatedOn.AddDate(0, 0, 14) // day 14 = week 3
	week3, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), week3Today, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week3: unexpected error: %v", err)
	}
	sq3 := findExercise(t, week3.Exercises, "Squat")
	if sq3.RestSeconds != 120 {
		t.Errorf("week3 Squat RestSeconds = %d, want 120", sq3.RestSeconds)
	}

	if week1.RestSecondsStatus == "" {
		t.Error("expected a non-empty RestSecondsStatus")
	}
}

// ============================================================================
// A Reduced session in week 3 shows the week-1 rest value
// ============================================================================

func TestAssemble_ReducedSessionWeek3UsesWeek1RestSeconds(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	a := baseAssignment()
	a.LoadTier = cascade.LoadReduced
	week3Today := activatedOn.AddDate(0, 0, 14)
	week3Reduced, err := Assemble(a, pinForBlock(1, activatedOn), week3Today, lib, cfg, selector)
	if err != nil {
		t.Fatalf("week3 reduced: unexpected error: %v", err)
	}

	sq3reduced := findExercise(t, week3Reduced.Exercises, "Squat")
	if sq3reduced.RestSeconds != 90 {
		t.Errorf("Reduced in week 3 should use week 1's rest value: got %d, want 90", sq3reduced.RestSeconds)
	}
}

// ============================================================================
// A type missing from rest_seconds_by_type fails the config load loudly
// ============================================================================

func TestLoadBlockConfigFromJSON_RestSecondsMissingTypeErrors(t *testing.T) {
	bad := []byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Squat A"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Squat A": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["3 × 8", "3 × 8", "3 × 8", "3 × 8"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "test_fixture",
			"applies_to_all_tiers": true,
			"ramp_up_note": "note",
			"zones": {"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["a"], "cooldown": ["b"]}}
		},
		"rest_seconds_by_type": {
			"_status": "test_fixture"
		}
	}`)
	if _, err := strengthlibrary.NewBlockConfigFromJSON(bad); err == nil {
		t.Fatal("expected an error for an exercise type (C) with no rest_seconds_by_type entry, got nil")
	}
}
