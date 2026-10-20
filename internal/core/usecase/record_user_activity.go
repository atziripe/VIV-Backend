package usecase

import (
	"context"
	"time"

	"viv/internal/core/domain"
)

// RecordUserActivityUseCase is EnsureUserMiddleware's other job on every
// authenticated request for a user that already exists: stamping
// User.LastActiveAt (at most once per UTC calendar day, to avoid a
// Firestore write on every single request) and, if this user has an
// inactivity-triggered deletion pending, auto-cancelling it — see
// domain.PendingDeletionReason's doc comment for why a user-requested one
// does NOT get the same treatment.
type RecordUserActivityUseCase struct {
	Users UserRepository
}

func NewRecordUserActivityUseCase(users UserRepository) *RecordUserActivityUseCase {
	return &RecordUserActivityUseCase{Users: users}
}

func (uc *RecordUserActivityUseCase) Execute(ctx context.Context, user *domain.User) error {
	now := time.Now().UTC()

	alreadyRecordedToday := user.LastActiveAt != nil && sameUTCDay(*user.LastActiveAt, now)
	autoCancelling := user.PendingDeletionAt != nil && user.PendingDeletionReason == domain.PendingDeletionInactivity

	if alreadyRecordedToday && !autoCancelling {
		return nil
	}

	user.LastActiveAt = &now
	if autoCancelling {
		user.PendingDeletionAt = nil
		user.PendingDeletionReason = ""
	}

	return uc.Users.Save(ctx, user)
}

func sameUTCDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
