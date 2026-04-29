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

	TrainingOften    string
	TrainingDuration string
	TrainingType     string
	TrainingTime     string
	TrainingGoals    string

	DietRestrictions     string
	DietProteinResources string
	MealsPerDay          string
	MealsTimingStability string
	DigestionConditions  string

	SleepWindow        string
	SleepContinuity    string
	RecoveryAfterSleep string
	StressLevel        string
	DailyActivityLevel string // "Mostly sitting", "Some movement", "On my feet most of the day", "Physically demanding work"
	StressReactivity   string
	LingeringMarker    string
	Priority           string

	HasActiveInjury      bool
	LastInjuryReportedAt *time.Time
	TrainingPaused       bool
	LastActivePlanID     *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
