package domain

import "time"

type LifestyleChange struct {
	ID           string
	UserID       string
	CreatedAt    time.Time
	Type         string // illness | travel | injury | sleep_disruption | other
	SpaceTrain   string //  "normal", "limited", "none"
	PossibleDiet string // "normal", "limited", "none"
	Energy       string
	AppliesTo    *time.Time
	PlanID       *string // Active plan to be affected
}
