// Package strengthsession is VIV-112's real Layer 3 content assembly
// for Strength only — every other activity type keeps being served by
// the existing internal/core/content session library untouched. Depends
// on VIV-105 (internal/core/cascade, for SlotAssignment's tier fields),
// VIV-108 (internal/core/mesocycle, for the pinned block/BlockExerciseSelector)
// and VIV-117 (internal/core/strengthlibrary, for the catalog and
// VIV_Strength_Block_Config.json's exercise_types/weekly_prescription_by_type/
// variant_rules) — explored all three, plus the existing
// usecase.SessionContentSelector (Layer 3 dispatch), before writing this.
//
// Assemble is pure logic: no persistence, no I/O, no logging — exactly
// the same convention as cascade and mesocycle. The caller (VIV-112's
// usecase wiring) decides what to persist and what to log (e.g. a High
// intensity downgrade, or a thin-slot report from the selector it
// passes in).
package strengthsession

import (
	"fmt"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
)

// AssembledExercise is one line of a Strength session's output —
// exactly the fields the task spec asks for: slot, priority, exercise
// name, equipment, prescription text. Deliberately no exercise ID and
// no decomposed sets/reps/RIR numbers: the prescription is a single
// piece of config-authored text, kept as written (see PrescriptionText's
// doc comment) — real-time set-by-set logging against this shape is a
// separate, not-yet-scoped concern.
type AssembledExercise struct {
	// ExerciseID is strengthlibrary.Exercise.ID — stable across blocks
	// and re-imports, and the same identity usecase.LogSetUseCase
	// expects in its exercise_id — so a client can log sets against
	// this exercise with no extra lookup.
	ExerciseID       string
	Slot             string
	Priority         strengthlibrary.SlotPriority
	ExerciseName     string
	Equipment        []string
	PrescriptionText string

	// Sets is the leading set count parsed from PrescriptionText's own
	// "N ×" opening (after any tier adjustment — e.g. Intensity=Low's
	// SetsDelta already applied) — never a separate, independently
	// authored number. It exists so a client knows how many set rows to
	// render, and so LogSetUseCase has an integer to validate
	// SetNumber against, without the client (or the log-set gate)
	// parsing PrescriptionText itself.
	Sets int
}

// TierChange is one plain-data record of a cascade (VIV-105) tier
// decision that actually changed something about this session —
// "Include which tier changes were applied, as plain data, so the UI
// can say why." Lever is a short machine-readable tag a client can
// switch on; Detail is the human-readable explanation.
type TierChange struct {
	Lever  string
	Detail string
}

const (
	LeverDurationShort           = "duration_short"
	LeverLoadReduced             = "load_reduced"
	LeverIntensityHighDowngraded = "intensity_high_downgraded"
)

// AssembledWarmupCooldown is one zone's warm-up or cool-down —
// strengthlibrary.WarmupCooldown's two halves, each carried separately
// since a session shows them as two distinct blocks. Movements are the
// config's own plain-text lines, kept exactly as written — never
// parsed into sets/reps, same convention as AssembledExercise's
// PrescriptionText.
type AssembledWarmupCooldown struct {
	DurationMinutes int
	Movements       []string
}

// AssembledSession is VIV-112's full output for one Strength slot.
type AssembledSession struct {
	Exercises []AssembledExercise

	Week        int
	BlockNumber int

	// ConfigStatus is VIV_Strength_Block_Config.json's own status field
	// verbatim (e.g. "draft_not_reviewed_by_sport_scientist") — the task
	// requires this stay traceable all the way to the client; never
	// removed, never left out.
	ConfigStatus string

	TierChanges []TierChange

	// HighIntensityDowngraded is true when SlotAssignment.Intensity was
	// High and got treated as Medium (variant_rules.intensity_mapping.High) —
	// broken out as its own field (not just a TierChanges entry) so the
	// caller can log it without string-matching TierChanges.
	HighIntensityDowngraded bool

	// Warmup/Cooldown come straight from
	// config.WarmupCooldownForZone(zone) — resolved once, up front, and
	// never touched by any tier lever. warmup_cooldown.applies_to_all_tiers
	// is validated true at config load time specifically so this stays
	// true: a Short session drops exercise SLOTS only (see entries/
	// dropForShort above), never shortens the warm-up/cool-down itself.
	Warmup   AssembledWarmupCooldown
	Cooldown AssembledWarmupCooldown

	// RampUpNote is warmup_cooldown.ramp_up_note verbatim — shown once
	// per session, before the first Main exercise; never per-exercise,
	// never per-zone.
	RampUpNote string

	// WarmupCooldownStatus is warmup_cooldown._status verbatim — passed
	// through next to ConfigStatus so it stays just as traceable; never
	// removed, never left out.
	WarmupCooldownStatus string
}

