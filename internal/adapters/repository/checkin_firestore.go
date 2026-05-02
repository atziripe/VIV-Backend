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
	WeekStart       time.Time  `firestore:"week_start"`
	CreatedAt       time.Time  `firestore:"created_at"`
	Sleep           string     `firestore:"sleep_quality"`
	Body            string     `firestore:"body_status"`
	AppetiteV2      string     `firestore:"appetite"`
	Demand          string     `firestore:"demand"`
	LastWeekFeel    string     `firestore:"last_week_feel"`
	CycleStart      *time.Time `firestore:"cycle_start"`
	PMSSymptoms     *string    `firestore:"pms_symptoms"`
	Predictability  string     `firestore:"predictability"`
	Readiness       string     `firestore:"readiness"`
	AdditionalNotes string     `firestore:"additional_notes"`
}

// Saving in
// users/{userID}/checkins/{checkinID}
func (r *FirestoreCheckinRepository) Create(ctx context.Context, c *domain.Checkin) error {
	if c == nil {
		return nil
	}

	doc := checkinDoc{
		WeekStart:       c.WeekStart,
		CreatedAt:       c.CreatedAt,
		Sleep:           string(c.Sleep),
		Body:            string(c.Body),
		AppetiteV2:      string(c.AppetiteV2),
		Demand:          string(c.Demand),
		LastWeekFeel:    string(c.LastWeekFeel),
		CycleStart:      c.CycleStart,
		PMSSymptoms:     (*string)(c.PMSSymptoms),
		Predictability:  string(c.Predictability),
		Readiness:       string(c.Readiness),
		AdditionalNotes: c.AdditionalNotes,
	}

	_, err := r.client.
		Collection("users").
		Doc(c.UserID).
		Collection("checkins").
		Doc(c.ID).
		Set(ctx, doc)

	return err
}

func (r *FirestoreCheckinRepository) GetByID(ctx context.Context, userID string, id string) (*domain.Checkin, error) {
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
		ID:              doc.Ref.ID,
		UserID:          userID,
		WeekStart:       cd.WeekStart,
		CreatedAt:       cd.CreatedAt,
		Sleep:           domain.CheckinSleep(cd.Sleep),
		Body:            domain.CheckinBody(cd.Body),
		AppetiteV2:      domain.CheckinAppetite(cd.AppetiteV2),
		Demand:          domain.CheckinDemand(cd.Demand),
		LastWeekFeel:    domain.CheckinLastWeek(cd.LastWeekFeel),
		CycleStart:      cd.CycleStart,
		PMSSymptoms:     (*domain.CheckinPMSSymptoms)(cd.PMSSymptoms),
		Predictability:  domain.CheckinPredictability(cd.Predictability),
		Readiness:       domain.CheckinReadiness(cd.Readiness),
		AdditionalNotes: cd.AdditionalNotes,
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
		ID:              doc.Ref.ID,
		UserID:          userID,
		WeekStart:       cd.WeekStart,
		CreatedAt:       cd.CreatedAt,
		Sleep:           domain.CheckinSleep(cd.Sleep),
		Body:            domain.CheckinBody(cd.Body),
		AppetiteV2:      domain.CheckinAppetite(cd.AppetiteV2),
		Demand:          domain.CheckinDemand(cd.Demand),
		LastWeekFeel:    domain.CheckinLastWeek(cd.LastWeekFeel),
		CycleStart:      cd.CycleStart,
		PMSSymptoms:     (*domain.CheckinPMSSymptoms)(cd.PMSSymptoms),
		Predictability:  domain.CheckinPredictability(cd.Predictability),
		Readiness:       domain.CheckinReadiness(cd.Readiness),
		AdditionalNotes: cd.AdditionalNotes,
	}

	return ch, nil
}
