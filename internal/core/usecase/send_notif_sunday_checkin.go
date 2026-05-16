package usecase

import (
	"context"
	"log"
	"math/rand"
	"strings"
)

type SendSundayCheckinUseCase struct {
	DeviceToken  DeviceTokenRepository
	Notification NotificationRepository
}

type checkinMessage struct {
	Title string
	Body  string
}

func NewSendSundayCheckinUseCase(deviceToken DeviceTokenRepository, notification NotificationRepository) *SendSundayCheckinUseCase {
	return &SendSundayCheckinUseCase{
		DeviceToken:  deviceToken,
		Notification: notification,
	}
}

func (uc *SendSundayCheckinUseCase) Execute(ctx context.Context) error {
	tokens, err := uc.DeviceToken.GetAllActive(ctx)
	if err != nil {
		return err
	}

	msg := sundayMessages[rand.Intn(len(sundayMessages))]

	for _, t := range tokens {
		err := uc.Notification.SendPush(ctx, t.Token, msg.Title, msg.Body)
		if err != nil {
			if isInvalidToken(err) {
				log.Printf("[sunday_checkin] invalid token for user %s, deactivating", t.UserID)
				_ = uc.DeviceToken.Deactivate(ctx, t.UserID)
			} else {
				log.Printf("[sunday_checkin] failed to send to user %s: %v", t.UserID, err)
			}
			continue
		}
		log.Printf("[sunday_checkin] sent to user %s", t.UserID)
	}

	return nil
}

func isInvalidToken(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "registration-token-not-registered") ||
		strings.Contains(errStr, "invalid-registration-token")
}

var sundayMessages = []checkinMessage{
	{
		Title: "Sunday already? 😅",
		Body:  "Wrap up last week + sneak peek at what's coming for you.",
	},
	{
		Title: "New week loading... ⚡️",
		Body:  "Quick sync before it drops, how was last week + what's ahead.",
	},
	{
		Title: "Don't skip this one.",
		Body:  "Your weekly recap + what your body's got planned for next week 🗓️",
	},
	{
		Title: "Sunday sync 🔁✨",
		Body:  "Last week? Done. Next week? Let's see what's coming...",
	},
	{
		Title: "It's giving new week energy ✨",
		Body:  "Close last week + see what your body has planned.",
	},
}
