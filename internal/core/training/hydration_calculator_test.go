package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

func TestHydration_FollicularRestDay(t *testing.T) {
	// 65kg, rest day, follicular (multiplier 0.00)
	// base = 65 × 0.033 = 2.145
	// activity = 0
	// phase = 2.145 × 0.00 = 0
	// total = 2.145 → 2.1L
	result := training.CalculateHydration(65.0, 0, domain.PhaseFollicular)

	if result.Liters != 2.1 {
		t.Errorf("expected 2.1L, got %.1f", result.Liters)
	}
	if result.ShowElectrolyte {
		t.Error("should not show electrolyte in follicular")
	}
}

func TestHydration_LateLutealTrainingDay(t *testing.T) {
	// 65kg, 40min session, late luteal (multiplier 0.15)
	// base = 65 × 0.033 = 2.145
	// activity = (40/30) × 0.35 = 0.467
	// phase = 2.145 × 0.15 = 0.322
	// total = 2.145 + 0.467 + 0.322 = 2.934 → 2.9L
	result := training.CalculateHydration(65.0, 40, domain.PhaseLateLuteal)

	if result.Liters != 2.9 {
		t.Errorf("expected 2.9L, got %.1f", result.Liters)
	}
	if !result.ShowElectrolyte {
		t.Error("should show electrolyte in late luteal")
	}
}

func TestHydration_MenstrualRestDay(t *testing.T) {
	// 65kg, rest day, menstrual (multiplier 0.10)
	// base = 65 × 0.033 = 2.145
	// activity = 0
	// phase = 2.145 × 0.10 = 0.215
	// total = 2.145 + 0 + 0.215 = 2.360 → 2.4L
	result := training.CalculateHydration(65.0, 0, domain.PhaseMenstrual)

	if result.Liters != 2.4 {
		t.Errorf("expected 2.4L, got %.1f", result.Liters)
	}
	if !result.ShowElectrolyte {
		t.Error("should show electrolyte in menstrual")
	}
}

func TestHydration_OvulatoryTrainingDay(t *testing.T) {
	// 65kg, 45min session, ovulatory (multiplier 0.10)
	// base = 65 × 0.033 = 2.145
	// activity = (45/30) × 0.35 = 0.525
	// phase = 2.145 × 0.10 = 0.215
	// total = 2.145 + 0.525 + 0.215 = 2.885 → 2.9L
	result := training.CalculateHydration(65.0, 45, domain.PhaseOvulatory)

	if result.Liters != 2.9 {
		t.Errorf("expected 2.9L, got %.1f", result.Liters)
	}
	if result.ShowElectrolyte {
		t.Error("should not show electrolyte in ovulatory")
	}
}

func TestHydration_CapsAt3Point5(t *testing.T) {
	// Heavy user, long session, late luteal — should cap
	// 90kg, 120min, late luteal
	// base = 90 × 0.033 = 2.97
	// activity = (120/30) × 0.35 = 1.4
	// phase = 2.97 × 0.15 = 0.446
	// total = 2.97 + 1.4 + 0.446 = 4.816 → capped at 3.5L
	result := training.CalculateHydration(90.0, 120, domain.PhaseLateLuteal)

	if result.Liters != 3.5 {
		t.Errorf("expected 3.5L (capped), got %.1f", result.Liters)
	}
	if !result.ShowCapNote {
		t.Error("should show cap note when capped")
	}
}

func TestHydration_NoCap_Normal(t *testing.T) {
	result := training.CalculateHydration(65.0, 0, domain.PhaseFollicular)

	if result.ShowCapNote {
		t.Error("should not show cap note for normal hydration")
	}
}

func TestHydration_GlassesCalculation(t *testing.T) {
	// 2.1L / 0.25 = 8.4 → 8 glasses
	result := training.CalculateHydration(65.0, 0, domain.PhaseFollicular)

	expectedGlasses := 8
	if result.Glasses != expectedGlasses {
		t.Errorf("expected %d glasses, got %d", expectedGlasses, result.Glasses)
	}
}

func TestHydration_ElectrolyteOnlyMenstrualAndLateLuteal(t *testing.T) {
	phases := []struct {
		phase    domain.CyclePhase
		expected bool
	}{
		{domain.PhaseMenstrual, true},
		{domain.PhaseFollicular, false},
		{domain.PhaseOvulatory, false},
		{domain.PhaseEarlyLuteal, false},
		{domain.PhaseLateLuteal, true},
	}

	for _, tt := range phases {
		result := training.CalculateHydration(65.0, 0, tt.phase)
		if result.ShowElectrolyte != tt.expected {
			t.Errorf("phase %s: expected electrolyte=%v, got %v",
				tt.phase, tt.expected, result.ShowElectrolyte)
		}
	}
}
