package training

import (
	"viv/internal/core/domain"
)

// ============================================================================
// MEAL STRUCTURE RESOLVER
// ============================================================================
// Determines which meal slots a day should have based on:
//   - Is it a training day or rest day?
//   - What time of day is the training session?
//
// Rules from nutrition spec:
//   Rest day:           Breakfast · Lunch · Dinner (3 slots)
//   Training (morning): Breakfast · Pre-training · Lunch · Dinner (4 slots)
//   Training (midday):  Breakfast · Pre-training · Post-training · Dinner (4 slots)
//   Training (evening): Breakfast · Lunch · Pre-training snack · Dinner/Post (4 slots)

// MealSlotType identifies the type of meal slot.
type MealSlotType string

const (
	MealBreakfast    MealSlotType = "breakfast"
	MealLunch        MealSlotType = "lunch"
	MealDinner       MealSlotType = "dinner"
	MealPreTraining  MealSlotType = "pre_training"
	MealPostTraining MealSlotType = "post_training"
)

// MealSlot represents a single meal slot in the day's structure.
type MealSlot struct {
	Type         MealSlotType `json:"type"`
	TimingWindow string       `json:"timing_window"`
	IsPrePost    bool         `json:"is_pre_post,omitempty"` // true for pre/post training slots
}

// TrainingTimeOfDay represents when the user typically trains.
type TrainingTimeOfDay string

const (
	TrainingMorning   TrainingTimeOfDay = "morning"
	TrainingAfternoon TrainingTimeOfDay = "afternoon"
	TrainingEvening   TrainingTimeOfDay = "evening"
)

// DayMealStructure holds the complete meal structure for a day.
type DayMealStructure struct {
	IsTrainingDay bool              `json:"is_training_day"`
	SessionTime   TrainingTimeOfDay `json:"session_time,omitempty"`
	Slots         []MealSlot        `json:"slots"`
}

// ResolveMealStructure determines the meal slots for a day.
//
// Parameters:
//   - isTrainingDay: whether the user has a training session this day
//   - sessionTime: when the session is (from user's TrainingTime onboarding field)
//   - phase: current cycle phase (affects pre-training importance)
func ResolveMealStructure(isTrainingDay bool, sessionTime TrainingTimeOfDay, phase domain.CyclePhase) DayMealStructure {
	if !isTrainingDay {
		return restDayStructure()
	}

	switch sessionTime {
	case TrainingMorning:
		return morningTrainingStructure()
	case TrainingAfternoon:
		return middayTrainingStructure()
	case TrainingEvening:
		return eveningTrainingStructure()
	default:
		// Default to evening if unknown
		return eveningTrainingStructure()
	}
}

func restDayStructure() DayMealStructure {
	return DayMealStructure{
		IsTrainingDay: false,
		Slots: []MealSlot{
			{Type: MealBreakfast, TimingWindow: "7–9am"},
			{Type: MealLunch, TimingWindow: "12–2pm"},
			{Type: MealDinner, TimingWindow: "6–8pm"},
		},
	}
}

func morningTrainingStructure() DayMealStructure {
	return DayMealStructure{
		IsTrainingDay: true,
		SessionTime:   TrainingMorning,
		Slots: []MealSlot{
			{Type: MealBreakfast, TimingWindow: "6–7am"},
			{Type: MealPreTraining, TimingWindow: "90min before session", IsPrePost: true},
			{Type: MealLunch, TimingWindow: "12–2pm"},
			{Type: MealDinner, TimingWindow: "6–8pm"},
		},
	}
}

func middayTrainingStructure() DayMealStructure {
	return DayMealStructure{
		IsTrainingDay: true,
		SessionTime:   TrainingAfternoon,
		Slots: []MealSlot{
			{Type: MealBreakfast, TimingWindow: "7–9am"},
			{Type: MealPreTraining, TimingWindow: "90min before session", IsPrePost: true},
			{Type: MealPostTraining, TimingWindow: "within 1hr after session", IsPrePost: true},
			{Type: MealDinner, TimingWindow: "6–8pm"},
		},
	}
}

func eveningTrainingStructure() DayMealStructure {
	return DayMealStructure{
		IsTrainingDay: true,
		SessionTime:   TrainingEvening,
		Slots: []MealSlot{
			{Type: MealBreakfast, TimingWindow: "7–9am"},
			{Type: MealLunch, TimingWindow: "12–2pm"},
			{Type: MealPreTraining, TimingWindow: "4–4:30pm", IsPrePost: true},
			{Type: MealDinner, TimingWindow: "post-session"},
		},
	}
}

// ParseTrainingTime converts the UI string from onboarding to TrainingTimeOfDay.
func ParseTrainingTime(s string) TrainingTimeOfDay {
	switch s {
	case "Morning":
		return TrainingMorning
	case "Afternoon":
		return TrainingAfternoon
	case "Evening":
		return TrainingEvening
	default:
		return TrainingEvening
	}
}
