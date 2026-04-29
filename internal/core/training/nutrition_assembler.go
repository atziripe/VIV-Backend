package training

import (
	"math"
	"viv/internal/core/domain"
)

// ============================================================================
// NUTRITION ASSEMBLER
// ============================================================================
// Combines macro targets + hydration + training plan to produce
// a per-day nutrition plan for the week.

// AssembleNutritionPlan builds the weekly nutrition plan based on
// the user's macro targets, the training plan (to know which days
// are training vs rest), and the cycle phase (for hydration).
func AssembleNutritionPlan(
	user domain.User,
	trainingPlan domain.TrainingWeekPlan,
	phase domain.CyclePhase,
) domain.NutritionWeekPlan {
	macros := CalculateMacros(user)

	weekdayNames := [7]string{
		"monday", "tuesday", "wednesday", "thursday",
		"friday", "saturday", "sunday",
	}

	var days [7]domain.NutritionDayPlan

	for i, trainingDay := range trainingPlan.Days {
		isTraining := !trainingDay.IsRestDay && trainingDay.Session != nil

		// Select correct macro set
		dayMacros := macros.RestDay
		if isTraining {
			dayMacros = macros.TrainingDay
		}

		// Calculate hydration for this specific day
		exerciseMinutes := 0
		if isTraining && trainingDay.Session != nil {
			exerciseMinutes = trainingDay.Session.DurationMinutes
		}
		hydration := CalculateHydration(user.WeightKg, exerciseMinutes, phase)

		days[i] = domain.NutritionDayPlan{
			Weekday:       weekdayNames[i],
			IsTrainingDay: isTraining,
			Macros:        dayMacros,
			Hydration: domain.HydrationInfo{
				Liters:          hydration.Liters,
				Glasses:         hydration.Glasses,
				ShowElectrolyte: hydration.ShowElectrolyte,
				ShowCapNote:     hydration.ShowCapNote,
			},
		}
	}

	return domain.NutritionWeekPlan{
		Phase:   phase,
		Targets: macros,
		Days:    days,
		Water:   math.Round(user.WeightKg*0.033*10) / 10,
	}
}
