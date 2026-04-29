package training

import (
	"math"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// ============================================================================
// MACRO CALCULATOR
// ============================================================================
// Calculates personalized macro targets based on:
//   - BMR via Mifflin-St Jeor
//   - TDEE via NEAT baseline + training session expenditure
//   - Goal-specific caloric adjustment
//   - Goal-specific protein scaling
//   - Training day vs rest day carb/fat split
//
// Science basis:
//   - Mifflin-St Jeor (1990) for BMR — most validated for healthy adults
//   - ISSN position stand (2017) for protein ranges: 1.4–2.0 g/kg
//   - ACSM guidelines for carb/fat distribution
//   - 2025 GSSI review: no phase-based macro changes supported

// CalculateMacros computes training day and rest day macro targets.
func CalculateMacros(user domain.User) domain.MacroTargets {
	bmr := calculateBMR(user)
	sessionsPerWeek := parseSessionsPerWeek(user.TrainingOften)
	neatMultiplier := parseNEATMultiplier(user.DailyActivityLevel)
	goal := primaryGoal(user.TrainingGoals)

	// TDEE = BMR × NEAT baseline + training expenditure
	avgSessionCalories := estimateSessionCalories(user.TrainingDuration, user.TrainingType)
	dailyTrainingCalories := float64(sessionsPerWeek) * avgSessionCalories / 7.0

	baseTDEE := bmr * neatMultiplier
	trainingDayTDEE := baseTDEE + avgSessionCalories
	restDayTDEE := baseTDEE

	// Goal-specific caloric adjustment
	caloricMultiplier := goalCaloricMultiplier(goal)
	trainingDayCalories := trainingDayTDEE * caloricMultiplier
	restDayCalories := restDayTDEE * caloricMultiplier

	// Protein — fixed per day, scaled by goal
	proteinPerKg := goalProteinPerKg(goal)
	proteinG := user.WeightKg * proteinPerKg
	proteinCalories := proteinG * 4.0

	// Training day macros
	trainingDay := distributeMacros(trainingDayCalories, proteinG, proteinCalories, false)

	// Rest day macros — carbs drop ~28%, fat compensates
	restDay := distributeMacros(restDayCalories, proteinG, proteinCalories, true)

	_ = dailyTrainingCalories // used conceptually in TDEE

	return domain.MacroTargets{
		TrainingDay: trainingDay,
		RestDay:     restDay,
	}
}

// ============================================================================
// BMR — Mifflin-St Jeor (female)
// ============================================================================
// BMR = 10 × weight(kg) + 6.25 × height(cm) - 5 × age(years) - 161

func calculateBMR(user domain.User) float64 {
	age := calculateAge(user.DOB)
	return 10.0*user.WeightKg + 6.25*user.HeightCm - 5.0*float64(age) - 161.0
}

func calculateAge(dob string) int {
	// Try parsing common formats
	formats := []string{"2006-01-02", "01/02/2006", "2006"}
	var birthDate time.Time
	var err error

	for _, f := range formats {
		birthDate, err = time.Parse(f, strings.TrimSpace(dob))
		if err == nil {
			break
		}
	}

	if err != nil {
		return 28 // safe default
	}

	now := time.Now()
	age := now.Year() - birthDate.Year()
	if now.YearDay() < birthDate.YearDay() {
		age--
	}
	return age
}

// ============================================================================
// NEAT MULTIPLIER — daily activity outside of training
// ============================================================================

func parseNEATMultiplier(level string) float64 {
	switch strings.TrimSpace(level) {
	case "Mostly sitting":
		return 1.2
	case "Some movement":
		return 1.375
	case "On my feet most of the day":
		return 1.55
	case "Physically demanding work":
		return 1.725
	default:
		return 1.375 // safe default — lightly active
	}
}

// ============================================================================
// SESSION CALORIE ESTIMATION
// ============================================================================
// Rough estimate based on duration and training type.
// These are averages for a ~65kg woman — not precise, but good enough
// for macro calculation purposes.

func estimateSessionCalories(duration string, trainingType string) float64 {
	minutes := parseDurationMinutes(duration)
	calPerMinute := estimateCalPerMinute(trainingType)
	return float64(minutes) * calPerMinute
}

func parseDurationMinutes(s string) int {
	switch strings.TrimSpace(s) {
	case "- 30 min":
		return 25
	case "30 - 45 min":
		return 40
	case "45 - 60 min":
		return 50
	case "1 - 2 hours":
		return 75
	case "+2 hours":
		return 120
	default:
		return 45
	}
}

func estimateCalPerMinute(trainingType string) float64 {
	// Weighted average based on selected modalities
	types := strings.Split(trainingType, ",")
	if len(types) == 0 {
		return 6.0 // default moderate
	}

	totalCal := 0.0
	count := 0
	for _, t := range types {
		switch strings.TrimSpace(t) {
		case "Strength":
			totalCal += 7.0 // moderate-high metabolic cost
		case "HIIT":
			totalCal += 10.0 // highest metabolic cost
		case "Pilates":
			totalCal += 4.0 // lower metabolic cost
		case "Cardio":
			totalCal += 8.0 // moderate-high depending on intensity
		default:
			totalCal += 6.0
		}
		count++
	}

	if count == 0 {
		return 6.0
	}
	return totalCal / float64(count)
}

func parseSessionsPerWeek(s string) int {
	switch strings.TrimSpace(s) {
	case "1-3 times":
		return 2
	case "3-5 times/week":
		return 4
	case "Everyday":
		return 6
	case "Rarely":
		return 1
	default:
		return 3
	}
}

// ============================================================================
// GOAL-SPECIFIC ADJUSTMENTS
// ============================================================================

func primaryGoal(goals string) string {
	if strings.TrimSpace(goals) == "" {
		return "maintenance"
	}
	parts := strings.Split(goals, ",")
	return strings.TrimSpace(parts[0])
}

// goalProteinPerKg returns protein in g/kg based on training goal.
// Ranges within ISSN guidelines (1.4–2.0 g/kg for active individuals).
func goalProteinPerKg(goal string) float64 {
	normalized := normalizeGoal(goal)
	switch normalized {
	case "body recomposition":
		return 2.0 // highest — preserve muscle in deficit
	case "performance and progression":
		return 1.9 // high — maximal adaptation
	case "strength and resilience":
		return 1.8 // high — muscle adaptation and recovery
	case "maintenance":
		return 1.6 // moderate — sustain current composition
	case "energy and consistency":
		return 1.4 // lower — sustainable, not performance-driven
	case "stress regulation":
		return 1.4 // lower — wellbeing focus
	default:
		return 1.6 // moderate default
	}
}

// goalCaloricMultiplier returns the caloric adjustment factor.
func goalCaloricMultiplier(goal string) float64 {
	normalized := normalizeGoal(goal)
	switch normalized {
	case "body recomposition":
		return 0.85 // -15% moderate deficit
	case "performance and progression":
		return 1.05 // +5% slight surplus
	default:
		return 1.0 // maintenance
	}
}

// ============================================================================
// MACRO DISTRIBUTION
// ============================================================================
// After protein is allocated, remaining calories split between carbs and fat.
// Training day: ~55% carbs, ~25% fat (from remaining)
// Rest day: carbs drop ~28%, fat compensates

func distributeMacros(totalCalories, proteinG, proteinCalories float64, isRestDay bool) domain.DayMacros {
	remainingCalories := totalCalories - proteinCalories

	var carbCalories, fatCalories float64

	if isRestDay {
		// Rest day: carbs ~40% of remaining, fat ~60%
		carbCalories = remainingCalories * 0.40
		fatCalories = remainingCalories * 0.60
	} else {
		// Training day: carbs ~60% of remaining, fat ~40%
		carbCalories = remainingCalories * 0.60
		fatCalories = remainingCalories * 0.40
	}

	carbsG := carbCalories / 4.0
	fatG := fatCalories / 9.0

	return domain.DayMacros{
		Calories: int(math.Round(totalCalories)),
		ProteinG: math.Round(proteinG),
		CarbsG:   math.Round(carbsG),
		FatG:     math.Round(fatG),
	}
}
