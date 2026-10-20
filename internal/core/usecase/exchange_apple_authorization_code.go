package usecase

import (
	"context"
	"fmt"
	"strings"
)

// ExchangeAppleAuthorizationCodeUseCase is POST /me/apple-credentials: the
// client calls this after every Sign in with Apple, not just the first —
// Apple issues a fresh authorization_code on every login, unlike the
// refresh_token (and the name/email), which only ever come the first time
// someone authorizes. Exchanging it is what lets an existing account get
// covered retroactively too, the next time that person signs in.
//
// Apple's token endpoint doesn't always return a new refresh_token on
// every exchange — when it doesn't, the previously stored one (if any) is
// left in place rather than being overwritten with nothing.
type ExchangeAppleAuthorizationCodeUseCase struct {
	Users    UserRepository
	Exchange AppleCodeExchanger
}

func NewExchangeAppleAuthorizationCodeUseCase(users UserRepository, exchange AppleCodeExchanger) *ExchangeAppleAuthorizationCodeUseCase {
	return &ExchangeAppleAuthorizationCodeUseCase{Users: users, Exchange: exchange}
}

func (uc *ExchangeAppleAuthorizationCodeUseCase) Execute(ctx context.Context, userID, authorizationCode string) error {
	authorizationCode = strings.TrimSpace(authorizationCode)
	if authorizationCode == "" {
		return fmt.Errorf("exchange apple authorization code: authorizationCode is required")
	}
	if uc.Exchange == nil {
		return fmt.Errorf("exchange apple authorization code: Sign in with Apple is not configured for this deployment")
	}

	refreshToken, err := uc.Exchange.ExchangeAuthorizationCode(ctx, authorizationCode)
	if err != nil {
		return fmt.Errorf("exchange apple authorization code: %w", err)
	}
	if refreshToken == "" {
		return nil
	}

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound(userID)
	}

	user.AppleRefreshToken = refreshToken
	return uc.Users.Save(ctx, user)
}
