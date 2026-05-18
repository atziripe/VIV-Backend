package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"

	fb "firebase.google.com/go"
	"firebase.google.com/go/messaging"
)

var _ usecase.DeviceTokenRepository = (*FirestoreDeviceTokenRepository)(nil)

type FirestoreDeviceTokenRepository struct {
	client *firestore.Client
}

func NewFirestoreDeviceTokenRepository(client *firestore.Client) *FirestoreDeviceTokenRepository {
	return &FirestoreDeviceTokenRepository{client: client}
}

type FCMRepository struct {
	firebaseApp *fb.App
}

func NewFCMRepository(app *fb.App) *FCMRepository {
	return &FCMRepository{firebaseApp: app}
}

type deviceTokenDoc struct {
	UserID    string    `firestore:"user_id"`
	Token     string    `firestore:"token"`
	Platform  string    `firestore:"platform"` // ios/android
	Timezone  string    `firestore:"timezone"`
	Active    bool      `firestore:"active"`
	CreatedAt time.Time `firestore:"created_at"`
	UpdatedAt time.Time `firestore:"updated_at"`
}

// /users/me/device-token
func (r *FirestoreDeviceTokenRepository) Upsert(ctx context.Context, token *domain.DeviceToken) error {
	ref := r.client.Collection("device_tokens").Doc(token.UserID)

	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		existing, err := tx.Get(ref)
		createdAt := token.CreatedAt
		if err == nil && existing.Exists() {
			if t, ok := existing.Data()["created_at"].(time.Time); ok {
				createdAt = t
			}
		}

		return tx.Set(ref, deviceTokenDoc{
			UserID:    token.UserID,
			Token:     token.Token,
			Platform:  token.Platform,
			Timezone:  token.Timezone,
			Active:    token.Active,
			CreatedAt: createdAt,
			UpdatedAt: token.UpdatedAt,
		})
	})
}

func (r *FirestoreDeviceTokenRepository) GetAllActive(ctx context.Context) ([]*domain.DeviceToken, error) {
	docs, err := r.client.Collection("device_tokens").
		Where("active", "==", true).
		Documents(ctx).
		GetAll()
	if err != nil {
		return nil, err
	}

	var tokens []*domain.DeviceToken
	for _, doc := range docs {
		var d deviceTokenDoc
		if err := doc.DataTo(&d); err != nil {
			continue
		}
		tokens = append(tokens, &domain.DeviceToken{
			UserID:    d.UserID,
			Token:     d.Token,
			Platform:  d.Platform,
			Timezone:  d.Timezone,
			Active:    d.Active,
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		})
	}

	return tokens, nil
}

func (r *FCMRepository) SendPush(ctx context.Context, token, title, body string) error {
	client, err := r.firebaseApp.Messaging(ctx)
	if err != nil {
		return err
	}

	_, err = client.Send(ctx, &messaging.Message{
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default", // sound and vibration
				},
			},
		},
		Android: &messaging.AndroidConfig{
			Notification: &messaging.AndroidNotification{
				Sound:       "default",
				Priority:    messaging.PriorityHigh,
				ClickAction: "FLUTTER_NOTIFICATION_CLICK",
			},
		},
		Token: token,
	})
	return err
}

func (r *FirestoreDeviceTokenRepository) Deactivate(ctx context.Context, userID string) error {
	_, err := r.client.Collection("device_tokens").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "active", Value: false},
		{Path: "updated_at", Value: time.Now().UTC()},
	})
	return err
}

func (r *FirestoreDeviceTokenRepository) GetAllActiveByTimezone(ctx context.Context) (map[string][]*domain.DeviceToken, error) {
	docs, err := r.client.Collection("device_tokens").
		Where("active", "==", true).
		Documents(ctx).
		GetAll()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]*domain.DeviceToken)
	for _, doc := range docs {
		var d deviceTokenDoc
		if err := doc.DataTo(&d); err != nil {
			continue
		}
		token := &domain.DeviceToken{
			UserID:    d.UserID,
			Token:     d.Token,
			Platform:  d.Platform,
			Timezone:  d.Timezone,
			Active:    d.Active,
			CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt,
		}
		result[d.Timezone] = append(result[d.Timezone], token)
	}

	return result, nil
}
