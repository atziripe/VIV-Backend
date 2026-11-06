package usecase

import (
	"context"
	"log"
	"time"

	"viv/internal/core/domain"
)

// InactivityThreshold is how long a user can go without an authenticated
// request before RunInactivitySweepUseCase's weekly sweep sends them a
// re-engagement email. Approximate (365-day years, no leap-year
// adjustment) — fine for a threshold measured in years.
const InactivityThreshold = 2 * 365 * 24 * time.Hour

// RunInactivitySweepUseCase is the weekly cron job behind "inactive for 2
// years → re-engagement email → 30-day notice → delete": it finds every
// candidate via Lister, emails each one through Email, and only then starts
// their AccountDeletionGracePeriod countdown by setting PendingDeletionAt —
// RunPendingAccountDeletionsUseCase's existing daily sweep is what deletes
// them once that elapses, unchanged.
//
// A user already mid-notice-window (either reason) is skipped, not
// re-emailed. A failure at any step for one user (contact lookup, email
// send, save) is logged and skipped — since the inactivity condition is
// still true, the next week's run retries them automatically, so there's
// no separate retry logic here. Deliberately: the 30-day clock only starts
// once the email actually sent, never on a guess that it did.
type RunInactivitySweepUseCase struct {
	Lister   InactivityLister
	Contacts UserContactLookup
	Email    ReEngagementEmailSender
	Users    UserRepository
}

func NewRunInactivitySweepUseCase(lister InactivityLister, contacts UserContactLookup, email ReEngagementEmailSender, users UserRepository) *RunInactivitySweepUseCase {
	return &RunInactivitySweepUseCase{Lister: lister, Contacts: contacts, Email: email, Users: users}
}

func (uc *RunInactivitySweepUseCase) Execute(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-InactivityThreshold)
	candidates, err := uc.Lister.ListInactiveSince(ctx, cutoff)
	if err != nil {
		return err
	}

	for _, user := range candidates {
		if user.PendingDeletionAt != nil {
			continue
		}

		email, name, err := uc.Contacts.GetContact(ctx, user.ID)
		if err != nil {
			log.Printf("[inactivity-sweep] failed to look up contact for user=%s: %v", user.ID, err)
			continue
		}

		if err := uc.Email.SendReEngagementEmail(ctx, email, name); err != nil {
			log.Printf("[inactivity-sweep] failed to send re-engagement email to user=%s: %v", user.ID, err)
			continue
		}

		deletionAt := time.Now().UTC().Add(AccountDeletionGracePeriod)
		user.PendingDeletionAt = &deletionAt
		user.PendingDeletionReason = domain.PendingDeletionInactivity
		if err := uc.Users.Save(ctx, user); err != nil {
			log.Printf("[inactivity-sweep] failed to save pending deletion for user=%s: %v", user.ID, err)
			continue
		}

		log.Printf("[inactivity-sweep] notified inactive user=%s, deletion pending at %s", user.ID, deletionAt.Format(time.RFC3339))
	}

	return nil
}
