package repository

import (
	"context"

	"firebase.google.com/go/auth"

	"viv/internal/core/usecase"
)

var _ usecase.AuthAccountDeleter = (*FirebaseAuthAccountDeleter)(nil)

// FirebaseAuthAccountDeleter deletes the actual Firebase Auth account —
// the last step of account deletion, after every trace of the user's data
// is confirmed gone. See usecase.DeleteAccountUseCase's doc comment for why
// this runs last.
type FirebaseAuthAccountDeleter struct {
	client *auth.Client
}

func NewFirebaseAuthAccountDeleter(client *auth.Client) *FirebaseAuthAccountDeleter {
	return &FirebaseAuthAccountDeleter{client: client}
}

func (d *FirebaseAuthAccountDeleter) DeleteUser(ctx context.Context, userID string) error {
	return d.client.DeleteUser(ctx, userID)
}
