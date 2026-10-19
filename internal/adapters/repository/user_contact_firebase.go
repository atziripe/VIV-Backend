package repository

import (
	"context"

	"firebase.google.com/go/auth"

	"viv/internal/core/usecase"
)

var _ usecase.UserContactLookup = (*FirebaseUserContactLookup)(nil)

// FirebaseUserContactLookup resolves a user's email/display name from
// Firebase Auth — domain.User has no email field of its own, same
// source-of-truth-for-identity reasoning as FirebaseAuthAccountDeleter.
type FirebaseUserContactLookup struct {
	client *auth.Client
}

func NewFirebaseUserContactLookup(client *auth.Client) *FirebaseUserContactLookup {
	return &FirebaseUserContactLookup{client: client}
}

func (l *FirebaseUserContactLookup) GetContact(ctx context.Context, userID string) (email, name string, err error) {
	record, err := l.client.GetUser(ctx, userID)
	if err != nil {
		return "", "", err
	}
	return record.Email, record.DisplayName, nil
}
