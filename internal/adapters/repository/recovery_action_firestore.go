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

var _ usecase.RecoveryActionRepository = (*FirestoreRecoveryActionRepository)(nil)

// FirestoreRecoveryActionRepository stores what the user did with a given
// day's recovery card under users/{userID}/recovery_actions/{date}, date
// formatted as "2006-01-02" — same deterministic-doc-ID-by-date
// convention as FirestoreDailyCheckinRepository/FirestoreSessionLogRepository,
// which is what makes Upsert idempotent for free.
type FirestoreRecoveryActionRepository struct {
	client *firestore.Client
}

func NewFirestoreRecoveryActionRepository(client *firestore.Client) *FirestoreRecoveryActionRepository {
	return &FirestoreRecoveryActionRepository{client: client}
}

type recoveryActionDoc struct {
	Kind     string    `firestore:"kind"`
	LoggedAt time.Time `firestore:"logged_at"`
}

func (r *FirestoreRecoveryActionRepository) col(userID string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(userID).Collection("recovery_actions")
}

func (r *FirestoreRecoveryActionRepository) Upsert(ctx context.Context, a *domain.RecoveryAction) error {
	if a == nil {
		return nil
	}

	doc := recoveryActionDoc{
		Kind:     string(a.Kind),
		LoggedAt: a.LoggedAt,
	}

	_, err := r.col(a.UserID).Doc(a.Date.Format("2006-01-02")).Set(ctx, doc)
	return err
}

func (r *FirestoreRecoveryActionRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*domain.RecoveryAction, error) {
	snap, err := r.col(userID).Doc(date.Format("2006-01-02")).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var d recoveryActionDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}

	return &domain.RecoveryAction{
		UserID:   userID,
		Date:     date,
		Kind:     domain.RecoveryActionKind(d.Kind),
		LoggedAt: d.LoggedAt,
	}, nil
}

// ListByUser returns every recovery action the user ever logged, newest
// first — see usecase.RecoveryActionRepository.ListByUser's doc comment.
// Ordering by firestore.DocumentID works because each doc's ID is already
// the ISO date string ("2006-01-02"), which sorts chronologically as text.
func (r *FirestoreRecoveryActionRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.RecoveryAction, error) {
	iter := r.col(userID).
		OrderBy(firestore.DocumentID, firestore.Desc).
		Limit(limit).
		Documents(ctx)

	var results []*domain.RecoveryAction
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var d recoveryActionDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, err
		}
		date, err := time.Parse("2006-01-02", doc.Ref.ID)
		if err != nil {
			return nil, err
		}

		results = append(results, &domain.RecoveryAction{
			UserID:   userID,
			Date:     date,
			Kind:     domain.RecoveryActionKind(d.Kind),
			LoggedAt: d.LoggedAt,
		})
	}
	return results, nil
}
