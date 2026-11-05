package strengthsession

import (
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
)

// allZonesFixtureLibrary/allZonesFixtureConfig give every zone (Lower/
// Upper/Full/Core) exactly one Main slot, one approved exercise, and a
// warmup_cooldown entry with movements/durations distinct per zone —
// so a test can tell "this zone's own warm-up came back" from "some
// other zone's did".
func allZonesFixtureLibrary(t *testing.T) *strengthlibrary.Library {
	t.Helper()
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"lower_ex","activity_type":"strength","name":"Lower Ex","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"upper_ex","activity_type":"strength","name":"Upper Ex","zone":"upper","movement_group":"Horizontal push","review_status":"approved"},
		{"id":"full_ex","activity_type":"strength","name":"Full Ex","zone":"full","movement_group":"Squat","review_status":"approved"},
		{"id":"core_ex","activity_type":"strength","name":"Core Ex","zone":"core","movement_group":"Anti-extension","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("allZonesFixtureLibrary: %v", err)
	}
	return lib
}

func allZonesFixtureConfig(t *testing.T) *strengthlibrary.BlockConfig {
	t.Helper()
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "draft_not_reviewed_by_sport_scientist",
		"zones": {
			"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Lower Ex"]}],
			"Upper body": [{"slot": "Push", "priority": "Main", "source_groups": ["Horizontal push"], "pool_order": ["Upper Ex"]}],
			"Full body":  [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Full Ex"]}],
			"Core":       [{"slot": "Anti-Extension", "priority": "Main", "source_groups": ["Anti-extension"], "pool_order": ["Core Ex"]}]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {
			"Lower Ex": {"type": "C", "per_side": false},
			"Upper Ex": {"type": "C", "per_side": false},
			"Full Ex": {"type": "C", "per_side": false},
			"Core Ex": {"type": "C", "per_side": false}
		},
		"weekly_prescription_by_type": {"C": ["3 × 8", "3 × 8", "3 × 8", "3 × 8"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional", "Accessory"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "draft_llm_generated_unreviewed",
			"applies_to_all_tiers": true,
			"ramp_up_note": "Before the first Main exercise, do 1-2 light sets at about half the working load.",
			"zones": {
				"Lower body": {"warmup_minutes": 7, "cooldown_minutes": 5, "warmup": ["Glute bridges"], "cooldown": ["Hamstring stretch"]},
				"Upper body": {"warmup_minutes": 6, "cooldown_minutes": 4, "warmup": ["Arm circles"], "cooldown": ["Chest stretch"]},
				"Full body":  {"warmup_minutes": 7, "cooldown_minutes": 4, "warmup": ["Bodyweight squats"], "cooldown": ["Forward fold"]},
				"Core":       {"warmup_minutes": 5, "cooldown_minutes": 4, "warmup": ["Dead bug"], "cooldown": ["Cobra stretch"]}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("allZonesFixtureConfig: %v", err)
	}
	return cfg
}

func assignmentFor(mg activity.MuscleGroup) cascade.SlotAssignment {
	return cascade.SlotAssignment{
		ActivityType: activity.Strength,
		MuscleGroup:  mg,
		Intensity:    activity.IntensityM,
		DurationTier: cascade.DurationStandard,
		LoadTier:     cascade.LoadStandard,
	}
}

func pinFor(mg activity.MuscleGroup, block int, activatedOn time.Time) mesocycle.PinnedExerciseSet {
	return mesocycle.PinnedExerciseSet{
		ActivityType: activity.Strength,
		MuscleGroup:  mg,
		BlockNumber:  block,
		ActivatedOn:  activatedOn,
		RotatesOn:    activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays),
	}
}

// ============================================================================
// Each of the 4 zones returns its own warm-up and cool-down
// ============================================================================

func TestAssemble_EachZoneReturnsItsOwnWarmupCooldown(t *testing.T) {
	lib, cfg := allZonesFixtureLibrary(t), allZonesFixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	cases := []struct {
		muscleGroup      activity.MuscleGroup
		wantWarmup       string
		wantCooldown     string
		wantWarmupMins   int
		wantCooldownMins int
	}{
		{activity.MuscleGroupLower, "Glute bridges", "Hamstring stretch", 7, 5},
		{activity.MuscleGroupUpper, "Arm circles", "Chest stretch", 6, 4},
		{activity.MuscleGroupFullBody, "Bodyweight squats", "Forward fold", 7, 4},
		{activity.MuscleGroupCore, "Dead bug", "Cobra stretch", 5, 4},
	}

	for _, tc := range cases {
		got, err := Assemble(assignmentFor(tc.muscleGroup), pinFor(tc.muscleGroup, 1, activatedOn), activatedOn, lib, cfg, selector)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.muscleGroup, err)
		}
		if len(got.Warmup.Movements) != 1 || got.Warmup.Movements[0] != tc.wantWarmup {
			t.Errorf("%s: Warmup.Movements = %v, want [%q]", tc.muscleGroup, got.Warmup.Movements, tc.wantWarmup)
		}
		if len(got.Cooldown.Movements) != 1 || got.Cooldown.Movements[0] != tc.wantCooldown {
			t.Errorf("%s: Cooldown.Movements = %v, want [%q]", tc.muscleGroup, got.Cooldown.Movements, tc.wantCooldown)
		}
		if got.Warmup.DurationMinutes != tc.wantWarmupMins {
			t.Errorf("%s: Warmup.DurationMinutes = %d, want %d", tc.muscleGroup, got.Warmup.DurationMinutes, tc.wantWarmupMins)
		}
		if got.Cooldown.DurationMinutes != tc.wantCooldownMins {
			t.Errorf("%s: Cooldown.DurationMinutes = %d, want %d", tc.muscleGroup, got.Cooldown.DurationMinutes, tc.wantCooldownMins)
		}
		if got.WarmupCooldownStatus != "draft_llm_generated_unreviewed" {
			t.Errorf("%s: WarmupCooldownStatus = %q, want the config's own status verbatim", tc.muscleGroup, got.WarmupCooldownStatus)
		}
	}
}

