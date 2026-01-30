package repository

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// Garantizamos que implementa la interfaz
var _ usecase.PlanRepository = (*FirestorePlanRepository)(nil)

type FirestorePlanRepository struct {
	client *firestore.Client
}

func NewFirestorePlanRepository(client *firestore.Client) *FirestorePlanRepository {
	return &FirestorePlanRepository{client: client}
}

// -------------------- Firestore DTO --------------------

type recommendationDoc struct {
	Title  string `firestore:"title"`
	Action string `firestore:"action"`
	Why    string `firestore:"why"`
}

type planDoc struct {
	StartDate time.Time `firestore:"start_date"`
	EndDate   time.Time `firestore:"end_date"`
	Status    string    `firestore:"status"`
	CheckinID *string   `firestore:"checkin_id,omitempty"`
	CreatedAt time.Time `firestore:"created_at"`

	WeeklyHeadline    string `firestore:"weekly_headline"`
	CyclePhaseSummary string `firestore:"cycle_phase_summary"`
	CycleDayRange     string `firestore:"cycle_day_range"`

	// Raw payloads as strings for UI/storage convenience
	TrainingJSON  string `firestore:"training_json"`
	NutritionJSON string `firestore:"nutrition_json"`
	RecoveryJSON  string `firestore:"recovery_json"`

	Recomendations []recommendationDoc `firestore:"recomendations"`

	GeneratedFrom string `firestore:"generated_from"`
	SourceEventID string `firestore:"source_event_id"`
	PlanVersion   int    `firestore:"plan_version"`
}

// -------------------- Repository --------------------

func (r *FirestorePlanRepository) Create(ctx context.Context, p *domain.Plan) error {
	if p == nil {
		return nil
	}

	var checkinIDPtr *string
	if strings.TrimSpace(p.CheckinID) != "" {
		v := strings.TrimSpace(p.CheckinID)
		checkinIDPtr = &v
	}

	recs := make([]recommendationDoc, 0, len(p.Recommendations))
	for _, rr := range p.Recommendations {
		recs = append(recs, recommendationDoc{
			Title:  rr.Title,
			Action: rr.Action,
			Why:    rr.Why,
		})
	}

	doc := planDoc{
		StartDate: p.StartDate,
		EndDate:   p.EndDate,
		Status:    p.Status,
		CheckinID: checkinIDPtr,
		CreatedAt: p.CreatedAt,

		WeeklyHeadline:    p.WeeklyHeadline,
		CyclePhaseSummary: p.CyclePhaseSummary,
		CycleDayRange:     p.CycleDayRange,

		TrainingJSON:  string(p.TrainingJSON),
		NutritionJSON: string(p.NutritionJSON),
		RecoveryJSON:  string(p.RecoveryJSON),

		Recomendations: recs,

		GeneratedFrom: p.GeneratedFrom,
		SourceEventID: p.SourceEventID,
		PlanVersion:   p.PlanVersion,
	}

	docRef := r.client.
		Collection("users").
		Doc(p.UserID).
		Collection("plans").
		NewDoc()

	p.ID = docRef.ID

	_, err := docRef.Set(ctx, doc)
	return err
}

func (r *FirestorePlanRepository) GetByID(ctx context.Context, userID, planID string) (*domain.Plan, error) {
	doc, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Doc(planID).
		Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var pd planDoc
	if err := doc.DataTo(&pd); err != nil {
		return nil, err
	}

	checkinID := ""
	if pd.CheckinID != nil {
		checkinID = *pd.CheckinID
	}

	recs := make([]domain.Recommendations, 0, len(pd.Recomendations))
	for _, rr := range pd.Recomendations {
		recs = append(recs, domain.Recommendations{
			Title:  rr.Title,
			Action: rr.Action,
			Why:    rr.Why,
		})
	}

	p := &domain.Plan{
		ID:        doc.Ref.ID,
		UserID:    userID,
		Status:    pd.Status,
		CheckinID: checkinID,
		CreatedAt: pd.CreatedAt,
		StartDate: pd.StartDate,
		EndDate:   pd.EndDate,

		WeeklyHeadline:    pd.WeeklyHeadline,
		CyclePhaseSummary: pd.CyclePhaseSummary,
		CycleDayRange:     pd.CycleDayRange,

		TrainingJSON:  []byte(pd.TrainingJSON),
		NutritionJSON: []byte(pd.NutritionJSON),
		RecoveryJSON:  []byte(pd.RecoveryJSON),

		Recommendations: recs,

		GeneratedFrom: pd.GeneratedFrom,
		SourceEventID: pd.SourceEventID,
		PlanVersion:   pd.PlanVersion,
	}

	return p, nil
}

func (r *FirestorePlanRepository) GetLatestByWeekStart(ctx context.Context, userID string, weekStart time.Time) (*domain.Plan, error) {
	start := time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	q := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Where("start_date", "==", start).
		OrderBy("created_at", firestore.Desc).
		Limit(1)

	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}

	var pd planDoc
	if err := docs[0].DataTo(&pd); err != nil {
		return nil, err
	}

	checkinID := ""
	if pd.CheckinID != nil {
		checkinID = *pd.CheckinID
	}

	recs := make([]domain.Recommendations, 0, len(pd.Recomendations))
	for _, rr := range pd.Recomendations {
		recs = append(recs, domain.Recommendations{
			Title:  rr.Title,
			Action: rr.Action,
			Why:    rr.Why,
		})
	}

	plan := &domain.Plan{
		ID:        docs[0].Ref.ID,
		UserID:    userID,
		Status:    pd.Status,
		CheckinID: checkinID,
		CreatedAt: pd.CreatedAt,
		StartDate: pd.StartDate,
		EndDate:   pd.EndDate,

		WeeklyHeadline:    pd.WeeklyHeadline,
		CyclePhaseSummary: pd.CyclePhaseSummary,
		CycleDayRange:     pd.CycleDayRange,

		TrainingJSON:  []byte(pd.TrainingJSON),
		NutritionJSON: []byte(pd.NutritionJSON),
		RecoveryJSON:  []byte(pd.RecoveryJSON),

		Recommendations: recs,

		GeneratedFrom: pd.GeneratedFrom,
		SourceEventID: pd.SourceEventID,
		PlanVersion:   pd.PlanVersion,
	}

	return plan, nil
}
