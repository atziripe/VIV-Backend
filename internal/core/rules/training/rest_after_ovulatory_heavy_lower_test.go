package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

// HELPERS specific to this rule

func cardioSession(intensity domain.Intensity) *domain.TrainingSession {
	return &domain.TrainingSession{
		Modality:        domain.ModalityCardio,
		MuscleGroup:     domain.MuscleGroupNone,
		Intensity:       intensity,
		DurationMinutes: 30,
	}
}

// TESTS

func TestOvulatoryLower_Violation_StrengthLowerThenStrength(t *testing.T) {
	// Ovulatory: lower strength lunes, upper strength martes → warning.
	// Upper strength no es "adequate recovery" para articulaciones.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh),     // Mon
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for lower strength followed by upper strength in ovulatory")
	}
	if v.Severity != rules.SeveritySoftWarning {
		t.Errorf("expected soft warning, got %q", v.Severity)
	}
	if v.AffectedDays[0] != 0 || v.AffectedDays[1] != 1 {
		t.Errorf("expected affected days [0, 1], got %v", v.AffectedDays)
	}
}

func TestOvulatoryLower_Violation_FullBodyThenHIIT(t *testing.T) {
	// Full body strength → HIIT next day.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityHigh), // Mon
		hiitSession(), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for full body followed by HIIT in ovulatory")
	}
}

func TestOvulatoryLower_Violation_HIITThenStrength(t *testing.T) {
	// HIIT (full body) → strength next day.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		hiitSession(), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for HIIT followed by strength in ovulatory")
	}
}

func TestOvulatoryLower_NoViolation_RestAfter(t *testing.T) {
	// Lower strength → rest day → OK.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Mon
		nil, // Tue (rest)
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation with rest after, got: %+v", v)
	}
}

func TestOvulatoryLower_NoViolation_PilatesAfter(t *testing.T) {
	// Lower strength → Pilates → OK (Pilates counts as adequate recovery).
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Mon
		pilatesSession(), // Tue
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation with Pilates after, got: %+v", v)
	}
}

func TestOvulatoryLower_NoViolation_CardioLowAfter(t *testing.T) {
	// Lower strength → Cardio low → OK.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Mon
		cardioSession(domain.IntensityLow),                             // Tue
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation with low cardio after, got: %+v", v)
	}
}

func TestOvulatoryLower_Violation_CardioModerateAfter(t *testing.T) {
	// Lower strength → Cardio moderate → warning (moderate cardio is not adequate recovery).
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Mon
		cardioSession(domain.IntensityModerate),                        // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for moderate cardio after lower strength in ovulatory")
	}
}

func TestOvulatoryLower_NoViolation_UpperStrengthOnly(t *testing.T) {
	// Upper strength → anything → no viola (upper no es joint-loading lower).
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		hiitSession(), // Tue
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for upper-only strength, got: %+v", v)
	}
}

func TestOvulatoryLower_NoViolation_CoreStrengthOnly(t *testing.T) {
	// Core strength → anything → no viola (core is not joint-loading lower).
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupCore, domain.IntensityHigh), // Mon
		hiitSession(), // Tue
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for core-only strength, got: %+v", v)
	}
}

func TestOvulatoryLower_Violation_EndOfWeek(t *testing.T) {
	// Sábado lower strength, domingo HIIT → warning.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil,
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Sat
		hiitSession(), // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation at end of week")
	}
}

func TestOvulatoryLower_NoViolation_LastDayIsLower(t *testing.T) {
	// Domingo lower strength → no viola.
	rule := training.RestAfterOvulatoryLowerRule{}

	ctx := makeContext(domain.PhaseOvulatory, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil, nil,
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Sun
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for last day with no next day, got: %+v", v)
	}
}
