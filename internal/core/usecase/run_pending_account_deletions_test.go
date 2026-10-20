package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakePendingDeletionLister struct {
	due []*domain.User
	err error
}

func (f *fakePendingDeletionLister) ListPendingDeletions(_ context.Context, _ time.Time) ([]*domain.User, error) {
	return f.due, f.err
}

type fakeAccountDataStore struct {
	deleted []string
	err     error
}

func (f *fakeAccountDataStore) DeleteAllUserData(_ context.Context, userID string) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, userID)
	return nil
}

type fakeAuthAccountDeleter struct {
	deleted []string
}

func (f *fakeAuthAccountDeleter) DeleteUser(_ context.Context, userID string) error {
	f.deleted = append(f.deleted, userID)
	return nil
}

func TestRunPendingAccountDeletions_DeletesEveryDueUser(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	due := []*domain.User{{ID: "u1", PendingDeletionAt: &past}, {ID: "u2", PendingDeletionAt: &past}}
	lister := &fakePendingDeletionLister{due: due}
	dataStore := &fakeAccountDataStore{}
	authDeleter := &fakeAuthAccountDeleter{}
	deleteAccountUC := usecase.NewDeleteAccountUseCase(dataStore, &fakeAccountAnonymizer{}, authDeleter, nil, nil)

	uc := usecase.NewRunPendingAccountDeletionsUseCase(lister, deleteAccountUC)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(dataStore.deleted) != 2 || len(authDeleter.deleted) != 2 {
		t.Fatalf("expected both due users deleted, got data=%v auth=%v", dataStore.deleted, authDeleter.deleted)
	}
}

func TestRunPendingAccountDeletions_OneFailureDoesNotBlockTheRest(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	due := []*domain.User{{ID: "bad", PendingDeletionAt: &past}, {ID: "good", PendingDeletionAt: &past}}
	lister := &fakePendingDeletionLister{due: due}
	authDeleter := &fakeAuthAccountDeleter{}
	// Neon is the fatal step now (Phase 3 of the Firestore→Neon cutover
	// inverted this), so the failure that must not block "good" is
	// simulated there, not on the Firestore side.
	failingAnonymizer := &fakeAccountAnonymizer{failFor: "bad"}
	deleteAccountUC := usecase.NewDeleteAccountUseCase(&fakeAccountDataStore{}, failingAnonymizer, authDeleter, nil, nil)

	uc := usecase.NewRunPendingAccountDeletionsUseCase(lister, deleteAccountUC)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(authDeleter.deleted) != 1 || authDeleter.deleted[0] != "good" {
		t.Fatalf("expected only the non-failing user to be fully deleted, got %v", authDeleter.deleted)
	}
}

func TestRunPendingAccountDeletions_ListerErrorPropagates(t *testing.T) {
	lister := &fakePendingDeletionLister{err: errors.New("boom")}
	deleteAccountUC := usecase.NewDeleteAccountUseCase(&fakeAccountDataStore{}, &fakeAccountAnonymizer{}, &fakeAuthAccountDeleter{}, nil, nil)

	uc := usecase.NewRunPendingAccountDeletionsUseCase(lister, deleteAccountUC)
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatal("expected error from lister to propagate")
	}
}
