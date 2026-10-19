package usecase

import (
	"context"
	"log"
	"time"

	"viv/internal/core/domain"
)

// formatPendingDeletionAt is purely for the sweep's log line — defensive
// against a lister implementation returning a user with no deadline set,
// which should never happen (ListPendingDeletions' whole filter is that
// field) but shouldn't crash the sweep if it ever did.
func formatPendingDeletionAt(user *domain.User) string {
	if user.PendingDeletionAt == nil {
		return "unknown"
	}
	return user.PendingDeletionAt.Format(time.RFC3339)
}

// RunPendingAccountDeletionsUseCase is the cron-driven other half of
// ScheduleAccountDeletionUseCase's grace period: it finds every user whose
// deadline has passed and actually deletes them via DeleteAccountUseCase —
// the same hard-delete logic that used to run synchronously inside DELETE
// /me before the grace period existed. One user's failure is logged and
// skipped rather than aborting the rest of the sweep, so a transient error
// doesn't block everyone else's already-overdue deletion.
type RunPendingAccountDeletionsUseCase struct {
	Lister        PendingDeletionLister
	DeleteAccount *DeleteAccountUseCase
}

func NewRunPendingAccountDeletionsUseCase(lister PendingDeletionLister, deleteAccount *DeleteAccountUseCase) *RunPendingAccountDeletionsUseCase {
	return &RunPendingAccountDeletionsUseCase{Lister: lister, DeleteAccount: deleteAccount}
}

func (uc *RunPendingAccountDeletionsUseCase) Execute(ctx context.Context) error {
	due, err := uc.Lister.ListPendingDeletions(ctx, time.Now().UTC())
	if err != nil {
		return err
	}

	for _, user := range due {
		if err := uc.DeleteAccount.Execute(ctx, user.ID); err != nil {
			log.Printf("[account-deletion-sweep] failed to delete user=%s: %v", user.ID, err)
			continue
		}
		log.Printf("[account-deletion-sweep] deleted user=%s (was scheduled for %s)", user.ID, formatPendingDeletionAt(user))
	}

	return nil
}
