package training_test

import (
	"math"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

func testUser() domain.User {
	return domain.User{
		WeightKg:           65.0,
		HeightCm:           165.0,
		DOB:                "1996-05-15",
		TrainingOften:      "3-5 times/week",
		TrainingDuration:   "45 - 60 min",
		TrainingType:       "Strength,Pilates,HIIT,Cardio",
		TrainingGoals:      "Strength & Resilience",
		DailyActivityLevel: "Some movement",
	}
}

func TestMacros_ProteinScalesByGoal(t *testing.T) {
	tests := []struct {
		goal          string
		expectedPerKg float64
	}{
		{"Body recomposition", 2.0},
		{"Performance & Progression", 1.9},
		{"Strength & Resilience", 1.8},
		{"Maintenance", 1.6},
		{"Energy & Consistency", 1.4},
		{"Stress regulation", 1.4},
	}

	for _, tt := range tests {
		user := testUser()
		user.TrainingGoals = tt.goal

		macros := training.CalculateMacros(user)

		expectedProtein := math.Round(user.WeightKg * tt.expectedPerKg)
		if macros.TrainingDay.ProteinG != expectedProtein {
			t.Errorf("goal %q: expected protein %.0fg, got %.0fg",
				tt.goal, expectedProtein, macros.TrainingDay.ProteinG)
		}
	}
}

func TestMacros_ProteinSameTrainingAndRest(t *testing.T) {
	user := testUser()
	macros := training.CalculateMacros(user)

	if macros.TrainingDay.ProteinG != macros.RestDay.ProteinG {
		t.Errorf("protein should be same on training and rest day: training=%.0f rest=%.0f",
			macros.TrainingDay.ProteinG, macros.RestDay.ProteinG)
	}
}

func TestMacros_CarbsLowerOnRestDay(t *testing.T) {
	user := testUser()
	macros := training.CalculateMacros(user)

	if macros.RestDay.CarbsG >= macros.TrainingDay.CarbsG {
		t.Errorf("rest day carbs (%.0f) should be lower than training day (%.0f)",
			macros.RestDay.CarbsG, macros.TrainingDay.CarbsG)
	}
}

func TestMacros_FatHigherOnRestDay(t *testing.T) {
	user := testUser()
	macros := training.CalculateMacros(user)

	if macros.RestDay.FatG <= macros.TrainingDay.FatG {
		t.Errorf("rest day fat (%.0f) should be higher than training day (%.0f)",
			macros.RestDay.FatG, macros.TrainingDay.FatG)
	}
}

func TestMacros_BodyRecompHasDeficit(t *testing.T) {
	user := testUser()
	user.TrainingGoals = "Body recomposition"
	recompMacros := training.CalculateMacros(user)

	user.TrainingGoals = "Maintenance"
	maintMacros := training.CalculateMacros(user)

	if recompMacros.TrainingDay.Calories >= maintMacros.TrainingDay.Calories {
		t.Errorf("body recomp calories (%d) should be less than maintenance (%d)",
			recompMacros.TrainingDay.Calories, maintMacros.TrainingDay.Calories)
	}
}

func TestMacros_PerformanceHasSurplus(t *testing.T) {
	user := testUser()
	user.TrainingGoals = "Performance & Progression"
	perfMacros := training.CalculateMacros(user)

	user.TrainingGoals = "Maintenance"
	maintMacros := training.CalculateMacros(user)

	if perfMacros.TrainingDay.Calories <= maintMacros.TrainingDay.Calories {
		t.Errorf("performance calories (%d) should be more than maintenance (%d)",
			perfMacros.TrainingDay.Calories, maintMacros.TrainingDay.Calories)
	}
}

func TestMacros_NEATAffectsTDEE(t *testing.T) {
	user := testUser()

	user.DailyActivityLevel = "Mostly sitting"
	sedentary := training.CalculateMacros(user)

	user.DailyActivityLevel = "On my feet most of the day"
	active := training.CalculateMacros(user)

	if active.TrainingDay.Calories <= sedentary.TrainingDay.Calories {
		t.Errorf("active TDEE (%d) should be higher than sedentary (%d)",
			active.TrainingDay.Calories, sedentary.TrainingDay.Calories)
	}
}

func TestMacros_CaloriesAddUp(t *testing.T) {
	user := testUser()
	macros := training.CalculateMacros(user)

	// Verify training day calories = protein×4 + carbs×4 + fat×9 (± rounding)
	calculated := macros.TrainingDay.ProteinG*4 + macros.TrainingDay.CarbsG*4 + macros.TrainingDay.FatG*9
	diff := math.Abs(float64(macros.TrainingDay.Calories) - calculated)

	if diff > 15 { // allow small rounding difference
		t.Errorf("training day calories don't add up: stated=%d, calculated=%.0f, diff=%.0f",
			macros.TrainingDay.Calories, calculated, diff)
	}

	// Same for rest day
	calculated = macros.RestDay.ProteinG*4 + macros.RestDay.CarbsG*4 + macros.RestDay.FatG*9
	diff = math.Abs(float64(macros.RestDay.Calories) - calculated)

	if diff > 15 {
		t.Errorf("rest day calories don't add up: stated=%d, calculated=%.0f, diff=%.0f",
			macros.RestDay.Calories, calculated, diff)
	}
}

func TestMacros_ReasonableRange(t *testing.T) {
	// A 65kg, 165cm, 30yo moderately active woman should get
	// roughly 1600-2200 calories on a training day
	user := testUser()
	macros := training.CalculateMacros(user)

	if macros.TrainingDay.Calories < 1400 || macros.TrainingDay.Calories > 2800 {
		t.Errorf("training day calories %d seems unreasonable for this profile",
			macros.TrainingDay.Calories)
	}

	if macros.RestDay.Calories < 1200 || macros.RestDay.Calories > 2400 {
		t.Errorf("rest day calories %d seems unreasonable for this profile",
			macros.RestDay.Calories)
	}
}
