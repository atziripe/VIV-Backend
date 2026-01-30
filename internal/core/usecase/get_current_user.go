package usecase

import (
	"context"
	"time"

	"viv/internal/core/domain"
)

type GetCurrentUserInput struct {
	UserID string
}

type GetCurrentUserOutput struct {
	User *domain.User
}

type GetCurrentUserUseCase struct {
	Users UserRepository
}

func NewGetCurrentUserUseCase(users UserRepository) *GetCurrentUserUseCase {
	return &GetCurrentUserUseCase{
		Users: users,
	}
}

func (uc *GetCurrentUserUseCase) Execute(ctx context.Context, in GetCurrentUserInput) (*GetCurrentUserOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("") // reminder to create our own error stack
	}

	user, err := uc.Users.GetByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(in.UserID)
	}

	if SyncUserCycleDaily(user, time.Now()) {
		// Persiste cambios en Firestore
		if err := uc.Users.Save(ctx, user); err != nil {
			return nil, err
		}
	}

	return &GetCurrentUserOutput{
		User: user,
	}, nil
}
