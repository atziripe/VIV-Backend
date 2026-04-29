package training_test

import (
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/training"
)

func TestMealStructure_RestDay_Has3Slots(t *testing.T) {
	structure := training.ResolveMealStructure(false, "", domain.PhaseFollicular)

	if len(structure.Slots) != 3 {
		t.Fatalf("rest day should have 3 slots, got %d", len(structure.Slots))
	}
	if structure.Slots[0].Type != training.MealBreakfast {
		t.Errorf("first slot should be breakfast, got %s", structure.Slots[0].Type)
	}
	if structure.Slots[1].Type != training.MealLunch {
		t.Errorf("second slot should be lunch, got %s", structure.Slots[1].Type)
	}
	if structure.Slots[2].Type != training.MealDinner {
		t.Errorf("third slot should be dinner, got %s", structure.Slots[2].Type)
	}
	if structure.IsTrainingDay {
		t.Error("rest day should not be marked as training day")
	}
}

func TestMealStructure_MorningTraining_Has4Slots(t *testing.T) {
	structure := training.ResolveMealStructure(true, training.TrainingMorning, domain.PhaseFollicular)

	if len(structure.Slots) != 4 {
		t.Fatalf("morning training should have 4 slots, got %d", len(structure.Slots))
	}
	if structure.Slots[0].Type != training.MealBreakfast {
		t.Errorf("first slot should be breakfast, got %s", structure.Slots[0].Type)
	}
	if structure.Slots[1].Type != training.MealPreTraining {
		t.Errorf("second slot should be pre_training, got %s", structure.Slots[1].Type)
	}
	if !structure.Slots[1].IsPrePost {
		t.Error("pre_training slot should be marked as pre/post")
	}
}

func TestMealStructure_MiddayTraining_HasPreAndPost(t *testing.T) {
	structure := training.ResolveMealStructure(true, training.TrainingAfternoon, domain.PhaseFollicular)

	if len(structure.Slots) != 4 {
		t.Fatalf("midday training should have 4 slots, got %d", len(structure.Slots))
	}

	hasPreTraining := false
	hasPostTraining := false
	for _, slot := range structure.Slots {
		if slot.Type == training.MealPreTraining {
			hasPreTraining = true
		}
		if slot.Type == training.MealPostTraining {
			hasPostTraining = true
		}
	}

	if !hasPreTraining {
		t.Error("midday training should have pre_training slot")
	}
	if !hasPostTraining {
		t.Error("midday training should have post_training slot")
	}
}

func TestMealStructure_EveningTraining_PreSnackBeforeDinner(t *testing.T) {
	structure := training.ResolveMealStructure(true, training.TrainingEvening, domain.PhaseFollicular)

	if len(structure.Slots) != 4 {
		t.Fatalf("evening training should have 4 slots, got %d", len(structure.Slots))
	}
	if structure.Slots[2].Type != training.MealPreTraining {
		t.Errorf("third slot should be pre_training snack, got %s", structure.Slots[2].Type)
	}
	if structure.Slots[3].Type != training.MealDinner {
		t.Errorf("fourth slot should be dinner (post-session), got %s", structure.Slots[3].Type)
	}
}

func TestMealStructure_RestDay_NoPrePostSlots(t *testing.T) {
	structure := training.ResolveMealStructure(false, "", domain.PhaseFollicular)

	for _, slot := range structure.Slots {
		if slot.IsPrePost {
			t.Errorf("rest day should not have pre/post slots, found %s", slot.Type)
		}
	}
}

func TestParseTrainingTime(t *testing.T) {
	tests := []struct {
		input    string
		expected training.TrainingTimeOfDay
	}{
		{"Morning", training.TrainingMorning},
		{"Afternoon", training.TrainingAfternoon},
		{"Evening", training.TrainingEvening},
		{"", training.TrainingEvening},
	}

	for _, tt := range tests {
		result := training.ParseTrainingTime(tt.input)
		if result != tt.expected {
			t.Errorf("input %q: expected %s, got %s", tt.input, tt.expected, result)
		}
	}
}
