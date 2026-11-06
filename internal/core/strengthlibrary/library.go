package strengthlibrary

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// exercisesJSON is embedded, not loaded from a directory path at
// startup — unlike every other content library in this codebase (see
// internal/core/content's doc comments: deliberately no go:embed
// anywhere else here). That means data/exercises.json has to live
// inside this package's own directory tree rather than under
// internal/content/ alongside the rest of the content data — go:embed
// cannot reach outside its source file's directory. Regenerate it with
// internal/scripts/strengthimport, never hand-edit it.
//
//go:embed data/exercises.json
var exercisesJSON []byte

// Library is the loaded, validated Strength movement catalog. Built
// once at boot, immutable, safe for concurrent use.
type Library struct {
	byID   map[string]Exercise
	byZone map[Zone]map[MovementGroup][]Exercise
}

// Load parses and validates the embedded catalog. Fails fast — any
// structural problem (malformed JSON, a duplicate ID, a missing
// zone/movement group) is returned as an error for the caller to treat
// as a boot-time fatal, exactly like every other content library's
// Load/LoadXLibrary function in this codebase.
func Load() (*Library, error) {
	return loadFromJSON(exercisesJSON)
}

// NewLibraryFromJSON builds a Library from arbitrary JSON instead of the
// embedded catalog — for other packages' tests (e.g. mesocycle's block
// pinning tests) that need a small, controlled fixture catalog rather
// than the real 113-exercise one. Production code always uses Load.
func NewLibraryFromJSON(data []byte) (*Library, error) {
	return loadFromJSON(data)
}

func loadFromJSON(data []byte) (*Library, error) {
	var exercises []Exercise
	if err := json.Unmarshal(data, &exercises); err != nil {
		return nil, fmt.Errorf("strengthlibrary: parsing exercises.json: %w", err)
	}

	lib := &Library{
		byID:   make(map[string]Exercise, len(exercises)),
		byZone: make(map[Zone]map[MovementGroup][]Exercise),
	}

	for _, e := range exercises {
		if e.ID == "" {
			return nil, fmt.Errorf("strengthlibrary: exercise %q (zone %q) has an empty id", e.Name, e.Zone)
		}
		if e.Zone == "" {
			return nil, fmt.Errorf("strengthlibrary: exercise %q (id %q) has no zone", e.Name, e.ID)
		}
		if e.MovementGroup == "" {
			return nil, fmt.Errorf("strengthlibrary: exercise %q (id %q) has no movement group", e.Name, e.ID)
		}
		if existing, ok := lib.byID[e.ID]; ok {
			return nil, fmt.Errorf(
				"strengthlibrary: duplicate exercise id %q — %q (group %q) and %q (group %q) both produce it",
				e.ID, existing.Name, existing.MovementGroup, e.Name, e.MovementGroup,
			)
		}

		lib.byID[e.ID] = e
		if lib.byZone[e.Zone] == nil {
			lib.byZone[e.Zone] = make(map[MovementGroup][]Exercise)
		}
		lib.byZone[e.Zone][e.MovementGroup] = append(lib.byZone[e.Zone][e.MovementGroup], e)
	}

	return lib, nil
}

// ExercisesForZone returns every exercise in zone grouped by movement
// group. includePending must come from config.Config.IncludePendingExercises
// — already fail-safed there to development/staging only — this method
// trusts the caller and does not re-check the deployment environment
// itself; nothing in this package knows what environment it's running
// in.
func (l *Library) ExercisesForZone(zone Zone, includePending bool) map[MovementGroup][]Exercise {
	out := make(map[MovementGroup][]Exercise)
	for group, list := range l.byZone[zone] {
		for _, e := range list {
			if !includePending && e.ReviewStatus != ReviewApproved {
				continue
			}
			out[group] = append(out[group], e)
		}
	}
	return out
}

// Lookup finds one exercise by ID, approved or pending — VIV-108's
// block pinning needs this to recover an exercise's own MovementGroup
// (the ActiveExercises map's key) from the ID a selection picked.
func (l *Library) Lookup(id string) (Exercise, bool) {
	e, ok := l.byID[id]
	return e, ok
}

// ApprovedExercises is the only entry point any production selection
// path (VIV-108's future curated exercise-set selector, above all) may
// use — it never returns a Pending exercise, regardless of the
// IncludePendingExercises flag. Use ExercisesForZone directly only for
// the dev/staging "try pending exercises internally" escape hatch.
func (l *Library) ApprovedExercises(zone Zone) map[MovementGroup][]Exercise {
	return l.ExercisesForZone(zone, false)
}
