package content_test

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/content"
)

const realMesocycleWarmupCooldownDir = "../../content/training_v2/mesocycle_warmup_cooldown"

func TestLoadMesocycleWarmupCooldownLibrary_LoadsRealDummyContent(t *testing.T) {
	lib, err := content.LoadMesocycleWarmupCooldownLibrary(realMesocycleWarmupCooldownDir)
	if err != nil {
		t.Fatalf("LoadMesocycleWarmupCooldownLibrary(%s) returned error: %v", realMesocycleWarmupCooldownDir, err)
	}

	for _, mg := range []activity.MuscleGroup{
		activity.MuscleGroupLower, activity.MuscleGroupUpper, activity.MuscleGroupFullBody, activity.MuscleGroupCore,
	} {
		wc, ok := lib.Lookup(mg)
		if !ok {
			t.Errorf("expected a warmup/cooldown pair for muscle group %q", mg)
			continue
		}
		if len(wc.Warmup.Movements) == 0 {
			t.Errorf("%s: expected a non-empty warmup", mg)
		}
		if len(wc.Cooldown.Movements) == 0 {
			t.Errorf("%s: expected a non-empty cooldown", mg)
		}
	}
}

func TestLoadMesocycleWarmupCooldownLibrary_MissingMuscleGroupReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	writeJSONFixture(t, dir, "lower.json", content.MesocycleWarmupCooldown{
		MuscleGroup: activity.MuscleGroupLower,
		Warmup:      content.ContentBlock{Movements: []string{"Leg swings"}},
		Cooldown:    content.ContentBlock{Movements: []string{"Quad stretch"}},
	})

	lib, err := content.LoadMesocycleWarmupCooldownLibrary(dir)
	if err != nil {
		t.Fatalf("LoadMesocycleWarmupCooldownLibrary: %v", err)
	}

	if _, ok := lib.Lookup(activity.MuscleGroupUpper); ok {
		t.Error("expected no entry for a muscle group that was never seeded")
	}
	if _, ok := lib.Lookup(activity.MuscleGroupLower); !ok {
		t.Error("expected to find the seeded lower entry")
	}
}

func TestLoadMesocycleWarmupCooldownLibrary_DuplicateMuscleGroupErrors(t *testing.T) {
	dir := t.TempDir()
	writeJSONFixture(t, dir, "lower_a.json", content.MesocycleWarmupCooldown{MuscleGroup: activity.MuscleGroupLower})
	writeJSONFixture(t, dir, "lower_b.json", content.MesocycleWarmupCooldown{MuscleGroup: activity.MuscleGroupLower})

	_, err := content.LoadMesocycleWarmupCooldownLibrary(dir)
	if err == nil {
		t.Fatal("expected an error for two files claiming the same muscle group")
	}
}
