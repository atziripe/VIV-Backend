package domain

import (
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/goal"
)

type User struct {
	ID                  string
	Username            string
	Name                string
	OnboardingCompleted bool
	DOB                 string
	WeightKg            float64
	HeightCm            float64
	CycleType           string
	CycleDay            int
	CycleDuration       string
	CyclePhase          string
	CycleUpdatedAt      *time.Time
	CycleAnchorAt       *time.Time
	PeriodDuration      string

	// CycleEstimationDisabled is the "it varies — stop estimating" opt-out
	// from the late-period nudge — when true, usecase.ExpectedPeriodDate/
	// DaysLate stop predicting a next period date at all, rather than
	// surfacing an estimate the user has explicitly said isn't reliable
	// for her. Cycle day/phase tracking from whatever anchor is on file is
	// unaffected; this only gates the *prediction* of when the next one
	// starts.
	CycleEstimationDisabled bool

	TrainingOften    string
	TrainingDuration string
	TrainingType     string
	TrainingTime     string
	TrainingGoals    string

	// UserCatalog and GoalID are the typed activity-catalog/goal
	// selections captured at onboarding (VIV-101 taxonomy, VIV-102 goal
	// profiles) — distinct from the free-text TrainingType/TrainingGoals
	// above, which predate this typed pipeline.
	UserCatalog []activity.ID
	GoalID      goal.ID

	DietRestrictions     string
	DietProteinResources string
	MealsPerDay          string
	MealsTimingStability string
	DigestionConditions  string
	EatingStyle          string

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
