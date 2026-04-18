package rules

import "viv/internal/core/domain"

// ============================================================================
// ENGINE — el motor de evaluación de reglas
// ============================================================================
// El Engine es stateless después de su construcción. Recibe las reglas al
// crearse y las mantiene inmutables. Se puede reutilizar para evaluar
// múltiples arrangements concurrentemente sin riesgo.
//
// Modo de operación: COLLECT-ALL
// Ejecuta TODAS las reglas aplicables y acumula TODAS las violaciones.
// No se detiene en la primera. Esto permite:
//   - Mostrar al usuario todos los problemas de un arreglo de una vez
//   - Darle al LLM feedback completo para que corrija todo en un solo reintento
//
// Alternativa descartada: FIRST-MATCH (parar en la primera violación)
// No sirve para training porque un arreglo puede violar varias reglas a la vez
// y queremos reportar todas. Si usáramos first-match, el ciclo sería:
// viola → corrige → viola otra → corrige → viola otra... Frustrante para la
// usuaria y caro en reintentos del LLM.

// Engine evalúa un WeekArrangement contra un conjunto de reglas.
type Engine struct {
	rules []Rule
}

// NewEngine crea un Engine con el set de reglas dado.
// El orden de las reglas NO importa — el engine las ejecuta todas y agrega
// resultados. No hay prioridad ni override entre reglas.
//
// Uso típico: crear UNA instancia al boot del servicio e inyectarla
// en los usecases que la necesiten. No crear una por request.
func NewEngine(rules []Rule) *Engine {
	return &Engine{rules: rules}
}

// Evaluate corre todas las reglas aplicables a la fase actual contra el
// arrangement y devuelve un Decision con el veredicto agregado.
//
// Es la única operación pública del engine. Todo lo demás es interno.
//
// Flujo:
//  1. Para cada regla registrada:
//     a. ¿Aplica en esta fase? Si no, skip.
//     b. Ejecutar regla contra el Context.
//     c. Si devuelve violación, acumular.
//  2. Calcular Status agregado a partir de las violaciones acumuladas.
//  3. Devolver Decision.
func (e *Engine) Evaluate(phase domain.CyclePhase, arrangement domain.WeekArrangement) Decision {
	ctx := Context{
		Phase:       phase,
		Arrangement: arrangement,
	}

	var violations []Violation

	for _, rule := range e.rules {
		if !ruleAppliesInPhase(rule, phase) {
			continue
		}

		if v := rule.Evaluate(ctx); v != nil {
			violations = append(violations, *v)
		}
	}

	return Decision{
		Status:     computeStatus(violations),
		Violations: violations,
	}
}

// ============================================================================
// FUNCIONES INTERNAS
// ============================================================================

// ruleAppliesInPhase verifica si una regla debe ejecutarse en la fase actual.
//
// Convención: si AppliesInPhases() devuelve nil o vacío, la regla aplica
// en TODAS las fases (es universal). Esto es el caso más común.
func ruleAppliesInPhase(rule Rule, phase domain.CyclePhase) bool {
	phases := rule.AppliesInPhases()
	if len(phases) == 0 {
		return true
	}
	for _, p := range phases {
		if p == phase {
			return true
		}
	}
	return false
}

// computeStatus deriva el Status del Decision a partir de las violaciones.
//
// Es una función pura — no lee ni modifica estado externo. Se puede testear
// en aislamiento si fuera necesario.
//
// Lógica:
//   - Sin violaciones      → Valid
//   - Solo soft warnings   → ValidWithWarnings
//   - Al menos un hard block → Blocked
func computeStatus(violations []Violation) Status {
	if len(violations) == 0 {
		return StatusValid
	}
	for _, v := range violations {
		if v.Severity == SeverityHardBlock {
			return StatusBlocked
		}
	}
	return StatusValidWithWarnings
}

// RuleCount devuelve la cantidad de reglas registradas en el engine.
// Útil para tests y observabilidad.
func (e *Engine) RuleCount() int {
	return len(e.rules)
}