// ============================================================================
// A Short session still returns the full warm-up and cool-down
// ============================================================================

func TestAssemble_ShortSessionStillReturnsFullWarmupCooldown(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	standard, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("standard: unexpected error: %v", err)
	}

	a := baseAssignment()
	a.DurationTier = cascade.DurationShort
	short, err := Assemble(a, pinForBlock(1, activatedOn), activatedOn, lib, cfg, selector)
	if err != nil {
		t.Fatalf("short: unexpected error: %v", err)
	}

	// Short drops exercise slots (see TestAssemble_ShortDropsOptionalBeforeAccessoryNeverMain)
	// but must never touch the warm-up/cool-down.
	if len(short.Exercises) >= len(standard.Exercises) {
		t.Fatalf("expected Short to actually drop some exercises for this comparison to be meaningful: standard=%d short=%d", len(standard.Exercises), len(short.Exercises))
	}
	if len(short.Warmup.Movements) == 0 {
		t.Error("expected a non-empty warm-up even for a Short session")
	}
	if len(short.Cooldown.Movements) == 0 {
		t.Error("expected a non-empty cool-down even for a Short session")
	}
	if short.Warmup.DurationMinutes != standard.Warmup.DurationMinutes || len(short.Warmup.Movements) != len(standard.Warmup.Movements) {
		t.Errorf("expected Short's warm-up to match the standard session's exactly: short=%+v standard=%+v", short.Warmup, standard.Warmup)
	}
	if short.Cooldown.DurationMinutes != standard.Cooldown.DurationMinutes || len(short.Cooldown.Movements) != len(standard.Cooldown.Movements) {
		t.Errorf("expected Short's cool-down to match the standard session's exactly: short=%+v standard=%+v", short.Cooldown, standard.Cooldown)
	}
}

// ============================================================================
// The ramp-up note is present once per session, not per exercise
// ============================================================================

func TestAssemble_RampUpNotePresentOnce(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	got, err := Assemble(baseAssignment(), pinForBlock(1, activatedOn), activatedOn, lib, cfg, mesocycle.DefaultBlockExerciseSelector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RampUpNote == "" {
		t.Error("expected a non-empty RampUpNote")
	}
	if got.RampUpNote != "Before the first Main exercise, do 1-2 light sets." {
		t.Errorf("RampUpNote = %q, want the config's ramp_up_note verbatim", got.RampUpNote)
	}
	// AssembledExercise has no field for it at all — it's a session-level
	// field, never duplicated per exercise. (Compile-time: if this ever
	// grows a RampUpNote field, this comment is the only signal — there's
	// nothing to assert at runtime for an absent field.)
}

// ============================================================================
// A missing zone entry in warmup_cooldown fails loudly
// ============================================================================

func TestAssemble_MissingWarmupCooldownZoneErrors(t *testing.T) {
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"lower_ex","activity_type":"strength","name":"Lower Ex","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"upper_ex","activity_type":"strength","name":"Upper Ex","zone":"upper","movement_group":"Horizontal push","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	// zones defines both Lower body and Upper body for exercise slots,
	// but warmup_cooldown.zones only has Lower body — so Upper's failure
	// is specifically a missing warmup_cooldown entry, not a missing
	// exercise-slot zone.
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "x",
		"zones": {
			"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Lower Ex"]}],
			"Upper body": [{"slot": "Push", "priority": "Main", "source_groups": ["Horizontal push"], "pool_order": ["Upper Ex"]}]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"Lower Ex": {"type": "C", "per_side": false}, "Upper Ex": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["3 × 8", "3 × 8", "3 × 8", "3 × 8"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": false},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "x",
			"applies_to_all_tiers": true,
			"ramp_up_note": "note",
			"zones": {
				"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["Leg swings"], "cooldown": ["Quad stretch"]}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	selector := mesocycle.DefaultBlockExerciseSelector{}

	// Lower body works fine (it has a warmup_cooldown entry).
	if _, err := Assemble(assignmentFor(activity.MuscleGroupLower), pinFor(activity.MuscleGroupLower, 1, activatedOn), activatedOn, lib, cfg, selector); err != nil {
		t.Fatalf("Lower body: unexpected error: %v", err)
	}

	// Upper body has exercise slots but no warmup_cooldown entry —
	// must fail loudly, never fall back to an empty warm-up.
	_, err = Assemble(assignmentFor(activity.MuscleGroupUpper), pinFor(activity.MuscleGroupUpper, 1, activatedOn), activatedOn, lib, cfg, selector)
	if err == nil {
		t.Fatal("expected an error for a zone with no warmup_cooldown entry, got nil")
	}
}
