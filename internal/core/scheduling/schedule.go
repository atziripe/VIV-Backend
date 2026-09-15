// Package scheduling implements VIV-109, design doc's "Layer 1": given a
// week's worth of already-resolved cascade.SlotAssignment (VIV-105/106
// output), decide which day each one goes on — nothing else. Depends on
// VIV-105 (internal/core/cascade) for the slot shape — explored the
// existing Layer 1 implementation (internal/adapters/llm/openai/
// training_structure.go) before writing this.
//
// That existing implementation asks an LLM to INVENT a week's modality/
// muscle_group/intensity/duration from a session budget — real freedom,
// appropriate for the old rule-engine pipeline it serves. This package is
// deliberately narrower: every slot's content is already final by the
// time scheduling runs (VIV-105 decided it), so the only decision left is
// day placement, and the hard validation below (ValidateAndApply) makes
// it structurally impossible for a scheduling response to change slot
// content even if it tried — never something left to prompt wording alone.
package scheduling

import (
	"fmt"
	"strings"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
)

// ============================================================================
// Recovery-cost-driven muscle-group spacing
// ============================================================================

// RestDaysRequired maps a RecoveryCost value (0-3, from activity.Type.
// RecoveryCost — VIV-101's first-draft seed data) to how many rest days
// must separate two sessions targeting the same primary MuscleGroup.
//
// This mapping is itself still a first-draft judgment call, not confirmed
// by Juli/Lina — grounded in the 48-72h neuromuscular fatigue window
// found in the literature review for high-recovery-cost activities
// (competitive team/racket sports, high-intensity strength work). See
// "Training Effect & Recovery Cost — First Draft." A configurable table,
// not inline conditionals, so retuning it is a one-place change.
var RestDaysRequired = map[int]int{
	0: 0,
	1: 0,
	2: 1,
	3: 2,
}

// RestDaysRequiredFor looks up RestDaysRequired, erroring on a
// RecoveryCost value the table doesn't cover rather than silently
// assuming "no constraint" — RecoveryCost is always one of 0-3 by
// construction (activity.Type.RecoveryCost), so an unrecognized value
// here means something upstream is wrong and should be surfaced, not
// papered over.
func RestDaysRequiredFor(recoveryCost int) (int, error) {
	days, ok := RestDaysRequired[recoveryCost]
	if !ok {
		return 0, fmt.Errorf("scheduling: no RestDaysRequired entry for RecoveryCost %d", recoveryCost)
	}
	return days, nil
}

// ============================================================================
// Spacing constraint check (soft — used to prompt/retry, not the hard gate)
// ============================================================================

// DayPlacement is one day's outcome after scheduling.
type DayPlacement struct {
	IsRestDay  bool
	Assignment cascade.SlotAssignment
}

// ViolationRule identifies which spacing constraint a Violation broke.
type ViolationRule string

const (
	RuleConsecutiveHighImpact ViolationRule = "consecutive_high_impact"
	RuleRecoveryGap           ViolationRule = "recovery_gap"
)

// Violation describes one spacing-rule breach. AffectedDays holds the
// day-indices (0-6, Monday-Sunday within the scheduling week) involved —
// same convention as the old rule engine's Violation.AffectedDays
// (internal/core/rules/decision.go), kept as indices rather than a
// pre-formatted string so a caller with real calendar context (VIV-110's
// RuleEngineValidator) can build a weekday-named message, while a caller
// without one can still log/compare structurally.
type Violation struct {
	Rule         ViolationRule
	AffectedDays []int
	MuscleGroup  activity.MuscleGroup // set only for RuleRecoveryGap
}

// CheckSpacing validates both spacing constraints over a finalized 7-day
// placement and returns every violation found (not just the first), so a
// caller can retry with concrete feedback — mirroring how the old Layer 1
// pipeline retries once against rule-engine violations
// (usecase.GenerateTrainingPlanUsecase).
func CheckSpacing(days [7]DayPlacement) []Violation {
	var violations []Violation

	// No two consecutive days with Impact = High.
	for i := 0; i < 6; i++ {
		a, b := days[i], days[i+1]
		if a.IsRestDay || b.IsRestDay {
			continue
		}
		if a.Assignment.Impact == activity.ImpactH && b.Assignment.Impact == activity.ImpactH {
			violations = append(violations, Violation{
				Rule:         RuleConsecutiveHighImpact,
				AffectedDays: []int{i, i + 1},
			})
		}
	}

	// Recovery-cost-driven muscle-group gap: for each muscle group, walk
	// its occurrences in day order and check every consecutive pair —
	// non-consecutive occurrences are automatically fine if their
	// consecutive neighbors already satisfy the gap.
	byMuscleGroup := map[activity.MuscleGroup][]int{}
	for i, d := range days {
		if d.IsRestDay {
			continue
		}
		byMuscleGroup[d.Assignment.MuscleGroup] = append(byMuscleGroup[d.Assignment.MuscleGroup], i)
	}

	for mg, indices := range byMuscleGroup {
		for k := 0; k+1 < len(indices); k++ {
			i, j := indices[k], indices[k+1]

			t, ok := activity.ByID(days[i].Assignment.ActivityType)
			if !ok {
				continue
			}
			cost, ok := t.RecoveryCost[days[i].Assignment.Intensity]
			if !ok {
				continue
			}
			required, err := RestDaysRequiredFor(cost)
			if err != nil {
				continue
			}

			gap := j - i - 1
			if gap < required {
				violations = append(violations, Violation{
					Rule:         RuleRecoveryGap,
					AffectedDays: []int{i, j},
					MuscleGroup:  mg,
				})
			}
		}
	}

	return violations
}

