package rules_test

import (
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
)

// ============================================================================
// REGLAS FAKE PARA TESTING
// ============================================================================
// Estas reglas no tienen lógica de dominio. Existen solo para verificar que
// el engine las recorre, filtra por fase, acumula violaciones, y computa
// el status correctamente.

// neverFails es una regla que siempre dice "todo bien".
type neverFails struct{}

func (r neverFails) ID() string                                  { return "never_fails" }
func (r neverFails) AppliesInPhases() []domain.CyclePhase        { return nil } // todas las fases
func (r neverFails) Evaluate(ctx rules.Context) *rules.Violation { return nil }

// alwaysWarns es una regla que siempre devuelve un soft warning.
type alwaysWarns struct{}

func (r alwaysWarns) ID() string                           { return "always_warns" }
func (r alwaysWarns) AppliesInPhases() []domain.CyclePhase { return nil }
func (r alwaysWarns) Evaluate(ctx rules.Context) *rules.Violation {
	return &rules.Violation{
		RuleID:       r.ID(),
		Severity:     rules.SeveritySoftWarning,
		Message:      "this is a test warning",
		AffectedDays: []int{0},
	}
}

// alwaysBlocks es una regla que siempre devuelve un hard block.
type alwaysBlocks struct{}

func (r alwaysBlocks) ID() string                           { return "always_blocks" }
func (r alwaysBlocks) AppliesInPhases() []domain.CyclePhase { return nil }
func (r alwaysBlocks) Evaluate(ctx rules.Context) *rules.Violation {
	return &rules.Violation{
		RuleID:       r.ID(),
		Severity:     rules.SeverityHardBlock,
		Message:      "this is a test block",
		AffectedDays: []int{1, 2},
	}
}

// onlyFollicular es una regla que solo corre en fase folicular.
// Devuelve un hard block siempre que se ejecuta.
type onlyFollicular struct{}

func (r onlyFollicular) ID() string { return "only_follicular" }
func (r onlyFollicular) AppliesInPhases() []domain.CyclePhase {
	return []domain.CyclePhase{domain.PhaseFollicular}
}
func (r onlyFollicular) Evaluate(ctx rules.Context) *rules.Violation {
	return &rules.Violation{
		RuleID:       r.ID(),
		Severity:     rules.SeverityHardBlock,
		Message:      "follicular-only rule triggered",
		AffectedDays: []int{0},
	}
}

// ============================================================================
// HELPER — arrangement vacío para no repetir boilerplate en cada test
// ============================================================================

func emptyArrangement() domain.WeekArrangement {
	return domain.WeekArrangement{
		WeekStart: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC), // lunes
		Days: [7]domain.TrainingDaySlot{
			{Weekday: time.Monday},
			{Weekday: time.Tuesday},
			{Weekday: time.Wednesday},
			{Weekday: time.Thursday},
			{Weekday: time.Friday},
			{Weekday: time.Saturday},
			{Weekday: time.Sunday},
		},
	}
}

// ============================================================================
// TESTS
// ============================================================================

func TestEngine_NoRules_ReturnsValid(t *testing.T) {
	engine := rules.NewEngine(nil)

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.Status != rules.StatusValid {
		t.Errorf("expected status %q, got %q", rules.StatusValid, decision.Status)
	}
	if len(decision.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(decision.Violations))
	}
}

func TestEngine_RuleCount(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		neverFails{},
		alwaysWarns{},
		alwaysBlocks{},
	})

	if engine.RuleCount() != 3 {
		t.Errorf("expected 3 rules, got %d", engine.RuleCount())
	}
}

func TestEngine_AllRulesPass_ReturnsValid(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		neverFails{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.Status != rules.StatusValid {
		t.Errorf("expected status %q, got %q", rules.StatusValid, decision.Status)
	}
	if len(decision.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(decision.Violations))
	}
}

func TestEngine_SoftWarningOnly_ReturnsValidWithWarnings(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		neverFails{},
		alwaysWarns{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.Status != rules.StatusValidWithWarnings {
		t.Errorf("expected status %q, got %q", rules.StatusValidWithWarnings, decision.Status)
	}
	if len(decision.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(decision.Violations))
	}
	if decision.Violations[0].RuleID != "always_warns" {
		t.Errorf("expected rule ID %q, got %q", "always_warns", decision.Violations[0].RuleID)
	}
	if decision.Violations[0].Severity != rules.SeveritySoftWarning {
		t.Errorf("expected severity %q, got %q", rules.SeveritySoftWarning, decision.Violations[0].Severity)
	}
}

func TestEngine_HardBlock_ReturnsBlocked(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		alwaysBlocks{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.Status != rules.StatusBlocked {
		t.Errorf("expected status %q, got %q", rules.StatusBlocked, decision.Status)
	}
	if len(decision.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(decision.Violations))
	}
	if decision.Violations[0].Severity != rules.SeverityHardBlock {
		t.Errorf("expected severity %q, got %q", rules.SeverityHardBlock, decision.Violations[0].Severity)
	}
}

