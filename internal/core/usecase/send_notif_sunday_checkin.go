package usecase

import (
	"context"
	"log"
	"math/rand"
	"strings"

	"golang.org/x/sync/errgroup"
)

// sundayCheckinConcurrency bounds how many SendPush calls run at once —
// unbounded fan-out against FCM (and the Neon pool, for the occasional
// Deactivate) isn't worth it for a once-a-week job; this just keeps the
// whole run from being N sequential round trips to FCM.
const sundayCheckinConcurrency = 20

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

	// Each token's send+deactivate is independent of every other's — no
	// shared state between iterations, so this fans out instead of
	// paying for N sequential FCM round trips. Every goroutine always
	// returns nil (a failed send/deactivate is logged, not propagated —
	// same as the sequential version), so errgroup is just doing bounded
	// concurrency + wait-for-all here, not error aggregation.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(sundayCheckinConcurrency)

	for _, t := range tokens {
		g.Go(func() error {
			if err := uc.Notification.SendPush(gctx, t.Token, msg.Title, msg.Body); err != nil {
				if isInvalidToken(err) {
					log.Printf("[sunday_checkin] invalid token for user %s, deactivating", t.UserID)
					_ = uc.DeviceToken.Deactivate(gctx, t.UserID)
				} else {
					log.Printf("[sunday_checkin] failed to send to user %s: %v", t.UserID, err)
				}
				return nil
			}
			log.Printf("[sunday_checkin] sent to user %s", t.UserID)
			return nil
		})
	}
	_ = g.Wait()

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
