package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

func TestCancelAccountDeletion_ClearsPendingDeadline(t *testing.T) {
	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", PendingDeletionAt: &future},
	}}
	uc := usecase.NewCancelAccountDeletionUseCase(repo)

	user, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.PendingDeletionAt != nil {
		t.Fatalf("expected PendingDeletionAt to be cleared, got %v", user.PendingDeletionAt)
	}
	if repo.users["u1"].PendingDeletionAt != nil {
		t.Fatal("expected saved user to have PendingDeletionAt cleared")
	}
}

func TestCancelAccountDeletion_NoopWhenNothingPending(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	uc := usecase.NewCancelAccountDeletionUseCase(repo)

	user, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.PendingDeletionAt != nil {
		t.Fatal("expected PendingDeletionAt to still be nil")
	}
}

func TestCancelAccountDeletion_UserNotFound(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewCancelAccountDeletionUseCase(repo)

	_, err := uc.Execute(context.Background(), "missing")
	var notFound usecase.UserNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected UserNotFoundError, got %v", err)
	}
}
