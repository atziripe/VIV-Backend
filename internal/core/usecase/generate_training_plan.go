package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/training"
)

// ============================================================================
// GENERATE TRAINING PLAN — the main orchestrator
// ============================================================================

// GenerateTrainingPlanUsecase orchestrates the entire plan generation pipeline:
//
//	Layer 0 (budget) → Layer 1 (LLM structure) → Engine (validate) →
//	retry if blocked → Layer 2 (transformations) → Layer 3 (content selection)
type GenerateTrainingPlanUsecase struct {
	generator TrainingStructureGenerator
	engine    *rules.Engine
	library   *training.Library
}

func NewGenerateTrainingPlanUsecase(
	generator TrainingStructureGenerator,
	engine *rules.Engine,
	library *training.Library,
) *GenerateTrainingPlanUsecase {
	return &GenerateTrainingPlanUsecase{
		generator: generator,
		engine:    engine,
		library:   library,
	}
}

// GenerateTrainingPlanInput contains everything needed to generate a plan.
type GenerateTrainingPlanInput struct {
	User    domain.User
	Checkin domain.Checkin
	Phase   domain.CyclePhase
}

// GenerateTrainingPlanOutput is the result of plan generation.
type GenerateTrainingPlanOutput struct {
	Plan        domain.TrainingWeekPlan
	Budget      domain.WeekBudget
	Decision    rules.Decision
	TokensUsed  TokenUsage
	RetriedOnce bool
}

func (uc *GenerateTrainingPlanUsecase) Execute(
	ctx context.Context,
	input GenerateTrainingPlanInput,
) (GenerateTrainingPlanOutput, error) {

	// ── Layer 0: Budget calculation ──────────────────────────────
	prefs := training.ParseTrainingPreferences(input.User)
	dimensions := training.TranslateCheckin(input.Checkin)
	budget := training.CalculateBudget(prefs, input.Phase, dimensions)

	// ── Build constraints for the LLM ────────────────────────────
	constraints := training.BuildConstraints(input.Phase)

	log.Printf("[training.generate] checkin input: %v",
		input.Checkin)

	log.Printf("[training.generate] checkin dimensions: recovery=%s bandwidth=%s build=%s",
		dimensions.Recovery, dimensions.Bandwidth, dimensions.Build)

	log.Printf("[training.generate] budget: total=%d sessions, rest_week=%v",
		budget.TotalSessions, budget.RestWeekOffered)

	// ── Layer 1: LLM generates structure ─────────────────────────
	genInput := StructureGenerationInput{
		Phase:       input.Phase,
		Budget:      budget,
		Constraints: constraints,
	}

	arrangement, tokens, err := uc.generator.GenerateWeekStructure(ctx, genInput)
	if err != nil {
		return GenerateTrainingPlanOutput{}, fmt.Errorf("layer 1 generation failed: %w", err)
	}

	// ── Engine: validate structure ───────────────────────────────
	decision := uc.engine.Evaluate(input.Phase, arrangement)
	retriedOnce := false

	if decision.HasHardBlock() {
		// ── Retry once with violation feedback ────────────────────
		log.Printf("[training.generate] first attempt blocked: %d violations, retrying",
			len(decision.HardBlocks()))

		feedback := formatViolationFeedback(decision)
		genInput.RetryFeedback = feedback

		arrangement, tokens, err = uc.generator.GenerateWeekStructure(ctx, genInput)
		if err != nil {
			return GenerateTrainingPlanOutput{}, fmt.Errorf("layer 1 retry failed: %w", err)
		}

		decision = uc.engine.Evaluate(input.Phase, arrangement)
		retriedOnce = true

		if decision.HasHardBlock() {
			log.Printf("[training.generate] retry also blocked: %d violations",
				len(decision.HardBlocks()))
			return GenerateTrainingPlanOutput{}, fmt.Errorf(
				"plan generation failed after retry — %d hard blocks remain",
				len(decision.HardBlocks()),
			)
		}
	}

	// ── Layer 2 + 3: Transform and assemble ──────────────────────
	plan := training.AssemblePlan(
		arrangement,
		input.Phase,
		uc.library,
		dimensions,
		budget.RestWeekOffered,
	)

	return GenerateTrainingPlanOutput{
		Plan:        plan,
		Budget:      budget,
		Decision:    decision,
		TokensUsed:  tokens,
		RetriedOnce: retriedOnce,
	}, nil
}

// formatViolationFeedback converts hard block violations into a message
// the LLM can understand and act on for its retry attempt.
func formatViolationFeedback(d rules.Decision) string {
	blocks := d.HardBlocks()
	if len(blocks) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Your previous proposal violated these rules:\n")
	for i, v := range blocks {
		b.WriteString(fmt.Sprintf("(%d) %s: %s\n", i+1, v.RuleID, v.Message))
	}
	b.WriteString("\nPlease regenerate respecting these constraints.")
	return b.String()
}
