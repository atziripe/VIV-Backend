package usecase_test

import (
	"context"
	"errors"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakeAppleTokenRevoker struct {
	revoked []string
	err     error
}

func (f *fakeAppleTokenRevoker) RevokeRefreshToken(_ context.Context, refreshToken string) error {
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, refreshToken)
	return nil
}

// fakeAccountAnonymizer is a minimal usecase.AccountAnonymizer fake —
// NeonData is required by DeleteAccountUseCase (Neon is the sole source
// of truth account deletion must succeed against), so every test needs
// one, even the ones only exercising the Apple-revocation behavior.
// failFor, if set, fails only that one userID — for tests that need one
// user's deletion to fail without blocking another's.
type fakeAccountAnonymizer struct {
	deleted []string
	err     error
	failFor string
}

func (f *fakeAccountAnonymizer) AnonymizeAndDeleteUser(_ context.Context, userID string) error {
	if f.err != nil {
		return f.err
	}
	if f.failFor != "" && userID == f.failFor {
		return errors.New("boom")
	}
	f.deleted = append(f.deleted, userID)
	return nil
}

func TestDeleteAccount_RevokesAppleTokenWhenPresent(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", AppleRefreshToken: "apple-refresh-abc"}}}
	revoker := &fakeAppleTokenRevoker{}
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountAnonymizer{}, &fakeAuthAccountDeleter{}, users, revoker)

	if err := uc.Execute(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0] != "apple-refresh-abc" {
		t.Fatalf("expected apple token revoked, got %v", revoker.revoked)
	}
}

func TestDeleteAccount_SkipsAppleRevocationWhenNoTokenStored(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	revoker := &fakeAppleTokenRevoker{}
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountAnonymizer{}, &fakeAuthAccountDeleter{}, users, revoker)

	if err := uc.Execute(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(revoker.revoked) != 0 {
		t.Fatal("expected no revocation attempt for a user with no stored Apple token")
	}
}

func TestDeleteAccount_AppleRevocationFailureDoesNotBlockDeletion(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", AppleRefreshToken: "apple-refresh-abc"}}}
	revoker := &fakeAppleTokenRevoker{err: errors.New("apple is down")}
	authDeleter := &fakeAuthAccountDeleter{}
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountAnonymizer{}, authDeleter, users, revoker)

	if err := uc.Execute(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authDeleter.deleted) != 1 {
		t.Fatal("expected account deletion to proceed despite Apple revocation failing")
	}
}

func TestDeleteAccount_NilRevokerSkipsAppleStep(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", AppleRefreshToken: "apple-refresh-abc"}}}
	authDeleter := &fakeAuthAccountDeleter{}
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountAnonymizer{}, authDeleter, users, nil)

	if err := uc.Execute(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authDeleter.deleted) != 1 {
		t.Fatal("expected deletion to still succeed with no Apple revoker configured")
	}
}
