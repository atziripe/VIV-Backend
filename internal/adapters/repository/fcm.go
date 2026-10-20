package repository

import (
	"context"

	fb "firebase.google.com/go"
	"firebase.google.com/go/messaging"
)

// FCMRepository sends push notifications via Firebase Cloud Messaging —
// rides on the same *fb.App used for Auth, never touches Firestore.
type FCMRepository struct {
	firebaseApp *fb.App
}

func NewFCMRepository(app *fb.App) *FCMRepository {
	return &FCMRepository{firebaseApp: app}
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
