package repository

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.NutritionPlanRepository = (*FirestoreNutritionPlanRepository)(nil)

// FirestoreNutritionPlanRepository stores the new pipeline's single active
// nutrition plan per user under users/{userID}/nutrition_plan/current —
// unlike FirestorePlanRepository, there's no per-generation versioning:
// each save overwrites the one document.
type FirestoreNutritionPlanRepository struct {
	client *firestore.Client
}

func NewFirestoreNutritionPlanRepository(client *firestore.Client) *FirestoreNutritionPlanRepository {
	return &FirestoreNutritionPlanRepository{client: client}
}

type nutritionPlanDoc struct {
	PlanJSON  string    `firestore:"plan_json"`
	CreatedAt time.Time `firestore:"created_at"`
	UpdatedAt time.Time `firestore:"updated_at"`
}

func (r *FirestoreNutritionPlanRepository) doc(userID string) *firestore.DocumentRef {
	return r.client.Collection("users").Doc(userID).Collection("nutrition_plan").Doc("current")
}

func (r *FirestoreNutritionPlanRepository) GetByUserID(ctx context.Context, userID string) (*domain.NutritionPlan, error) {
	snap, err := r.doc(userID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var d nutritionPlanDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}

	var plan domain.NutritionWeekPlan
	if err := json.Unmarshal([]byte(d.PlanJSON), &plan); err != nil {
		return nil, err
	}

	return &domain.NutritionPlan{
		UserID:    userID,
		Plan:      plan,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}, nil
}

func (r *FirestoreNutritionPlanRepository) Save(ctx context.Context, userID string, plan domain.NutritionWeekPlan) error {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	createdAt := now
	if existing, err := r.doc(userID).Get(ctx); err == nil {
		var d nutritionPlanDoc
		if err := existing.DataTo(&d); err == nil && !d.CreatedAt.IsZero() {
			createdAt = d.CreatedAt
		}
	}

	doc := nutritionPlanDoc{
		PlanJSON:  string(planJSON),
		CreatedAt: createdAt,
		UpdatedAt: now,
	}

	_, err = r.doc(userID).Set(ctx, doc)
	return err
}
