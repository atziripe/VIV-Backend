package usecase

import (
	"context"
	"log"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

// GenerateNutritionPlanInput contains everything needed to generate nutrition.
type GenerateNutritionPlanInput struct {
	User         domain.User
	Checkin      domain.Checkin
	Phase        domain.CyclePhase
	TrainingPlan domain.TrainingWeekPlan // needed to know training vs rest days
}

// GenerateNutritionPlanOutput is the result of nutrition generation.
type GenerateNutritionPlanOutput struct {
	Plan       domain.NutritionWeekPlan
	TokensUsed TokenUsage
}

// GenerateNutritionPlanUsecase orchestrates the nutrition pipeline:
//
//	Layer 0: macro calculation + hydration (Go pure)
//	Layer 1: meal content generation (LLM)
//	Assembly: merge meals into the nutrition day plans
type GenerateNutritionPlanUsecase struct {
	mealGenerator MealContentGenerator
}

func NewGenerateNutritionPlanUsecase(
	mealGenerator MealContentGenerator,
) *GenerateNutritionPlanUsecase {
	return &GenerateNutritionPlanUsecase{
		mealGenerator: mealGenerator,
	}
}

func (uc *GenerateNutritionPlanUsecase) Execute(
	ctx context.Context,
	input GenerateNutritionPlanInput,
) (GenerateNutritionPlanOutput, error) {

	// ── Layer 0: Macros + Hydration (Go pure) ────────────────────
	nutritionPlan := training.AssembleNutritionPlan(
		input.User,
		input.TrainingPlan,
		input.Phase,
	)

	// ── Build meal generation input ──────────────────────────────
	sessionTime := training.ParseTrainingTime(input.User.TrainingTime)
	mealInput := buildMealInput(
		input.User,
		nutritionPlan,
		input.Phase,
		sessionTime,
		input.Checkin,
	)

	// ── Layer 1: Meal content via LLM (with 1 retry) ────────────────────────────
	var tokens TokenUsage

	mealDays, mealTokens, err := uc.mealGenerator.GenerateWeekMeals(ctx, mealInput)
	if err != nil {
		// Don't fail entirely — return nutrition plan without meal content
		log.Printf("[nutrition.generate] first attempt failed: %v, retrying", err)

		mealDays, mealTokens, err = uc.mealGenerator.GenerateWeekMeals(ctx, mealInput)
		if err != nil {
			log.Printf("[nutrition.generate] retry also failed: %v", err)
			return GenerateNutritionPlanOutput{
				Plan:       nutritionPlan,
				TokensUsed: tokens,
			}, nil
		}
	}
	tokens = mealTokens

	// ── Merge meals into nutrition plan ───────────────────────────
	for _, dayMeals := range mealDays {
		for i := range nutritionPlan.Days {
			if nutritionPlan.Days[i].Weekday == dayMeals.Weekday {
				nutritionPlan.Days[i].Meals = dayMeals.Meals
				break
			}
		}
	}

	return GenerateNutritionPlanOutput{
		Plan:       nutritionPlan,
		TokensUsed: tokens,
	}, nil
}

func buildMealInput(
	user domain.User,
	nutritionPlan domain.NutritionWeekPlan,
	phase domain.CyclePhase,
	sessionTime training.TrainingTimeOfDay,
	checkin domain.Checkin,
) MealGenerationInput {
	var days []MealDayInput

	for _, day := range nutritionPlan.Days {
		var slots []string
		structure := training.ResolveMealStructure(day.IsTrainingDay, sessionTime, phase)
		for _, slot := range structure.Slots {
			slots = append(slots, string(slot.Type))
		}

		dayInput := MealDayInput{
			Weekday:       day.Weekday,
			IsTrainingDay: day.IsTrainingDay,
			SessionTime:   string(sessionTime),
			Macros:        day.Macros,
			MealSlots:     slots,
		}

		if !day.IsTrainingDay {
			dayInput.SessionTime = ""
		}

		days = append(days, dayInput)
	}

	return MealGenerationInput{
		Phase:               phase,
		Targets:             nutritionPlan.Targets,
		Days:                days,
		DietRestrictions:    training.ParseDietRestrictions(user.DietRestrictions),
		ProteinPreference:   user.DietProteinResources,
		DigestiveConditions: training.ParseDigestiveConditions(user.DigestionConditions),
		AppetiteStatus:      string(checkin.AppetiteV2),
		EatingStyle:         user.EatingStyle,
	}
}
