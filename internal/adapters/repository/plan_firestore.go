package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.PlanRepository = (*FirestorePlanRepository)(nil)

type FirestorePlanRepository struct {
	client *firestore.Client
}

func NewFirestorePlanRepository(client *firestore.Client) *FirestorePlanRepository {
	return &FirestorePlanRepository{client: client}
}

// -------------------- Firestore DTO --------------------

type planDoc struct {
	StartDate time.Time `firestore:"start_date"`
	EndDate   time.Time `firestore:"end_date"`
	Status    string    `firestore:"status"`
	CheckinID *string   `firestore:"checkin_id,omitempty"`
	CreatedAt time.Time `firestore:"created_at"`

	CycleDayRange string `firestore:"cycle_day_range"`

	// Raw payloads as strings for UI/storage convenience
	TrainingJSON  string `firestore:"training_json"`
	NutritionJSON string `firestore:"nutrition_json"`
	RecoveryJSON  string `firestore:"recovery_json"`

	TrainingCompleted map[string]bool                      `firestore:"training_completed,omitempty"`
	MealSelections    map[string]map[string]int            `firestore:"meal_selections,omitempty"`
	PhaseFeedback     map[string]domain.PhaseFeedbackEntry `firestore:"phase_feedback,omitempty"`

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

	doc := planDoc{
		StartDate: p.StartDate,
		EndDate:   p.EndDate,
		Status:    p.Status,
		CheckinID: checkinIDPtr,
		CreatedAt: p.CreatedAt,

		CycleDayRange: p.CycleDayRange,

		TrainingJSON:  string(p.TrainingJSON),
		NutritionJSON: string(p.NutritionJSON),
		RecoveryJSON:  string(p.RecoveryJSON),

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

	p := &domain.Plan{
		ID:        doc.Ref.ID,
		UserID:    userID,
		Status:    pd.Status,
		CheckinID: checkinID,
		CreatedAt: pd.CreatedAt,
		StartDate: pd.StartDate,
		EndDate:   pd.EndDate,

		CycleDayRange: pd.CycleDayRange,

		TrainingJSON:  []byte(pd.TrainingJSON),
		NutritionJSON: []byte(pd.NutritionJSON),
		RecoveryJSON:  []byte(pd.RecoveryJSON),

		TrainingCompleted: pd.TrainingCompleted,
		MealSelections:    pd.MealSelections,
		PhaseFeedback:     pd.PhaseFeedback,

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

	plan := &domain.Plan{
		ID:        docs[0].Ref.ID,
		UserID:    userID,
		Status:    pd.Status,
		CheckinID: checkinID,
		CreatedAt: pd.CreatedAt,
		StartDate: pd.StartDate,
		EndDate:   pd.EndDate,

		CycleDayRange: pd.CycleDayRange,

		TrainingJSON:  []byte(pd.TrainingJSON),
		NutritionJSON: []byte(pd.NutritionJSON),
		RecoveryJSON:  []byte(pd.RecoveryJSON),

		TrainingCompleted: pd.TrainingCompleted,
		MealSelections:    pd.MealSelections,
		PhaseFeedback:     pd.PhaseFeedback,

		GeneratedFrom: pd.GeneratedFrom,
		SourceEventID: pd.SourceEventID,
		PlanVersion:   pd.PlanVersion,
	}

	return plan, nil
}

func (r *FirestorePlanRepository) GetLatest(ctx context.Context, userID string) (*domain.Plan, error) {
	q := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
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

	plan := &domain.Plan{
		ID:        docs[0].Ref.ID,
		UserID:    userID,
		Status:    pd.Status,
		CheckinID: checkinID,
		CreatedAt: pd.CreatedAt,
		StartDate: pd.StartDate,
		EndDate:   pd.EndDate,

		CycleDayRange: pd.CycleDayRange,

		TrainingJSON:  []byte(pd.TrainingJSON),
		NutritionJSON: []byte(pd.NutritionJSON),
		RecoveryJSON:  []byte(pd.RecoveryJSON),

		TrainingCompleted: pd.TrainingCompleted,
		MealSelections:    pd.MealSelections,
		PhaseFeedback:     pd.PhaseFeedback,

		GeneratedFrom: pd.GeneratedFrom,
		SourceEventID: pd.SourceEventID,
		PlanVersion:   pd.PlanVersion,
	}

	return plan, nil
}

func (r *FirestorePlanRepository) UpdateTrainingCompleted(ctx context.Context, userID, planID string, completed map[string]bool) error {
	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Doc(planID).
		Update(ctx, []firestore.Update{
			{Path: "training_completed", Value: completed},
		})
	return err
}

func (r *FirestorePlanRepository) UpdatedTrainingJSON(ctx context.Context, userID, trainingPlanID string, trainingJson []byte) error {
	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Doc(trainingPlanID).
		Update(ctx, []firestore.Update{
			{Path: "training_json", Value: string(trainingJson)},
		})
	return err
}

func (r *FirestorePlanRepository) UpdateNutritionJSON(ctx context.Context, userID, planID string, nutritionJSON []byte) error {
	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Doc(planID).
		Update(ctx, []firestore.Update{
			{Path: "nutrition_json", Value: string(nutritionJSON)},
		})
	return err
}

func (r *FirestorePlanRepository) UpdatePhaseFeedback(ctx context.Context, userID, planID string, feedback map[string]domain.PhaseFeedbackEntry) error {
	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Doc(planID).
		Update(ctx, []firestore.Update{
			{Path: "phase_feedback", Value: feedback},
		})
	return err
}

func (r *FirestorePlanRepository) UpdateMealSelections(ctx context.Context, userID, planID string, selections map[string]map[string]int) error {
	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("plans").
		Doc(planID).
		Update(ctx, []firestore.Update{
			{Path: "meal_selections", Value: selections},
		})
	return err
}

func (r *FirestorePlanRepository) IsTrainingDay(ctx context.Context, userID, weekday string) (bool, error) {
	plan, err := r.GetLatest(ctx, userID)
	if err != nil || plan == nil {
		return false, err
	}

	var trainingPlan struct {
		Days []struct {
			Weekday   string `json:"weekday"`
			IsRestDay bool   `json:"is_rest_day"`
		} `json:"days"`
	}

	if err := json.Unmarshal(plan.TrainingJSON, &trainingPlan); err != nil {
		return false, err
	}

	for _, day := range trainingPlan.Days {
		if day.Weekday == weekday && !day.IsRestDay {
			return true, nil
		}
	}

	return false, nil
}
