package usecase

import (
	"context"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// LogPeriodStartInput reports that a period started, independent of the
// weekly check-in — always available so it can be logged the moment it
// happens instead of waiting for the next check-in window. Mom Test P0
// fix: I04/I07/I08 each named "had to wait until Sunday to log it" as an
// active abandonment trigger.
type LogPeriodStartInput struct {
	UserID string
	// Date defaults to today (UTC) when zero — the common case is logging
	// the day it happens, but reporting a day or two late is allowed.
	Date time.Time
}

type LogPeriodStartOutput struct {
	User               *domain.User
	CycleDay           int
	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int
}

type LogPeriodStartUseCase struct {
	Users UserRepository
}

func NewLogPeriodStartUseCase(users UserRepository) *LogPeriodStartUseCase {
	return &LogPeriodStartUseCase{Users: users}
}

func (uc *LogPeriodStartUseCase) Execute(ctx context.Context, in LogPeriodStartInput) (*LogPeriodStartOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return nil, ErrUserNotFound("")
	}

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(userID)
	}

	now := time.Now().UTC()
	date := in.Date
	if date.IsZero() {
		date = now
	}

	// Reuses the exact recalibration the weekly check-in's optional
	// cycle_start field and onboarding already use — one source of truth
	// for what "reporting a period start" does to the cycle, instead of
	// duplicating that math here.
	ApplyCycleStartOverride(user, date, now)
	user.UpdatedAt = now

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}

	phase := mapCyclePhase(user.CyclePhase)
	return &LogPeriodStartOutput{
		User:               user,
		CycleDay:           user.CycleDay,
		CurrentPhase:       string(phase),
		NextPhase:          NextPhaseName(phase),
		DaysUntilNextPhase: DaysUntilNextPhase(user),
	}, nil
}
