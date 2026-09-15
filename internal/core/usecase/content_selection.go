package usecase

import (
	"context"
	"fmt"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
	"viv/internal/core/mesocycle"
)

// ============================================================================
// CONTENT SELECTION — VIV-112, design doc's "Layer 3"
// ============================================================================
//
// Explored VIV-105 (cascade.SlotAssignment's tier fields, ContentLibrary/
// DefaultContentLibrary), VIV-108 (mesocycle pinning) and the existing
// content library (internal/core/training/library.go, a filesystem walk —
// no go:embed anywhere in this codebase) before writing this.
//
// SessionContentSelector implements usecase.ContentSelectionLayer. It has
// exactly two jobs, dispatched per slot by mesocycle.IsMesocycleEligible:
//
//  1. Mesocycle-eligible (Strength, Functional-if-enabled): fetch the
//     pinned exercise set from VIV-108 (never free-select), hydrate each
//     pinned ID from the exercise library, and pick Standard/Reduced
//     prescription by the slot's LoadTier — the exercise SET never
//     changes, only sets/reps/load.
//  2. Everything else: look up one pre-built session keyed by the slot's
//     full resolved attributes (ActivityType/Intensity/Impact/MuscleGroup
//     plus DurationTier/ComplexityTier/LoadTier).
//
// In both cases, a requested non-standard tier that the library doesn't
// actually have content for is a hard error — never silently served as
// the standard version. Silently downgrading would misrepresent an
// adjustment the cascade (or the rule engine relaxing a lever) explicitly
// decided should happen.
type SessionContentSelector struct {
	sessions       *content.SessionLibrary
	exercises      *content.ExerciseLibrary
	warmupCooldown *content.MesocycleWarmupCooldownLibrary
	pins           ExercisePinResolver
}

// ExercisePinResolver is the subset of *ExercisePinService content
// selection needs — an interface so tests can fake it without a real
// ExercisePinRepository.
type ExercisePinResolver interface {
	ResolvePinnedExercises(ctx context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) ([]mesocycle.ExerciseID, error)
}

func NewSessionContentSelector(
	sessions *content.SessionLibrary,
	exercises *content.ExerciseLibrary,
	warmupCooldown *content.MesocycleWarmupCooldownLibrary,
	pins ExercisePinResolver,
) *SessionContentSelector {
	return &SessionContentSelector{sessions: sessions, exercises: exercises, warmupCooldown: warmupCooldown, pins: pins}
}

// SelectedContent is Layer 3's hydrated output for one non-rest day.
// Exactly one of Session or Exercises is ever set, depending on which
// path (mesocycle-pinned vs library-selected) produced it.
type SelectedContent struct {
	Session   *content.Session
	Exercises []PrescribedExercise

	// Warmup/Cooldown are populated for the mesocycle-pinned path
	// (Exercises) only — session-library days carry their own
	// warmup/cooldown nested inside Session.Content instead, so these
	// stay nil there rather than duplicating it.
	Warmup   *content.ContentBlock
	Cooldown *content.ContentBlock
}

// PrescribedExercise pairs a hydrated exercise with the specific
// prescription (Standard or Reduced) selected for it by this slot's
// LoadTier.
type PrescribedExercise struct {
	Exercise     content.Exercise
	Prescription content.ExercisePrescription
}

// SelectContent implements usecase.ContentSelectionLayer.
func (s *SessionContentSelector) SelectContent(ctx context.Context, draft WeekDraft) (WeekDraft, error) {
	for i, d := range draft.Days {
		if d.IsRestDay {
			continue
		}

		var selected SelectedContent
		var err error

		if mesocycle.IsMesocycleEligible(d.Assignment.ActivityType) {
			selected, err = s.selectPinnedExercises(ctx, draft.UserID, d.Assignment)
		} else {
			selected, err = s.selectSession(d.Assignment)
		}
		if err != nil {
			return WeekDraft{}, fmt.Errorf("content selection: %s (%s): %w", d.Weekday, d.Assignment.ActivityType, err)
		}

		draft.Days[i].Content = &selected
	}

	return draft, nil
}

