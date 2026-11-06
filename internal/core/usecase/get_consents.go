package usecase

import (
	"context"
	"fmt"
	"strings"

	"viv/internal/core/domain"
)

type GetConsentsInput struct {
	UserID string
}

type GetConsentsOutput struct {
	Consents map[domain.ConsentPurpose]domain.Consent
}

type GetConsentsUseCase struct {
	Consents ConsentRepository
}

func NewGetConsentsUseCase(consents ConsentRepository) *GetConsentsUseCase {
	return &GetConsentsUseCase{Consents: consents}
}

func (uc *GetConsentsUseCase) Execute(ctx context.Context, in GetConsentsInput) (GetConsentsOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return GetConsentsOutput{}, fmt.Errorf("get consents: userID is required")
	}

	all, err := uc.Consents.GetAll(ctx, userID)
	if err != nil {
		return GetConsentsOutput{}, fmt.Errorf("get consents: %w", err)
	}
	return GetConsentsOutput{Consents: all}, nil
}
