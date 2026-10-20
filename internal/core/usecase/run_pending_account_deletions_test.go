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
	anonymizer := &fakeAccountAnonymizer{}
	authDeleter := &fakeAuthAccountDeleter{}
	deleteAccountUC := usecase.NewDeleteAccountUseCase(anonymizer, authDeleter, nil, nil)

	uc := usecase.NewRunPendingAccountDeletionsUseCase(lister, deleteAccountUC)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(anonymizer.deleted) != 2 || len(authDeleter.deleted) != 2 {
		t.Fatalf("expected both due users deleted, got neon=%v auth=%v", anonymizer.deleted, authDeleter.deleted)
	}
}

func TestRunPendingAccountDeletions_OneFailureDoesNotBlockTheRest(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	due := []*domain.User{{ID: "bad", PendingDeletionAt: &past}, {ID: "good", PendingDeletionAt: &past}}
	lister := &fakePendingDeletionLister{due: due}
	authDeleter := &fakeAuthAccountDeleter{}
	// Neon is the fatal step (DeleteAccountUseCase's sole data-deletion
	// dependency now that Firestore is gone), so the failure that must
	// not block "good" is simulated there.
	failingAnonymizer := &fakeAccountAnonymizer{failFor: "bad"}
	deleteAccountUC := usecase.NewDeleteAccountUseCase(failingAnonymizer, authDeleter, nil, nil)

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
	deleteAccountUC := usecase.NewDeleteAccountUseCase(&fakeAccountAnonymizer{}, &fakeAuthAccountDeleter{}, nil, nil)

	uc := usecase.NewRunPendingAccountDeletionsUseCase(lister, deleteAccountUC)
	if err := uc.Execute(context.Background()); err == nil {
		t.Fatal("expected error from lister to propagate")
	}
}
