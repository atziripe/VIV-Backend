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

// -------- consent --------
type InvalidConsentPurposeError struct {
	Purpose string
}

func (e InvalidConsentPurposeError) Error() string {
	return "unrecognized consent purpose: " + e.Purpose
}

func ErrInvalidConsentPurpose(purpose string) error {
	return InvalidConsentPurposeError{Purpose: purpose}
}

// ConsentRequiredError means the request can't proceed without a consent
// this codebase treats as non-negotiable for the operation attempted (e.g.
// completing onboarding without granting health_data_processing).
type ConsentRequiredError struct {
	Purpose string
}

func (e ConsentRequiredError) Error() string {
	return "consent required: " + e.Purpose
}

func ErrConsentRequired(purpose string) error {
	return ConsentRequiredError{Purpose: purpose}
}

// -------- ineligibility --------
type InvalidIneligibilityReasonError struct {
	Reason string
}

func (e InvalidIneligibilityReasonError) Error() string {
	return "unrecognized ineligibility reason: " + e.Reason
}

func ErrInvalidIneligibilityReason(reason string) error {
	return InvalidIneligibilityReasonError{Reason: reason}
}

// -------- minimum age --------
type InvalidDOBError struct {
	DOB string
}

func (e InvalidDOBError) Error() string {
	return "invalid date of birth: " + e.DOB
}

func ErrInvalidDOB(dob string) error {
	return InvalidDOBError{DOB: dob}
}

// UnderMinimumAgeError means the submitted dob computes to an age below
// MinimumAge. Carries Age so the handler/client can show a precise
// message without recomputing it.
type UnderMinimumAgeError struct {
	Age int
}

func (e UnderMinimumAgeError) Error() string {
	return fmt.Sprintf("must be %d or older to use VIV", MinimumAge)
}

func ErrUnderMinimumAge(age int) error {
	return UnderMinimumAgeError{Age: age}
}

// -------- check ins --------
type CheckinLockedError struct {
	NextAvailableAt time.Time
}

func (e *CheckinLockedError) Error() string {
	return "checkin_not_available_yet"
}
