package usecase

import (
	"context"

	"viv/internal/core/domain"
)

type ListLifestyleChangesInput struct {
	UserID string
	Limit  int
}

type ListLifestyleChangesOutput struct {
	Items []*domain.LifestyleChange
}

type ListLifestyleChangesUseCase struct {
	LifestyleChanges LifestyleChangeRepository
}

func NewListLifestyleChangesUseCase(repo LifestyleChangeRepository) *ListLifestyleChangesUseCase {
	return &ListLifestyleChangesUseCase{
		LifestyleChanges: repo,
	}
}

func (uc *ListLifestyleChangesUseCase) Execute(ctx context.Context, in ListLifestyleChangesInput) (*ListLifestyleChangesOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("")
	}

	limit := in.Limit
	if limit <= 0 || limit > 50 {
		limit = 10 // valor por defecto razonable
	}

	items, err := uc.LifestyleChanges.ListByUser(ctx, in.UserID, limit)
	if err != nil {
		return nil, err
	}

	return &ListLifestyleChangesOutput{
		Items: items,
	}, nil
}
