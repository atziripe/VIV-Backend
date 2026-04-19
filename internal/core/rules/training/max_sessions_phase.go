package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

// MaxSessionsPhaseRule verifica que el número total de sesiones en la semana
// no exceda el máximo permitido para la fase del ciclo actual.
//
// Fundamento: en late luteal y menstrual, el esfuerzo percibido es mayor,
// la recuperación más lenta, y el riesgo de lesión elevado. 4+ sesiones
// acumulan fatiga más rápido que en otras fases.
//
// Los máximos vienen de la matriz fase × modalidad del spec:
//   - Menstrual:    3
//   - Follicular:   5
//   - Ovulatory:    5
//   - Early luteal: 4
//   - Late luteal:  3
//
// Severity: soft warning. VIV no bloquea, pero advierte que podría ser
// demasiado para esta fase. La usuaria puede override si quiere.
//
// Aplica en todas las fases (cada una con su propio máximo).

type MaxSessionsPhaseRule struct{}

func (r MaxSessionsPhaseRule) ID() string {
	return "max_sessions_phase"
}

func (r MaxSessionsPhaseRule) AppliesInPhases() []domain.CyclePhase {
	return nil // todas
}

// Max sessions per phase according to specs.
var phaseMaxSessions = map[domain.CyclePhase]int{
	domain.PhaseMenstrual:   3,
	domain.PhaseFollicular:  5,
	domain.PhaseOvulatory:   5,
	domain.PhaseEarlyLuteal: 4,
	domain.PhaseLateLuteal:  3,
}

func (r MaxSessionsPhaseRule) Evaluate(ctx rules.Context) *rules.Violation {
	max, ok := phaseMaxSessions[ctx.Phase]
	if !ok {
		return nil // unknon phase, don't block
	}

	sessionDays := make([]int, 0)
	for i, day := range ctx.Arrangement.Days {
		if day.Session != nil {
			sessionDays = append(sessionDays, i)
		}
	}

	if len(sessionDays) > max {
		return &rules.Violation{
			RuleID:   r.ID(),
			Severity: rules.SeveritySoftWarning,
			Message: fmt.Sprintf(
				"You have %d sessions planned but %s phase supports up to %d. Consider dropping one to protect your recovery.",
				len(sessionDays), ctx.Phase, max,
			),
			AffectedDays: sessionDays,
		}
	}

	return nil
}
