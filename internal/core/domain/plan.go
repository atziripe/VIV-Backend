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

	WeeklyHeadline    string
	CyclePhaseSummary string
	CycleDayRange     string

	// Raw payloads (source of truth for UI)
	TrainingJSON  []byte
	NutritionJSON []byte
	RecoveryJSON  []byte

	// TrainingCompleted tracks which days the user marked as done.
	// Keys are weekday names ("monday", "tuesday", etc.), values are always true.
	// Absent key = not completed.
	TrainingCompleted map[string]bool `json:"training_completed,omitempty"`

	Recommendations []Recommendations

	GeneratedFrom string
	SourceEventID string
	PlanVersion   int
}

type PlanGenerationMeta struct {
	ModelName     string
	PromptVersion string
	TokensInput   int
	TokensOutput  int
}

type Recommendations struct {
	Title  string
	Action string
	Why    string
}

type PlanGenerationResult struct {
	Plan *Plan
	Meta *PlanGenerationMeta
}
