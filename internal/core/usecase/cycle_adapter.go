package usecase

import (
	"context"
	"fmt"

	"viv/internal/core/domain"
)

// CyclePhaseAdapter implements CyclePhaseLookup by reading
// the phase directly from the User struct (which cycle_sync keeps updated).
type CyclePhaseAdapter struct {
	userRepo UserRepository
}

func NewCyclePhaseAdapter(userRepo UserRepository) *CyclePhaseAdapter {
	return &CyclePhaseAdapter{userRepo: userRepo}
}

func (a *CyclePhaseAdapter) CurrentPhase(ctx context.Context, userID string) (domain.CyclePhase, error) {
	user, err := a.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("loading user: %w", err)
	}
	if user == nil {
		return "", fmt.Errorf("user not found: %s", userID)
	}

	return mapCyclePhase(user.CyclePhase), nil
}

func mapCyclePhase(raw string) domain.CyclePhase {
	switch raw {
	case "menstrual":
		return domain.PhaseMenstrual
	case "early_follicular", "late_follicular", "follicular":
		return domain.PhaseFollicular
	case "ovulation", "ovulatory":
		return domain.PhaseOvulatory
	case "early_luteal":
		return domain.PhaseEarlyLuteal
	case "late_luteal":
		return domain.PhaseLateLuteal
	default:
		return domain.PhaseFollicular
	}
}
