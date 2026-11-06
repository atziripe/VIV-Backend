package usecase

import (
	"context"
	"log"

	"viv/internal/core/nutrition"
)

// ============================================================================
// RESYNC NUTRITION PLAN — keeps an already-generated standalone nutrition
// plan (see submit_nutrition_onboarding.go) aligned with the training week
// that backs it, whenever that week changes (a fresh VIV-106 generation, or
// a VIV-107 daily adaptation that actually changed today's slot). Mirrors
// what save_training_arrangement.go already does for the old pipeline via
// nutrition.ReassembleDays: macros/hydration/day-type are recalculated
// deterministically, and already-selected meal content is preserved
// (borrowed from an existing day of the same new type) rather than
// discarded. A no-op when the user hasn't opted into nutrition yet — most
// users, since it's generated on demand (see submit_nutrition_onboarding.go).
// ============================================================================

type ResyncNutritionPlanUseCase struct {
	NutritionPlans NutritionPlanRepository
	Users          UserRepository
}

func NewResyncNutritionPlanUseCase(nutritionPlans NutritionPlanRepository, users UserRepository) *ResyncNutritionPlanUseCase {
	return &ResyncNutritionPlanUseCase{NutritionPlans: nutritionPlans, Users: users}
}

// Execute is intentionally best-effort from the caller's point of view — it
// returns an error rather than swallowing it, but callers on the daily
// check-in path treat a resync failure as log-and-continue, same as
// CopyEnricher/triggerFirstWeeklyPlan elsewhere: a stale nutrition plan is
// never worth blocking or failing the check-in over.
func (uc *ResyncNutritionPlanUseCase) Execute(ctx context.Context, userID string, draft WeekDraft) error {
	existing, err := uc.NutritionPlans.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil // user hasn't opted into the nutrition module yet
	}

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}

	updated := nutrition.ReassembleDays(existing.Plan, trainingWeekPlanFromDraft(draft), user.WeightKg)
	if err := uc.NutritionPlans.Save(ctx, userID, updated); err != nil {
		return err
	}
	log.Printf("[nutrition.resync] realigned nutrition plan with training week user=%s", userID)
	return nil
}
