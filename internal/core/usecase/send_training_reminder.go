package usecase

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"viv/internal/core/domain"
)

// trainingReminderConcurrency bounds how many per-token IsTrainingDay+
// SendPush sequences run at once — same reasoning as
// sundayCheckinConcurrency, just against the Neon pool as well as FCM
// here (IsTrainingDay is a DB read per token).
const trainingReminderConcurrency = 20

type SendTrainingReminderUseCase struct {
	DeviceToken  DeviceTokenRepository
	Notification NotificationRepository
	DB           PlanRepository
}

func NewSendTrainingReminderUseCase(
	deviceToken DeviceTokenRepository,
	notification NotificationRepository,
	db PlanRepository,
) *SendTrainingReminderUseCase {
	return &SendTrainingReminderUseCase{
		DeviceToken:  deviceToken,
		Notification: notification,
		DB:           db,
	}
}

var trainingMessages = []checkinMessage{
	{Title: "Today's a training day 💪", Body: "Complete your session and mark it as done in VIV, it helps us learn what works for you."},
	{Title: "Time to move ⚡️", Body: "Your workout is waiting. Once you're done, mark it in VIV so we can track your progress."},
	{Title: "Training day 🔥", Body: "Check today's session in VIV. And don't forget to mark it done when you're finished 🎯"},
	{Title: "Move it, move it 🎯", Body: "Today's session is locked in. Do it, mark it done, let VIV learn from it."},
}

func (uc *SendTrainingReminderUseCase) ExecuteForTimezone(ctx context.Context, timezone string, tokens []*domain.DeviceToken) error {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("[training_reminder] invalid timezone %s: %v", timezone, err)
		return nil
	}

	now := time.Now().In(loc)
	weekday := strings.ToLower(now.Weekday().String())
	log.Printf("[training_reminder] timezone: %s, weekday: %s, hour: %d, tokens: %d", timezone, weekday, now.Hour(), len(tokens))

	msg := trainingMessages[rand.Intn(len(trainingMessages))]

	// Each token's IsTrainingDay-check+send is independent of every
	// other's — fans out instead of paying for up to 2N sequential round
	// trips (one DB read + one FCM send per token). Every goroutine
	// always returns nil (failures are logged, not propagated — same as
	// the sequential version).
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(trainingReminderConcurrency)

	for _, t := range tokens {
		g.Go(func() error {
			isTrainingDay, err := uc.DB.IsTrainingDay(gctx, t.UserID, weekday)
			if err != nil {
				log.Printf("[training_reminder] error checking plan for user %s: %v", t.UserID, err)
				return nil
			}
			if !isTrainingDay {
				return nil
			}

			if err := uc.Notification.SendPush(gctx, t.Token, msg.Title, msg.Body); err != nil {
				if isInvalidToken(err) {
					_ = uc.DeviceToken.Deactivate(gctx, t.UserID)
				}
				return nil
			}
			log.Printf("[training_reminder] sent to user %s", t.UserID)
			return nil
		})
	}
	_ = g.Wait()

	return nil
}
