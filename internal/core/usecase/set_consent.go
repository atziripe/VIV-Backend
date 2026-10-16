package usecase

import (
	"context"
	"fmt"
	"strings"

	"viv/internal/core/domain"
)

var validConsentPurposes = map[domain.ConsentPurpose]bool{
	domain.ConsentHealthDataProcessing:       true,
	domain.ConsentAIProcessing:               true,
	domain.ConsentResearchProductImprovement: true,
}

type SetConsentInput struct {
	UserID  string
	Purpose domain.ConsentPurpose
	Granted bool
}

type SetConsentOutput struct {
	// AccountDeleted is true when this call revoked
	// ConsentHealthDataProcessing — which this usecase treats as "the user
	// no longer wants VIV processing their data at all" and routes
	// straight into the same account-deletion flow DELETE /me uses (see
	// AccountDeleter's doc comment on why there's no middle ground). The
	// caller's account and all their data are gone by the time this
	// returns true.
	AccountDeleted bool
}

// AccountDeleter is DeleteAccountUseCase, declared narrowly here — same
// reasoning as every other narrow collaborator port in this package.
type AccountDeleter interface {
	Execute(ctx context.Context, userID string) error
}

// SetConsentUseCase records a consent grant or revoke. Revoking
// ConsentHealthDataProcessing is special-cased: the product has no
// functioning "keep the account but stop processing health data" state
// (everything from onboarding onward depends on it), so rather than offer
// a revoke that leaves the account in a broken, half-consented limbo, it
// deletes the account outright — same outcome as calling DELETE /me
// directly, just reachable from the consent screen too.
type SetConsentUseCase struct {
	Consents ConsentRepository
	Deleter  AccountDeleter
}

func NewSetConsentUseCase(consents ConsentRepository, deleter AccountDeleter) *SetConsentUseCase {
	return &SetConsentUseCase{Consents: consents, Deleter: deleter}
}

func (uc *SetConsentUseCase) Execute(ctx context.Context, in SetConsentInput) (SetConsentOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return SetConsentOutput{}, fmt.Errorf("set consent: userID is required")
	}
	if !validConsentPurposes[in.Purpose] {
		return SetConsentOutput{}, ErrInvalidConsentPurpose(string(in.Purpose))
	}

	if in.Purpose == domain.ConsentHealthDataProcessing && !in.Granted {
		if err := uc.Deleter.Execute(ctx, userID); err != nil {
			return SetConsentOutput{}, fmt.Errorf("set consent: revoking health data processing: %w", err)
		}
		return SetConsentOutput{AccountDeleted: true}, nil
	}

	if err := uc.Consents.Grant(ctx, userID, in.Purpose, in.Granted, domain.CurrentPrivacyPolicyVersion); err != nil {
		return SetConsentOutput{}, fmt.Errorf("set consent: %w", err)
	}
	return SetConsentOutput{}, nil
}
