package domain

import "time"

// SessionLogStatus is the lifecycle of a real-time-logged session.
type SessionLogStatus string

const (
	SessionLogInProgress SessionLogStatus = "in_progress"
	SessionLogDone       SessionLogStatus = "done"
)

// SessionFeedback is the optional "how did that feel?" a user gives when
// completing a session — design doc's session-done screen. "" means not
// given, not a fourth value.
type SessionFeedback string

const (
	FeedbackEasy    SessionFeedback = "easy"
	FeedbackRight   SessionFeedback = "right"
	FeedbackTooMuch SessionFeedback = "too_much"
)

// SetLog is one actually-performed set — weight/reps as logged by the
// user, distinct from the prescribed target on the plan. Identified by
// (ExerciseID, SetNumber): logging the same pair again updates in place
// rather than appending, mirroring the idempotent-by-key convention
// DailyCheckin/PlanJob already use elsewhere in this pipeline.
type SetLog struct {
	ExerciseID   string
	ExerciseName string
	SetNumber    int
	WeightKg     float64
	Reps         int

	// RIR is the reps-in-reserve the user reported for this set — nil
	// when not given (session-library/mesocycle-pinned logging never
	// asked for it before VIV-112's Strength assembly, whose
	// prescriptions are RIR-based). Never defaulted to 0: a literal "0
	// RIR" (to failure) must stay distinguishable from "not logged".
	RIR *int

	LoggedAt time.Time
}

// SessionLog is the real-time execution record for one Loggable day —
// the old mesocycle-pinned path, or VIV-112's real Strength assembly
// (see usecase.WeeklyPlanDayDetail.Loggable) — distinct from
// WeekDraft/DayPlan, which is the PLAN, not what actually happened. One
// record per (UserID, Date).
type SessionLog struct {
	UserID       string
	Date         time.Time
	ActivityType string

	Status    SessionLogStatus
	StartedAt time.Time
	EndedAt   *time.Time // nil while Status == SessionLogInProgress

	Sets     []SetLog
	Feedback SessionFeedback // "" if not given

	UpdatedAt time.Time
}
