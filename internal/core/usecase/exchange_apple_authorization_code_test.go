package usecase_test

import (
	"context"
	"errors"
	"testing"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakeAppleCodeExchanger struct {
	refreshToken string
	err          error
	received     []string
}

func (f *fakeAppleCodeExchanger) ExchangeAuthorizationCode(_ context.Context, authorizationCode string) (string, error) {
	f.received = append(f.received, authorizationCode)
	if f.err != nil {
		return "", f.err
	}
	return f.refreshToken, nil
}

func TestExchangeAppleAuthorizationCode_SavesReturnedRefreshToken(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	exchanger := &fakeAppleCodeExchanger{refreshToken: "new-refresh-token"}
	uc := usecase.NewExchangeAppleAuthorizationCodeUseCase(repo, exchanger)

	if err := uc.Execute(context.Background(), "u1", "auth-code-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exchanger.received) != 1 || exchanger.received[0] != "auth-code-123" {
		t.Fatalf("expected the code to be exchanged, got %v", exchanger.received)
	}
	if repo.users["u1"].AppleRefreshToken != "new-refresh-token" {
		t.Fatalf("expected refresh token saved, got %q", repo.users["u1"].AppleRefreshToken)
	}
}

func TestExchangeAppleAuthorizationCode_KeepsExistingTokenWhenAppleReturnsNone(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", AppleRefreshToken: "old-token"}}}
	exchanger := &fakeAppleCodeExchanger{refreshToken: ""}
	uc := usecase.NewExchangeAppleAuthorizationCodeUseCase(repo, exchanger)

	if err := uc.Execute(context.Background(), "u1", "auth-code-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.users["u1"].AppleRefreshToken != "old-token" {
		t.Fatalf("expected the existing token to be left in place, got %q", repo.users["u1"].AppleRefreshToken)
	}
}

func TestExchangeAppleAuthorizationCode_EmptyCodeRejected(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	exchanger := &fakeAppleCodeExchanger{refreshToken: "irrelevant"}
	uc := usecase.NewExchangeAppleAuthorizationCodeUseCase(repo, exchanger)

	if err := uc.Execute(context.Background(), "u1", "   "); err == nil {
		t.Fatal("expected error for empty/whitespace-only authorization code")
	}
	if len(exchanger.received) != 0 {
		t.Fatal("expected no exchange attempt for an empty code")
	}
}

func TestExchangeAppleAuthorizationCode_ExchangeFailurePropagates(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	exchanger := &fakeAppleCodeExchanger{err: errors.New("apple rejected the code")}
	uc := usecase.NewExchangeAppleAuthorizationCodeUseCase(repo, exchanger)

	if err := uc.Execute(context.Background(), "u1", "auth-code-123"); err == nil {
		t.Fatal("expected the exchange failure to propagate")
	}
}

func TestExchangeAppleAuthorizationCode_UserNotFound(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	exchanger := &fakeAppleCodeExchanger{refreshToken: "new-refresh-token"}
	uc := usecase.NewExchangeAppleAuthorizationCodeUseCase(repo, exchanger)

	err := uc.Execute(context.Background(), "missing", "auth-code-123")
	var notFound usecase.UserNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected UserNotFoundError, got %v", err)
	}
}
