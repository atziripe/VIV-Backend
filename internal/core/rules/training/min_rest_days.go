package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

/* MinRestDaysRule ensure there are at least two rest days per week.
Rationale: ACSM guidelines recommend a minimum of two to three rest days per week for recreational athletes.
The body builds adaptation during rest, not during training.

Severity: Soft warning. Does not block, but provides a warning. Applies to all phases.*/

type MinRestDaysRule struct{}

func (r MinRestDaysRule) ID() string {
	return "min_2_rest_days"
}

func (r MinRestDaysRule) AppliesInPhases() []domain.CyclePhase {
	return nil // all
}

func (r MinRestDaysRule) Evaluate(ctx rules.Context) *rules.Violation {
	restCount := 0
	trainingDays := make([]int, 0)

	for i, day := range ctx.Arrangement.Days {
		if day.Session == nil { //different of rest day
			restCount++
		} else {
			trainingDays = append(trainingDays, i)
		}
	}

	if restCount < 2 {
		return &rules.Violation{
			RuleID:   r.ID(),
			Severity: rules.SeveritySoftWarning,
			Message: fmt.Sprintf(
				"Less than 2 rest days per week reduces adaptation. You have %d rest day(s), your body builds during recovery, not during training.",
				restCount,
			),
			AffectedDays: trainingDays,
		}
	}

	return nil
}
