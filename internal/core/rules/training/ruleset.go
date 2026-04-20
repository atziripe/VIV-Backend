package training

import "viv/internal/core/rules"

/* NewTrainingRuleSet SINGLE SOURCE OF TRUTH
Record of all rules which apply.
Use:
	engine := rules.NewEngine(training.NewTrainingRuleSet())
	decision := engine.Evaluate(phase, arrangement)*/

func NewTrainingRuleSet() []rules.Rule {
	return []rules.Rule{
		// Hard blocks — not negotiables
		SameMuscle48hRule{},
		SameMuscle72hLateLutealRule{},
		MaxConsecutiveDaysRule{},

		// Soft warnings — recommendations, can override
		MinRestDaysRule{},
		MaxSessionsPhaseRule{},
		NoConsecutiveHighIntensityRule{},
		RestAfterOvulatoryLowerRule{},
	}
}
