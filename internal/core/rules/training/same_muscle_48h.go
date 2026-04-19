package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

/* The SameMuscle48hRule ensures that no two high-intensity muscle training sessions (STRENGTH or HIIT)
targeting the same muscle group occur on adjacent days.
Rationale: Less than 48 hours between sessions targeting the same muscle group reduces the quality of adaptation
and increases the risk of injury due to accumulated fatigue.
This rule applies to all phases except the late luteal phase, where the 72-hour rule (SameMuscle72hLateLutealRule)
takes precedence.*/

type SameMuscle48hRule struct{}

func (r SameMuscle48hRule) ID() string {
	return "same_muscle_48h"
}

func (r SameMuscle48hRule) AppliesInPhases() []domain.CyclePhase {
	return []domain.CyclePhase{
		domain.PhaseMenstrual,
		domain.PhaseFollicular,
		domain.PhaseOvulatory,
		domain.PhaseEarlyLuteal,
	}
}

func (r SameMuscle48hRule) Evaluate(ctx rules.Context) *rules.Violation {
	days := ctx.Arrangement.Days

	for i := 0; i < len(days)-1; i++ {
		if !isMuscleDemanding(days[i].Session) {
			continue
		}

		if !isMuscleDemanding(days[i+1].Session) {
			continue
		}

		if days[i].Session.MuscleGroup == days[i+1].Session.MuscleGroup {
			return &rules.Violation{
				RuleID:   r.ID(),
				Severity: rules.SeverityHardBlock,
				Message: fmt.Sprintf(
					"These sessions target the same muscles (%s) on consecutive days. Add at least one rest day between them.",
					days[i].Session.MuscleGroup,
				),
				AffectedDays: []int{i, i + 1},
			}
		}
	}

	return nil
}

func isMuscleDemanding(s *domain.TrainingSession) bool {
	if s == nil {
		return false
	}
	return s.Modality == domain.ModalityStrength || s.Modality == domain.ModalityHIIT
}
