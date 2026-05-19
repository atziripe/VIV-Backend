package usecase

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"time"

	"viv/internal/core/domain"
)

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

	for _, t := range tokens {
		// Obtener el plan del user
		isTrainingDay, err := uc.DB.IsTrainingDay(ctx, t.UserID, weekday)
		if err != nil {
			log.Printf("[training_reminder] error checking plan for user %s: %v", t.UserID, err)
			continue
		}
		if !isTrainingDay {
			continue
		}

		err = uc.Notification.SendPush(ctx, t.Token, msg.Title, msg.Body)
		if err != nil {
			if isInvalidToken(err) {
				_ = uc.DeviceToken.Deactivate(ctx, t.UserID)
			}
			continue
		}
		log.Printf("[training_reminder] sent to user %s", t.UserID)
	}

	return nil
}
