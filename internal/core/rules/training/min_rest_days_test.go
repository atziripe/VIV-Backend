package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

func TestMinRestDays_Violation_OnlyOneRestDay(t *testing.T) {
	rule := training.MinRestDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		pilatesSession(), // Wed
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate), // Thu
		hiitSession(),    // Fri
		pilatesSession(), // Sat
		nil,              // Sun — only rest day
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for only 1 rest day, got nil")
	}
	if v.Severity != rules.SeveritySoftWarning {
		t.Errorf("expected soft warning, got %q", v.Severity)
	}
	if len(v.AffectedDays) != 6 {
		t.Errorf("expected 6 affected days (training days), got %d", len(v.AffectedDays))
	}
}

func TestMinRestDays_Violation_ZeroRestDays(t *testing.T) {
	rule := training.MinRestDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate),
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate),
		pilatesSession(),
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate),
		hiitSession(),
		pilatesSession(),
		strengthSession(domain.MuscleGroupCore, domain.IntensityModerate),
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 0 rest days, got nil")
	}
	if len(v.AffectedDays) != 7 {
		t.Errorf("expected 7 affected days, got %d", len(v.AffectedDays))
	}
}

func TestMinRestDays_NoViolation_ExactlyTwoRestDays(t *testing.T) {
	rule := training.MinRestDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		nil,              // Wed (rest)
		pilatesSession(), // Thu
		hiitSession(),    // Fri
		nil,              // Sat (rest)
		pilatesSession(), // Sun
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for exactly 2 rest days, got: %+v", v)
	}
}

func TestMinRestDays_NoViolation_ThreeRestDays(t *testing.T) {
	rule := training.MinRestDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		nil,              // Tue (rest)
		pilatesSession(), // Wed
		nil,              // Thu (rest)
		hiitSession(),    // Fri
		nil,              // Sat (rest)
		pilatesSession(), // Sun
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for 3 rest days, got: %+v", v)
	}
}

func TestMinRestDays_NoViolation_AllRest(t *testing.T) {
	rule := training.MinRestDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for all rest, got: %+v", v)
	}
}
