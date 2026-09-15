// Package mesocycle implements exercise pinning for mesocycle-eligible
// activity types — "VIV Training Algorithm — Design Overview" §14: once a
// user starts a strength block for a given muscle-group slot, the specific
// exercises stay the same for a few weeks (a "mesocycle") rather than
// being re-picked every session, so progressive overload has something
// stable to build on. Depends on VIV-101 (internal/core/activity) for
// activity.ID and activity.MuscleGroup — explored before writing this.
//
// This package is pure logic: no persistence, no I/O. Loading/saving the
// pinned state per user lives in the usecase layer (ExercisePinService),
// which this package's ResolvePin is designed to be trivially wrapped by.
package mesocycle

import (
	"fmt"
	"time"

	"viv/internal/core/activity"
)

// StrengthMesocycleLengthDays is how long a pinned exercise set stays
// active before rotating. A starting default (Design Overview §14), NOT a
// clinically validated value — change this constant to retune it; nothing
// else in this package hardcodes 28.
const StrengthMesocycleLengthDays = 28

// FunctionalMesocycleEnabled toggles whether Functional sessions get the
// same exercise-pinning treatment as Strength. Off by default: Design
// Overview §14 flags Functional as a candidate for mesocycle treatment,
// but this is pending confirmation, not a settled decision. Flip this once
// it's confirmed — IsMesocycleEligible is the only place that reads it.
//
// HIIT is deliberately NOT represented here at all, not even as a
// disabled flag — whether HIIT sessions even decompose into a "pinned set
// of exercises" the way Strength/Functional do, and who curates that
// content, is an open item for whoever owns the HIIT library. Don't add
// it without that answer.
var FunctionalMesocycleEnabled = false

// IsMesocycleEligible reports whether activityType gets exercise pinning
// at all.
func IsMesocycleEligible(activityType activity.ID) bool {
	switch activityType {
	case activity.Strength:
		return true
	case activity.Functional:
		return FunctionalMesocycleEnabled
	default:
		return false
	}
}

// mesocycleLengthDays returns the cycle length for an eligible activity
// type. Functional shares Strength's constant for now, since no separate
// value has ever been specified for it — introduce a second named
// constant here (not a magic number) if Functional ever needs its own.
func mesocycleLengthDays(activityType activity.ID) int {
	return StrengthMesocycleLengthDays
}

// ExerciseID identifies one exercise in the (not-yet-fully-built) content
// library.
type ExerciseID string

// PinnedExerciseSet is the persisted state for one user's one
// muscle-group slot (Lower/Upper/FullBody/Core — activity.MuscleGroup, no
// new enum needed), for one mesocycle-eligible activity type.
type PinnedExerciseSet struct {
	ActivityType activity.ID
	MuscleGroup  activity.MuscleGroup
	ExerciseIDs  []ExerciseID
	ActivatedOn  time.Time
	RotatesOn    time.Time
}

// ExerciseLibrary is what SelectExerciseSet needs from the content
// library — real curated exercise content doesn't exist yet (the same gap
// cascade.ContentLibrary's stub documents for session variants), so this
// is deliberately minimal: just "what exercises exist for this muscle
// group."
type ExerciseLibrary interface {
	ExercisesFor(muscleGroup activity.MuscleGroup) []ExerciseID
}

// ExerciseSetSelector picks which exercises make up a pinned set for a
// muscle-group slot. Modeled as an interface specifically so real curated
// selection logic (progression, equipment, variety-vs-repetition
// trade-offs) can replace StubExerciseSetSelector later without touching
// ResolvePin or its callers.
type ExerciseSetSelector interface {
	SelectExerciseSet(muscleGroup activity.MuscleGroup, library ExerciseLibrary) []ExerciseID
}

// StubExerciseLibrary is a placeholder: three deterministically-named
// exercise IDs per muscle group, just enough for the pinning logic to be
// exercised end-to-end and tested today. Real content curation is a
// separate, not-yet-scoped piece of work — swap this out then.
type StubExerciseLibrary struct{}

func (StubExerciseLibrary) ExercisesFor(mg activity.MuscleGroup) []ExerciseID {
	return []ExerciseID{
		ExerciseID(string(mg) + "_exercise_1"),
		ExerciseID(string(mg) + "_exercise_2"),
		ExerciseID(string(mg) + "_exercise_3"),
	}
}

// StubExerciseSetSelector takes everything the library offers for the
// muscle group — no real curation exists yet.
type StubExerciseSetSelector struct{}

func (StubExerciseSetSelector) SelectExerciseSet(mg activity.MuscleGroup, library ExerciseLibrary) []ExerciseID {
	return library.ExercisesFor(mg)
}

// ResolvePin decides what a user's pin for (activityType, muscleGroup)
// should be as of today, given whatever pin already exists (nil if none):
//
//   - existing == nil: create one — select a fresh set, ActivatedOn =
//     today, RotatesOn = today + the type's mesocycle length.
//   - today >= existing.RotatesOn: rotate — same as creating, a fresh
//     selection with reset dates.
//   - otherwise: reuse existing completely unchanged.
//
// changed is true exactly when a new PinnedExerciseSet was produced
// (created or rotated) — the caller's signal that this needs to be
// persisted; reuse never returns changed=true, so a caller can skip a
// write on every ordinary session.
//
// Pure/no I/O: loading `existing` and persisting the result are the
// caller's job (see usecase.ExercisePinService).
func ResolvePin(
	existing *PinnedExerciseSet,
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
	today time.Time,
	selector ExerciseSetSelector,
	library ExerciseLibrary,
) (pin PinnedExerciseSet, changed bool, err error) {
	if !IsMesocycleEligible(activityType) {
		return PinnedExerciseSet{}, false, fmt.Errorf("mesocycle: %q is not mesocycle-eligible", activityType)
	}

	if existing == nil {
		return newPin(activityType, muscleGroup, today, selector, library), true, nil
	}

	if existing.ActivityType != activityType || existing.MuscleGroup != muscleGroup {
		return PinnedExerciseSet{}, false, fmt.Errorf(
			"mesocycle: existing pin is for %s/%s, not %s/%s",
			existing.ActivityType, existing.MuscleGroup, activityType, muscleGroup,
		)
	}

	if !today.Before(existing.RotatesOn) { // today >= RotatesOn
		return newPin(activityType, muscleGroup, today, selector, library), true, nil
	}

	return *existing, false, nil
}

func newPin(
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
	today time.Time,
	selector ExerciseSetSelector,
	library ExerciseLibrary,
) PinnedExerciseSet {
	return PinnedExerciseSet{
		ActivityType: activityType,
		MuscleGroup:  muscleGroup,
		ExerciseIDs:  selector.SelectExerciseSet(muscleGroup, library),
		ActivatedOn:  today,
		RotatesOn:    today.AddDate(0, 0, mesocycleLengthDays(activityType)),
	}
}
