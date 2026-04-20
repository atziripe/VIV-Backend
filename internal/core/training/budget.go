package training

import (
	"viv/internal/core/domain"
)

// ============================================================================
// PHASE x MODALITY MATRIX — from the spec
// ============================================================================
// Each entry defines the max sessions per modality per phase.
// These are hard ceilings — the budget never exceeds them.

type phaseModalityLimit struct {
	Strength int
	HIIT     int
	Pilates  int
	Cardio   int
	MaxTotal int //advisory warning
}

var phaseLimits = map[domain.CyclePhase]phaseModalityLimit{
	domain.PhaseMenstrual:   {Strength: 1, HIIT: 0, Pilates: 3, Cardio: 1, MaxTotal: 3},
	domain.PhaseFollicular:  {Strength: 3, HIIT: 2, Pilates: 2, Cardio: 2, MaxTotal: 5},
	domain.PhaseOvulatory:   {Strength: 3, HIIT: 1, Pilates: 1, Cardio: 2, MaxTotal: 5},
	domain.PhaseEarlyLuteal: {Strength: 3, HIIT: 1, Pilates: 2, Cardio: 1, MaxTotal: 4},
	domain.PhaseLateLuteal:  {Strength: 2, HIIT: 0, Pilates: 2, Cardio: 1, MaxTotal: 3},
}

// Minimum strength sessions VIV recommends regardless of user preference.
// Lower in menstrual/late_luteal to avoid being restrictive.
var minStrengthByPhase = map[domain.CyclePhase]int{
	domain.PhaseMenstrual:   1,
	domain.PhaseFollicular:  2,
	domain.PhaseOvulatory:   2,
	domain.PhaseEarlyLuteal: 2,
	domain.PhaseLateLuteal:  1,
}

// ============================================================================
// CALCULATOR
// ============================================================================

// CalculateBudget computes the weekly session budget.
//
// Key design decision: the phase matrix does NOT cap total sessions.
// The user's requested frequency is respected. The matrix influences
// distribution (which modalities get how many slots) and generates
// advisory warnings when exceeded.
//
// The only hard exclusion is HIIT = 0 in menstrual/late_luteal.
func CalculateBudget(
	prefs ParsedTrainingPreferences,
	phase domain.CyclePhase,
	dimensions domain.CheckinDimensions,
) domain.WeekBudget {
	limits := phaseLimits[phase]

	// Step 1: Start with user's requested sessions, cap by phase max
	totalSessions := prefs.SessionsPerWeek

	// Step 2: Reduce based on check-in dimensions
	totalSessions = applyDimensionReductions(totalSessions, dimensions)

	// Step 3: Intersect user's selected modalities with phase-allowed modalities
	available := filterAvailableModalities(prefs.Modalities, limits)

	// Step 4: Check if Strength needs to be injected
	userSelectedStrength := hasModality(prefs.Modalities, domain.ModalityStrength)

	// Step 5: Distribute sessions across available modalities
	modalities := distributeModalities(available, totalSessions, limits, phase, dimensions, userSelectedStrength)

	// Step 6: Determine rest week flag
	restWeekOffered := dimensions.RestWeekRecommended()

	return domain.WeekBudget{
		TotalSessions:   totalSessions,
		Modalities:      modalities,
		DurationMinutes: prefs.DurationMinutes,
		RestWeekOffered: restWeekOffered,
	}
}

// applyDimensionReductions lowers the session count when the user's
// check-in signals low capacity. Maintains count but adjusts down
// only in extreme cases.
//
// Policy: maintain session count but lower intensity (user's choice).
// Only reduce count when BOTH recovery AND bandwidth are low.
func applyDimensionReductions(total int, dims domain.CheckinDimensions) int {
	if dims.Recovery == domain.RecoveryLow && dims.Bandwidth == domain.BandwidthLow {
		total--
	}

	// Floor: never go below 1 session
	if total < 1 {
		total = 1
	}

	return total
}

// filterAvailableModalities returns only the modalities the user selected
// that are also allowed in the current phase.
func filterAvailableModalities(selected []domain.Modality, limits phaseModalityLimit) []domain.Modality {
	var available []domain.Modality

	for _, m := range selected {
		switch m {
		case domain.ModalityStrength:
			if limits.Strength > 0 {
				available = append(available, m)
			}
		case domain.ModalityHIIT:
			if limits.HIIT > 0 {
				available = append(available, m)
			}
		case domain.ModalityPilates:
			if limits.Pilates > 0 {
				available = append(available, m)
			}
		case domain.ModalityCardio:
			if limits.Cardio > 0 {
				available = append(available, m)
			}
		}
	}

	return available
}

