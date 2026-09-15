package content_test

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
)

// Loads the real dummy content shipped under internal/content/training_v2/
// — a sanity check that the files are valid JSON matching the schema and
// that the loader/lookup actually work end-to-end against them, not just
// against in-memory fixtures.
const realSessionsDir = "../../content/training_v2/sessions"

func TestLoadSessionLibrary_LoadsRealDummyContent(t *testing.T) {
	lib, err := content.LoadSessionLibrary(realSessionsDir)
	if err != nil {
		t.Fatalf("LoadSessionLibrary(%s) returned error: %v", realSessionsDir, err)
	}

	standard, ok := lib.Lookup(content.SessionKey{
		ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
		DurationTier: cascade.DurationStandard, ComplexityTier: cascade.ComplexityStandard, LoadTier: cascade.LoadStandard,
	})
	if !ok {
		t.Fatal("expected to find the standard yoga session in the real dummy content")
	}
	if standard.ID != "yoga_fullbody_l_l_standard_v1" {
		t.Errorf("ID = %q, want yoga_fullbody_l_l_standard_v1", standard.ID)
	}
	if len(standard.Content.MainExercises) == 0 {
		t.Error("expected non-empty MainExercises")
	}

	if !lib.HasVariant(activity.Yoga, cascade.VariantShort) {
		t.Error("expected HasVariant(yoga, short) = true — the dummy short yoga session exists")
	}
	if lib.HasVariant(activity.Yoga, cascade.VariantSimplified) {
		t.Error("expected HasVariant(yoga, simplified) = false — no dummy simplified yoga session exists")
	}
	if lib.HasVariant(activity.Running, cascade.VariantShort) {
		t.Error("expected HasVariant(running, short) = false — the dummy running session is standard-only")
	}

	if _, ok := lib.Lookup(content.SessionKey{
		ActivityType: activity.Running, Intensity: activity.IntensityL, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupLower,
		DurationTier: cascade.DurationStandard, ComplexityTier: cascade.ComplexityStandard, LoadTier: cascade.LoadStandard,
	}); !ok {
		t.Error("expected to find the standard running session in the real dummy content")
	}
}

// ============================================================================
// KeyFor
// ============================================================================

func TestKeyFor_NormalizesEmptyTiersToStandard(t *testing.T) {
	key := content.KeyFor(cascade.SlotAssignment{
		ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
	})
	if key.DurationTier != cascade.DurationStandard || key.ComplexityTier != cascade.ComplexityStandard || key.LoadTier != cascade.LoadStandard {
		t.Errorf("KeyFor did not normalize empty tiers to standard: %+v", key)
	}
}

// ============================================================================
// In-memory fixtures — exact-match Lookup and duplicate detection
// ============================================================================

func writeSessionFixture(t *testing.T, dir, filename string, s content.Session) {
	t.Helper()
	writeJSONFixture(t, dir, filename, s)
}

func TestSessionLibrary_LookupRequiresExactTierMatch(t *testing.T) {
	dir := t.TempDir()
	writeSessionFixture(t, dir, "a.json", content.Session{
		ID: "a", ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
		DurationTier: cascade.DurationStandard, ComplexityTier: cascade.ComplexityStandard, LoadTier: cascade.LoadStandard,
	})

	lib, err := content.LoadSessionLibrary(dir)
	if err != nil {
		t.Fatalf("LoadSessionLibrary returned error: %v", err)
	}

	// Same everything except DurationTier=short — must NOT match.
	if _, ok := lib.Lookup(content.SessionKey{
		ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
		DurationTier: cascade.DurationShort, ComplexityTier: cascade.ComplexityStandard, LoadTier: cascade.LoadStandard,
	}); ok {
		t.Error("expected no match when DurationTier differs from the loaded session")
	}
}

func TestSessionLibrary_DuplicateKeyErrors(t *testing.T) {
	dir := t.TempDir()
	dup := content.Session{
		ID: "a", ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
		DurationTier: cascade.DurationStandard, ComplexityTier: cascade.ComplexityStandard, LoadTier: cascade.LoadStandard,
	}
	writeSessionFixture(t, dir, "a.json", dup)
	dup.ID = "b"
	writeSessionFixture(t, dir, "b.json", dup)

	if _, err := content.LoadSessionLibrary(dir); err == nil {
		t.Fatal("expected an error for two sessions claiming the same SessionKey")
	}
}
