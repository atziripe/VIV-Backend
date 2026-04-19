package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

/* MaxConsecutiveDaysRule ensures that there are no more than 3 consecutive training days.
Any session counts (STRENGTH, HIIT, PILATES, CARDIO).

Rationale: 4+ consecutive training days reduces the quality of the sessions and increases the risk of injury
due to accumulated fatigue, even if the individual sessions are low intensity.

Applies to all phases*/

type MaxConsecutiveDaysRule struct{}

func (r MaxConsecutiveDaysRule) ID() string {
	return "max_3_consecutive_days"
}

func (r MaxConsecutiveDaysRule) AppliesInPhases() []domain.CyclePhase {
	return nil // all of them
}

func (r MaxConsecutiveDaysRule) Evaluate(ctx rules.Context) *rules.Violation {
	days := ctx.Arrangement.Days
	consecutive := 0
	startIdx := 0

	for i, day := range days {
		if day.Session != nil {
			if consecutive == 0 {
				startIdx = i
			}
			consecutive++

			if consecutive > 3 {
				affected := make([]int, 0, consecutive)
				for j := startIdx; j <= i; j++ {
					affected = append(affected, j)
				}
				return &rules.Violation{
					RuleID:   r.ID(),
					Severity: rules.SeverityHardBlock,
					Message: fmt.Sprintf(
						"%d days in a row reduces session quality. VIV suggests a rest day here.",
						consecutive,
					),
					AffectedDays: affected,
				}
			}
		} else {
			consecutive = 0
		}
	}

	return nil
}
