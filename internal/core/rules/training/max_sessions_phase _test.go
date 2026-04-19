package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

func TestMaxSessionsPhase_Violation_LateLuteal4Sessions(t *testing.T) {
	// Late luteal max = 3. 4 sesiones → warning.
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		nil,              // Tue
		pilatesSession(), // Wed
		nil,              // Thu
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Fri
		nil,              // Sat
		pilatesSession(), // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 4 sessions in late luteal, got nil")
	}
	if v.Severity != rules.SeveritySoftWarning {
		t.Errorf("expected soft warning, got %q", v.Severity)
	}
	if len(v.AffectedDays) != 4 {
		t.Errorf("expected 4 affected days, got %d", len(v.AffectedDays))
	}
}

func TestMaxSessionsPhase_Violation_Menstrual4Sessions(t *testing.T) {
	// Menstrual max = 3. 4 sesiones → warning.
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseMenstrual, [7]*domain.TrainingSession{
		pilatesSession(), // Mon
		nil,              // Tue
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Wed
		pilatesSession(), // Thu
		nil,              // Fri
		pilatesSession(), // Sat
		nil,              // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 4 sessions in menstrual, got nil")
	}
}

func TestMaxSessionsPhase_NoViolation_LateLuteal3Sessions(t *testing.T) {
	// Late luteal max = 3. Exactamente 3 → OK.
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		nil, nil,
		pilatesSession(), // Thu
		nil,
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Sat
		nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for exactly 3 sessions in late luteal, got: %+v", v)
	}
}

func TestMaxSessionsPhase_NoViolation_Follicular5Sessions(t *testing.T) {
	// Follicular max = 5. 5 sesiones → OK.
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		pilatesSession(), // Tue
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Wed
		nil,              // Thu
		hiitSession(),    // Fri
		pilatesSession(), // Sat
		nil,              // Sun
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for 5 sessions in follicular, got: %+v", v)
	}
}

func TestMaxSessionsPhase_Violation_Follicular6Sessions(t *testing.T) {
	// Follicular max = 5. 6 sesiones → warning.
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Mon
		pilatesSession(), // Tue
		strengthSession(domain.MuscleGroupLower, domain.IntensityHigh), // Wed
		hiitSession(), // Thu
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate), // Fri
		pilatesSession(), // Sat
		nil,              // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for 6 sessions in follicular, got nil")
	}
}

func TestMaxSessionsPhase_NoViolation_EarlyLuteal4Sessions(t *testing.T) {
	// Early luteal max = 4. Exactamente 4 → OK.
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseEarlyLuteal, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Mon
		nil,              // Tue
		pilatesSession(), // Wed
		nil,              // Thu
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Fri
		pilatesSession(), // Sat
		nil,              // Sun
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for 4 sessions in early luteal, got: %+v", v)
	}
}

func TestMaxSessionsPhase_NoViolation_AllRest(t *testing.T) {
	rule := training.MaxSessionsPhaseRule{}

	ctx := makeContext(domain.PhaseLateLuteal, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for all rest, got: %+v", v)
	}
}
