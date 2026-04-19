package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

func TestSameMuscle72h_Violation_OneDayGap(t *testing.T) {
	// Lunes lower, miércoles lower → solo 1 día entre medio → viola 72h.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		nil, // Tue
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Wed
		nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 1-day gap in late luteal, got nil")
	}
	if v.RuleID != "same_muscle_72h_late_luteal" {
		t.Errorf("expected rule ID %q, got %q", "same_muscle_72h_late_luteal", v.RuleID)
	}
	if v.Severity != rules.SeverityHardBlock {
		t.Errorf("expected hard block, got %q", v.Severity)
	}
}

func TestSameMuscle72h_Violation_Consecutive(t *testing.T) {
	// Lunes lower, martes lower → adyacentes → también viola 72h.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for consecutive days in late luteal, got nil")
	}
}

func TestSameMuscle72h_NoViolation_TwoDayGap(t *testing.T) {
	// Lunes lower, jueves lower → 2 días entre medio → 72h cumplidas.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		nil, // Tue
		nil, // Wed
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Thu
		nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for 2-day gap, got: %+v", v)
	}
}

func TestSameMuscle72h_NoViolation_DifferentMuscleGroups(t *testing.T) {
	// Lunes lower, miércoles upper → distinto muscle group → no viola.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		nil, // Tue
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Wed
		nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for different muscle groups, got: %+v", v)
	}
}

func TestSameMuscle72h_NoViolation_PilatesInBetween(t *testing.T) {
	// Lunes lower, martes pilates, miércoles lower → pilates no es muscle
	// demanding, PERO solo hay 1 día de separación real → VIOLA.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		pilatesSession(), // Tue (not muscle demanding, but still only 1 day gap)
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Wed
		nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation — pilates between doesn't extend the 72h window")
	}
}

func TestSameMuscle72h_Violation_HIITAndStrength(t *testing.T) {
	// Lunes HIIT full_body, miércoles strength full_body → 1 día gap → viola.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		hiitSession(), // Mon (full_body)
		nil,           // Tue
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate), // Wed
		nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for HIIT + Strength same muscle within 72h")
	}
}

func TestSameMuscle72h_Violation_EndOfWeek(t *testing.T) {
	// Viernes lower, domingo lower → 1 día gap → viola.
	rule := training.SameMuscle72hLateLutealRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		nil, nil, nil, nil,
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Fri
		nil, // Sat
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation at end of week, got nil")
	}
}
