package training

import (
	"fmt"

	"viv/internal/core/domain"
)

// BuildConstraints generates human-readable spacing rules for the LLM prompt.
// These match the rules implemented in the engine — the LLM should respect them
// to minimize validation failures and retries.
func BuildConstraints(phase domain.CyclePhase) []string {
	constraints := []string{
		"No more than 3 consecutive training days — insert a rest day",
		"At least 2 rest days in the week",
		"STRENGTH and HIIT sessions should not be on consecutive days",
	}

	switch phase {
	case domain.PhaseLateLuteal:
		constraints = append(constraints,
			"Same muscle group needs 72hrs gap (at least 2 days between) — recovery is slower this phase",
			"No HIIT sessions — this phase cannot support high-intensity interval training",
		)
	case domain.PhaseMenstrual:
		constraints = append(constraints,
			"Same muscle group needs 48hrs gap (not on consecutive days)",
			"No HIIT sessions — cortisol cost is too high in this phase",
		)
	case domain.PhaseOvulatory:
		constraints = append(constraints,
			"Same muscle group needs 48hrs gap (not on consecutive days)",
			"After a lower body or full body STRENGTH session, place a rest day or Pilates — ligament laxity peaks now",
		)
	default:
		constraints = append(constraints,
			"Same muscle group needs 48hrs gap (not on consecutive days)",
		)
	}

	if phase == domain.PhaseLateLuteal || phase == domain.PhaseMenstrual {
		constraints = append(constraints,
			fmt.Sprintf("This is %s phase — favor Pilates on gap days for recovery", phase),
		)
	}

	return constraints
}
