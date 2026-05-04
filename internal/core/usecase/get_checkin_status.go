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

	lastCheckin, err := uc.Checkins.GetLatestByUser(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	nextAvailable := time.Now().UTC()

	// nunca ha hecho check-in
	need_checkin := false
	can_checkin := false
	if lastCheckin == nil {
		nextAvailable = nextSunday(user.UpdatedAt)
		if time.Since(user.UpdatedAt) >= 7*24*time.Hour {
			need_checkin = true
			can_checkin = true
		}
		return &GetCheckinStatusOutput{
			CanCheckin:      can_checkin,
			NextAvailableAt: &nextAvailable,
			NeedCheckin:     need_checkin,
		}, nil
	}

	nextAvailable = nextSunday(lastCheckin.CreatedAt)

	if now.Before(nextAvailable) {
		return &GetCheckinStatusOutput{
			CanCheckin:      false,
			NextAvailableAt: &nextAvailable,
			NeedCheckin:     false,
		}, nil
	}

	if time.Since(lastCheckin.CreatedAt) > 7*24*time.Hour {
		need_checkin = true
	}
	return &GetCheckinStatusOutput{
		CanCheckin:      true,
		NextAvailableAt: &nextAvailable,
		NeedCheckin:     need_checkin,
	}, nil
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
