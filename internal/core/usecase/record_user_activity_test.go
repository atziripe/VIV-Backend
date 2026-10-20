package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

func TestRecordUserActivity_FirstTimeSetsLastActiveAt(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	uc := usecase.NewRecordUserActivityUseCase(repo)

	user := repo.users["u1"]
	if err := uc.Execute(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.LastActiveAt == nil {
		t.Fatal("expected LastActiveAt to be set")
	}
	if repo.users["u1"].LastActiveAt == nil {
		t.Fatal("expected saved user to have LastActiveAt persisted")
	}
}

func TestRecordUserActivity_SameDaySkipsSave(t *testing.T) {
	earlierToday := time.Now().UTC().Add(-time.Hour)
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", LastActiveAt: &earlierToday, UpdatedAt: earlierToday},
	}}
	uc := usecase.NewRecordUserActivityUseCase(repo)

	user := repo.users["u1"]
	originalLastActive := user.LastActiveAt
	if err := uc.Execute(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.LastActiveAt != originalLastActive {
		t.Fatal("expected LastActiveAt to be left untouched for a same-day repeat visit")
	}
}

func TestRecordUserActivity_NewDayUpdatesLastActiveAt(t *testing.T) {
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", LastActiveAt: &yesterday},
	}}
	uc := usecase.NewRecordUserActivityUseCase(repo)

	user := repo.users["u1"]
	if err := uc.Execute(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !user.LastActiveAt.After(yesterday) {
		t.Fatalf("expected LastActiveAt to advance past %v, got %v", yesterday, user.LastActiveAt)
	}
}

func TestRecordUserActivity_AutoCancelsInactivityTriggeredDeletion(t *testing.T) {
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	deadline := time.Now().UTC().Add(20 * 24 * time.Hour)
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {
			ID:                    "u1",
			LastActiveAt:          &yesterday,
			PendingDeletionAt:     &deadline,
			PendingDeletionReason: domain.PendingDeletionInactivity,
		},
	}}
	uc := usecase.NewRecordUserActivityUseCase(repo)

	user := repo.users["u1"]
	if err := uc.Execute(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.PendingDeletionAt != nil {
		t.Fatal("expected inactivity-triggered pending deletion to be auto-cancelled")
	}
	if user.PendingDeletionReason != "" {
		t.Fatalf("expected PendingDeletionReason cleared, got %q", user.PendingDeletionReason)
	}
}

func TestRecordUserActivity_DoesNotCancelUserRequestedDeletion(t *testing.T) {
	deadline := time.Now().UTC().Add(20 * 24 * time.Hour)
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {
			ID:                    "u1",
			PendingDeletionAt:     &deadline,
			PendingDeletionReason: domain.PendingDeletionUserRequested,
		},
	}}
	uc := usecase.NewRecordUserActivityUseCase(repo)

	user := repo.users["u1"]
	if err := uc.Execute(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.PendingDeletionAt == nil {
		t.Fatal("expected user-requested pending deletion to survive ordinary app usage")
	}
	if user.PendingDeletionReason != domain.PendingDeletionUserRequested {
		t.Fatalf("expected reason to stay user_requested, got %q", user.PendingDeletionReason)
	}
}
