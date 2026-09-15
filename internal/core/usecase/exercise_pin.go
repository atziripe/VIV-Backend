package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
)

// ============================================================================
// EXERCISE PINNING — VIV-108
// ============================================================================
//
// ExercisePinRepository persists mesocycle.PinnedExerciseSet — the "new
// persisted state per user, per muscle-group slot, per mesocycle-eligible
// activity type" this task introduces.
type ExercisePinRepository interface {
	Get(ctx context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) (*mesocycle.PinnedExerciseSet, error)
	Save(ctx context.Context, userID string, pin mesocycle.PinnedExerciseSet) error
}

// ExercisePinService resolves the exercise set pinned for a user's
// (activityType, muscleGroup) slot — creating or rotating it via
// mesocycle.ResolvePin (VIV-101-dependent, pure logic) and persisting only
// when something actually changed.
//
// This applies identically whichever pipeline is asking — weekly
// generation (VIV-106) or daily adaptation (VIV-107) — because neither of
// those ever changes ActivityType or MuscleGroup for a slot: a
// ProtectType/RelaxIntensity/RelaxImpact adjustment only ever changes
// Intensity/Impact/LoadTier, and this service's lookup key never includes
// either of those. See TestExercisePinService_CascadeAdjustmentNeverChangesPinnedExercises.
//
// Deliberately NOT called from GenerateWeeklyPlanUsecase or
// AdaptDailySlotUsecase's Execute bodies by this task: neither
// cascade.SlotAssignment nor usecase.DayPlan has anywhere to attach the
// resolved exercise IDs yet — that's content selection's job (VIV-112).
// This service is the piece VIV-112 (or whichever ticket wires content
// into the plan output) calls; VIV-108 is scoped to the pinning/rotation
// logic and its persistence, not to threading exercise content through
// the plan.
type ExercisePinService struct {
	repo     ExercisePinRepository
	selector mesocycle.ExerciseSetSelector
	library  mesocycle.ExerciseLibrary
	now      func() time.Time
}

func NewExercisePinService(
	repo ExercisePinRepository,
	selector mesocycle.ExerciseSetSelector,
	library mesocycle.ExerciseLibrary,
) *ExercisePinService {
	return &ExercisePinService{
		repo:     repo,
		selector: selector,
		library:  library,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// NewExercisePinServiceWithClock is NewExercisePinService with an
// injectable clock, for deterministic rotation tests.
func NewExercisePinServiceWithClock(
	repo ExercisePinRepository,
	selector mesocycle.ExerciseSetSelector,
	library mesocycle.ExerciseLibrary,
	now func() time.Time,
) *ExercisePinService {
	s := NewExercisePinService(repo, selector, library)
	s.now = now
	return s
}

// ResolvePinnedExercises returns the exercise IDs pinned for userID's
// (activityType, muscleGroup) slot, creating or rotating the pin as
// needed. Returns (nil, nil) when activityType isn't mesocycle-eligible
// (mesocycle.IsMesocycleEligible) — callers should read that as "nothing
// to pin for this slot", not as an error.
func (s *ExercisePinService) ResolvePinnedExercises(
	ctx context.Context,
	userID string,
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
) ([]mesocycle.ExerciseID, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("exercise pin: userID is required")
	}
	if !mesocycle.IsMesocycleEligible(activityType) {
		return nil, nil
	}

	existing, err := s.repo.Get(ctx, userID, activityType, muscleGroup)
	if err != nil {
		return nil, fmt.Errorf("exercise pin: loading existing pin: %w", err)
	}

	pin, changed, err := mesocycle.ResolvePin(existing, activityType, muscleGroup, s.now(), s.selector, s.library)
	if err != nil {
		return nil, fmt.Errorf("exercise pin: resolving pin: %w", err)
	}

	if changed {
		if err := s.repo.Save(ctx, userID, pin); err != nil {
			return nil, fmt.Errorf("exercise pin: saving pin: %w", err)
		}
	}

	return pin.ExerciseIDs, nil
}
