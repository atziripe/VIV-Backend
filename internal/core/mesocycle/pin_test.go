package mesocycle_test

import (
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
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

func TestResolvePin_CreatesNewPinWhenNoneExists(t *testing.T) {
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	pin, changed, err := mesocycle.ResolvePin(
		nil, activity.Strength, activity.MuscleGroupLower, today,
		mesocycle.StubExerciseSetSelector{}, mesocycle.StubExerciseLibrary{},
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
	if len(pin.ExerciseIDs) == 0 {
		t.Error("expected a non-empty exercise set")
	}
	if !pin.ActivatedOn.Equal(today) {
		t.Errorf("ActivatedOn = %v, want %v", pin.ActivatedOn, today)
	}
	wantRotatesOn := today.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays)
	if !pin.RotatesOn.Equal(wantRotatesOn) {
		t.Errorf("RotatesOn = %v, want %v (ActivatedOn + %d days)", pin.RotatesOn, wantRotatesOn, mesocycle.StrengthMesocycleLengthDays)
	}
}

func TestResolvePin_ReusesWithinCycle(t *testing.T) {
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	existing := mesocycle.PinnedExerciseSet{
		ActivityType: activity.Strength,
		MuscleGroup:  activity.MuscleGroupLower,
		ExerciseIDs:  []mesocycle.ExerciseID{"deadlift", "leg_press"},
		ActivatedOn:  activatedOn,
		RotatesOn:    activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays),
	}

	// One day in, and the day right before RotatesOn — both must reuse.
	for _, today := range []time.Time{
		activatedOn.AddDate(0, 0, 1),
		activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays-1),
	} {
		pin, changed, err := mesocycle.ResolvePin(
			&existing, activity.Strength, activity.MuscleGroupLower, today,
			mesocycle.StubExerciseSetSelector{}, mesocycle.StubExerciseLibrary{},
		)
		if err != nil {
			t.Fatalf("today=%v: unexpected error: %v", today, err)
		}
		if changed {
			t.Errorf("today=%v: expected changed=false (still within the cycle)", today)
		}
		if pin.ActivatedOn != existing.ActivatedOn || pin.RotatesOn != existing.RotatesOn {
			t.Errorf("today=%v: dates changed on reuse: %+v", today, pin)
		}
		if len(pin.ExerciseIDs) != 2 || pin.ExerciseIDs[0] != "deadlift" || pin.ExerciseIDs[1] != "leg_press" {
			t.Errorf("today=%v: ExerciseIDs = %v, want the exact existing set untouched", today, pin.ExerciseIDs)
		}
	}
}

func TestResolvePin_RotatesOnOrAfterRotatesOn(t *testing.T) {
	activatedOn := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	rotatesOn := activatedOn.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays) // day 29
	existing := mesocycle.PinnedExerciseSet{
		ActivityType: activity.Strength,
		MuscleGroup:  activity.MuscleGroupLower,
		ExerciseIDs:  []mesocycle.ExerciseID{"deadlift", "leg_press"},
		ActivatedOn:  activatedOn,
		RotatesOn:    rotatesOn,
	}

	for _, today := range []time.Time{rotatesOn, rotatesOn.AddDate(0, 0, 5)} {
		pin, changed, err := mesocycle.ResolvePin(
			&existing, activity.Strength, activity.MuscleGroupLower, today,
			mesocycle.StubExerciseSetSelector{}, mesocycle.StubExerciseLibrary{},
		)
		if err != nil {
			t.Fatalf("today=%v: unexpected error: %v", today, err)
		}
		if !changed {
			t.Errorf("today=%v: expected changed=true (today >= RotatesOn)", today)
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
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, _, err := mesocycle.ResolvePin(
		nil, activity.HIIT, activity.MuscleGroupFullBody, today,
		mesocycle.StubExerciseSetSelector{}, mesocycle.StubExerciseLibrary{},
	)
	if err == nil {
		t.Fatal("expected an error for a non-mesocycle-eligible activity type")
	}
}

func TestResolvePin_MismatchedExistingErrors(t *testing.T) {
	today := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	existing := mesocycle.PinnedExerciseSet{
		ActivityType: activity.Strength,
		MuscleGroup:  activity.MuscleGroupUpper,
		ActivatedOn:  today,
		RotatesOn:    today.AddDate(0, 0, mesocycle.StrengthMesocycleLengthDays),
	}

	_, _, err := mesocycle.ResolvePin(
		&existing, activity.Strength, activity.MuscleGroupLower, today, // different muscle group
		mesocycle.StubExerciseSetSelector{}, mesocycle.StubExerciseLibrary{},
	)
	if err == nil {
		t.Fatal("expected an error when existing pin's identity doesn't match the requested one")
	}
}
