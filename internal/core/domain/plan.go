package domain

import "time"

type Plan struct {
	ID        string
	UserID    string
	Status    string // active | archived
	CheckinID string
	CreatedAt time.Time
	StartDate time.Time
	EndDate   time.Time

	CycleDayRange string

	// Raw payloads (source of truth for UI)
	TrainingJSON  []byte
	NutritionJSON []byte
	RecoveryJSON  []byte

	// TrainingCompleted tracks which days the user marked as done.
	// Keys are weekday names ("monday", "tuesday", etc.), values are always true.
	// Absent key = not completed.
	TrainingCompleted map[string]bool `json:"training_completed,omitempty"`

	GeneratedFrom string
	SourceEventID string
	PlanVersion   int

	PhaseFeedback map[string]PhaseFeedbackEntry `json:"phase_feedback,omitempty"`

	// MealSelections tracks which meal option the user chose per day per slot.
	// Keys are weekday names, values are maps of slot name → option index.
	MealSelections map[string]map[string]int `json:"meal_selections,omitempty"`
}

type PhaseFeedbackEntry struct {
	HormonalBriefingResonates *bool  `json:"hormonal_briefing_resonates"`
	RespondedAt               string `json:"responded_at"`
}

type PlanGenerationMeta struct {
	ModelName     string
	PromptVersion string
	TokensInput   int
	TokensOutput  int
}

type PlanGenerationResult struct {
	Plan *Plan
	Meta *PlanGenerationMeta
}
