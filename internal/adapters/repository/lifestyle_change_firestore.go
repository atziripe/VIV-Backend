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

// Aseguramos que implementa la interfaz
var _ usecase.LifestyleChangeRepository = (*FirestoreLifestyleChangeRepository)(nil)

type FirestoreLifestyleChangeRepository struct {
	client *firestore.Client
}

func NewFirestoreLifestyleChangeRepository(client *firestore.Client) *FirestoreLifestyleChangeRepository {
	return &FirestoreLifestyleChangeRepository{client: client}
}

// DTO para Firestore
type lifestyleChangeDoc struct {
	CreatedAt    time.Time  `firestore:"created_at"`
	Type         string     `firestore:"type"`
	SpaceTrain   string     `firestore:"space_train"`
	PossibleDiet string     `firestore:"possible_diet"`
	Energy       string     `firestore:"energy"`
	AppliesTo    *time.Time `firestore:"applies_to"`
	PlanID       *string    `firestore:"plan_id"`
}

// Create guarda en:
// users/{userId}/change_lifestyle/{changeId}
func (r *FirestoreLifestyleChangeRepository) Create(ctx context.Context, e *domain.LifestyleChange) error {
	if e == nil {
		return nil
	}

	doc := lifestyleChangeDoc{
		CreatedAt:    e.CreatedAt,
		Type:         e.Type,
		SpaceTrain:   e.SpaceTrain,
		PossibleDiet: e.PossibleDiet,
		Energy:       e.Energy,
		AppliesTo:    e.AppliesTo,
		PlanID:       e.PlanID,
	}

	_, err := r.client.
		Collection("users").
		Doc(e.UserID).
		Collection("change_lifestyle").
		Doc(e.ID).
		Set(ctx, doc)

	return err
}

func (r *FirestoreLifestyleChangeRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.LifestyleChange, error) {
	iter := r.client.
		Collection("users").
		Doc(userID).
		Collection("change_lifestyle").
		OrderBy("created_at", firestore.Desc).
		Limit(limit).
		Documents(ctx)

	var results []*domain.LifestyleChange

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var ld lifestyleChangeDoc
		if err := doc.DataTo(&ld); err != nil {
			return nil, err
		}

		change := &domain.LifestyleChange{
			ID:           doc.Ref.ID,
			UserID:       userID,
			CreatedAt:    ld.CreatedAt,
			Type:         ld.Type,
			SpaceTrain:   ld.SpaceTrain,
			PossibleDiet: ld.PossibleDiet,
			Energy:       ld.Energy,
			AppliesTo:    ld.AppliesTo,
			PlanID:       ld.PlanID,
		}

		results = append(results, change)
	}

	return results, nil
}

func (r *FirestoreLifestyleChangeRepository) GetByID(ctx context.Context, userID string, changeID string) (*domain.LifestyleChange, error) {

	doc, err := r.client.Collection("users").Doc(userID).Collection("change_lifestyle").Doc(changeID).Get(ctx)

	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var ch domain.LifestyleChange
	if err := doc.DataTo(&ch); err != nil {
		return nil, err
	}

	ch.ID = doc.Ref.ID
	ch.UserID = userID

	return &ch, nil
}

func (r *FirestoreLifestyleChangeRepository) SetPlanID(
	ctx context.Context,
	userID string,
	changeID string,
	planID string,
) error {

	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("change_lifestyle").
		Doc(changeID).
		Update(ctx, []firestore.Update{
			{
				Path:  "plan_id",
				Value: planID,
			},
		})

	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return err
	}

	return nil
}
