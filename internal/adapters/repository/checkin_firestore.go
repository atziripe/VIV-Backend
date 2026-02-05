package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.CheckinRepository = (*FirestoreCheckinRepository)(nil)

type FirestoreCheckinRepository struct {
	client *firestore.Client
}

func NewFirestoreCheckinRepository(client *firestore.Client) *FirestoreCheckinRepository {
	return &FirestoreCheckinRepository{client: client}
}

// Firestore DTO (para no acoplar domain.Checkin a tags específicos)
type checkinDoc struct {
	WeekStart          time.Time  `firestore:"week_start"`
	CreatedAt          time.Time  `firestore:"created_at"`
	SleepQuality       string     `firestore:"sleep_quality"`
	BodyStatus         string     `firestore:"body_status"`
	Appetite           string     `firestore:"appetite"`
	StressLevel        string     `firestore:"stress_level"`
	LastWeekFeeling    string     `firestore:"last_week_feeling"`
	CycleStart         *time.Time `firestore:"cycle_start"`
	WorkloadPrediction string     `firestore:"workload_prediction"`
	MentalEnergy       string     `firestore:"mental_energy"`
	PromptVersion      string     `firestore:"prompt_version"`
}

// Saving in
// users/{userID}/checkins/{checkinID}
func (r *FirestoreCheckinRepository) Create(ctx context.Context, c *domain.Checkin) error {
	if c == nil {
		return nil
	}

	doc := checkinDoc{
		WeekStart:          c.WeekStart,
		CreatedAt:          c.CreatedAt,
		SleepQuality:       c.SleepQuality,
		BodyStatus:         c.BodyStatus,
		Appetite:           c.Appetite,
		StressLevel:        c.StressLevel,
		LastWeekFeeling:    c.LastWeekFeeling,
		CycleStart:         c.CycleStart,
		WorkloadPrediction: c.WorkloadPrediction,
		MentalEnergy:       c.MentalEnergy,
		PromptVersion:      c.PromptVersion,
	}

	_, err := r.client.
		Collection("users").
		Doc(c.UserID).
		Collection("checkins").
		Doc(c.ID).
		Set(ctx, doc)

	return err
}

func (r *FirestoreCheckinRepository) GetByID(ctx context.Context, id string, userID string) (*domain.Checkin, error) {
	doc, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("checkins").
		Doc(id).
		Get(ctx)

	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var cd checkinDoc
	if err := doc.DataTo(&cd); err != nil {
		return nil, err
	}

	ch := &domain.Checkin{
		ID:                 doc.Ref.ID,
		UserID:             userID,
		WeekStart:          cd.WeekStart,
		CreatedAt:          cd.CreatedAt,
		SleepQuality:       cd.SleepQuality,
		BodyStatus:         cd.BodyStatus,
		Appetite:           cd.Appetite,
		StressLevel:        cd.StressLevel,
		LastWeekFeeling:    cd.LastWeekFeeling,
		CycleStart:         cd.CycleStart,
		WorkloadPrediction: cd.WorkloadPrediction,
		MentalEnergy:       cd.MentalEnergy,
		PromptVersion:      cd.PromptVersion,
	}

	return ch, nil
}

// GetLatestByUser obtiene el último check-in por created_at desc, limit 1
func (r *FirestoreCheckinRepository) GetLatestByUser(ctx context.Context, userID string) (*domain.Checkin, error) {
	iter := r.client.
		Collection("users").
		Doc(userID).
		Collection("checkins").
		OrderBy("created_at", firestore.Desc).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil // no hay check-ins aún
	}
	if err != nil {
		return nil, err
	}

	var cd checkinDoc
	if err := doc.DataTo(&cd); err != nil {
		return nil, err
	}

	ch := &domain.Checkin{
		ID:                 doc.Ref.ID,
		UserID:             userID,
		WeekStart:          cd.WeekStart,
		CreatedAt:          cd.CreatedAt,
		SleepQuality:       cd.SleepQuality,
		BodyStatus:         cd.BodyStatus,
		Appetite:           cd.Appetite,
		StressLevel:        cd.StressLevel,
		LastWeekFeeling:    cd.LastWeekFeeling,
		CycleStart:         cd.CycleStart,
		WorkloadPrediction: cd.WorkloadPrediction,
		MentalEnergy:       cd.MentalEnergy,
		PromptVersion:      cd.PromptVersion,
	}

	return ch, nil
}
