package training

import (
	"fmt"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

//CHEQUEAR ESTA REGLA CON STEFY

/* NoConsecutiveHighIntensityRule verifica que no haya dos sesiones de alta
intensidad (STRENGTH o HIIT) en días consecutivos.

Fundamento: dos sesiones de alta intensidad back-to-back generan fatiga
metabólica significativa, reducen la calidad de la segunda sesión, y
acumulan carga de cortisol. Esto es independiente del grupo muscular —
incluso upper lunes + lower martes es subóptimo si ambas son high intensity.

PILATES y CARDIO no cuentan como high intensity para esta regla.

Un día de PILATES o rest entre dos sesiones de alta intensidad rompe la cadena.
Severity: soft warning. VIV advierte pero no bloquea — "consider a rest day
between them" es una recomendación, no una prohibición.
Aplica en todas las fases. */

type NoConsecutiveHighIntensityRule struct{}

func (r NoConsecutiveHighIntensityRule) ID() string {
	return "no_consecutive_high_intensity"
}

func (r NoConsecutiveHighIntensityRule) AppliesInPhases() []domain.CyclePhase {
	return nil // all
}

func (r NoConsecutiveHighIntensityRule) Evaluate(ctx rules.Context) *rules.Violation {
	days := ctx.Arrangement.Days

	for i := 0; i < len(days)-1; i++ {
		if !isHighIntensitySession(days[i].Session) {
			continue
		}

		if !isHighIntensitySession(days[i+1].Session) {
			continue
		}

		return &rules.Violation{
			RuleID:   r.ID(),
			Severity: rules.SeveritySoftWarning,
			Message: fmt.Sprintf(
				"Two high-intensity sessions back-to-back (%s + %s) — the second will be lower quality. Consider a rest day between them.",
				days[i].Session.Modality,
				days[i+1].Session.Modality,
			),
			AffectedDays: []int{i, i + 1},
		}
	}

	return nil
}

func isHighIntensitySession(s *domain.TrainingSession) bool {
	if s == nil {
		return false
	}
	return s.Modality == domain.ModalityStrength || s.Modality == domain.ModalityHIIT
}