// distributeModalities allocates sessions across available modalities
// respecting phase limits and check-in dimensions.
//
// Priority order:
//  1. Strength gets slots first (primary training modality for most goals)
//  2. HIIT gets slots if available and recovery allows
//  3. Remaining slots filled with Pilates and Cardio
//
// This is intentionally simple. The LLM distributes across days;
// this function only decides HOW MANY of each.
func distributeModalities(
	available []domain.Modality,
	totalSessions int,
	limits phaseModalityLimit,
	phase domain.CyclePhase,
	dims domain.CheckinDimensions,
	userSelectedStrength bool,
) []domain.ModalityBudget {
	if totalSessions == 0 {
		return nil
	}

	remaining := totalSessions
	var result []domain.ModalityBudget

	has := func(m domain.Modality) bool {
		for _, a := range available {
			if a == m {
				return true
			}
		}
		return false
	}

	// Strength allocation
	if has(domain.ModalityStrength) {
		count := min(limits.Strength, remaining)

		// Determine intensity override
		var intensityOverride *domain.Intensity
		if phase == domain.PhaseLateLuteal || phase == domain.PhaseMenstrual {
			moderate := domain.IntensityModerate
			intensityOverride = &moderate
		}
		if dims.Recovery == domain.RecoveryLow {
			moderate := domain.IntensityModerate
			intensityOverride = &moderate
		}

		result = append(result, domain.ModalityBudget{
			Modality:          domain.ModalityStrength,
			Count:             count,
			IntensityOverride: intensityOverride,
		})
		remaining -= count
	} else if !userSelectedStrength && remaining > 1 && limits.Strength > 0 {
		// User didn't select Strength — VIV injects minimum
		minStrength := minStrengthByPhase[phase]
		count := min(minStrength, remaining-1) // leave at least 1 slot for what she chose

		low := domain.IntensityLow
		result = append(result, domain.ModalityBudget{
			Modality:          domain.ModalityStrength,
			Count:             count,
			IntensityOverride: &low,
			VivRecommended:    true,
		})
		remaining -= count
	}

	// HIIT allocation — skip if recovery is low
	if has(domain.ModalityHIIT) && remaining > 0 && dims.Recovery != domain.RecoveryLow {
		count := min(limits.HIIT, remaining)
		result = append(result, domain.ModalityBudget{
			Modality: domain.ModalityHIIT,
			Count:    count,
		})
		remaining -= count
	}

	// Cardio allocation
	if has(domain.ModalityCardio) && remaining > 0 {
		count := min(limits.Cardio, remaining)

		var intensityOverride *domain.Intensity
		if phase == domain.PhaseLateLuteal || phase == domain.PhaseMenstrual {
			low := domain.IntensityLow
			intensityOverride = &low
		}

		result = append(result, domain.ModalityBudget{
			Modality:          domain.ModalityCardio,
			Count:             count,
			IntensityOverride: intensityOverride,
		})
		remaining -= count
	}

	// Pilates fills remaining slots (recovery filler per spec Rule 4)
	if has(domain.ModalityPilates) && remaining > 0 {
		count := min(limits.Pilates, remaining)
		result = append(result, domain.ModalityBudget{
			Modality: domain.ModalityPilates,
			Count:    count,
		})
		remaining -= count
	}

	// If there are still remaining slots and we have modalities,
	// distribute back to available modalities (can happen when
	// phase limits per modality are low but total is higher)
	if remaining > 0 {
		for i := range result {
			if remaining == 0 {
				break
			}
			limit := modalityLimit(result[i].Modality, limits)
			if result[i].Count < limit {
				add := min(limit-result[i].Count, remaining)
				result[i].Count += add
				remaining -= add
			}
		}
	}

	return result
}

func hasModality(mods []domain.Modality, target domain.Modality) bool {
	for _, m := range mods {
		if m == target {
			return true
		}
	}
	return false
}

// modalityLimit returns the phase limit for a specific modality.
func modalityLimit(m domain.Modality, limits phaseModalityLimit) int {
	switch m {
	case domain.ModalityStrength:
		return limits.Strength
	case domain.ModalityHIIT:
		return limits.HIIT
	case domain.ModalityPilates:
		return limits.Pilates
	case domain.ModalityCardio:
		return limits.Cardio
	default:
		return 0
	}
}
