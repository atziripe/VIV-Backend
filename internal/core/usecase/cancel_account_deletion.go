package usecase

import (
	"context"

	"viv/internal/core/domain"
)

// CancelAccountDeletionUseCase reverses ScheduleAccountDeletionUseCase —
// called from POST /me/cancel-deletion when a user changes their mind
// during the grace period. Idempotent: calling it when nothing is pending
// is a no-op, not an error.
type CancelAccountDeletionUseCase struct {
	Users UserRepository
}

func NewCancelAccountDeletionUseCase(users UserRepository) *CancelAccountDeletionUseCase {
	return &CancelAccountDeletionUseCase{Users: users}
}

func (uc *CancelAccountDeletionUseCase) Execute(ctx context.Context, userID string) (*domain.User, error) {
	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(userID)
	}

	if user.PendingDeletionAt == nil {
		return user, nil
	}

	user.PendingDeletionAt = nil
	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
