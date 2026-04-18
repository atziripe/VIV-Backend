package rules

// ============================================================================
// STATUS — Final veredict to check arrangements
// ============================================================================
//   - No violation - Valid
//   - Soft warnings only - ValidWithWarnings
//   - At least one hard block -  Blocked

type Status string

const (
	StatusValid             Status = "valid"
	StatusValidWithWarnings Status = "valid_with_warnings"
	StatusBlocked           Status = "blocked"
)

// ============================================================================
// SEVERITY — qué tan grave es una violación individual
// ============================================================================
// Hard block  = el arrangement es fisiológicamente problemático. No se permite.
// Soft warning = el arrangement es subóptimo pero no peligroso. La usuaria
//                puede "overridear" si insiste (en el frontend).

type Severity string

const (
	SeverityHardBlock   Severity = "hard_block"
	SeveritySoftWarning Severity = "soft_warning"
)

// ============================================================================
// VIOLATION — una infracción individual detectada por una regla
// ============================================================================
// Cada violación identifica:
//   - QUÉ regla disparó (RuleID)
//   - QUÉ TAN GRAVE es (Severity)
//   - POR QUÉ (Message — texto human-readable, listo para UI o para feedback al LLM)
//   - DÓNDE (AffectedDays — índices 0..6 de los días involucrados, para highlighting en UI)

type Violation struct {
	RuleID       string   `json:"rule_id"`
	Severity     Severity `json:"severity"`
	Message      string   `json:"message"`
	AffectedDays []int    `json:"affected_days"`
}

// ============================================================================
// DECISION — veredicto completo que devuelve el engine
// ============================================================================
// Es lo que retorna Engine.Evaluate().
// El frontend/usecase lee Status para decidir qué hacer, y Violations para
// mostrar detalles o construir feedback al LLM.

type Decision struct {
	Status     Status      `json:"status"`
	Violations []Violation `json:"violations"`
}

// ============================================================================
// HELPERS — métodos útiles sobre Decision
// ============================================================================

// HasHardBlock devuelve true si al menos una violación es hard block.
// Útil para decidir si el arrangement se puede persistir o hay que reintentar.
func (d Decision) HasHardBlock() bool {
	for _, v := range d.Violations {
		if v.Severity == SeverityHardBlock {
			return true
		}
	}
	return false
}

// HardBlocks devuelve solo las violaciones de severidad hard block.
// Útil para formatear feedback al LLM (solo los hard blocks ameritan reintento;
// los soft warnings son aceptables).
func (d Decision) HardBlocks() []Violation {
	var out []Violation
	for _, v := range d.Violations {
		if v.Severity == SeverityHardBlock {
			out = append(out, v)
		}
	}
	return out
}

// SoftWarnings devuelve solo las violaciones de severidad soft warning.
// Útil para mostrar al usuario advertencias que no bloquean la acción.
func (d Decision) SoftWarnings() []Violation {
	var out []Violation
	for _, v := range d.Violations {
		if v.Severity == SeveritySoftWarning {
			out = append(out, v)
		}
	}
	return out
}
