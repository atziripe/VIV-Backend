package usecase

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
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
// mesocycle.ResolvePin (VIV-101/106/117-dependent, pure logic) and
// persisting only when something actually changed.
//
// This applies identically whichever pipeline is asking — weekly
// generation (VIV-106) or daily adaptation (VIV-107) — because neither of
// those ever changes ActivityType or MuscleGroup for a slot: a
// ProtectType/RelaxIntensity/RelaxImpact adjustment only ever changes
// Intensity/Impact/LoadTier, and this service's lookup key never includes
// either of those. See TestExercisePinService_CascadeAdjustmentNeverChangesPinnedExercises.
//
// ResolvePinnedExercises's return type is deliberately still the flat
// []mesocycle.ExerciseID it always was — content_selection.go (VIV-112)
// already depends on that exact shape, and this task is scoped to the
// pinning/rotation/selection logic + persistence, not to content
// selection's hydration step. The real selector (mesocycle.
// DefaultBlockExerciseSelector) now returns real strengthlibrary IDs
// instead of the old StubExerciseLibrary's placeholders — those won't
// resolve against content.ExerciseLibrary's current seed data
// (internal/content/training_v2/exercises/*.json, 3 placeholder
// exercises per muscle group) until that content is replaced with real
// prescriptions for the 113-exercise catalog. Known, accepted gap — see
// the commit that wired this in for the full context.
type ExercisePinService struct {
	repo     ExercisePinRepository
	selector mesocycle.BlockExerciseSelector
	library  *strengthlibrary.Library
	config   *strengthlibrary.BlockConfig
	now      func() time.Time
}

func NewExercisePinService(
	repo ExercisePinRepository,
	selector mesocycle.BlockExerciseSelector,
	library *strengthlibrary.Library,
	config *strengthlibrary.BlockConfig,
) *ExercisePinService {
	return &ExercisePinService{
		repo:     repo,
		selector: selector,
		library:  library,
		config:   config,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// NewExercisePinServiceWithClock is NewExercisePinService with an
// injectable clock, for deterministic rotation tests.
func NewExercisePinServiceWithClock(
	repo ExercisePinRepository,
	selector mesocycle.BlockExerciseSelector,
	library *strengthlibrary.Library,
	config *strengthlibrary.BlockConfig,
	now func() time.Time,
) *ExercisePinService {
	s := NewExercisePinService(repo, selector, library, config)
	s.now = now
	return s
}

// ResolvePinnedExercises returns the exercise IDs pinned for userID's
// (activityType, muscleGroup) slot, creating or rotating the pin as
// needed. Returns (nil, nil) when activityType isn't mesocycle-eligible
// (mesocycle.IsMesocycleEligible) — callers should read that as "nothing
// to pin for this slot", not as an error. The returned slice is sorted
// by MovementGroup name for determinism — ActiveExercises is a map, with
// no inherent order of its own.
func (s *ExercisePinService) ResolvePinnedExercises(
	ctx context.Context,
	userID string,
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
) ([]mesocycle.ExerciseID, error) {
	if !mesocycle.IsMesocycleEligible(activityType) {
		return nil, nil
	}
	pin, err := s.ResolvePinnedBlock(ctx, userID, activityType, muscleGroup)
	if err != nil {
		return nil, err
	}

	groups := make([]string, 0, len(pin.ActiveExercises))
	for group := range pin.ActiveExercises {
		groups = append(groups, string(group))
	}
	sort.Strings(groups)

	ids := make([]mesocycle.ExerciseID, len(groups))
	for i, group := range groups {
		ids[i] = pin.ActiveExercises[strengthlibrary.MovementGroup(group)]
	}
	return ids, nil
}

// ResolvePinnedBlock is ResolvePinnedExercises's richer sibling — VIV-112
// (Strength session assembly) needs the full pin, not just the flat ID
// list: BlockNumber and ActivatedOn are what let it compute which of the
// 4 weeks the user is in and re-derive the per-slot exercise map (via
// mesocycle.DefaultBlockExerciseSelector.SelectBlockExercises(zone,
// pin.BlockNumber, ...), a pure function of zone+block number — cheaper
// and simpler than reverse-mapping ActiveExercises' MovementGroup keys
// back to slot names). Unlike ResolvePinnedExercises, this errors for a
// non-eligible activityType rather than returning a zero value — every
// caller of this method already knows it's asking about Strength.
func (s *ExercisePinService) ResolvePinnedBlock(
	ctx context.Context,
	userID string,
	activityType activity.ID,
	muscleGroup activity.MuscleGroup,
) (mesocycle.PinnedExerciseSet, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return mesocycle.PinnedExerciseSet{}, fmt.Errorf("exercise pin: userID is required")
	}

	existing, err := s.repo.Get(ctx, userID, activityType, muscleGroup)
	if err != nil {
		return mesocycle.PinnedExerciseSet{}, fmt.Errorf("exercise pin: loading existing pin: %w", err)
	}

	pin, changed, thinSlots, err := mesocycle.ResolvePin(existing, activityType, muscleGroup, s.now(), s.selector, s.library, s.config)
	if err != nil {
		return mesocycle.PinnedExerciseSet{}, fmt.Errorf("exercise pin: resolving pin: %w", err)
	}

	if changed {
		if err := s.repo.Save(ctx, userID, pin); err != nil {
			return mesocycle.PinnedExerciseSet{}, fmt.Errorf("exercise pin: saving pin: %w", err)
		}
		for _, slotName := range thinSlots {
			log.Printf("[exercise_pin] thin slot: zone slot %q for %s/%s has only 1 approved exercise — needs more library variety", slotName, activityType, muscleGroup)
		}
	}

	return pin, nil
}