// slotEntry pairs one configured slot with the exercise
// mesocycle.BlockExerciseSelector picked for it in this block.
type slotEntry struct {
	slot     strengthlibrary.Slot
	exercise strengthlibrary.Exercise
}

// Assemble builds the full Strength session for one slot assignment —
// VIV-112's entry point.
//
//  1. Zone/slots: assignment.MuscleGroup -> zone -> config.SlotsForZone,
//     in the config's own order.
//  2. Exercises: selector.SelectBlockExercises(zone, pin.BlockNumber, ...) —
//     VIV-108's own pure, deterministic function; never a fresh
//     selection, never a read of the general (non-Strength) library.
//     Only approved exercises can ever come back from it.
//  3. Week: derived from pin.ActivatedOn and today (see weekInBlock) —
//     fails loudly if it's not 1-4.
//  4. Prescription: config.TypeOf + config.PrescriptionText, verbatim,
//     plus " per side" when the config says so.
//  5. Tiers, all read from config.VariantRules() (never hardcoded):
//     DurationTier=Short drops slots (dropForShort); LoadTier=Reduced
//     substitutes week 1's text; Intensity=Low rewrites RIR/sets
//     (applyLowIntensity); Intensity=High is treated as Medium.
//     ComplexityTier=Simplified never reaches here — see
//     NewContentLibraryAdapter, which is what keeps the cascade from
//     ever setting it for Strength in the first place.
//  6. Warm-up/cool-down: config.WarmupCooldownForZone(zone), resolved
//     once up front and never touched by any tier lever (the config's
//     applies_to_all_tiers is validated true at load time for exactly
//     this reason) — a Short session still gets the full warm-up/
//     cool-down, only its exercise slots are trimmed.
func Assemble(
	assignment cascade.SlotAssignment,
	pin mesocycle.PinnedExerciseSet,
	today time.Time,
	library *strengthlibrary.Library,
	config *strengthlibrary.BlockConfig,
	selector mesocycle.BlockExerciseSelector,
) (AssembledSession, error) {
	if assignment.ActivityType != activity.Strength {
		return AssembledSession{}, fmt.Errorf("strengthsession: Assemble only supports Strength, got %q", assignment.ActivityType)
	}

	zone, ok := strengthlibrary.ZoneFromMuscleGroup(assignment.MuscleGroup)
	if !ok {
		return AssembledSession{}, fmt.Errorf("strengthsession: muscle group %q has no strength zone", assignment.MuscleGroup)
	}

	warmupCooldown, ok := config.WarmupCooldownForZone(zone)
	if !ok {
		return AssembledSession{}, fmt.Errorf("strengthsession: zone %q has no warmup_cooldown entry in the config", zone)
	}

	week, err := weekInBlock(pin.ActivatedOn, today)
	if err != nil {
		return AssembledSession{}, err
	}

	bySlot, _, err := selector.SelectBlockExercises(zone, pin.BlockNumber, library, config)
	if err != nil {
		return AssembledSession{}, fmt.Errorf("strengthsession: %w", err)
	}

	slots := config.SlotsForZone(zone)
	entries := make([]slotEntry, 0, len(slots))
	for _, slot := range slots {
		exercise, ok := bySlot[slot.Name]
		if !ok {
			return AssembledSession{}, fmt.Errorf("strengthsession: slot %q (zone %q) has no selected exercise", slot.Name, zone)
		}
		entries = append(entries, slotEntry{slot: slot, exercise: exercise})
	}

	var tierChanges []TierChange

	if assignment.DurationTier == cascade.DurationShort {
		kept, dropped := dropForShort(entries, config.VariantRules().Short)
		entries = kept
		if len(dropped) > 0 {
			tierChanges = append(tierChanges, TierChange{
				Lever:  LeverDurationShort,
				Detail: fmt.Sprintf("shortened session: dropped %s", joinDropped(dropped)),
			})
		}
	}

	prescriptionWeek := week
	if assignment.LoadTier == cascade.LoadReduced {
		prescriptionWeek = 1
		tierChanges = append(tierChanges, TierChange{
			Lever:  LeverLoadReduced,
			Detail: "using week 1's prescription for every exercise, regardless of the actual current week",
		})
	}

	intensity := assignment.Intensity
	highDowngraded := false
	if intensity == activity.IntensityH {
		intensity = activity.IntensityM
		highDowngraded = true
		tierChanges = append(tierChanges, TierChange{
			Lever:  LeverIntensityHighDowngraded,
			Detail: "High intensity isn't reviewed yet — treated as Medium",
		})
	}

	exercises := make([]AssembledExercise, 0, len(entries))
	for _, e := range entries {
		code, perSide, ok := config.TypeOf(e.exercise.Name)
		if !ok {
			return AssembledSession{}, fmt.Errorf("strengthsession: exercise %q has no type configured in exercise_types", e.exercise.Name)
		}
		text, err := config.PrescriptionText(code, prescriptionWeek)
		if err != nil {
			return AssembledSession{}, fmt.Errorf("strengthsession: exercise %q: %w", e.exercise.Name, err)
		}

		if intensity == activity.IntensityL {
			text, err = applyLowIntensity(text, config.VariantRules().IntensityLow)
			if err != nil {
				return AssembledSession{}, fmt.Errorf("strengthsession: exercise %q: %w", e.exercise.Name, err)
			}
		}

		sets, err := parseLeadingSetCount(text)
		if err != nil {
			return AssembledSession{}, fmt.Errorf("strengthsession: exercise %q: %w", e.exercise.Name, err)
		}

		if perSide {
			text += " per side"
		}

		exercises = append(exercises, AssembledExercise{
			ExerciseID:       e.exercise.ID,
			Slot:             e.slot.Name,
			Priority:         e.slot.Priority,
			ExerciseName:     e.exercise.Name,
			Equipment:        e.exercise.Equipment,
			PrescriptionText: text,
			Sets:             sets,
		})
	}

	return AssembledSession{
		Exercises:               exercises,
		Week:                    week,
		BlockNumber:             pin.BlockNumber,
		ConfigStatus:            config.Status(),
		TierChanges:             tierChanges,
		HighIntensityDowngraded: highDowngraded,
		Warmup: AssembledWarmupCooldown{
			DurationMinutes: warmupCooldown.WarmupMinutes,
			Movements:       warmupCooldown.Warmup,
		},
		Cooldown: AssembledWarmupCooldown{
			DurationMinutes: warmupCooldown.CooldownMinutes,
			Movements:       warmupCooldown.Cooldown,
		},
		RampUpNote:           config.RampUpNote(),
		WarmupCooldownStatus: config.WarmupCooldownStatus(),
	}, nil
}

