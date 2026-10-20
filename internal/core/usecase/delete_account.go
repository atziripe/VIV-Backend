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
	FirestoreData AccountDataStore   // required
	NeonData      AccountAnonymizer  // optional — nil when Neon isn't configured for this deployment
	Auth          AuthAccountDeleter // required

	// Users/AppleRevoker are optional together — nil means "don't bother
	// with Apple revocation" (e.g. Apple Developer credentials aren't
	// configured for this deployment yet). Users is only ever read here to
	// fetch AppleRefreshToken before it's wiped by the Firestore step
	// below; never needed if AppleRevoker is nil.
	Users        UserRepository
	AppleRevoker AppleTokenRevoker
}

func NewDeleteAccountUseCase(firestoreData AccountDataStore, neonData AccountAnonymizer, auth AuthAccountDeleter, users UserRepository, appleRevoker AppleTokenRevoker) *DeleteAccountUseCase {
	return &DeleteAccountUseCase{FirestoreData: firestoreData, NeonData: neonData, Auth: auth, Users: users, AppleRevoker: appleRevoker}
}

// Execute deletes userID's account and all of their identifying data.
// The order below is chosen for safety, not just correctness:
//
//  1. Apple: if this user ever submitted an Apple refresh token (see
//     domain.User.AppleRefreshToken), revoke it with Apple's own servers.
//     Best-effort — logged, never blocks the rest of this — same
//     reasoning as Neon below. Must run before step 3 wipes the token.
//  2. Neon: anonymize + cascade-delete, best-effort. Never blocks the
//     rest of this — matches how every other write in this codebase
//     treats Neon as a secondary, not the source of truth. If NeonData
//     is nil (Neon isn't configured for this deployment), no anonymized
//     snapshot is taken anywhere — that's a real limitation of running
//     Firestore-only, not a silently-accepted gap; see main.go's wiring
//     and the Runbook/Audit Map for how this is called out.
//  3. Firestore: delete every subcollection, the standalone
//     device_tokens document, and the user document itself. This step
//     is fatal — if it fails, Execute returns the error WITHOUT
//     touching the auth account, so the caller is still logged in and
//     can simply retry the whole request, rather than being left
//     half-deleted with no way back in to try again.
//  4. Auth: delete the Firebase Auth user last, only once every trace
//     of their data is confirmed gone. Deleting this first — or
//     Firestore failing after this already ran — would leave orphaned
//     PII with no account left that could ever request its own
//     deletion again.
func (uc *DeleteAccountUseCase) Execute(ctx context.Context, userID string) error {
	if uc.Users != nil && uc.AppleRevoker != nil {
		uc.revokeAppleToken(ctx, userID)
	}

	if uc.NeonData != nil {
		if err := uc.NeonData.AnonymizeAndDeleteUser(ctx, userID); err != nil {
			log.Printf("[delete_account] neon anonymize+delete failed, continuing: user=%s err=%v", userID, err)
		}
	}

	if err := uc.FirestoreData.DeleteAllUserData(ctx, userID); err != nil {
		return fmt.Errorf("delete account: wiping firestore data: %w", err)
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
