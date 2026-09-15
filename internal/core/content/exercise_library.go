package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/mesocycle"
)

// ============================================================================
// Exercise content — for mesocycle-eligible activity types (Strength,
// and Functional if enabled — mesocycle.IsMesocycleEligible). These are
// individual exercises, hydrated for whichever IDs VIV-108's
// ExercisePinService says are pinned for a slot — never freely selected
// here. See usecase.SessionContentSelector.
// ============================================================================

// ExercisePrescription is how an exercise is actually performed — the
// part that's allowed to vary by LoadTier (VIV-105's ProtectType lever).
// The exercise identity/selection never varies with it.
type ExercisePrescription struct {
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"`
	RestSeconds  int    `json:"rest_seconds"`
	LoadGuidance string `json:"load_guidance"`
	FormCue      string `json:"form_cue"`
}

// Exercise is one exercise's content, on disk as an entry in a
// muscle-group JSON file. Standard/Reduced are the only two
// prescriptions modeled — mirrors cascade.LoadTier's exact two values
// (LoadStandard/LoadReduced), so there's no way for a caller to ask for
// a prescription this type can't represent.
type Exercise struct {
	ID          mesocycle.ExerciseID `json:"id"`
	Name        string               `json:"name"`
	MuscleGroup activity.MuscleGroup `json:"muscle_group"`

	Standard ExercisePrescription `json:"standard"`
	Reduced  ExercisePrescription `json:"reduced"`
}

// ExerciseLibrary is the loaded set of exercise content. Implements
// mesocycle.ExerciseLibrary directly (ExercisesFor), so this can replace
// mesocycle.StubExerciseLibrary wherever that's wired in, in addition to
// serving VIV-112's own Lookup-by-ID hydration.
type ExerciseLibrary struct {
	byID          map[mesocycle.ExerciseID]Exercise
	byMuscleGroup map[activity.MuscleGroup][]mesocycle.ExerciseID
}

var _ mesocycle.ExerciseLibrary = (*ExerciseLibrary)(nil)

// LoadExerciseLibrary reads every *.json file under dir (recursively).
// Each file holds a JSON array of Exercise entries — one file per muscle
// group is the convention the dummy content under
// internal/content/training_v2/exercises/ follows, but this loader
// doesn't require that split; it just collects every entry it finds.
func LoadExerciseLibrary(dir string) (*ExerciseLibrary, error) {
	lib := &ExerciseLibrary{
		byID:          map[mesocycle.ExerciseID]Exercise{},
		byMuscleGroup: map[activity.MuscleGroup][]mesocycle.ExerciseID{},
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

		var exercises []Exercise
		if err := json.Unmarshal(data, &exercises); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		for _, e := range exercises {
			if e.ID == "" {
				return fmt.Errorf("exercise in %s has empty id", path)
			}
			if existing, ok := lib.byID[e.ID]; ok {
				return fmt.Errorf("duplicate exercise id %q in %s (already loaded as %q)", e.ID, path, existing.Name)
			}
			lib.byID[e.ID] = e
			lib.byMuscleGroup[e.MuscleGroup] = append(lib.byMuscleGroup[e.MuscleGroup], e.ID)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading exercise library from %s: %w", dir, err)
	}

	return lib, nil
}

// ExercisesFor implements mesocycle.ExerciseLibrary.
func (l *ExerciseLibrary) ExercisesFor(mg activity.MuscleGroup) []mesocycle.ExerciseID {
	return l.byMuscleGroup[mg]
}

// Lookup finds an exercise's content by ID — VIV-112's hydration step for
// a mesocycle-pinned exercise. ok=false means the pinned ID isn't in the
// library (a hard-error case for the caller — a pin should never
// reference an exercise the library doesn't actually have).
func (l *ExerciseLibrary) Lookup(id mesocycle.ExerciseID) (Exercise, bool) {
	e, ok := l.byID[id]
	return e, ok
}

// PrescriptionFor picks Standard or Reduced from an already-looked-up
// Exercise based on a slot's LoadTier — the only thing allowed to vary
// per the task ("only vary load/reps/sets based on LoadTier, never the
// exercise selection itself"). Errors on any LoadTier value other than
// the two cascade.LoadTier defines, rather than silently defaulting to
// Standard for an unrecognized value.
func PrescriptionFor(e Exercise, loadTier cascade.LoadTier) (ExercisePrescription, error) {
	switch loadTier {
	case cascade.LoadStandard:
		return e.Standard, nil
	case cascade.LoadReduced:
		return e.Reduced, nil
	default:
		return ExercisePrescription{}, fmt.Errorf("content: unknown LoadTier %q for exercise %q", loadTier, e.ID)
	}
}
