package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"viv/internal/core/activity"
)

// ============================================================================
// Mesocycle warmup/cooldown — generic bracketing content for
// mesocycle-pinned days (Strength, and Functional if ever enabled; see
// mesocycle.IsMesocycleEligible). Unlike session-library content, a
// pinned exercise set (VIV-108) carries no warmup/cooldown of its own —
// neither the cascade nor the pinning logic model which specific moves
// bracket a session, only what the working sets are. This is generic per
// muscle group, not per exercise: real per-exercise-aware warmup/cooldown
// selection would be a separate, richer feature to build if ever needed.
// ============================================================================

// MesocycleWarmupCooldown is one muscle group's generic warmup/cooldown
// pair, on disk as one JSON file.
type MesocycleWarmupCooldown struct {
	MuscleGroup activity.MuscleGroup `json:"muscle_group"`
	Warmup      ContentBlock         `json:"warmup"`
	Cooldown    ContentBlock         `json:"cooldown"`
}

// MesocycleWarmupCooldownLibrary is the loaded set, indexed by muscle
// group. Built once at boot, immutable, safe for concurrent use — same
// convention as SessionLibrary/ExerciseLibrary.
type MesocycleWarmupCooldownLibrary struct {
	byMuscleGroup map[activity.MuscleGroup]MesocycleWarmupCooldown
}

// LoadMesocycleWarmupCooldownLibrary reads every *.json file under dir
// (recursively) — same filesystem-walk convention as
// LoadSessionLibrary/LoadExerciseLibrary.
func LoadMesocycleWarmupCooldownLibrary(dir string) (*MesocycleWarmupCooldownLibrary, error) {
	lib := &MesocycleWarmupCooldownLibrary{byMuscleGroup: map[activity.MuscleGroup]MesocycleWarmupCooldown{}}

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

		var w MesocycleWarmupCooldown
		if err := json.Unmarshal(data, &w); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		if w.MuscleGroup == "" {
			return fmt.Errorf("mesocycle warmup/cooldown in %s has empty muscle_group", path)
		}
		if existing, ok := lib.byMuscleGroup[w.MuscleGroup]; ok {
			return fmt.Errorf("duplicate mesocycle warmup/cooldown for muscle group %q: %s and %s both claim it", w.MuscleGroup, path, existing.MuscleGroup)
		}
		lib.byMuscleGroup[w.MuscleGroup] = w
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading mesocycle warmup/cooldown library from %s: %w", dir, err)
	}

	return lib, nil
}

// Lookup finds the warmup/cooldown pair for a muscle group. ok=false
// means no content exists for it — the caller decides whether that's a
// hard error (see usecase.SessionContentSelector, which treats it as one
// rather than silently serving an empty warmup/cooldown).
func (l *MesocycleWarmupCooldownLibrary) Lookup(mg activity.MuscleGroup) (MesocycleWarmupCooldown, bool) {
	w, ok := l.byMuscleGroup[mg]
	return w, ok
}
