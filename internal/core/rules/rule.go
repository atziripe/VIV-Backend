package rules

import "viv/internal/core/domain"

// ============================================================================
// CONTEXT — el snapshot inmutable que cada regla recibe para evaluar
// ============================================================================
// Context contiene TODO lo que una regla podría necesitar para decidir.
// Es construido por el usecase antes de llamar al engine, y las reglas
// lo reciben como lectura — nunca lo modifican.
//
// Diseño intencionalmente mínimo para MVP:
//   - Phase: determina qué reglas corren (filtrado) y modifica comportamiento
//     de algunas reglas (ej: 48h vs 72h para same muscle).
//   - Arrangement: los 7 días con sus sesiones — lo que las reglas validan.
//
// Campos que NO están todavía (y cuándo agregarlos):
//   - CheckinDimensions (RecoveryCapacity, LifeBandwidth, BuildReadiness):
//     cuando implementemos el selector de Fase 2. El engine de reglas no los
//     necesita — las reglas son fisiológicas, no de preferencia.
//   - UserPreferences: idem, son para el selector, no para validación.
//   - CurrentArrangement (para comparar con propuesto en drag validation):
//     cuando implementemos las reglas UX de "no mover sesión pasada". Por ahora
//     esas reglas viven en el handler HTTP, no en el engine.

type Context struct {
	Phase       domain.CyclePhase
	Arrangement domain.WeekArrangement
}

// ============================================================================
// RULE — la interface que toda regla del sistema implementa
// ============================================================================
// Cada regla es un struct que implementa esta interface. Usamos interface
// (en vez de una función suelta) porque cada regla necesita:
//   - Identificarse con un ID estable (para logs, UI, feedback al LLM)
//   - Declarar en qué fases del ciclo aplica (para que el engine filtre)
//   - Ejecutar su lógica de evaluación
//
// Contrato:
//   - Evaluate recibe un Context de solo lectura.
//   - Devuelve nil si no hay violación (el arrangement es válido para esta regla).
//   - Devuelve un *Violation si hay infracción.
//   - Una regla puede devolver como máximo UNA violación por evaluación.
//     Si el arrangement viola la misma regla en múltiples lugares (ej: same muscle
//     en lunes-martes Y jueves-viernes), la regla reporta la primera instancia.
//     Esto es suficiente para MVP — el LLM o la usuaria corrige, y la siguiente
//     evaluación atrapa la siguiente violación si aún existe.

type Rule interface {
	// ID devuelve el identificador único de esta regla.
	// Convención: snake_case, descriptivo. Ej: "same_muscle_48h", "max_3_consecutive".
	// Se usa en Violation.RuleID, en logs, y en métricas.
	ID() string

	// AppliesInPhases devuelve las fases del ciclo donde esta regla debe correr.
	// El engine llama a Evaluate SOLO si la fase actual está en esta lista.
	//
	// Devolver nil o slice vacío significa "aplica en TODAS las fases".
	// Esto es el default — la mayoría de las reglas son universales.
	//
	// Ejemplo de uso:
	//   - SameMuscle48h aplica en todas EXCEPTO late luteal → devuelve
	//     [menstrual, follicular, ovulatory, early_luteal]
	//   - SameMuscle72h aplica SOLO en late luteal → devuelve [late_luteal]
	AppliesInPhases() []domain.CyclePhase

	// Evaluate ejecuta la lógica de la regla contra el Context.
	// Devuelve nil si el arrangement es válido para esta regla.
	// Devuelve *Violation con los detalles si hay infracción.
	Evaluate(ctx Context) *Violation
}
