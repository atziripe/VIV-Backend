// Package mesocycle implements exercise pinning for mesocycle-eligible
// activity types — "VIV Training Algorithm — Design Overview" §14: once a
// user starts a strength block for a given muscle-group slot, the specific
// exercises stay the same for a few weeks (a "mesocycle") rather than
// being re-picked every session, so progressive overload has something
// stable to build on.
//
// VIV-108 real implementation. Explored VIV-101 (internal/core/activity,
// for activity.ID/activity.MuscleGroup), VIV-106 (usecase.GenerateWeeklyPlanUsecase)
// and VIV-117 (internal/core/strengthlibrary — the reviewed Strength
// movement catalog: Zone/MovementGroup/ReviewStatus) before writing this.
// Replaces the original StubExerciseLibrary/StubExerciseSetSelector
// placeholder (three deterministically-named fake IDs per muscle group)
// with real selection driven by strengthlibrary's approved catalog and
// VIV_Strength_Block_Config.json's slot/rotation-policy config.
//
// This package is pure logic: no persistence, no I/O. Loading/saving the
// pinned state per user lives in the usecase layer (ExercisePinService),
// which this package's ResolvePin is designed to be trivially wrapped by.
package mesocycle

import (
	"fmt"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/strengthlibrary"
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

// ExerciseID identifies one exercise — a strengthlibrary.Exercise.ID
// string, wrapped in this package's own named type so a caller (e.g.
// usecase.ExercisePinResolver) depends on mesocycle, not on
// strengthlibrary directly, for this value's type.
type ExerciseID string

// PinnedExerciseSet is the persisted state for one user's one
// muscle-group slot (Lower/Upper/FullBody/Core — activity.MuscleGroup, no
// new enum needed), for one mesocycle-eligible activity type.
type PinnedExerciseSet struct {
	ActivityType activity.ID
	MuscleGroup  activity.MuscleGroup

	// ActiveExercises holds exactly one exercise ID per MovementGroup —
	// keyed by the SELECTED exercise's own native MovementGroup, not by
	// Slot name, so a slot that merges several groups (e.g. Full body's
	// "Pull") still produces an unambiguous, single-exercise entry per
	// group it actually picked from this block. See BlockExerciseSelector.
	ActiveExercises map[strengthlibrary.MovementGroup]ExerciseID

	// BlockNumber starts at 1 and increments by exactly 1 every time
	// ResolvePin rotates — rotation (round-robin position in a slot's
	// pool) is derived from this number, never stored per exercise.
	BlockNumber int

	ActivatedOn time.Time
	RotatesOn   time.Time
}

// BlockExerciseSelector picks one exercise per configured slot for
// (zone, blockNumber) — Block 1 always takes each slot's first approved
// exercise; later blocks hold (Main) or round-robin-advance (Accessory/
// Optional, or any slot with an explicit override) per config's
// rotation_policy. Modeled as an interface so the policy can change (or
// be swapped entirely) without touching ResolvePin.
//
// thinSlots lists every slot whose approved pool has exactly one
// exercise (so it necessarily repeats every block) — purely
// informational, for the caller to log; never an error by itself.
type BlockExerciseSelector interface {
	SelectBlockExercises(
		zone strengthlibrary.Zone,
		blockNumber int,
		library *strengthlibrary.Library,
		config *strengthlibrary.BlockConfig,
	) (bySlot map[string]strengthlibrary.Exercise, thinSlots []string, err error)
}

// DefaultBlockExerciseSelector is the real (non-stub) selection policy:
// deterministic, config-driven slot-pool rotation — never random, never
// a hand-curated set. See SelectBlockExercises.
type DefaultBlockExerciseSelector struct{}

// SelectBlockExercises implements BlockExerciseSelector.
//
// For each of zone's configured slots: build its candidate pool via
// strengthlibrary.BuildSlotPool, which implements
// VIV_Strength_Block_Config.json's real ordering_rule (ascending by
// (technical, balance) demand, ties interleaved across the slot's
// source movement groups) — never re-sorted here. Then:
//
//   - a Main slot (or any slot BlockConfig.HoldsForever says holds)
//     always takes pool[0], for every block — "the Block 1 exercise
//     stays in every later block."
//   - any other slot takes pool[((blockNumber-1)/period) % len(pool)],
//     where period is BlockConfig.RotationPeriodBlocks(slot) — this is
//     exactly round-robin, advancing one pool position every `period`
//     blocks, wrapping back to the start once the pool is exhausted.
//
// An empty pool (no approved exercises at all for the slot) is a hard
// error — nothing to pin. A single-exercise pool is NOT an error: index
// math above always lands on 0, so it simply repeats; its slot name is
// added to thinSlots instead.
func (DefaultBlockExerciseSelector) SelectBlockExercises(
	zone strengthlibrary.Zone,
	blockNumber int,
	library *strengthlibrary.Library,
	config *strengthlibrary.BlockConfig,
) (map[string]strengthlibrary.Exercise, []string, error) {
	slots := config.SlotsForZone(zone)
	if len(slots) == 0 {
		return nil, nil, fmt.Errorf("mesocycle: no block-config slots defined for zone %q", zone)
	}

	approvedByGroup := library.ApprovedExercises(zone)

	bySlot := make(map[string]strengthlibrary.Exercise, len(slots))
	var thinSlots []string
	for _, slot := range slots {
		pool := strengthlibrary.BuildSlotPool(slot, approvedByGroup)
		if len(pool) == 0 {
			return nil, nil, fmt.Errorf("mesocycle: slot %q (zone %q) has no approved exercises to select from", slot.Name, zone)
		}
		if len(pool) == 1 {
			thinSlots = append(thinSlots, slot.Name)
		}

		idx := 0
		if !config.HoldsForever(slot) {
			period := config.RotationPeriodBlocks(slot)
			idx = ((blockNumber - 1) / period) % len(pool)
		}
		bySlot[slot.Name] = pool[idx]
	}

	return bySlot, thinSlots, nil
}

// ResolvePin decides what a user's pin for (activityType, muscleGroup)
// should be as of today, given whatever pin already exists (nil if none):
//
//   - existing == nil: create one — block 1, select, ActivatedOn =
//     today, RotatesOn = today + the type's mesocycle length.
//   - today >= existing.RotatesOn: rotate — select the NEXT block (never
//     re-select block 1), reset ActivatedOn/RotatesOn.
//   - otherwise: reuse existing completely unchanged — this is the path
//     every ordinary session takes, regardless of whether it came from
//     weekly generation (VIV-106) or daily adaptation (VIV-107): an
//     Intensity/Impact/LoadTier adjustment from the cascade (VIV-105)
//     never reaches this function at all, since this lookup key is only
//     (activityType, muscleGroup).
//
// changed is true exactly when a new PinnedExerciseSet was produced
// (created or rotated) — the caller's signal that this needs to be
// persisted; reuse never returns changed=true, so a caller can skip a
// write on every ordinary session. thinSlots is only populated when
// changed is true (nothing was (re)selected on a pure reuse).
//
// Pure/no I/O: loading `existing` and persisting the result, and any
// logging of thinSlots, are the caller's job (see
// usecase.ExercisePinService).
func ResolvePin(
	existing *PinnedExerciseSet,
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
	today time.Time,
	selector BlockExerciseSelector,
	library *strengthlibrary.Library,
	config *strengthlibrary.BlockConfig,
) (pin PinnedExerciseSet, changed bool, thinSlots []string, err error) {
	if !IsMesocycleEligible(activityType) {
		return PinnedExerciseSet{}, false, nil, fmt.Errorf("mesocycle: %q is not mesocycle-eligible", activityType)
	}
	zone, ok := strengthlibrary.ZoneFromMuscleGroup(muscleGroup)
	if !ok {
		return PinnedExerciseSet{}, false, nil, fmt.Errorf("mesocycle: muscle group %q has no corresponding strength zone", muscleGroup)
	}

	if existing == nil {
		pin, thin, err := newPin(activityType, muscleGroup, zone, 1, today, selector, library, config)
		return pin, true, thin, err
	}

	if existing.ActivityType != activityType || existing.MuscleGroup != muscleGroup {
		return PinnedExerciseSet{}, false, nil, fmt.Errorf(
			"mesocycle: existing pin is for %s/%s, not %s/%s",
			existing.ActivityType, existing.MuscleGroup, activityType, muscleGroup,
		)
	}

	if !today.Before(existing.RotatesOn) { // today >= RotatesOn
		pin, thin, err := newPin(activityType, muscleGroup, zone, existing.BlockNumber+1, today, selector, library, config)
		return pin, true, thin, err
	}

	return *existing, false, nil, nil
}

func newPin(
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
	zone strengthlibrary.Zone,
	blockNumber int,
	today time.Time,
	selector BlockExerciseSelector,
	library *strengthlibrary.Library,
	config *strengthlibrary.BlockConfig,
) (PinnedExerciseSet, []string, error) {
	bySlot, thinSlots, err := selector.SelectBlockExercises(zone, blockNumber, library, config)
	if err != nil {
		return PinnedExerciseSet{}, nil, fmt.Errorf("mesocycle: selecting block %d exercises for zone %q: %w", blockNumber, zone, err)
	}

	active := make(map[strengthlibrary.MovementGroup]ExerciseID, len(bySlot))
	for _, exercise := range bySlot {
		active[exercise.MovementGroup] = ExerciseID(exercise.ID)
	}

	return PinnedExerciseSet{
		ActivityType:    activityType,
		MuscleGroup:     muscleGroup,
		ActiveExercises: active,
		BlockNumber:     blockNumber,
		ActivatedOn:     today,
		RotatesOn:       today.AddDate(0, 0, mesocycleLengthDays(activityType)),
	}, thinSlots, nil
}
