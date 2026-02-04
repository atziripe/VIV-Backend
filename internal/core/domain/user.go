package domain

import "time"

type User struct {
	ID                  string
	Username            string
	Name                string
	OnboardingCompleted bool
	DOB                 string
	WeightKg            float64
	HeightCm            float64
	CycleType           string
	CycleDay            string
	CycleDuration       string
	CyclePhase          string
	CycleUpdatedAt      *time.Time
	CycleAnchorAt       *time.Time
	PeriodDuration      string
	PMSSymptoms         string

	TrainingOften         string
	TrainingDuration      string
	TrainingType          string
	TrainingTime          string
	TrainingGuidanceLevel string
	TrainingGoals         string

	DietRestrictions       string
	DietProteinResources   string
	MealsPerDay            string
	MealsTimingStability   string
	DigestionConditions    string
	NutritionIntent        string
	NutritionGuidanceLevel string

	SleepWindow        string
	RecoveryAfterSleep string
	SleepContinuity    string
	LingeringMarker    string
	StressReactivity   string
	StressLevel        string
	Priority           string

	HasActiveInjury      bool
	LastInjuryReportedAt *time.Time
	TrainingPaused       bool
	LastActivePlanID     *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
