package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.DailyCheckinRepository = (*FirestoreDailyCheckinRepository)(nil)

// FirestoreDailyCheckinRepository stores VIV-103 daily check-ins under
// users/{userID}/daily_checkins/{date}, date formatted as "2006-01-02".
// Keying the document ID by date is what makes Upsert idempotent for
// free — a second submission for the same date overwrites the same
// document via Set, rather than needing a read-then-branch to decide
// create vs. update.
type FirestoreDailyCheckinRepository struct {
	client *firestore.Client
}

func NewFirestoreDailyCheckinRepository(client *firestore.Client) *FirestoreDailyCheckinRepository {
	return &FirestoreDailyCheckinRepository{client: client}
}

type dailyCheckinDoc struct {
	Sleep       string    `firestore:"sleep"`
	Body        string    `firestore:"body"`
	Demand      string    `firestore:"demand"`
	Need        string    `firestore:"need"`
	Recovery    string    `firestore:"recovery_capacity"`
	Bandwidth   string    `firestore:"life_bandwidth"`
	Build       string    `firestore:"build_readiness"`
	SubmittedAt time.Time `firestore:"submitted_at"`
}

func (r *FirestoreDailyCheckinRepository) Upsert(ctx context.Context, c *domain.DailyCheckin) error {
	if c == nil {
		return nil
	}

	doc := dailyCheckinDoc{
		Sleep:       string(c.Answers.Sleep),
		Body:        string(c.Answers.Body),
		Demand:      string(c.Answers.Demand),
		Need:        string(c.Answers.Need),
		Recovery:    string(c.Readiness.RecoveryCapacity),
		Bandwidth:   string(c.Readiness.LifeBandwidth),
		Build:       string(c.Readiness.BuildReadiness),
		SubmittedAt: c.SubmittedAt,
	}

	_, err := r.client.
		Collection("users").Doc(c.UserID).
		Collection("daily_checkins").Doc(c.Date.Format("2006-01-02")).
		Set(ctx, doc)
	return err
}
