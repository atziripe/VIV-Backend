package usecase

import (
	"context"

	"viv/internal/core/domain"
)

type GetLatestCheckinInput struct {
	UserID string
}

type GetLatestCheckinOutput struct {
	Checkin *domain.Checkin
}

type GetLatestCheckinUseCase struct {
	Checkins CheckinRepository
}

func NewGetLatestCheckinUseCase(checkins CheckinRepository) *GetLatestCheckinUseCase {
	return &GetLatestCheckinUseCase{
		Checkins: checkins,
	}
}

func (uc *GetLatestCheckinUseCase) Execute(ctx context.Context, in GetLatestCheckinInput) (*GetLatestCheckinOutput, error) {
	if in.UserID == "" {
		return nil, ErrUserNotFound("")
	}

	ch, err := uc.Checkins.GetLatestByUser(ctx, in.UserID)
	if err != nil {
		return nil, err
	}

	return &GetLatestCheckinOutput{
		Checkin: ch,
	}, nil
}