// weekInBlock computes which of the 4 weeks today falls in, given the
// block's ActivatedOn date — days 0-6 = week 1, 7-13 = week 2, 14-20 =
// week 3, 21-27 = week 4 (week 4 is "consolidation," not a deload — no
// behavior difference from week 3 beyond whatever
// weekly_prescription_by_type's own week-4 text says). Errors (never a
// silent clamp) if today is before ActivatedOn, or the computed week
// falls outside 1-4 — the latter should be unreachable in practice
// since VIV-108's ResolvePin always rotates at day 28, but a stale
// caller asking with an old pin must fail loudly, not silently serve
// the wrong week's prescription.
func weekInBlock(activatedOn, today time.Time) (int, error) {
	days := int(today.Sub(activatedOn).Hours() / 24)
	if days < 0 {
		return 0, fmt.Errorf("strengthsession: today (%s) is before the block's start date (%s)", today.Format("2006-01-02"), activatedOn.Format("2006-01-02"))
	}
	week := days/7 + 1
	if week < 1 || week > 4 {
		return 0, fmt.Errorf("strengthsession: computed week %d is outside 1-4 (%d day(s) since block start %s)", week, days, activatedOn.Format("2006-01-02"))
	}
	return week, nil
}

func joinDropped(dropped []slotEntry) string {
	out := ""
	for i, e := range dropped {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%s (%s)", e.slot.Name, e.slot.Priority)
	}
	return out
}
