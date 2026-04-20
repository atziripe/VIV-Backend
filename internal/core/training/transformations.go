package training

import (
	"viv/internal/core/domain"
)

// ============================================================================
// LAYER 2 — DETERMINISTIC TRANSFORMATIONS
// ============================================================================
// Applied AFTER engine validation passes, BEFORE content selection.
// These annotate the plan with phase-specific intelligence.
// They don't validate — they enrich.

// TransformationResult holds the annotations for a single session.
// These get merged into the final TrainingPlanDay.
type TransformationResult struct {
	LoadLabel    string // "moderate", "high", or "" (no override)
	JointWarning bool   // true if ligament laxity warning should show
}

// ApplyTransformations runs all deterministic transformations on a session
// given the current cycle phase. Returns annotations to apply.
//
// Current transformations:
//   - Late luteal: Strength sessions get load_label = "moderate"
//   - Menstrual: Strength sessions get load_label = "moderate"
//   - Ovulatory: Strength lower/full_body and HIIT get joint_warning = true
func ApplyTransformations(session domain.TrainingSession, phase domain.CyclePhase) TransformationResult {
	result := TransformationResult{}

	applyLoadOverride(&result, session, phase)
	applyJointWarning(&result, session, phase)

	return result
}

// applyLoadOverride sets load_label to "moderate" for Strength sessions
// in phases where high load is contraindicated.
//
// Science: late luteal and menstrual phases have elevated metabolic cost
// and reduced recovery capacity. Strength work is still beneficial but
// should be performed at moderate load regardless of the user's usual
// intensity preference.
func applyLoadOverride(r *TransformationResult, s domain.TrainingSession, phase domain.CyclePhase) {
	if s.Modality != domain.ModalityStrength {
		return
	}

	if phase == domain.PhaseLateLuteal || phase == domain.PhaseMenstrual {
		r.LoadLabel = "moderate"
	}
}

// applyJointWarning flags sessions that carry elevated joint injury risk
// during ovulatory phase.
//
// Science: ACL and ligament injury risk peaks at ovulation due to
// hormonal effects on connective tissue laxity. High-load lower body
// and full-body sessions carry the highest risk. HIIT is included
// because VIV's HIIT is always full-body with impact movements.
func applyJointWarning(r *TransformationResult, s domain.TrainingSession, phase domain.CyclePhase) {
	if phase != domain.PhaseOvulatory {
		return
	}

	if s.Modality == domain.ModalityHIIT {
		r.JointWarning = true
		return
	}

	if s.Modality == domain.ModalityStrength {
		if s.MuscleGroup == domain.MuscleGroupLower || s.MuscleGroup == domain.MuscleGroupFullBody {
			r.JointWarning = true
		}
	}
}

// ============================================================================
// PLAN ASSEMBLY — combines everything into the final plan
// ============================================================================

// AssemblePlan takes a validated WeekArrangement, selects content from the
// library, applies transformations, and produces the final TrainingWeekPlan.
//
// This is the last step before the plan is returned to the user.
func AssemblePlan(
	arrangement domain.WeekArrangement,
	phase domain.CyclePhase,
	lib *Library,
	dimensions domain.CheckinDimensions,
	restWeekOffered bool,
) domain.TrainingWeekPlan {

	weekdayNames := [7]string{
		"monday", "tuesday", "wednesday", "thursday",
		"friday", "saturday", "sunday",
	}

	var days [7]domain.TrainingPlanDay

	for i, slot := range arrangement.Days {
		day := domain.TrainingPlanDay{
			Weekday:   weekdayNames[i],
			IsRestDay: slot.Session == nil,
		}

		if slot.Session != nil {
			day.Session = slot.Session

			// Layer 2: apply transformations
			transforms := ApplyTransformations(*slot.Session, phase)
			day.LoadLabel = transforms.LoadLabel
			day.JointWarning = transforms.JointWarning

			// Layer 3: select content from library
			criteria := SelectionCriteria{
				Session:          *slot.Session,
				Phase:            phase,
				RecoveryCapacity: string(dimensions.Recovery),
				LifeBandwidth:    string(dimensions.Bandwidth),
				BuildReadiness:   string(dimensions.Build),
			}

			if content, found := lib.SelectSession(criteria); found {
				day.Content = &content

				// Inject the phase-specific note
				if note, ok := content.PhaseNotes[phase]; ok {
					day.PhaseNote = note
				}
			}
		}

		days[i] = day
	}

	return domain.TrainingWeekPlan{
		Phase:           phase,
		Days:            days,
		RestWeekOffered: restWeekOffered,
	}
}
