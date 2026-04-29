package usecase

import (
	"context"
	"errors"
	"time"

	"viv/internal/core/domain"

	"github.com/google/uuid"
)

type CreateCheckinInput struct {
	UserID             string
	WeekStart          time.Time
	SleepQuality       string
	BodyStatus         string
	Appetite           string
	StressLevel        string
	LastWeekFeeling    string
	CycleStart         *time.Time
	WorkloadPrediction string
	MentalEnergy       string
}

type CreateCheckinOutput struct {
	Checkin *domain.Checkin
}

type CreateCheckinUseCase struct {
	Checkins      CheckinRepository
	PromptVersion string
	Users         UserRepository
}

func NewCreateCheckinUseCase(repo CheckinRepository, userRepo UserRepository, promptVersion string) *CreateCheckinUseCase {
	return &CreateCheckinUseCase{
		Checkins:      repo,
		Users:         userRepo,
		PromptVersion: promptVersion,
	}
}

func (uc *CreateCheckinUseCase) Execute(ctx context.Context, in CreateCheckinInput) (*CreateCheckinOutput, error) {
	now := time.Now().UTC()

	lastCheckin, err := uc.Checkins.GetLatestByUser(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	if lastCheckin != nil {
		nextAvailable := nextSunday(lastCheckin.CreatedAt)

		if now.Before(nextAvailable) {
			return nil, &CheckinLockedError{NextAvailableAt: nextAvailable}
		}
	}

	checkin := &domain.Checkin{
		ID:                 uuid.NewString(),
		UserID:             in.UserID,
		WeekStart:          in.WeekStart,
		CreatedAt:          now,
		SleepQuality:       in.SleepQuality,
		BodyStatus:         in.BodyStatus,
		Appetite:           in.Appetite,
		StressLevel:        in.StressLevel,
		LastWeekFeeling:    in.LastWeekFeeling,
		CycleStart:         in.CycleStart,
		WorkloadPrediction: in.WorkloadPrediction,
		MentalEnergy:       in.MentalEnergy,
		PromptVersion:      uc.PromptVersion,
	}

	if err := uc.Checkins.Create(ctx, checkin); err != nil {
		return nil, err
	}

	if checkin.CycleStart != nil {
		if uc.Users == nil {
			return nil, errors.New("CreateCheckinUseCase: Users repository is nil")
		}
		user, err := uc.Users.GetByID(ctx, in.UserID)
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, ErrUserNotFound(in.UserID)
		}

		ApplyCycleStartOverride(user, *checkin.CycleStart, time.Now())

		if err := uc.Users.Save(ctx, user); err != nil {
			return nil, err
		}
	}

	return &CreateCheckinOutput{Checkin: checkin}, nil
}
