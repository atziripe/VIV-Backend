package usecase

import (
	"context"
	"errors"
	"time"

	"viv/internal/core/domain"

	"github.com/google/uuid"
)

type CreateCheckinInput struct {
	UserID          string
	WeekStart       time.Time
	Sleep           string
	Body            string
	AppetiteV2      string
	Demand          string
	StressLevel     string
	LastWeekFeel    string
	CycleStart      *time.Time
	PMSSymptoms     *string
	Predictability  string
	Readiness       string
	AdditionalNotes string
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
		ID:              uuid.NewString(),
		UserID:          in.UserID,
		WeekStart:       in.WeekStart,
		CreatedAt:       now,
		Sleep:           domain.CheckinSleep(in.Sleep),
		Body:            domain.CheckinBody(in.Body),
		AppetiteV2:      domain.CheckinAppetite(in.AppetiteV2),
		Demand:          domain.CheckinDemand(in.Demand),
		LastWeekFeel:    domain.CheckinLastWeek(in.LastWeekFeel),
		CycleStart:      in.CycleStart,
		PMSSymptoms:     (*domain.CheckinPMSSymptoms)(in.PMSSymptoms),
		Predictability:  domain.CheckinPredictability(in.Predictability),
		Readiness:       domain.CheckinReadiness(in.Readiness),
		AdditionalNotes: in.AdditionalNotes,
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
