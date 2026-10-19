package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

func TestScheduleAccountDeletion_SetsDeadlineThirtyDaysOut(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	uc := usecase.NewScheduleAccountDeletionUseCase(repo)

	before := time.Now().UTC()
	user, err := uc.Execute(context.Background(), "u1")
	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.PendingDeletionAt == nil {
		t.Fatal("expected PendingDeletionAt to be set")
	}

	wantMin := before.Add(usecase.AccountDeletionGracePeriod)
	wantMax := after.Add(usecase.AccountDeletionGracePeriod)
	if user.PendingDeletionAt.Before(wantMin) || user.PendingDeletionAt.After(wantMax) {
		t.Fatalf("PendingDeletionAt = %v, want between %v and %v", user.PendingDeletionAt, wantMin, wantMax)
	}

	saved := repo.users["u1"]
	if saved.PendingDeletionAt == nil {
		t.Fatal("expected saved user to have PendingDeletionAt persisted")
	}
}

func TestScheduleAccountDeletion_CalledAgainRestartsClock(t *testing.T) {
	past := time.Now().UTC().Add(-24 * time.Hour)
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", PendingDeletionAt: &past},
	}}
	uc := usecase.NewScheduleAccountDeletionUseCase(repo)

	user, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !user.PendingDeletionAt.After(past) {
		t.Fatalf("expected deadline to be restarted, still at %v", user.PendingDeletionAt)
	}
}

func TestScheduleAccountDeletion_UserNotFound(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewScheduleAccountDeletionUseCase(repo)

	_, err := uc.Execute(context.Background(), "missing")
	var notFound usecase.UserNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected UserNotFoundError, got %v", err)
	}
}
