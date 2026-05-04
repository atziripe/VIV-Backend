package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.DeviceTokenRepository = (*FirestoreDeviceTokenRepository)(nil)

type FirestoreDeviceTokenRepository struct {
	client *firestore.Client
}

func NewFirestoreDeviceTokenRepository(client *firestore.Client) *FirestoreDeviceTokenRepository {
	return &FirestoreDeviceTokenRepository{client: client}
}

type deviceTokenDoc struct {
	UserID    string    `firestore:"user_id"`
	Token     string    `firestore:"user_id"`
	Platform  string    `firestore:"platform"` // ios/android
	Active    bool      `firestore:"active"`
	CreatedAt time.Time `firestore:"created_at"`
	UpdatedAt time.Time `firestore:"updated_at"`
}

// /users/me/device-token
func (r *FirestoreDeviceTokenRepository) Upsert(ctx context.Context, token *domain.DeviceToken) error {
	if token == nil {
		return nil
	}

	doc := deviceTokenDoc{}

	_, err := r.client.
		Collection("device_tokens").
		Doc(token.UserID).
		Set(ctx, doc)

	return err
}
