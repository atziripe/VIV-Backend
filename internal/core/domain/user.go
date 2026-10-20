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

	// PendingDeletionAt is set by ScheduleAccountDeletionUseCase (DELETE
	// /me) or RunInactivitySweepUseCase, and cleared by
	// CancelAccountDeletionUseCase (or automatically by
	// RecordUserActivityUseCase, for an inactivity-triggered one) — nil
	// means no deletion is scheduled. The account stays fully usable while
	// this is set; RunPendingAccountDeletionsUseCase's daily sweep is what
	// actually deletes the account once this timestamp passes.
	PendingDeletionAt     *time.Time
	PendingDeletionReason PendingDeletionReason

	// LastActiveAt is stamped by RecordUserActivityUseCase on at most one
	// authenticated request per UTC calendar day — the closest proxy this
	// backend has to "last login" (Firebase Auth sign-in itself happens
	// client-side and is never observed here). RunInactivitySweepUseCase's
	// weekly sweep is what reads it.
	LastActiveAt *time.Time

	// AppleRefreshToken is submitted once, via POST /me/apple-credentials,
	// right after the client completes Sign in with Apple — Apple only
	// issues it on a user's first authorization, so the client is
	// responsible for capturing and sending it then. Empty for anyone who
	// never used Sign in with Apple (or on Android). DeleteAccountUseCase
	// reads it to revoke the grant with Apple (App Store guideline 5.1.1v)
	// as part of account deletion — never exposed back to any client, and
	// deliberately not mirrored to Neon (see AppleTokenRevoker's doc
	// comment).
	AppleRefreshToken string
}
