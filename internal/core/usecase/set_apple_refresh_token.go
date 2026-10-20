package usecase

import (
	"context"
	"fmt"
	"strings"
)

// SetAppleRefreshTokenUseCase is POST /me/apple-credentials: the client
// calls this once, right after Sign in with Apple completes, to hand the
// backend the refresh token it'll need to revoke the grant with Apple at
// account-deletion time — see domain.User.AppleRefreshToken's doc comment.
// A later call simply overwrites the stored token (the client only ever
// has one valid one at a time).
type SetAppleRefreshTokenUseCase struct {
	Users UserRepository
}

func NewSetAppleRefreshTokenUseCase(users UserRepository) *SetAppleRefreshTokenUseCase {
	return &SetAppleRefreshTokenUseCase{Users: users}
}

func (uc *SetAppleRefreshTokenUseCase) Execute(ctx context.Context, userID, refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return fmt.Errorf("set apple refresh token: refreshToken is required")
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
