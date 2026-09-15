package usecase

import (
	"fmt"
	"time"
)

// -------- user not found --------
type UserNotFoundError struct {
	UserID string
}

func (e UserNotFoundError) Error() string {
	if e.UserID == "" {
		return "user not found"
	}
	return "user not found: " + e.UserID
}

func ErrUserNotFound(id string) error {
	return UserNotFoundError{UserID: id}
}

// -------- no active plan --------
type NoActivePlanError struct {
	UserID string
}

func (e NoActivePlanError) Error() string {
	return "no active plan for user: " + e.UserID
}

func ErrNoActivePlan(userID string) error {
	return NoActivePlanError{UserID: userID}
}

// -------- plan not found --------
type PlanNotFoundError struct {
	PlanID string
}

func (e PlanNotFoundError) Error() string {
	return "plan not found: " + e.PlanID
}

func ErrPlanNotFound(id string) error {
	return PlanNotFoundError{PlanID: id}
}

// -------- onboarding catalog/goal validation --------
type InvalidActivityError struct {
	ActivityID string
}

func (e InvalidActivityError) Error() string {
	return "unrecognized activity id: " + e.ActivityID
}

func ErrInvalidActivity(id string) error {
	return InvalidActivityError{ActivityID: id}
}

type InvalidGoalError struct {
	GoalID string
}

func (e InvalidGoalError) Error() string {
	return "unrecognized goal id: " + e.GoalID
}

func ErrInvalidGoal(id string) error {
	return InvalidGoalError{GoalID: id}
}

// -------- session logging --------
//
// One error type for every "this request doesn't make sense right now"
// case in session_logging.go (no plan for the date, rest day, not
// Loggable, unknown exercise, set number out of range, no session
// started yet, session already done) — lets the HTTP handler map all of
// them to 400 via errors.As, while a real infrastructure error from a
// repository call (wrapped with %w, not this type) still falls through
// to 500 instead of being misreported as a bad request.
type SessionLoggingValidationError struct {
	Message string
}

func (e SessionLoggingValidationError) Error() string { return e.Message }

func errSessionLogging(format string, args ...any) error {
	return SessionLoggingValidationError{Message: fmt.Sprintf(format, args...)}
}

// -------- check ins --------
type CheckinLockedError struct {
	NextAvailableAt time.Time
}

func (e *CheckinLockedError) Error() string {
	return "checkin_not_available_yet"
}
