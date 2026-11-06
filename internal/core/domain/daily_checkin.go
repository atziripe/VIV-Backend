package domain

import (
	"time"

	"viv/internal/core/checkin"
)

// DailyCheckin is VIV-103's persisted daily check-in — the raw answers
// plus their derived readiness dimensions, one record per (UserID, Date).
// Distinct from the older weekly Checkin above: different answer
// vocabulary, different cadence (daily vs weekly), different repository
// (usecase.DailyCheckinRepository vs usecase.CheckinRepository).
type DailyCheckin struct {
	UserID    string
	Date      time.Time // the user's local calendar date this check-in is for, midnight-normalized
	Answers   checkin.DailyCheckin
	Readiness checkin.ReadinessDimensions

	// SubmittedAt is when this record was last written. A second
	// submission for the same (UserID, Date) overwrites the record in
	// place — see usecase.DailyCheckinRepository.Upsert — so this always
	// reflects the most recent submission, not the first one.
	SubmittedAt time.Time
}
