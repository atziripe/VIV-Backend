// Package content is VIV-112, design doc's "Layer 3": it hydrates an
// already-resolved cascade.SlotAssignment (VIV-105 output) with real
// session content, loaded at boot from JSON files on disk. Depends on
// VIV-105 (internal/core/cascade, for the tier enums a slot may request)
// and VIV-108 (internal/core/mesocycle, for the exercise-pinning
// contract) — explored both, the existing content library
// (internal/core/training/library.go — filesystem walk + os.ReadFile,
// NOT go:embed, despite the doc-comment path suggesting one) and its real
// JSON files under internal/content/training/ before writing this.
//
// That existing library is keyed to the OLD taxonomy (domain.Modality /
// domain.Intensity, no separate Impact dimension) and lives under
// internal/content/training/ — this package is deliberately its own,
// keyed to VIV-101's activity.ID/IntensityLevel/ImpactLevel/MuscleGroup
// plus VIV-105's tier enums, loading from a SEPARATE directory
// (internal/content/training_v2/) so the two content sets never collide
// or get confused for one another.
package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
)

// ============================================================================
// Session content — for activity types that are NOT mesocycle-eligible
// (see internal/core/mesocycle.IsMesocycleEligible). Strength/Functional
// use ExerciseLibrary + the mesocycle-pinned set instead — see
// usecase.SessionContentSelector for how the two are dispatched.
// ============================================================================

// ContentBlock and ExerciseDetail mirror the old system's
// domain.ContentBlock/domain.ExerciseDetail shape (same field names/JSON
// tags) — no reason to invent a different warmup/cooldown/exercise
// representation for the new taxonomy when the old one already works.
type ContentBlock struct {
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
	Movements       []string `json:"movements"`
}

type ExerciseDetail struct {
	Name         string `json:"name"`
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"`
	RestSeconds  int    `json:"rest_seconds"`
	LoadGuidance string `json:"load_guidance"`
	FormCue      string `json:"form_cue"`
}

type SessionContent struct {
	Warmup        ContentBlock     `json:"warmup"`
	MainExercises []ExerciseDetail `json:"main_exercises"`
	Cooldown      ContentBlock     `json:"cooldown"`
}

// Session is one pre-built, fully-hydrated session — the content library
// entry, on disk as one JSON file. SessionKey's fields are exactly the
// attributes VIV-112 filters by: the full set of resolved attributes on a
// SlotAssignment, per the task.
type Session struct {
	ID            string `json:"id"`
	Version       int    `json:"version"`
	ContentStatus string `json:"content_status"`

	ActivityType   activity.ID             `json:"activity_type"`
	Intensity      activity.IntensityLevel `json:"intensity"`
	Impact         activity.ImpactLevel    `json:"impact"`
	MuscleGroup    activity.MuscleGroup    `json:"muscle_group"`
	DurationTier   cascade.DurationTier    `json:"duration_tier"`
	ComplexityTier cascade.ComplexityTier  `json:"complexity_tier"`
	LoadTier       cascade.LoadTier        `json:"load_tier"`

	DurationMinutes int               `json:"duration_minutes"`
	Content         SessionContent    `json:"content"`
	PhaseNotes      map[string]string `json:"phase_notes"`

	Author    *string `json:"author"`
	CreatedAt string  `json:"created_at"`
}

// SessionKey identifies WHICH session content to select — the full set
// of resolved attributes on a SlotAssignment that determine content,
// never the content itself.
type SessionKey struct {
	ActivityType   activity.ID
	Intensity      activity.IntensityLevel
	Impact         activity.ImpactLevel
	MuscleGroup    activity.MuscleGroup
	DurationTier   cascade.DurationTier
	ComplexityTier cascade.ComplexityTier
	LoadTier       cascade.LoadTier
}

// KeyFor builds the lookup key for an already-resolved slot. Zero-value
// DurationTier/ComplexityTier/LoadTier on the assignment are normalized
// to "standard" (VIV-105's baseAssignment always sets these explicitly
// today, but normalizing here means a caller building a SlotAssignment by
// hand — e.g. a test — doesn't have to remember to).
func KeyFor(a cascade.SlotAssignment) SessionKey {
	k := SessionKey{
		ActivityType:   a.ActivityType,
		Intensity:      a.Intensity,
		Impact:         a.Impact,
		MuscleGroup:    a.MuscleGroup,
		DurationTier:   a.DurationTier,
		ComplexityTier: a.ComplexityTier,
		LoadTier:       a.LoadTier,
	}
	if k.DurationTier == "" {
		k.DurationTier = cascade.DurationStandard
	}
	if k.ComplexityTier == "" {
		k.ComplexityTier = cascade.ComplexityStandard
	}
	if k.LoadTier == "" {
		k.LoadTier = cascade.LoadStandard
	}
	return k
}

func keyOf(s Session) SessionKey {
	return SessionKey{
		ActivityType: s.ActivityType, Intensity: s.Intensity, Impact: s.Impact, MuscleGroup: s.MuscleGroup,
		DurationTier: s.DurationTier, ComplexityTier: s.ComplexityTier, LoadTier: s.LoadTier,
	}
}

// SessionLibrary is the loaded set of session content, indexed by
// SessionKey for O(1) lookup. Built once at boot, immutable, safe for
// concurrent use — same convention as training.Library.
type SessionLibrary struct {
	byKey      map[SessionKey]Session
	byActivity map[activity.ID][]Session // for HasVariant's coarser existence check
}

var _ cascade.ContentLibrary = (*SessionLibrary)(nil)

// LoadSessionLibrary reads every *.json file under dir (recursively) into
// a SessionLibrary. Mirrors training.LoadLibrary's loading convention
// (filesystem walk, not go:embed — there is no go:embed anywhere in this
// codebase to be consistent with).
func LoadSessionLibrary(dir string) (*SessionLibrary, error) {
	lib := &SessionLibrary{
		byKey:      map[SessionKey]Session{},
		byActivity: map[activity.ID][]Session{},
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if s.ID == "" {
			return fmt.Errorf("session in %s has empty id", path)
		}

		key := keyOf(s)
		if existing, ok := lib.byKey[key]; ok {
			return fmt.Errorf("duplicate session content for %+v: %s and %s both claim it", key, existing.ID, s.ID)
		}
		lib.byKey[key] = s
		lib.byActivity[s.ActivityType] = append(lib.byActivity[s.ActivityType], s)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading session library from %s: %w", dir, err)
	}

	return lib, nil
}

// Lookup finds the session content for an exact SessionKey match — every
// resolved attribute must match, including tiers. ok=false means no
// content exists for this exact combination (the caller decides whether
// that's a hard error — see usecase.SessionContentSelector).
func (l *SessionLibrary) Lookup(key SessionKey) (Session, bool) {
	s, ok := l.byKey[key]
	return s, ok
}

// HasVariant implements cascade.ContentLibrary: does ANY session for this
// activity type carry the given variant, regardless of the specific
// intensity/impact/muscle-group it's for? This intentionally matches
// ResolveSlotConflict's coarser question ("is ReduceDuration/
// SimplifyComplexity even worth trying for this activity type") —
// Lookup's exact-key match is the finer-grained question VIV-112 itself
// asks once a specific slot needs hydrating.
func (l *SessionLibrary) HasVariant(id activity.ID, v cascade.Variant) bool {
	for _, s := range l.byActivity[id] {
		switch v {
		case cascade.VariantShort:
			if s.DurationTier == cascade.DurationShort {
				return true
			}
		case cascade.VariantSimplified:
			if s.ComplexityTier == cascade.ComplexitySimplified {
				return true
			}
		}
	}
	return false
}
