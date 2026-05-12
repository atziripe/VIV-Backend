package nutrition

import (
	"viv/internal/core/domain"
	"viv/internal/core/training"
)

// ReassembleDays updates a NutritionWeekPlan to reflect a new training arrangement.
// LLM-generated meals are preserved by borrowing from an existing day of the same type.
// Macros and hydration are recalculated deterministically.
func ReassembleDays(
	existing domain.NutritionWeekPlan,
	updatedTraining domain.TrainingWeekPlan,
	weightKg float64,
) domain.NutritionWeekPlan {
	result := existing

	// Pre-index meal templates from the existing plan:
	// find the first training day and first rest day that have meals.
	var trainingDayMeals []domain.MealSlot
	var restDayMeals []domain.MealSlot

	for _, d := range existing.Days {
		if d.IsTrainingDay && trainingDayMeals == nil && len(d.Meals) > 0 {
			trainingDayMeals = d.Meals
		}
		if !d.IsTrainingDay && restDayMeals == nil && len(d.Meals) > 0 {
			restDayMeals = d.Meals
		}
		if trainingDayMeals != nil && restDayMeals != nil {
			break
		}
	}

	phase := existing.Phase

	for i, trainingDay := range updatedTraining.Days {
		if i >= len(result.Days) {
			break
		}

		isTrainingDay := !trainingDay.IsRestDay && trainingDay.Session != nil

		// Recalculate macros
		if isTrainingDay {
			result.Days[i].Macros = existing.Targets.TrainingDay
		} else {
			result.Days[i].Macros = existing.Targets.RestDay
		}

		// Recalculate hydration
		durationMin := 0
		if isTrainingDay && trainingDay.Session != nil {
			durationMin = trainingDay.Session.DurationMinutes
		}
		h := training.CalculateHydration(weightKg, durationMin, phase)
		result.Days[i].Hydration = domain.HydrationInfo{
			Liters:          h.Liters,
			Glasses:         h.Glasses,
			ShowElectrolyte: h.ShowElectrolyte,
			ShowCapNote:     h.ShowCapNote,
		}

		// Swap meals only if the day type changed
		dayTypeChanged := result.Days[i].IsTrainingDay != isTrainingDay
		if dayTypeChanged {
			if isTrainingDay {
				result.Days[i].Meals = trainingDayMeals
			} else {
				result.Days[i].Meals = restDayMeals
			}
		}

		result.Days[i].IsTrainingDay = isTrainingDay
	}

	return result
}
