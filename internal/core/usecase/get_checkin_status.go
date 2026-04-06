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
}

type GetCheckinStatusUseCase struct {
	Checkins CheckinRepository
}

func NewGetCheckinStatusUseCase(checkins CheckinRepository) *GetCheckinStatusUseCase {
	return &GetCheckinStatusUseCase{
		Checkins: checkins,
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

	// nunca ha hecho check-in
	if lastCheckin == nil {
		return &GetCheckinStatusOutput{
			CanCheckin:      true,
			NextAvailableAt: nil,
		}, nil
	}

	now := time.Now().UTC()
	nextAvailable := nextSunday(lastCheckin.CreatedAt)

	if now.Before(nextAvailable) {
		return &GetCheckinStatusOutput{
			CanCheckin:      false,
			NextAvailableAt: &nextAvailable,
		}, nil
	}

	return &GetCheckinStatusOutput{
		CanCheckin:      true,
		NextAvailableAt: &nextAvailable,
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
