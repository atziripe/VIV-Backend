package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

func TestMaxConsecutiveDays_Violation_FourConsecutive(t *testing.T) {
	rule := training.MaxConsecutiveDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		pilatesSession(), // Wed
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate), // Thu
		nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 4 consecutive days, got nil")
	}
	if v.Severity != rules.SeverityHardBlock {
		t.Errorf("expected hard block, got %q", v.Severity)
	}
	if len(v.AffectedDays) != 4 {
		t.Errorf("expected 4 affected days, got %d", len(v.AffectedDays))
	}
}

func TestMaxConsecutiveDays_NoViolation_ThreeConsecutive(t *testing.T) {
	rule := training.MaxConsecutiveDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		pilatesSession(), // Wed
		nil,              // Thu (rest)
		nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for 3 consecutive days, got: %+v", v)
	}
}

func TestMaxConsecutiveDays_NoViolation_AllRest(t *testing.T) {
	rule := training.MaxConsecutiveDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for all rest, got: %+v", v)
	}
}

func TestMaxConsecutiveDays_Violation_FiveConsecutive(t *testing.T) {
	rule := training.MaxConsecutiveDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		pilatesSession(), // Wed
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate), // Thu
		hiitSession(), // Fri
		nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 5 consecutive days, got nil")
	}
	// Reporta en el 4to día (primera violación)
	if len(v.AffectedDays) != 4 {
		t.Errorf("expected 4 affected days (stops at first violation), got %d", len(v.AffectedDays))
	}
}

func TestMaxConsecutiveDays_Violation_EndOfWeek(t *testing.T) {
	rule := training.MaxConsecutiveDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		nil, nil, nil,
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Thu
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Fri
		pilatesSession(), // Sat
		strengthSession(domain.MuscleGroupCore, domain.IntensityModerate), // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation at end of week, got nil")
	}
}

func TestMaxConsecutiveDays_NoViolation_SplitByRest(t *testing.T) {
	// 3 + rest + 3 → no violation.
	rule := training.MaxConsecutiveDaysRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		pilatesSession(), // Wed
		nil,              // Thu (rest — resets counter)
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Fri
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Sat
		pilatesSession(), // Sun
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for 3+rest+3, got: %+v", v)
	}
}
