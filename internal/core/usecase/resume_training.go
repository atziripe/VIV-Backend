package usecase

import (
	"context"

	"viv/internal/core/domain"
)

type ResumeTrainingInput struct {
	UserID      string
	ClearInjury bool
}

type ResumeTrainingOutput struct {
	User *domain.User
}

type ResumeTrainingUseCase struct {
	Users UserRepository
}

func NewResumeTrainingUseCase(users UserRepository) *ResumeTrainingUseCase {
	return &ResumeTrainingUseCase{
		Users: users,
	}
}

func (uc *ResumeTrainingUseCase) Execute(ctx context.Context, in ResumeTrainingInput) (*ResumeTrainingOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("")
	}

	u, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	// Cambios en flags
	u.TrainingPaused = false
	if in.ClearInjury {
		u.HasActiveInjury = false
		u.LastInjuryReportedAt = nil
	}

	if err := uc.Users.Save(ctx, u); err != nil {
		return nil, err
	}

	return &ResumeTrainingOutput{User: u}, nil
}
