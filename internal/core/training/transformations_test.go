package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

// ============================================================================
// LOAD OVERRIDE TESTS
// ============================================================================

func TestTransform_LoadOverride_LateLutealStrength(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupLower,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseLateLuteal)

	if result.LoadLabel != "moderate" {
		t.Errorf("expected load_label 'moderate' in late luteal, got %q", result.LoadLabel)
	}
}

func TestTransform_LoadOverride_MenstrualStrength(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupUpper,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseMenstrual)

	if result.LoadLabel != "moderate" {
		t.Errorf("expected load_label 'moderate' in menstrual, got %q", result.LoadLabel)
	}
}

func TestTransform_LoadOverride_NotApplied_Follicular(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupLower,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseFollicular)

	if result.LoadLabel != "" {
		t.Errorf("expected no load override in follicular, got %q", result.LoadLabel)
	}
}

func TestTransform_LoadOverride_NotApplied_HIIT(t *testing.T) {
	// Load override only applies to Strength, not HIIT
	session := domain.TrainingSession{
		Modality:    domain.ModalityHIIT,
		MuscleGroup: domain.MuscleGroupFullBody,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseLateLuteal)

	if result.LoadLabel != "" {
		t.Errorf("expected no load override for HIIT, got %q", result.LoadLabel)
	}
}

func TestTransform_LoadOverride_NotApplied_Pilates(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityPilates,
		MuscleGroup: domain.MuscleGroupCore,
		Intensity:   domain.IntensityLow,
	}

	result := training.ApplyTransformations(session, domain.PhaseLateLuteal)

	if result.LoadLabel != "" {
		t.Errorf("expected no load override for Pilates, got %q", result.LoadLabel)
	}
}

// ============================================================================
// JOINT WARNING TESTS
// ============================================================================

func TestTransform_JointWarning_OvulatoryLowerStrength(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupLower,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if !result.JointWarning {
		t.Error("expected joint warning for lower strength in ovulatory")
	}
}

func TestTransform_JointWarning_OvulatoryFullBodyStrength(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupFullBody,
		Intensity:   domain.IntensityModerate,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if !result.JointWarning {
		t.Error("expected joint warning for full body strength in ovulatory")
	}
}

func TestTransform_JointWarning_OvulatoryHIIT(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityHIIT,
		MuscleGroup: domain.MuscleGroupFullBody,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if !result.JointWarning {
		t.Error("expected joint warning for HIIT in ovulatory")
	}
}

func TestTransform_JointWarning_NotApplied_UpperStrength(t *testing.T) {
	// Upper body strength in ovulatory — no joint warning
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupUpper,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if result.JointWarning {
		t.Error("should not warn for upper body strength in ovulatory")
	}
}

func TestTransform_JointWarning_NotApplied_CoreStrength(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupCore,
		Intensity:   domain.IntensityModerate,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if result.JointWarning {
		t.Error("should not warn for core strength in ovulatory")
	}
}

func TestTransform_JointWarning_NotApplied_Follicular(t *testing.T) {
	// Lower strength in follicular — no warning (not ovulatory)
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupLower,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseFollicular)

	if result.JointWarning {
		t.Error("should not warn outside ovulatory phase")
	}
}

func TestTransform_JointWarning_NotApplied_Pilates(t *testing.T) {
	session := domain.TrainingSession{
		Modality:    domain.ModalityPilates,
		MuscleGroup: domain.MuscleGroupFullBody,
		Intensity:   domain.IntensityLow,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if result.JointWarning {
		t.Error("should not warn for Pilates in ovulatory")
	}
}

// ============================================================================
// COMBINED TRANSFORMATIONS
// ============================================================================

func TestTransform_BothApplied_LateLutealLowerStrength(t *testing.T) {
	// Late luteal should NOT have joint warning (that's ovulatory only)
	// but SHOULD have load override
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupLower,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseLateLuteal)

	if result.LoadLabel != "moderate" {
		t.Errorf("expected load override in late luteal, got %q", result.LoadLabel)
	}
	if result.JointWarning {
		t.Error("should not have joint warning in late luteal")
	}
}

func TestTransform_OvulatoryLower_NoLoadOverride_ButJointWarning(t *testing.T) {
	// Ovulatory should have joint warning but NO load override
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupLower,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseOvulatory)

	if result.LoadLabel != "" {
		t.Errorf("should not have load override in ovulatory, got %q", result.LoadLabel)
	}
	if !result.JointWarning {
		t.Error("expected joint warning in ovulatory")
	}
}

func TestTransform_NothingApplied_FollicularUpper(t *testing.T) {
	// Follicular upper strength — no transformations
	session := domain.TrainingSession{
		Modality:    domain.ModalityStrength,
		MuscleGroup: domain.MuscleGroupUpper,
		Intensity:   domain.IntensityHigh,
	}

	result := training.ApplyTransformations(session, domain.PhaseFollicular)

	if result.LoadLabel != "" {
		t.Errorf("expected no load override, got %q", result.LoadLabel)
	}
	if result.JointWarning {
		t.Error("expected no joint warning")
	}
}
