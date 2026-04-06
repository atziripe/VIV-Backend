package usecase

import "time"

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

// -------- check ins --------
type CheckinLockedError struct {
	NextAvailableAt time.Time
}

func (e *CheckinLockedError) Error() string {
	return "checkin_not_available_yet"
}
