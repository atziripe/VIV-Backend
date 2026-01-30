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

// Asegura en compilación que cumple la interfaz UserRepository
var _ usecase.UserRepository = (*FirestoreUserRepository)(nil)

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
