package training_test

import (
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/rules/training"
)

// HELPERS

func strengthSession(mg domain.MuscleGroup, intensity domain.Intensity) *domain.TrainingSession {
	return &domain.TrainingSession{
		Modality:        domain.ModalityStrength,
		MuscleGroup:     mg,
		Intensity:       intensity,
		DurationMinutes: 45,
	}
}

func hiitSession() *domain.TrainingSession {
	return &domain.TrainingSession{
		Modality:        domain.ModalityHIIT,
		MuscleGroup:     domain.MuscleGroupFullBody,
		Intensity:       domain.IntensityHigh,
		DurationMinutes: 30,
	}
}

func pilatesSession() *domain.TrainingSession {
	return &domain.TrainingSession{
		Modality:        domain.ModalityPilates,
		MuscleGroup:     domain.MuscleGroupCore,
		Intensity:       domain.IntensityLow,
		DurationMinutes: 40,
	}
}

func makeArrangement(sessions [7]*domain.TrainingSession) domain.WeekArrangement {
	weekdays := [7]time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday, time.Thursday,
		time.Friday, time.Saturday, time.Sunday,
	}
	var days [7]domain.TrainingDaySlot
	for i, s := range sessions {
		days[i] = domain.TrainingDaySlot{
			Weekday: weekdays[i],
			Session: s,
		}
	}
	return domain.WeekArrangement{
		WeekStart: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		Days:      days,
	}
}

func makeContext(phase domain.CyclePhase, sessions [7]*domain.TrainingSession) rules.Context {
	return rules.Context{
		Phase:       phase,
		Arrangement: makeArrangement(sessions),
	}
}

// TESTS

func TestSameMuscle48h_Violation_SameMuscleConsecutive(t *testing.T) {
	// Monday & Tuesday both lower body strength → violation.
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Tue
		nil, // Wed
		nil, // Thu
		nil, // Fri
		nil, // Sat
		nil, // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation, got nil")
	}
	if v.RuleID != "same_muscle_48h" {
		t.Errorf("expected rule ID %q, got %q", "same_muscle_48h", v.RuleID)
	}
	if v.Severity != rules.SeverityHardBlock {
		t.Errorf("expected hard block, got %q", v.Severity)
	}
	if len(v.AffectedDays) != 2 || v.AffectedDays[0] != 0 || v.AffectedDays[1] != 1 {
		t.Errorf("expected affected days [0, 1], got %v", v.AffectedDays)
	}
}

func TestSameMuscle48h_NoViolation_DifferentMuscleGroups(t *testing.T) {
	// Monday lower, tuesday upper → no violation.
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		strengthSession(domain.MuscleGroupUpper, domain.IntensityModerate), // Tue
		nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation, got: %+v", v)
	}
}

func TestSameMuscle48h_NoViolation_RestDayBetween(t *testing.T) {
	// Mon lower, Tue rest, Wed lower → no violation (48h check).
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		nil, // Tue (rest)
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Wed
		nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation, got: %+v", v)
	}
}

func TestSameMuscle48h_NoViolation_PilatesBetween(t *testing.T) {
	// Mon lower strength, Tue pilates, Wed lower strength → no violation.
	// Pilates is not a "muscle demanding" workout.
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Mon
		pilatesSession(), // Tue
		strengthSession(domain.MuscleGroupLower, domain.IntensityModerate), // Wed
		nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation, got: %+v", v)
	}
}

func TestSameMuscle48h_Violation_HIITAndStrengthSameMuscle(t *testing.T) {
	// Mon HIIT full body, tue strength full body → violation.
	// Both muscle demanding y muscle group sharing.
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		hiitSession(), // Mon (full_body)
		strengthSession(domain.MuscleGroupFullBody, domain.IntensityModerate), // Tue
		nil, nil, nil, nil, nil,
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation for HIIT + Strength same muscle group, got nil")
	}
}

func TestSameMuscle48h_Violation_EndOfWeek(t *testing.T) {
	// Sat & sun both upper → violation.
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil,
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Sat
		strengthSession(domain.MuscleGroupUpper, domain.IntensityHigh), // Sun
	})

	v := rule.Evaluate(ctx)

	if v == nil {
		t.Fatal("expected violation at end of week, got nil")
	}
	if v.AffectedDays[0] != 5 || v.AffectedDays[1] != 6 {
		t.Errorf("expected affected days [5, 6], got %v", v.AffectedDays)
	}
}

func TestSameMuscle48h_NoViolation_AllRestDays(t *testing.T) {
	// Semana completa de rest → sin violación.
	rule := training.SameMuscle48hRule{}

	ctx := makeContext(domain.PhaseFollicular, [7]*domain.TrainingSession{
		nil, nil, nil, nil, nil, nil, nil,
	})

	if v := rule.Evaluate(ctx); v != nil {
		t.Errorf("expected no violation for all rest days, got: %+v", v)
	}
}
