package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.SessionLogRepository = (*FirestoreSessionLogRepository)(nil)

// FirestoreSessionLogRepository stores real-time session logs under
// users/{userID}/session_logs/{date}, date formatted as "2006-01-02" —
// same deterministic-doc-ID-by-date convention as
// FirestoreDailyCheckinRepository, which is what makes Upsert idempotent
// for free.
type FirestoreSessionLogRepository struct {
	client *firestore.Client
}

func NewFirestoreSessionLogRepository(client *firestore.Client) *FirestoreSessionLogRepository {
	return &FirestoreSessionLogRepository{client: client}
}

type setLogDoc struct {
	ExerciseID   string    `firestore:"exercise_id"`
	ExerciseName string    `firestore:"exercise_name"`
	SetNumber    int       `firestore:"set_number"`
	WeightKg     float64   `firestore:"weight_kg"`
	Reps         int       `firestore:"reps"`
	LoggedAt     time.Time `firestore:"logged_at"`
}

type sessionLogDoc struct {
	ActivityType string      `firestore:"activity_type"`
	Status       string      `firestore:"status"`
	StartedAt    time.Time   `firestore:"started_at"`
	EndedAt      *time.Time  `firestore:"ended_at"`
	Sets         []setLogDoc `firestore:"sets"`
	Feedback     string      `firestore:"feedback"`
	UpdatedAt    time.Time   `firestore:"updated_at"`
}

func (r *FirestoreSessionLogRepository) col(userID string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(userID).Collection("session_logs")
}

func (r *FirestoreSessionLogRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*domain.SessionLog, error) {
	doc, err := r.col(userID).Doc(date.Format("2006-01-02")).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var d sessionLogDoc
	if err := doc.DataTo(&d); err != nil {
		return nil, err
	}

	sets := make([]domain.SetLog, len(d.Sets))
	for i, s := range d.Sets {
		sets[i] = domain.SetLog{
			ExerciseID: s.ExerciseID, ExerciseName: s.ExerciseName, SetNumber: s.SetNumber,
			WeightKg: s.WeightKg, Reps: s.Reps, LoggedAt: s.LoggedAt,
		}
	}

	return &domain.SessionLog{
		UserID:       userID,
		Date:         date,
		ActivityType: d.ActivityType,
		Status:       domain.SessionLogStatus(d.Status),
		StartedAt:    d.StartedAt,
		EndedAt:      d.EndedAt,
		Sets:         sets,
		Feedback:     domain.SessionFeedback(d.Feedback),
		UpdatedAt:    d.UpdatedAt,
	}, nil
}

func (r *FirestoreSessionLogRepository) Upsert(ctx context.Context, log *domain.SessionLog) error {
	if log == nil {
		return nil
	}
	log.UpdatedAt = time.Now().UTC()

	sets := make([]setLogDoc, len(log.Sets))
	for i, s := range log.Sets {
		sets[i] = setLogDoc{
			ExerciseID: s.ExerciseID, ExerciseName: s.ExerciseName, SetNumber: s.SetNumber,
			WeightKg: s.WeightKg, Reps: s.Reps, LoggedAt: s.LoggedAt,
		}
	}

	doc := sessionLogDoc{
		ActivityType: log.ActivityType,
		Status:       string(log.Status),
		StartedAt:    log.StartedAt,
		EndedAt:      log.EndedAt,
		Sets:         sets,
		Feedback:     string(log.Feedback),
		UpdatedAt:    log.UpdatedAt,
	}

	_, err := r.col(log.UserID).Doc(log.Date.Format("2006-01-02")).Set(ctx, doc)
	return err
}
