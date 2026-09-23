package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// SaveRecoveryActionInput/Output/UseCase persist what the user did with a
// given day's recovery card — the mockup's Done/Not today (Low/Medium
// cost) or Got it/I trained (High cost) buttons.
type SaveRecoveryActionInput struct {
	UserID string
	Date   time.Time
	Kind   domain.RecoveryActionKind
}

type SaveRecoveryActionUseCase struct {
	Actions RecoveryActionRepository
}

func NewSaveRecoveryActionUseCase(actions RecoveryActionRepository) *SaveRecoveryActionUseCase {
	return &SaveRecoveryActionUseCase{Actions: actions}
}

func (uc *SaveRecoveryActionUseCase) Execute(ctx context.Context, in SaveRecoveryActionInput) error {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return fmt.Errorf("save recovery action: userID is required")
	}

	switch in.Kind {
	case domain.RecoveryActionDone, domain.RecoveryActionNotToday,
		domain.RecoveryActionAcknowledged, domain.RecoveryActionTrainedAnyway:
		// valid
	default:
		return fmt.Errorf("save recovery action: unknown kind %q", in.Kind)
	}

	date := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)

	return uc.Actions.Upsert(ctx, &domain.RecoveryAction{
		UserID:   userID,
		Date:     date,
		Kind:     in.Kind,
		LoggedAt: time.Now().UTC(),
	})
}
