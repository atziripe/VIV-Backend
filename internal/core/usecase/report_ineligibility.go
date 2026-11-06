package usecase

import (
	"context"
	"log"

	"viv/internal/core/domain"
)

var validIneligibilityReasons = map[domain.IneligibilityReason]bool{
	domain.IneligibilityPregnantOrPostpartum:     true,
	domain.IneligibilityPerimenopauseOrMenopause: true,
	domain.IneligibilityHormonalContraception:    true,
	domain.IneligibilityNotSure:                  true,
}

// ReportIneligibilityUseCase handles someone turned away at the
// pre-onboarding eligibility screen: it records the reason as an anonymous
// statistic (no user reference at all — see IneligibilityEventRecorder)
// and then deletes their just-created account outright via the same
// immediate DeleteAccountUseCase DELETE /me used before the 30-day grace
// period existed. Deliberately NOT routed through
// ScheduleAccountDeletionUseCase: this person was never eligible to begin
// with, so there's nothing to hold open for 30 days — without this, their
// Firebase Auth account (and any bare Firestore user doc
// EnsureUserMiddleware already created) would otherwise sit around
// indefinitely, unable to ever complete onboarding.
//
// Recording the statistic is best-effort: a failure there is logged and
// does not block deleting the account, since the account teardown is the
// part that actually matters here. The caller gets back whatever error the
// deletion itself produced, never one from the statistic.
type ReportIneligibilityUseCase struct {
	// Events is optional — nil when Neon isn't configured for this
	// deployment (same convention as DeleteAccountUseCase.NeonData): the
	// statistic is silently skipped, account deletion still proceeds.
	Events  IneligibilityEventRecorder
	Account AccountDeleter
}

func NewReportIneligibilityUseCase(events IneligibilityEventRecorder, account AccountDeleter) *ReportIneligibilityUseCase {
	return &ReportIneligibilityUseCase{Events: events, Account: account}
}

func (uc *ReportIneligibilityUseCase) Execute(ctx context.Context, userID string, reason domain.IneligibilityReason) error {
	if !validIneligibilityReasons[reason] {
		return ErrInvalidIneligibilityReason(string(reason))
	}

	if uc.Events != nil {
		if err := uc.Events.RecordIneligibility(ctx, reason); err != nil {
			log.Printf("[report-ineligibility] failed to record statistic reason=%s: %v", reason, err)
		}
	}

	return uc.Account.Execute(ctx, userID)
}