// selectSession looks up one pre-built session by the slot's full
// resolved key. A requested non-standard tier (Duration=Short,
// Complexity=Simplified, or Load=Reduced) that isn't in the library for
// this exact slot fails loudly rather than silently falling back to the
// standard content — that would misrepresent an adjustment that was
// supposed to happen.
func (s *SessionContentSelector) selectSession(a cascade.SlotAssignment) (SelectedContent, error) {
	key := content.KeyFor(a)

	session, ok := s.sessions.Lookup(key)
	if ok {
		return SelectedContent{Session: &session}, nil
	}

	if key.DurationTier != cascade.DurationStandard || key.ComplexityTier != cascade.ComplexityStandard || key.LoadTier != cascade.LoadStandard {
		return SelectedContent{}, fmt.Errorf(
			"requested variant (duration=%s complexity=%s load=%s) doesn't exist in the library for %s/%s/%s/%s — refusing to silently serve the standard version instead",
			key.DurationTier, key.ComplexityTier, key.LoadTier, key.ActivityType, key.Intensity, key.Impact, key.MuscleGroup,
		)
	}
	return SelectedContent{}, fmt.Errorf(
		"no session content found for %s/%s/%s/%s", key.ActivityType, key.Intensity, key.Impact, key.MuscleGroup,
	)
}

// selectPinnedExercises resolves the mesocycle-pinned exercise set for
// this slot's (activityType, muscleGroup) — never free-selecting from
// the library — and hydrates each pinned ID with the prescription that
// matches the slot's LoadTier.
func (s *SessionContentSelector) selectPinnedExercises(ctx context.Context, userID string, a cascade.SlotAssignment) (SelectedContent, error) {
	if a.DurationTier != "" && a.DurationTier != cascade.DurationStandard {
		return SelectedContent{}, fmt.Errorf("duration variants aren't modeled for mesocycle-pinned content yet (got %s)", a.DurationTier)
	}
	if a.ComplexityTier != "" && a.ComplexityTier != cascade.ComplexityStandard {
		return SelectedContent{}, fmt.Errorf("complexity variants aren't modeled for mesocycle-pinned content yet (got %s)", a.ComplexityTier)
	}

	pinnedIDs, err := s.pins.ResolvePinnedExercises(ctx, userID, a.ActivityType, a.MuscleGroup)
	if err != nil {
		return SelectedContent{}, fmt.Errorf("resolving pinned exercise set: %w", err)
	}
	if len(pinnedIDs) == 0 {
		return SelectedContent{}, fmt.Errorf("no pinned exercise set for %s/%s", a.ActivityType, a.MuscleGroup)
	}

	loadTier := a.LoadTier
	if loadTier == "" {
		loadTier = cascade.LoadStandard
	}

	prescribed := make([]PrescribedExercise, 0, len(pinnedIDs))
	for _, id := range pinnedIDs {
		exercise, ok := s.exercises.Lookup(id)
		if !ok {
			return SelectedContent{}, fmt.Errorf("pinned exercise %q not found in the exercise library", id)
		}
		prescription, err := content.PrescriptionFor(exercise, loadTier)
		if err != nil {
			return SelectedContent{}, err
		}
		prescribed = append(prescribed, PrescribedExercise{Exercise: exercise, Prescription: prescription})
	}

	wc, ok := s.warmupCooldown.Lookup(a.MuscleGroup)
	if !ok {
		return SelectedContent{}, fmt.Errorf("no mesocycle warmup/cooldown found for muscle group %q", a.MuscleGroup)
	}
	warmup, cooldown := wc.Warmup, wc.Cooldown

	return SelectedContent{Exercises: prescribed, Warmup: &warmup, Cooldown: &cooldown}, nil
}
