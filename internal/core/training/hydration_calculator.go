package training

import (
	"math"

	"viv/internal/core/domain"
)

// ============================================================================
// HYDRATION CALCULATOR
// ============================================================================
// Formula from VIV hydration spec:
//   base           = weight_kg × 0.033
//   activity_add   = (exercise_minutes / 30) × 0.35
//   phase_add      = base × phase_multiplier
//   total          = base + activity_add + phase_add
//   display        = round(total, 1)
//
// Science basis:
//   - IOM/EFSA: 33ml/kg baseline (mid-range of 30-35ml/kg)
//   - ACSM: 350ml per 30 min exercise
//   - Phase multipliers: VIV-specific, based on hormonal fluid dynamics

// HydrationResult holds the calculated hydration target for a single day.
type HydrationResult struct {
	Liters          float64 `json:"liters"`
	Glasses         int     `json:"glasses"` // 250ml per glass
	ShowElectrolyte bool    `json:"show_electrolyte"`
	ShowCapNote     bool    `json:"show_cap_note"` // true if capped at 3.5L
}

// Phase multipliers — hardcoded per spec
var phaseHydrationMultiplier = map[domain.CyclePhase]float64{
	domain.PhaseMenstrual:   0.10, // fluid + electrolyte loss from bleeding
	domain.PhaseFollicular:  0.00, // most stable, no adjustment
	domain.PhaseOvulatory:   0.10, // ligament laxity, joint lubrication
	domain.PhaseEarlyLuteal: 0.05, // core temp rising, thirst less reliable
	domain.PhaseLateLuteal:  0.15, // highest adjustment — core temp peak, thirst suppressed
}

// CalculateHydration computes the daily water target for a specific day.
//
// Parameters:
//   - weightKg: user's weight from onboarding
//   - exerciseMinutes: duration of training session that day (0 for rest days)
//   - phase: current cycle phase
func CalculateHydration(weightKg float64, exerciseMinutes int, phase domain.CyclePhase) HydrationResult {
	// Step 1: baseline
	base := weightKg * 0.033

	// Step 2: activity adjustment
	activityAdd := (float64(exerciseMinutes) / 30.0) * 0.35

	// Step 3: phase multiplier
	multiplier := phaseHydrationMultiplier[phase]
	phaseAdd := base * multiplier

	// Step 4: total
	total := base + activityAdd + phaseAdd

	// Step 5: cap at 3.5L
	showCapNote := false
	if total > 3.5 {
		total = 3.5
		showCapNote = true
	}

	// Step 6: round to 1 decimal place
	display := math.Round(total*10) / 10

	// Glasses: 250ml per glass, rounded
	glasses := int(math.Round(display / 0.25))

	// Electrolyte flag: menstrual or late luteal
	showElectrolyte := phase == domain.PhaseMenstrual || phase == domain.PhaseLateLuteal

	return HydrationResult{
		Liters:          display,
		Glasses:         glasses,
		ShowElectrolyte: showElectrolyte,
		ShowCapNote:     showCapNote,
	}
}
