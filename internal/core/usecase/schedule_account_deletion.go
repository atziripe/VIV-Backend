package usecase

import (
	"context"
	"time"

	"viv/internal/core/domain"
)

// AccountDeletionGracePeriod is how long a scheduled account deletion stays
// cancellable before RunPendingAccountDeletionsUseCase's daily sweep makes
// it permanent.
const AccountDeletionGracePeriod = 30 * 24 * time.Hour

// ScheduleAccountDeletionUseCase is what DELETE /me does: rather than
// wiping the account immediately, it marks it for deletion
// AccountDeletionGracePeriod out and leaves it fully usable (and
// cancellable via CancelAccountDeletionUseCase) until then. Calling this
// again while a deletion is already pending restarts the clock from now,
// rather than leaving the original deadline in place.
type ScheduleAccountDeletionUseCase struct {
	Users UserRepository
}

func NewScheduleAccountDeletionUseCase(users UserRepository) *ScheduleAccountDeletionUseCase {
	return &ScheduleAccountDeletionUseCase{Users: users}
}

func (uc *ScheduleAccountDeletionUseCase) Execute(ctx context.Context, userID string) (*domain.User, error) {
	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(userID)
	}

	deletionAt := time.Now().UTC().Add(AccountDeletionGracePeriod)
	user.PendingDeletionAt = &deletionAt

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
