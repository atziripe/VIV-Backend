package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/checkin"
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

func (r *FirestoreDailyCheckinRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*domain.DailyCheckin, error) {
	snap, err := r.client.
		Collection("users").Doc(userID).
		Collection("daily_checkins").Doc(date.Format("2006-01-02")).
		Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var d dailyCheckinDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}

	return &domain.DailyCheckin{
		UserID: userID,
		Date:   date,
		Answers: checkin.DailyCheckin{
			Sleep:  checkin.SleepAnswer(d.Sleep),
			Body:   checkin.BodyAnswer(d.Body),
			Demand: checkin.DemandAnswer(d.Demand),
			Need:   checkin.NeedAnswer(d.Need),
		},
		Readiness: checkin.ReadinessDimensions{
			RecoveryCapacity: checkin.RecoveryCapacity(d.Recovery),
			LifeBandwidth:    checkin.LifeBandwidth(d.Bandwidth),
			BuildReadiness:   checkin.BuildReadiness(d.Build),
		},
		SubmittedAt: d.SubmittedAt,
	}, nil
}
