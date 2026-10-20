package usecase_test

import (
	"context"
	"errors"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

func TestSetAppleRefreshToken_SavesToken(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	uc := usecase.NewSetAppleRefreshTokenUseCase(repo)

	if err := uc.Execute(context.Background(), "u1", "abc123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.users["u1"].AppleRefreshToken != "abc123" {
		t.Fatalf("expected token saved, got %q", repo.users["u1"].AppleRefreshToken)
	}
}

func TestSetAppleRefreshToken_OverwritesExisting(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", AppleRefreshToken: "old"}}}
	uc := usecase.NewSetAppleRefreshTokenUseCase(repo)

	if err := uc.Execute(context.Background(), "u1", "new"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.users["u1"].AppleRefreshToken != "new" {
		t.Fatalf("expected token overwritten, got %q", repo.users["u1"].AppleRefreshToken)
	}
}

func TestSetAppleRefreshToken_EmptyTokenRejected(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	uc := usecase.NewSetAppleRefreshTokenUseCase(repo)

	if err := uc.Execute(context.Background(), "u1", "   "); err == nil {
		t.Fatal("expected error for empty/whitespace-only token")
	}
}

func TestSetAppleRefreshToken_UserNotFound(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewSetAppleRefreshTokenUseCase(repo)

	err := uc.Execute(context.Background(), "missing", "abc123")
	var notFound usecase.UserNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected UserNotFoundError, got %v", err)
	}
}
