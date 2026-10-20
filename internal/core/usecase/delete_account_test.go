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

func TestDeleteAccount_RevokesAppleTokenWhenPresent(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", AppleRefreshToken: "apple-refresh-abc"}}}
	revoker := &fakeAppleTokenRevoker{}
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountDataStore{}, nil, &fakeAuthAccountDeleter{}, users, revoker)

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
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountDataStore{}, nil, &fakeAuthAccountDeleter{}, users, revoker)

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
	dataStore := &fakeAccountDataStore{}
	authDeleter := &fakeAuthAccountDeleter{}
	uc := usecase.NewDeleteAccountUseCase(dataStore, nil, authDeleter, users, revoker)

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
	uc := usecase.NewDeleteAccountUseCase(&fakeAccountDataStore{}, nil, authDeleter, users, nil)

	if err := uc.Execute(context.Background(), "u1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(authDeleter.deleted) != 1 {
		t.Fatal("expected deletion to still succeed with no Apple revoker configured")
	}
}