// ============================================================================
// Hard backend validation — the mandatory check
// ============================================================================

// Placement is one entry of the LLM's raw scheduling response. It
// deliberately echoes the slot's already-resolved attributes rather than
// a bare index+weekday: not extra freedom, the opposite of it — the LLM
// restates what it was given so ValidateAndApply can catch a changed
// value explicitly (a loud, specific rejection) instead of only ever
// trusting SlotIndex as a reference. Either way, ValidateAndApply never
// actually uses these echoed fields to build its output — see the field
// comment on ActivityType below.
type Placement struct {
	SlotIndex int
	Weekday   string // lowercase, "monday".."sunday"

	// ActivityType, Intensity, Impact, and MuscleGroup are checked against
	// the real resolved slot at SlotIndex and REJECTED if they don't
	// match exactly — but even a match is never used to build the output.
	// ValidateAndApply always copies from `original`, never from here.
	// This is the actual enforcement; the equality check exists only to
	// make a mismatch loud instead of silently discarded.
	ActivityType activity.ID
	Intensity    activity.IntensityLevel
	Impact       activity.ImpactLevel
	MuscleGroup  activity.MuscleGroup
}

var weekdayIndex = map[string]int{
	"monday": 0, "tuesday": 1, "wednesday": 2, "thursday": 3, "friday": 4, "saturday": 5, "sunday": 6,
}

// ValidateAndApply is the mandatory hard backend check: the LLM's only
// allowed output is which day each already-resolved slot goes on. This
// is not something left to prompt instructions alone — every rule below
// is enforced in Go, regardless of what the LLM's raw response claims.
//
// original is the week's resolved slots (VIV-105 output, rest days
// excluded); restDayCount is how many of the 7 days have no slot at all.
// proposed is the LLM's raw response, one entry per resolved slot.
//
// Rejects (and returns an error, never a partial result) if the proposal:
//   - has a slot count that doesn't match len(original) (drops or invents
//     placements),
//   - references a SlotIndex outside [0, len(original)) (invents a slot),
//   - references the same SlotIndex or weekday twice,
//   - names an invalid weekday, or
//   - echoes ActivityType/Intensity/Impact/MuscleGroup that don't exactly
//     match the real resolved slot at that index (attempts to modify a
//     slot).
//
// On success, returns the finalized 7-day placement built ENTIRELY from
// `original` — the LLM's echoed attribute fields are never copied into
// the result, even when they matched.
func ValidateAndApply(original []cascade.SlotAssignment, restDayCount int, proposed []Placement) ([7]DayPlacement, error) {
	var days [7]DayPlacement

	if len(original)+restDayCount != 7 {
		return days, fmt.Errorf("scheduling: %d resolved slot(s) + %d rest day(s) != 7", len(original), restDayCount)
	}
	if len(proposed) != len(original) {
		return days, fmt.Errorf("scheduling: proposed %d placement(s), want exactly %d — one per resolved slot, no more, no fewer", len(proposed), len(original))
	}

	usedSlots := make(map[int]bool, len(original))
	usedDays := make(map[int]bool, len(original))

	for _, p := range proposed {
		if p.SlotIndex < 0 || p.SlotIndex >= len(original) {
			return [7]DayPlacement{}, fmt.Errorf(
				"scheduling: proposed slot index %d is out of range (%d resolved slots exist) — the LLM may not invent slots",
				p.SlotIndex, len(original),
			)
		}
		if usedSlots[p.SlotIndex] {
			return [7]DayPlacement{}, fmt.Errorf("scheduling: slot index %d was placed more than once", p.SlotIndex)
		}
		usedSlots[p.SlotIndex] = true

		want := original[p.SlotIndex]
		if p.ActivityType != want.ActivityType || p.Intensity != want.Intensity ||
			p.Impact != want.Impact || p.MuscleGroup != want.MuscleGroup {
			return [7]DayPlacement{}, fmt.Errorf(
				"scheduling: slot %d's echoed attributes (%s/%s/%s/%s) don't match the resolved assignment (%s/%s/%s/%s) — scheduling may only choose which day a slot goes on, never change the slot itself",
				p.SlotIndex, p.ActivityType, p.Intensity, p.Impact, p.MuscleGroup,
				want.ActivityType, want.Intensity, want.Impact, want.MuscleGroup,
			)
		}

		dayIdx, ok := weekdayIndex[strings.ToLower(p.Weekday)]
		if !ok {
			return [7]DayPlacement{}, fmt.Errorf("scheduling: invalid weekday %q", p.Weekday)
		}
		if usedDays[dayIdx] {
			return [7]DayPlacement{}, fmt.Errorf("scheduling: weekday %q was assigned more than once", p.Weekday)
		}
		usedDays[dayIdx] = true

		// Built from `want` (original), never from `p` — see the type doc.
		days[dayIdx] = DayPlacement{IsRestDay: false, Assignment: want}
	}

	if len(usedSlots) != len(original) {
		return [7]DayPlacement{}, fmt.Errorf(
			"scheduling: only %d of %d resolved slots were placed — the LLM may not drop slots",
			len(usedSlots), len(original),
		)
	}

	for i := range days {
		if !usedDays[i] {
			days[i] = DayPlacement{IsRestDay: true}
		}
	}

	return days, nil
}
