package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

/* The SameMuscle72hLateLutealRule ensures that no two high-intensity muscle training sessions
(STRENGTH or HIIT) targeting the same muscle group occur within 72 hours of each other during the late luteal phase.
Rationale: Recovery is slower in the late luteal phase due to the increased basal metabolic rate and higher metabolic cost.
The standard 48-hour window is insufficient during this phase.
In practice, 72 hours means at least two days between sessions targeting the same muscle group.

This applies ONLY to the late luteal phase. SameMuscle48hRule covers the other phases.*/

type SameMuscle72hLateLutealRule struct{}

func (r SameMuscle72hLateLutealRule) ID() string {
	return "same_muscle_72h_late_luteal"
}

func (r SameMuscle72hLateLutealRule) AppliesInPhases() []domain.CyclePhase {
	return []domain.CyclePhase{domain.PhaseLateLuteal}
}

func (r SameMuscle72hLateLutealRule) Evaluate(ctx rules.Context) *rules.Violation {
	days := ctx.Arrangement.Days

	for i := 0; i < len(days); i++ {
		if !isMuscleDemanding(days[i].Session) {
			continue
		}

		// 2 days following, not the adjacent one as general same muscle rule.
		for offset := 1; offset <= 2; offset++ {
			j := i + offset
			if j >= len(days) {
				break
			}

			if !isMuscleDemanding(days[j].Session) {
				continue
			}

			if days[i].Session.MuscleGroup == days[j].Session.MuscleGroup {
				return &rules.Violation{
					RuleID:   r.ID(),
					Severity: rules.SeveritySoftWarning,
					Message: fmt.Sprintf(
						"Recovery is slower this phase, your muscles need an extra day. %s sessions on day %d and day %d are less than 72hrs apart.",
						days[i].Session.MuscleGroup,
						i+1, j+1,
					),
					AffectedDays: []int{i, j},
				}
			}
		}
	}

	return nil
}
