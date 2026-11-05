package usecase

import (
	"context"
	"fmt"
	"log"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
	"viv/internal/core/strengthsession"
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
// SessionContentSelector implements usecase.ContentSelectionLayer. It
// dispatches per slot:
//
//  1. ActivityType == Strength AND UseRealStrengthSessions: the real
//     VIV-112 assembly (internal/core/strengthsession.Assemble) — VIV-108's
//     pinned block, hydrated against the real strengthlibrary catalog and
//     VIV_Strength_Block_Config.json's exercise_types/
//     weekly_prescription_by_type/variant_rules. See the commit that added
//     this for why it's a feature flag (default on in dev, off in prod):
//     the OLD path below hydrates against content.ExerciseLibrary's
//     placeholder seed data, which doesn't have real prescriptions for
//     the 113-exercise catalog yet.
//  2. Mesocycle-eligible otherwise (Strength with the flag off, or
//     Functional-if-ever-enabled): the OLD path — fetch the pinned
//     exercise set from VIV-108 (never free-select), hydrate each pinned
//     ID from content.ExerciseLibrary, and pick Standard/Reduced
//     prescription by the slot's LoadTier.
//  3. Everything else — EVERY other activity type (Pilates, Cardio,
//     HIIT, etc.) — untouched, exactly as before: look up one pre-built
//     session keyed by the slot's full resolved attributes.
//
// In every case, a requested non-standard tier the library doesn't
// actually have content for is a hard error — never silently served as
// the standard version. Silently downgrading would misrepresent an
// adjustment the cascade (or the rule engine relaxing a lever) explicitly
// decided should happen.
type SessionContentSelector struct {
	sessions       *content.SessionLibrary
	exercises      *content.ExerciseLibrary
	warmupCooldown *content.MesocycleWarmupCooldownLibrary
	pins           ExercisePinResolver

	strengthLibrary         *strengthlibrary.Library
	strengthConfig          *strengthlibrary.BlockConfig
	strengthSelector        mesocycle.BlockExerciseSelector
	useRealStrengthSessions bool
	now                     func() time.Time
}

// ExercisePinResolver is the subset of *ExercisePinService content
// selection needs — an interface so tests can fake it without a real
// ExercisePinRepository.
type ExercisePinResolver interface {
	ResolvePinnedExercises(ctx context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) ([]mesocycle.ExerciseID, error)
	ResolvePinnedBlock(ctx context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) (mesocycle.PinnedExerciseSet, error)
}

func NewSessionContentSelector(
	sessions *content.SessionLibrary,
	exercises *content.ExerciseLibrary,
	warmupCooldown *content.MesocycleWarmupCooldownLibrary,
	pins ExercisePinResolver,
	strengthLib *strengthlibrary.Library,
	strengthConfig *strengthlibrary.BlockConfig,
	useRealStrengthSessions bool,
) *SessionContentSelector {
	return &SessionContentSelector{
		sessions:                sessions,
		exercises:               exercises,
		warmupCooldown:          warmupCooldown,
		pins:                    pins,
		strengthLibrary:         strengthLib,
		strengthConfig:          strengthConfig,
		strengthSelector:        mesocycle.DefaultBlockExerciseSelector{},
		useRealStrengthSessions: useRealStrengthSessions,
		now:                     func() time.Time { return time.Now().UTC() },
	}
}

// NewSessionContentSelectorWithClock is NewSessionContentSelector with
// an injectable clock, for deterministic week-number tests against the
// real Strength assembly path.
func NewSessionContentSelectorWithClock(
	sessions *content.SessionLibrary,
	exercises *content.ExerciseLibrary,
	warmupCooldown *content.MesocycleWarmupCooldownLibrary,
	pins ExercisePinResolver,
	strengthLib *strengthlibrary.Library,
	strengthConfig *strengthlibrary.BlockConfig,
	useRealStrengthSessions bool,
	now func() time.Time,
) *SessionContentSelector {
	s := NewSessionContentSelector(sessions, exercises, warmupCooldown, pins, strengthLib, strengthConfig, useRealStrengthSessions)
	s.now = now
	return s
}

// SelectedContent is Layer 3's hydrated output for one non-rest day.
// Exactly one of Session, Exercises, or StrengthBlock is ever set,
// depending on which path produced it.
type SelectedContent struct {
	Session       *content.Session
	Exercises     []PrescribedExercise
	StrengthBlock *strengthsession.AssembledSession

	// Warmup/Cooldown are populated for the OLD mesocycle-pinned path
	// (Exercises) AND the real VIV-112 Strength assembly path
	// (StrengthBlock, from AssembledSession.Warmup/Cooldown) — only
	// session-library days carry their own warmup/cooldown nested
	// inside Session.Content instead, so these stay nil only there.
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

		switch {
		case d.Assignment.ActivityType == activity.Strength && s.useRealStrengthSessions:
			selected, err = s.selectStrengthBlock(ctx, draft.UserID, d.Assignment)
		case mesocycle.IsMesocycleEligible(d.Assignment.ActivityType):
			selected, err = s.selectPinnedExercises(ctx, draft.UserID, d.Assignment)
		default:
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

// selectStrengthBlock is the real VIV-112 path: resolve userID's pinned
// block for this slot (creating/rotating it via VIV-108 exactly like
// the OLD path does), then hand it to strengthsession.Assemble —
// VIV-105's tier decisions, VIV-108's pin, and VIV-117's catalog/config
// are the only inputs; this method never selects an exercise itself.
func (s *SessionContentSelector) selectStrengthBlock(ctx context.Context, userID string, a cascade.SlotAssignment) (SelectedContent, error) {
	pin, err := s.pins.ResolvePinnedBlock(ctx, userID, a.ActivityType, a.MuscleGroup)
	if err != nil {
		return SelectedContent{}, fmt.Errorf("resolving pinned block: %w", err)
	}

	assembled, err := strengthsession.Assemble(a, pin, s.now(), s.strengthLibrary, s.strengthConfig, s.strengthSelector)
	if err != nil {
		return SelectedContent{}, fmt.Errorf("assembling strength session: %w", err)
	}

	if assembled.HighIntensityDowngraded {
		log.Printf("[content_selection] High intensity downgraded to Medium for user=%s muscle_group=%s (Strength High isn't reviewed yet)", userID, a.MuscleGroup)
	}

	warmup := content.ContentBlock{DurationMinutes: assembled.Warmup.DurationMinutes, Movements: assembled.Warmup.Movements}
	cooldown := content.ContentBlock{DurationMinutes: assembled.Cooldown.DurationMinutes, Movements: assembled.Cooldown.Movements}

	return SelectedContent{StrengthBlock: &assembled, Warmup: &warmup, Cooldown: &cooldown}, nil
}
