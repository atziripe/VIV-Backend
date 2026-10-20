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

// Asegura en compilación que cumple la interfaz UserRepository
var _ usecase.UserRepository = (*FirestoreUserRepository)(nil)
var _ usecase.PendingDeletionLister = (*FirestoreUserRepository)(nil)
var _ usecase.InactivityLister = (*FirestoreUserRepository)(nil)

type FirestoreUserRepository struct {
	client *firestore.Client
}

func NewFirestoreUserRepository(client *firestore.Client) *FirestoreUserRepository {
	return &FirestoreUserRepository{client: client}
}

func (r *FirestoreUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	doc, err := r.client.Collection("users").Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var u domain.User
	if err := doc.DataTo(&u); err != nil {
		return nil, err
	}
	u.ID = doc.Ref.ID
	return &u, nil
}

func (r *FirestoreUserRepository) Save(ctx context.Context, user *domain.User) error {
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	user.UpdatedAt = time.Now().UTC()

	_, err := r.client.Collection("users").Doc(user.ID).Set(ctx, user)
	return err
}

// ListPendingDeletions returns every user whose ScheduleAccountDeletionUseCase
// deadline is at or before "before" — see RunPendingAccountDeletionsUseCase's
// doc comment for how this feeds the daily sweep.
func (r *FirestoreUserRepository) ListPendingDeletions(ctx context.Context, before time.Time) ([]*domain.User, error) {
	iter := r.client.Collection("users").
		Where("PendingDeletionAt", "<=", before).
		Documents(ctx)

	var results []*domain.User
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var u domain.User
		if err := doc.DataTo(&u); err != nil {
			return nil, err
		}
		u.ID = doc.Ref.ID
		results = append(results, &u)
	}

	return results, nil
}

// ListInactiveSince returns every user whose LastActiveAt is at or before
// cutoff — see RunInactivitySweepUseCase's doc comment. Filters out users
// who already have a deletion pending in application code rather than a
// second Firestore equality filter, to avoid needing a composite index for
// what's otherwise a single-field inequality query.
func (r *FirestoreUserRepository) ListInactiveSince(ctx context.Context, cutoff time.Time) ([]*domain.User, error) {
	iter := r.client.Collection("users").
		Where("LastActiveAt", "<=", cutoff).
		Documents(ctx)

	var results []*domain.User
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var u domain.User
		if err := doc.DataTo(&u); err != nil {
			return nil, err
		}
		u.ID = doc.Ref.ID

		if u.PendingDeletionAt != nil {
			continue
		}
		results = append(results, &u)
	}

	return results, nil
}
