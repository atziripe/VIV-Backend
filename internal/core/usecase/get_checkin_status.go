package usecase

import (
	"context"
	"time"
)

type GetCheckinStatusInput struct {
	UserID string
}

type GetCheckinStatusOutput struct {
	CanCheckin      bool
	NextAvailableAt *time.Time
	NeedCheckin     bool
}

type GetCheckinStatusUseCase struct {
	Checkins CheckinRepository
	Users    UserRepository
}

func NewGetCheckinStatusUseCase(checkins CheckinRepository, users UserRepository) *GetCheckinStatusUseCase {
	return &GetCheckinStatusUseCase{
		Checkins: checkins, Users: users,
	}
}

func (uc *GetCheckinStatusUseCase) Execute(ctx context.Context, in GetCheckinStatusInput) (*GetCheckinStatusOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("")
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	lastCheckin, err := uc.Checkins.GetLatestByUser(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	// The last Sunday that has already passed (or today if today is Sunday)
	lastSunday := lastSundayFrom(now)
	nextSundayAt := lastSunday.AddDate(0, 0, 7)

	// Case 1: never checked in — use onboarding date as reference
	if lastCheckin == nil {
		// If a Sunday has passed since onboarding, they need to check in
		if lastSunday.After(user.CreatedAt.UTC()) {
			return &GetCheckinStatusOutput{
				CanCheckin:      true,
				NeedCheckin:     false,
				NextAvailableAt: &nextSundayAt,
			}, nil
		}
		// No Sunday has passed yet since onboarding — just wait
		return &GetCheckinStatusOutput{
			CanCheckin:      false,
			NeedCheckin:     false,
			NextAvailableAt: &nextSundayAt,
		}, nil
	}

	// Case 2: already checked in — did they check in after the last Sunday?
	checkedInAfterLastSunday := lastCheckin.CreatedAt.UTC().After(lastSunday)

	if checkedInAfterLastSunday {
		// All good, wait for next Sunday
		return &GetCheckinStatusOutput{
			CanCheckin:      false,
			NeedCheckin:     false,
			NextAvailableAt: &nextSundayAt,
		}, nil
	}

	// Last checkin was before the last Sunday — they missed it
	return &GetCheckinStatusOutput{
		CanCheckin:      true,
		NeedCheckin:     false,
		NextAvailableAt: &nextSundayAt,
	}, nil
}

// lastSundayFrom returns the most recent Sunday at 00:00:00 UTC,
// including today if today is Sunday.
func lastSundayFrom(t time.Time) time.Time {
	daysSinceSunday := int(t.Weekday()) // Sunday=0, Monday=1, ...
	last := t.AddDate(0, 0, -daysSinceSunday)
	return time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)
}

func nextSunday(t time.Time) time.Time {
	t = t.UTC()

	daysUntilSunday := (7 - int(t.Weekday())) % 7
	if daysUntilSunday == 0 {
		daysUntilSunday = 7
	}

	next := t.AddDate(0, 0, daysUntilSunday)

	return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, time.UTC)
}
