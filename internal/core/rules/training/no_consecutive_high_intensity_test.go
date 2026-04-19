package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

func TestNoConsecutiveHigh_Violation_StrengthThenHIIT(t *testing.T) {
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		hiitSession(), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for STRENGTH + HIIT consecutive, got nil")
	}
	if v.Severity != rules.SeveritySoftWarning {
		t.Errorf("expected soft warning, got %q", v.Severity)
	}
	if len(v.AffectedDays) != 2 || v.AffectedDays[0] != 0 || v.AffectedDays[1] != 1 {
		t.Errorf("expected affected days [0, 1], got %v", v.AffectedDays)
	}
}

func TestNoConsecutiveHigh_Violation_HIITThenStrength(t *testing.T) {
	// Dirección inversa: HIIT → Strength.
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		hiitSession(), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for HIIT + STRENGTH consecutive, got nil")
	}
}

func TestNoConsecutiveHigh_Violation_StrengthThenStrength(t *testing.T) {
	// Dos strength consecutivos (distinto muscle group) → warning.
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for STRENGTH + STRENGTH consecutive, got nil")
	}
}

func TestNoConsecutiveHigh_NoViolation_PilatesBetween(t *testing.T) {
	// Strength → Pilates → HIIT → no viola (Pilates rompe la cadena).
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		pilatesSession(), // Tue
		hiitSession(),    // Wed
		nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation with Pilates between, got: %+v", v)
	}
}

func TestNoConsecutiveHigh_NoViolation_RestBetween(t *testing.T) {
	// Strength → rest → HIIT → no viola.
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		nil,           // Tue (rest)
		hiitSession(), // Wed
		nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation with rest between, got: %+v", v)
	}
}

func TestNoConsecutiveHigh_NoViolation_PilatesConsecutive(t *testing.T) {
	// Pilates + Pilates consecutivos → no viola (no son high intensity).
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		pilatesSession(), // Mon
		pilatesSession(), // Tue
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for Pilates + Pilates, got: %+v", v)
	}
}

func TestNoConsecutiveHigh_NoViolation_CardioBetween(t *testing.T) {
	// Strength → Cardio → Strength → no viola (Cardio no es high intensity).
	rule := training.NoConsecutiveHighIntensityRule{}

	cardio := &domain.TrainingSession{
		Modality:        domain.ModalityCardio,
		MuscleGroup:     domain.MuscleGroupNone,
		Intensity:       domain.IntensityModerate,
		DurationMinutes: 30,
	}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		cardio, // Tue
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Wed
		nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation with Cardio between, got: %+v", v)
	}
}

func TestNoConsecutiveHigh_Violation_EndOfWeek(t *testing.T) {
	rule := training.NoConsecutiveHighIntensityRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil,
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Sat
		hiitSession(), // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation at end of week, got nil")
	}
	if v.AffectedDays[0] != 5 || v.AffectedDays[1] != 6 {
		t.Errorf("expected affected days [5, 6], got %v", v.AffectedDays)
	}
}
