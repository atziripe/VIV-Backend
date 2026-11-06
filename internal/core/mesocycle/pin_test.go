package mesocycle_test

import (
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
)

func TestIsMesocycleEligible(t *testing.T) {
	if !mesocycle.IsMesocycleEligible(activity.Strength) {
		t.Error("Strength should be mesocycle-eligible")
	}
	if mesocycle.IsMesocycleEligible(activity.Functional) {
		t.Error("Functional should NOT be mesocycle-eligible by default")
	}
	if mesocycle.IsMesocycleEligible(activity.HIIT) {
		t.Error("HIIT must never be mesocycle-eligible — open item, not to be guessed at")
	}
	if mesocycle.IsMesocycleEligible(activity.Yoga) {
		t.Error("Yoga should not be mesocycle-eligible")
	}
}

func TestIsMesocycleEligible_FunctionalToggle(t *testing.T) {
	orig := mesocycle.FunctionalMesocycleEnabled
	defer func() { mesocycle.FunctionalMesocycleEnabled = orig }()

	mesocycle.FunctionalMesocycleEnabled = true
	if !mesocycle.IsMesocycleEligible(activity.Functional) {
		t.Error("Functional should become eligible once the flag is flipped")
	}
}

// ============================================================================
// Fixtures — a small, controlled catalog/config instead of the real
// 113-exercise one, so block-number math is easy to verify by hand.
// ============================================================================

// fixtureLibrary: lower zone, two slots worth of groups —
//   - "Squat" (1 approved + 1 pending): Main slot, should always hold squat_a.
//   - "Hip thrust / bridge" (3 approved): Accessory slot, round-robins.
//   - "Calves" (1 approved): Optional slot, thin.
func fixtureLibrary(t *testing.T) *strengthlibrary.Library {
	t.Helper()
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"squat_a","activity_type":"strength","name":"Squat A","zone":"lower","movement_group":"Squat","review_status":"approved"},
		{"id":"squat_b","activity_type":"strength","name":"Squat B","zone":"lower","movement_group":"Squat","review_status":"pending"},
		{"id":"hip_thrust_a","activity_type":"strength","name":"Hip Thrust A","zone":"lower","movement_group":"Hip thrust / bridge","review_status":"approved"},
		{"id":"hip_thrust_b","activity_type":"strength","name":"Hip Thrust B","zone":"lower","movement_group":"Hip thrust / bridge","review_status":"approved"},
		{"id":"hip_thrust_c","activity_type":"strength","name":"Hip Thrust C","zone":"lower","movement_group":"Hip thrust / bridge","review_status":"approved"},
		{"id":"calf_a","activity_type":"strength","name":"Calf A","zone":"lower","movement_group":"Calves","review_status":"approved"}
	]`))
	if err != nil {
		t.Fatalf("fixtureLibrary: %v", err)
	}
	return lib
}

// minimalPrescriptionJSON is the smallest valid status/exercise_types/
// weekly_prescription_by_type/variant_rules section a BlockConfig
// fixture needs to pass validation — these tests don't exercise
// prescription lookups, so the exercise name/type code are arbitrary
// placeholders.
const minimalPrescriptionJSON = `
	"status": "test_fixture",
	"exercise_types": {"Placeholder Exercise": {"type": "C", "per_side": false}},
	"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
	"variant_rules": {
		"short": {"drop_order": ["Optional", "Accessory"], "never_drop": ["Main"], "min_exercises": 3},
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
	},
	"rest_seconds_by_type": {
		"_status": "test_fixture",
		"C": [90, 90, 120, 120]
	}
`

func fixtureConfig(t *testing.T) *strengthlibrary.BlockConfig {
	t.Helper()
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"zones": {
			"Lower body": [
				{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Squat A", "Squat B"]},
				{"slot": "Hip Thrust", "priority": "Accessory", "source_groups": ["Hip thrust / bridge"], "pool_order": ["Hip Thrust A", "Hip Thrust B", "Hip Thrust C"]},
				{"slot": "Calves", "priority": "Optional", "source_groups": ["Calves"], "pool_order": ["Calf A"]}
			]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},` + minimalPrescriptionJSON + `
	}`))
	if err != nil {
		t.Fatalf("fixtureConfig: %v", err)
	}
	return cfg
}

