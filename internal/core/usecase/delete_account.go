package usecase

import (
	"context"
	"fmt"
	"log"
)

// DeleteAccountUseCase permanently closes a user's account — the App
// Store account-deletion requirement (guideline 5.1.1v): a user must be
// able to delete their account and their data from inside the app, not
// just deactivate it. Anonymized/aggregate data survives on purpose
// (see AccountAnonymizer) — everything that could identify the person
// does not.
type DeleteAccountUseCase struct {
	NeonData AccountAnonymizer  // required — Neon is the source of truth
	Auth     AuthAccountDeleter // required

	// Users/AppleRevoker are optional together — nil means "don't bother
	// with Apple revocation" (e.g. Apple Developer credentials aren't
	// configured for this deployment yet). Users is only ever read here to
	// fetch AppleRefreshToken (from Neon — see domain.User.AppleRefreshToken)
	// before the NeonData step below deletes the row it lives on.
	Users        UserRepository
	AppleRevoker AppleTokenRevoker
}

func NewDeleteAccountUseCase(neonData AccountAnonymizer, auth AuthAccountDeleter, users UserRepository, appleRevoker AppleTokenRevoker) *DeleteAccountUseCase {
	return &DeleteAccountUseCase{NeonData: neonData, Auth: auth, Users: users, AppleRevoker: appleRevoker}
}

// Execute deletes userID's account and all of their identifying data.
// The order below is chosen for safety, not just correctness:
//
//  1. Apple: if this user ever submitted an Apple refresh token (see
//     domain.User.AppleRefreshToken), revoke it with Apple's own servers.
//     Best-effort — logged, never blocks the rest of this. Must run
//     before step 2 deletes the users row the token lives on.
//  2. Neon: anonymize + cascade-delete. This step is fatal — if it
//     fails, Execute returns the error WITHOUT touching the auth
//     account, so the caller is still logged in and can simply retry
//     the whole request, rather than being left half-deleted with no
//     way back in to try again. Neon is the sole source of truth for
//     every entity this touches (Firestore was removed entirely in
//     Phase 4 of the Firestore→Neon cutover).
//  3. Auth: delete the Firebase Auth user last, only once every trace
//     of their data is confirmed gone from Neon. Deleting this first —
//     or Neon failing after this already ran — would leave orphaned
//     PII with no account left that could ever request its own
//     deletion again.
func (uc *DeleteAccountUseCase) Execute(ctx context.Context, userID string) error {
	if uc.Users != nil && uc.AppleRevoker != nil {
		uc.revokeAppleToken(ctx, userID)
	}

	if err := uc.NeonData.AnonymizeAndDeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("delete account: anonymizing/deleting neon data: %w", err)
	}

	if err := uc.Auth.DeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("delete account: deleting auth user: %w", err)
	}

	return nil
}

func (uc *DeleteAccountUseCase) revokeAppleToken(ctx context.Context, userID string) {
	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		log.Printf("[delete_account] failed to load user for apple revocation, continuing: user=%s err=%v", userID, err)
		return
	}
	if user == nil || user.AppleRefreshToken == "" {
		return
	}

	if err := uc.AppleRevoker.RevokeRefreshToken(ctx, user.AppleRefreshToken); err != nil {
		log.Printf("[delete_account] apple token revocation failed, continuing: user=%s err=%v", userID, err)
	}
}
