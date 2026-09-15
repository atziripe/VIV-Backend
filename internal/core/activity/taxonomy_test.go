package activity_test

import (
	"os"
	"strings"
	"testing"

	"viv/internal/core/activity"
)

func TestEveryTypeHasNonEmptyIntensityAndImpactRange(t *testing.T) {
	for _, ty := range activity.Types {
		if len(ty.IntensityRange) == 0 {
			t.Errorf("%s: IntensityRange is empty", ty.ID)
		}
		if len(ty.ImpactRange) == 0 {
			t.Errorf("%s: ImpactRange is empty", ty.ID)
		}
	}
}

// TestFixedRangesExposeExactlyOneValue is the case the conflict-resolution
// cascade (built later, design doc §8) depends on: a lever that tries to
// "lower intensity" or "lower impact" on a genuinely fixed-range activity
// must be able to detect there's nothing to relax — which only works if a
// fixed range really is a single-element slice, not a range that happens
// to have been narrowed to one value by the seed data.
func TestFixedRangesExposeExactlyOneValue(t *testing.T) {
	fixedIntensity := map[activity.ID]bool{
		activity.HIIT: true,
	}
	fixedImpact := map[activity.ID]bool{
		activity.Pilates:  true,
		activity.Running:  true,
		activity.Cycling:  true,
		activity.Yoga:     true,
		activity.Mobility: true,
		activity.Swimming: true,
	}

	for _, ty := range activity.Types {
		wantFixedIntensity := fixedIntensity[ty.ID]
		gotFixedIntensity := len(ty.IntensityRange) == 1
		if wantFixedIntensity != gotFixedIntensity {
			t.Errorf("%s: IntensityRange fixed=%v (len=%d), want fixed=%v",
				ty.ID, gotFixedIntensity, len(ty.IntensityRange), wantFixedIntensity)
		}

		wantFixedImpact := fixedImpact[ty.ID]
		gotFixedImpact := len(ty.ImpactRange) == 1
		if wantFixedImpact != gotFixedImpact {
			t.Errorf("%s: ImpactRange fixed=%v (len=%d), want fixed=%v",
				ty.ID, gotFixedImpact, len(ty.ImpactRange), wantFixedImpact)
		}
	}
}

func TestUserSelectableMuscleGroupTypesAreFlaggedCorrectly(t *testing.T) {
	userSelectable := map[activity.ID]bool{
		activity.Strength:   true,
		activity.HIIT:       true,
		activity.Functional: true,
	}

	for _, ty := range activity.Types {
		want := activity.MuscleGroupFixed
		if userSelectable[ty.ID] {
			want = activity.MuscleGroupUserSelectable
		}
		if ty.MuscleGroupMode != want {
			t.Errorf("%s: MuscleGroupMode = %q, want %q", ty.ID, ty.MuscleGroupMode, want)
		}
	}
}

// TestTrainingEffectAndRecoveryCostCoverExactlyTheIntensityRange checks
// there are no gaps (an intensity the activity supports with no effect/
// cost defined) and no orphans (an effect/cost entry for an intensity the
// activity doesn't actually expose in IntensityRange).
func TestTrainingEffectAndRecoveryCostCoverExactlyTheIntensityRange(t *testing.T) {
	for _, ty := range activity.Types {
		inRange := make(map[activity.IntensityLevel]bool, len(ty.IntensityRange))
		for _, lvl := range ty.IntensityRange {
			inRange[lvl] = true
		}

		for lvl := range inRange {
			if _, ok := ty.TrainingEffect[lvl]; !ok {
				t.Errorf("%s: intensity %s is in IntensityRange but has no TrainingEffect entry", ty.ID, lvl)
			}
			if len(ty.TrainingEffect[lvl]) == 0 {
				t.Errorf("%s: TrainingEffect[%s] is present but empty", ty.ID, lvl)
			}
			if _, ok := ty.RecoveryCost[lvl]; !ok {
				t.Errorf("%s: intensity %s is in IntensityRange but has no RecoveryCost entry", ty.ID, lvl)
			}
		}

		for lvl := range ty.TrainingEffect {
			if !inRange[lvl] {
				t.Errorf("%s: TrainingEffect has an orphaned entry for %s, which isn't in IntensityRange", ty.ID, lvl)
			}
		}
		for lvl, cost := range ty.RecoveryCost {
			if !inRange[lvl] {
				t.Errorf("%s: RecoveryCost has an orphaned entry for %s, which isn't in IntensityRange", ty.ID, lvl)
			}
			if cost < 0 || cost > 3 {
				t.Errorf("%s: RecoveryCost[%s] = %d, want 0-3", ty.ID, lvl, cost)
			}
		}
	}
}

// TestEverySeededValueIsMarkedFirstDraft is a grep-able check, not just a
// code-review nicety: every TrainingEffect/RecoveryCost entry in the seed
// data must carry the "pending Lina/Juli sign-off" marker so nothing
// downstream treats first-draft literature-review data as final. Counts
// against the actual number of seeded entries rather than a hardcoded
// number, so it stays correct as entries are added.
func TestEverySeededValueIsMarkedFirstDraft(t *testing.T) {
	src, err := os.ReadFile("taxonomy.go")
	if err != nil {
		t.Fatalf("reading taxonomy.go: %v", err)
	}

	const marker = "first-draft, pending Lina/Juli sign-off — see TBD_Stefy_Lina.md"
	occurrences := strings.Count(string(src), marker)

	wantAtLeast := 0
	for _, ty := range activity.Types {
		wantAtLeast += len(ty.TrainingEffect) + len(ty.RecoveryCost)
	}

	if occurrences < wantAtLeast {
		t.Errorf("found %d occurrences of the first-draft marker in taxonomy.go, want at least %d (one per seeded TrainingEffect/RecoveryCost entry: %d types × their entries)",
			occurrences, wantAtLeast, len(activity.Types))
	}
}

func TestByID(t *testing.T) {
	ty, ok := activity.ByID(activity.Strength)
	if !ok {
		t.Fatal("ByID(Strength) not found")
	}
	if ty.ID != activity.Strength {
		t.Errorf("ByID(Strength).ID = %q, want %q", ty.ID, activity.Strength)
	}

	if _, ok := activity.ByID("not_a_real_activity"); ok {
		t.Error("ByID of an unknown ID should return ok=false")
	}
}

func TestAllElevenActivityTypesArePresent(t *testing.T) {
	want := []activity.ID{
		activity.Strength, activity.Pilates, activity.Barre, activity.Running,
		activity.Cycling, activity.HIIT, activity.Functional, activity.Yoga,
		activity.Mobility, activity.Swimming, activity.TeamRacketSports,
	}
	if len(activity.Types) != len(want) {
		t.Fatalf("len(Types) = %d, want %d", len(activity.Types), len(want))
	}
	for _, id := range want {
		if _, ok := activity.ByID(id); !ok {
			t.Errorf("missing activity type %q", id)
		}
	}
}