// fixtureConfigWithOverride is fixtureConfig but "Hip Thrust" rotates
// every 2 blocks instead of the Accessory default of 1.
func fixtureConfigWithOverride(t *testing.T) *strengthlibrary.BlockConfig {
	t.Helper()
	cfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"zones": {
			"Lower body": [
				{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["Squat A", "Squat B"]},
				{"slot": "Hip Thrust", "priority": "Accessory", "source_groups": ["Hip thrust / bridge"], "pool_order": ["Hip Thrust A", "Hip Thrust B", "Hip Thrust C"], "rotation_period_blocks": 2},
				{"slot": "Calves", "priority": "Optional", "source_groups": ["Calves"], "pool_order": ["Calf A"]}
			]
		},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},` + minimalPrescriptionJSON + `
	}`))
	if err != nil {
		t.Fatalf("fixtureConfigWithOverride: %v", err)
	}
	return cfg
}

// ============================================================================
// Selection: pending never selected, block 1 = pool[0], rotation,
// thin-slot reporting, policy override
// ============================================================================

func TestSelectBlockExercises_PendingNeverSelected(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	bySlot, _, err := mesocycle.DefaultBlockExerciseSelector{}.SelectBlockExercises(strengthlibrary.ZoneLower, 1, lib, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := bySlot["Squat"].ID; got != "squat_a" {
		t.Errorf("Squat slot = %q, want squat_a (squat_b is pending)", got)
	}
}

func TestSelectBlockExercises_Block1TakesFirstOfEachPool(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	bySlot, thin, err := mesocycle.DefaultBlockExerciseSelector{}.SelectBlockExercises(strengthlibrary.ZoneLower, 1, lib, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bySlot["Squat"].ID != "squat_a" || bySlot["Hip Thrust"].ID != "hip_thrust_a" || bySlot["Calves"].ID != "calf_a" {
		t.Errorf("block 1 selection = %+v, want each slot's first approved exercise", bySlot)
	}
	// Squat is thin too: its pool has 1 approved exercise (squat_b is
	// pending, so it never counts) — this is exactly why ResolvePin's
	// Main-slot-holds behavior is worth testing separately from
	// thinness; a slot can be both at once.
	if len(thin) != 2 || thin[0] != "Squat" || thin[1] != "Calves" {
		t.Errorf("thinSlots = %v, want [\"Squat\", \"Calves\"]", thin)
	}
}

func TestSelectBlockExercises_MainSlotHoldsAcrossBlocks(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	for block := 1; block <= 3; block++ {
		bySlot, _, err := mesocycle.DefaultBlockExerciseSelector{}.SelectBlockExercises(strengthlibrary.ZoneLower, block, lib, cfg)
		if err != nil {
			t.Fatalf("block %d: unexpected error: %v", block, err)
		}
		if got := bySlot["Squat"].ID; got != "squat_a" {
			t.Errorf("block %d: Squat (Main) = %q, want squat_a in every block", block, got)
		}
	}
}

func TestSelectBlockExercises_AccessorySlotRotatesEveryBlock(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	want := []string{"hip_thrust_a", "hip_thrust_b", "hip_thrust_c", "hip_thrust_a"} // wraps after 3
	for block := 1; block <= 4; block++ {
		bySlot, _, err := mesocycle.DefaultBlockExerciseSelector{}.SelectBlockExercises(strengthlibrary.ZoneLower, block, lib, cfg)
		if err != nil {
			t.Fatalf("block %d: unexpected error: %v", block, err)
		}
		if got := bySlot["Hip Thrust"].ID; got != want[block-1] {
			t.Errorf("block %d: Hip Thrust = %q, want %q", block, got, want[block-1])
		}
	}
}

func TestSelectBlockExercises_ThinSlotRepeatsWithoutError(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	for block := 1; block <= 3; block++ {
		bySlot, thin, err := mesocycle.DefaultBlockExerciseSelector{}.SelectBlockExercises(strengthlibrary.ZoneLower, block, lib, cfg)
		if err != nil {
			t.Fatalf("block %d: unexpected error: %v", block, err)
		}
		if got := bySlot["Calves"].ID; got != "calf_a" {
			t.Errorf("block %d: Calves (thin) = %q, want calf_a every time", block, got)
		}
		if len(thin) != 2 || thin[0] != "Squat" || thin[1] != "Calves" {
			t.Errorf("block %d: thinSlots = %v, want [\"Squat\", \"Calves\"] (Squat has 1 approved exercise too)", block, thin)
		}
	}
}

func TestSelectBlockExercises_NoApprovedExerciseIsHardError(t *testing.T) {
	lib, err := strengthlibrary.NewLibraryFromJSON([]byte(`[
		{"id":"squat_pending","activity_type":"strength","name":"Squat","zone":"lower","movement_group":"Squat","review_status":"pending"}
	]`))
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	cfg := fixtureConfig(t)
	selector := mesocycle.DefaultBlockExerciseSelector{}
	if _, _, err := selector.SelectBlockExercises(strengthlibrary.ZoneLower, 1, lib, cfg); err == nil {
		t.Fatal("expected an error when a slot has zero approved exercises, got nil")
	}
}

func TestSelectBlockExercises_SlotPolicyOverrideChangesRotation(t *testing.T) {
	lib := fixtureLibrary(t)
	cfg := fixtureConfigWithOverride(t)
	// period=2: blocks 1,2 -> idx0 ; blocks 3,4 -> idx1.
	want := []string{"hip_thrust_a", "hip_thrust_a", "hip_thrust_b", "hip_thrust_b"}
	for block := 1; block <= 4; block++ {
		bySlot, _, err := mesocycle.DefaultBlockExerciseSelector{}.SelectBlockExercises(strengthlibrary.ZoneLower, block, lib, cfg)
		if err != nil {
			t.Fatalf("block %d: unexpected error: %v", block, err)
		}
		if got := bySlot["Hip Thrust"].ID; got != want[block-1] {
			t.Errorf("block %d: Hip Thrust = %q, want %q (policy overridden to rotate every 2 blocks)", block, got, want[block-1])
		}
	}
}

// ============================================================================
// ResolvePin: create / reuse / rotate, using the fixtures above
// ============================================================================

func TestResolvePin_CreatesNewPinWhenNoneExists(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	pin, changed, thin, err := mesocycle.ResolvePin(
		nil, activity.Strength, activity.MuscleGroupLower, today,
		mesocycle.DefaultBlockExerciseSelector{}, lib, cfg,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true when creating a new pin")
	}
	if pin.ActivityType != activity.Strength || pin.MuscleGroup != activity.MuscleGroupLower {
		t.Errorf("pin identity = (%s,%s), want (strength,lower)", pin.ActivityType, pin.MuscleGroup)
	}
	if pin.BlockNumber != 1 {
		t.Errorf("BlockNumber = %d, want 1", pin.BlockNumber)
	}
	if len(pin.ActiveExercises) != 3 {
		t.Errorf("ActiveExercises = %v, want one entry per movement group (3)", pin.ActiveExercises)
	}
	if !pin.ActivatedOn.Equal(today) {
		t.Errorf("ActivatedOn = %v, want %v", pin.ActivatedOn, today)
	}
	wantRotatesOn := today.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays)
	if !pin.RotatesOn.Equal(wantRotatesOn) {
		t.Errorf("RotatesOn = %v, want %v (ActivatedOn + %d days)", pin.RotatesOn, wantRotatesOn, mesocycle.StrengthMesocycleLengthDays)
	}
	if len(thin) != 2 || thin[0] != "Squat" || thin[1] != "Calves" {
		t.Errorf("thinSlots = %v, want [\"Squat\", \"Calves\"]", thin)
	}
}

func TestResolvePin_ReusesWithinCycle(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	existing := mesocycle.PinnedExerciseSet{
		ActivityType:    activity.Strength,
		MuscleGroup:     activity.MuscleGroupLower,
		ActiveExercises: map[strengthlibrary.MovementGroup]mesocycle.ExerciseID{"Squat": "squat_a"},
		BlockNumber:     1,
		ActivatedOn:     activatedOn,
		RotatesOn:       activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays),
	}

	for _, today := range []time.Time{
		activatedOn.AddDate(0, 0, 1),
		activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays-1),
	} {
		pin, changed, thin, err := mesocycle.ResolvePin(
			&existing, activity.Strength, activity.MuscleGroupLower, today,
			mesocycle.DefaultBlockExerciseSelector{}, lib, cfg,
		)
		if err != nil {
			t.Fatalf("today=%v: unexpected error: %v", today, err)
		}
		if changed {
			t.Errorf("today=%v: expected changed=false (still within the cycle)", today)
		}
		if thin != nil {
			t.Errorf("today=%v: expected nil thinSlots on a pure reuse, got %v", today, thin)
		}
		if pin.BlockNumber != 1 || pin.ActiveExercises["Squat"] != "squat_a" {
			t.Errorf("today=%v: pin changed on reuse: %+v", today, pin)
		}
	}
}

func TestResolvePin_RotatesOnOrAfterRotatesOn(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	rotatesOn := activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays) // day 29
	existing := mesocycle.PinnedExerciseSet{
		ActivityType:    activity.Strength,
		MuscleGroup:     activity.MuscleGroupLower,
		ActiveExercises: map[strengthlibrary.MovementGroup]mesocycle.ExerciseID{"Hip thrust / bridge": "hip_thrust_a"},
		BlockNumber:     1,
		ActivatedOn:     activatedOn,
		RotatesOn:       rotatesOn,
	}

	for _, today := range []time.Time{rotatesOn, rotatesOn.AddDate(0, 0, 5)} {
		pin, changed, _, err := mesocycle.ResolvePin(
			&existing, activity.Strength, activity.MuscleGroupLower, today,
			mesocycle.DefaultBlockExerciseSelector{}, lib, cfg,
		)
		if err != nil {
			t.Fatalf("today=%v: unexpected error: %v", today, err)
		}
		if !changed {
			t.Errorf("today=%v: expected changed=true (today >= RotatesOn)", today)
		}
		if pin.BlockNumber != 2 {
			t.Errorf("today=%v: BlockNumber = %d, want 2 (never re-selects block 1)", today, pin.BlockNumber)
		}
		if got := pin.ActiveExercises["Hip thrust / bridge"]; got != "hip_thrust_b" {
			t.Errorf("today=%v: Hip thrust exercise after rotation = %q, want hip_thrust_b (block 2)", today, got)
		}
		if !pin.ActivatedOn.Equal(today) {
			t.Errorf("today=%v: new ActivatedOn = %v, want %v", today, pin.ActivatedOn, today)
		}
		wantRotatesOn := today.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays)
		if !pin.RotatesOn.Equal(wantRotatesOn) {
			t.Errorf("today=%v: new RotatesOn = %v, want %v", today, pin.RotatesOn, wantRotatesOn)
		}
	}
}

func TestResolvePin_NotEligibleErrors(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, _, _, err := mesocycle.ResolvePin(
		nil, activity.HIIT, activity.MuscleGroupFullBody, today,
		mesocycle.DefaultBlockExerciseSelector{}, lib, cfg,
	)
	if err == nil {
		t.Fatal("expected an error for a non-mesocycle-eligible activity type")
	}
}

func TestResolvePin_MismatchedExistingErrors(t *testing.T) {
	lib, cfg := fixtureLibrary(t), fixtureConfig(t)
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	existing := mesocycle.PinnedExerciseSet{
		ActivityType: activity.Strength,
		MuscleGroup:  activity.MuscleGroupUpper,
		ActivatedOn:  today,
		RotatesOn:    today.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays),
	}

	_, _, _, err := mesocycle.ResolvePin(
		&existing, activity.Strength, activity.MuscleGroupLower, today, // different muscle group
		mesocycle.DefaultBlockExerciseSelector{}, lib, cfg,
	)
	if err == nil {
		t.Fatal("expected an error when existing pin's identity doesn't match the requested one")
	}
}