func TestEngine_CollectsAll_WarningAndBlock(t *testing.T) {
	// Verifica que el engine acumula violaciones de MÚLTIPLES reglas,
	// no se detiene en la primera.
	engine := rules.NewEngine([]rules.Rule{
		alwaysWarns{},
		alwaysBlocks{},
		neverFails{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.Status != rules.StatusBlocked {
		t.Errorf("expected status %q, got %q", rules.StatusBlocked, decision.Status)
	}
	if len(decision.Violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(decision.Violations))
	}

	// Verificar que tenemos ambos tipos
	hasWarning := false
	hasBlock := false
	for _, v := range decision.Violations {
		if v.Severity == rules.SeveritySoftWarning {
			hasWarning = true
		}
		if v.Severity == rules.SeverityHardBlock {
			hasBlock = true
		}
	}
	if !hasWarning {
		t.Error("expected at least one soft warning")
	}
	if !hasBlock {
		t.Error("expected at least one hard block")
	}
}

func TestEngine_PhaseFiltering_RuleSkippedInWrongPhase(t *testing.T) {
	// onlyFollicular solo corre en follicular.
	// Si evaluamos en menstrual, la regla no debería ejecutarse.
	engine := rules.NewEngine([]rules.Rule{
		onlyFollicular{},
	})

	decision := engine.Evaluate(domain.PhaseMenstrual, emptyArrangement())

	if decision.Status != rules.StatusValid {
		t.Errorf("expected status %q (rule should be skipped), got %q", rules.StatusValid, decision.Status)
	}
	if len(decision.Violations) != 0 {
		t.Errorf("expected 0 violations (rule should be skipped), got %d", len(decision.Violations))
	}
}

func TestEngine_PhaseFiltering_RuleRunsInCorrectPhase(t *testing.T) {
	// Misma regla, pero ahora evaluamos en follicular → sí debe correr.
	engine := rules.NewEngine([]rules.Rule{
		onlyFollicular{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.Status != rules.StatusBlocked {
		t.Errorf("expected status %q, got %q", rules.StatusBlocked, decision.Status)
	}
	if len(decision.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(decision.Violations))
	}
	if decision.Violations[0].RuleID != "only_follicular" {
		t.Errorf("expected rule ID %q, got %q", "only_follicular", decision.Violations[0].RuleID)
	}
}

func TestEngine_PhaseFiltering_MixedRules(t *testing.T) {
	// Mezcla: una regla universal + una regla solo-follicular.
	// Evaluamos en menstrual → solo la universal corre.
	engine := rules.NewEngine([]rules.Rule{
		alwaysWarns{},    // universal
		onlyFollicular{}, // solo follicular
	})

	decision := engine.Evaluate(domain.PhaseMenstrual, emptyArrangement())

	if decision.Status != rules.StatusValidWithWarnings {
		t.Errorf("expected status %q, got %q", rules.StatusValidWithWarnings, decision.Status)
	}
	if len(decision.Violations) != 1 {
		t.Fatalf("expected 1 violation (only universal rule), got %d", len(decision.Violations))
	}
	if decision.Violations[0].RuleID != "always_warns" {
		t.Errorf("expected rule ID %q, got %q", "always_warns", decision.Violations[0].RuleID)
	}
}

func TestDecision_HasHardBlock(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		alwaysWarns{},
		alwaysBlocks{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if !decision.HasHardBlock() {
		t.Error("expected HasHardBlock() to be true")
	}
}

func TestDecision_HasHardBlock_FalseWhenOnlyWarnings(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		alwaysWarns{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())

	if decision.HasHardBlock() {
		t.Error("expected HasHardBlock() to be false with only warnings")
	}
}

func TestDecision_HardBlocks_ReturnsOnlyHardBlocks(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		alwaysWarns{},
		alwaysBlocks{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())
	blocks := decision.HardBlocks()

	if len(blocks) != 1 {
		t.Fatalf("expected 1 hard block, got %d", len(blocks))
	}
	if blocks[0].RuleID != "always_blocks" {
		t.Errorf("expected rule ID %q, got %q", "always_blocks", blocks[0].RuleID)
	}
}

func TestDecision_SoftWarnings_ReturnsOnlyWarnings(t *testing.T) {
	engine := rules.NewEngine([]rules.Rule{
		alwaysWarns{},
		alwaysBlocks{},
	})

	decision := engine.Evaluate(domain.PhaseFollicular, emptyArrangement())
	warnings := decision.SoftWarnings()

	if len(warnings) != 1 {
		t.Fatalf("expected 1 soft warning, got %d", len(warnings))
	}
	if warnings[0].RuleID != "always_warns" {
		t.Errorf("expected rule ID %q, got %q", "always_warns", warnings[0].RuleID)
	}
}
