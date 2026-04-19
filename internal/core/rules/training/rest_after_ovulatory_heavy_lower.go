package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

/* RestAfterOvulatoryLowerRule check high that intensity sessions with lower body involved during ovulatory phase, has a rest
the right after day (or non-demanding session).

Fundamento: el riesgo de lesión de ACL y ligamentos pico en ovulación debido al efecto hormonal sobre la laxitud del
tejido conectivo. Las sesiones de lower body y full body con alta carga son las de mayor riesgo. Un día
de descanso inmediatamente después reduce el estrés articular acumulativo.

Trigger rules:
  - STRENGTH w/ muscle_group lower or full_body
  - HIIT

What is a valdi rest dafy after:
  - Rest day (nil)
  - PILATES (low articular load)
  - CARDIO low intensity

Severity: soft warning.

OVULATORY ONLY.*/

type RestAfterOvulatoryLowerRule struct{}

func (r RestAfterOvulatoryLowerRule) ID() string {
	return "rest_after_ovulatory_lower"
}

func (r RestAfterOvulatoryLowerRule) AppliesInPhases() []domain.CyclePhase {
	return []domain.CyclePhase{domain.PhaseOvulatory}
}

func (r RestAfterOvulatoryLowerRule) Evaluate(ctx rules.Context) *rules.Violation {
	days := ctx.Arrangement.Days

	for i := 0; i < len(days)-1; i++ {
		if !isJointLoadingLower(days[i].Session) {
			continue
		}

		if !isAdequateRecovery(days[i+1].Session) {
			return &rules.Violation{
				RuleID:   r.ID(),
				Severity: rules.SeveritySoftWarning,
				Message: fmt.Sprintf(
					"Ligament laxity peaks now. A rest day after %s %s reduces ACL injury risk. Consider a rest or Pilates day after.",
					days[i].Session.Modality,
					days[i].Session.MuscleGroup,
				),
				AffectedDays: []int{i, i + 1},
			}
		}
	}

	return nil
}

// isJointLoadingLower true if:
//   - STRENGTH lower body
//   - STRENGTH full body (legs involved)
//   - HIIT (heavy joint load)

func isJointLoadingLower(s *domain.TrainingSession) bool {
	if s == nil {
		return false
	}

	if s.Modality == domain.ModalityHIIT {
		return true
	}

	if s.Modality == domain.ModalityStrength {
		return s.MuscleGroup == domain.MuscleGroupLower ||
			s.MuscleGroup == domain.MuscleGroupFullBody
	}

	return false
}

// isAdequateRecovery true if adequate recovery session after joint intense impact
//   - rest day
//   - PILATES
//   - CARDIO low

func isAdequateRecovery(s *domain.TrainingSession) bool {
	if s == nil {
		return true // rest day
	}

	if s.Modality == domain.ModalityPilates {
		return true
	}

	if s.Modality == domain.ModalityCardio && s.Intensity == domain.IntensityLow {
		return true
	}

	return false
}
