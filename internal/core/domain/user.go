package domain

import "time"

type User struct {
	ID                    string
	Username              string
	Name                  string
	OnboardingCompleted   bool
	DOB                   string
	WeightKg              float64
	HeightCm              float64
	CycleType             string
	CycleDay              string
	CycleDuration         string
	CyclePhase            string
	CycleUpdatedAt        *time.Time
	CycleAnchorAt         *time.Time
	PeriodDuration        string
	PMSSymptoms           string
	PeriodDetails         string
	TrainingOften         string
	TrainingDuration      string
	TrainingType          string
	TrainingTime          string
	TrainingGuidanceLevel string
	TrainingGoals         string
	DietRestrictions      string
	DietType              string
	MealsPerDay           string
	SleepWindow           string
	StressLevel           string
	Priority              string
	GuidanceLevel         string
	HasActiveInjury       bool
	LastInjuryReportedAt  *time.Time
	TrainingPaused        bool
	LastActivePlanID      *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
