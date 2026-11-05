package strengthsession

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/strengthlibrary"
)

func TestContentLibraryAdapter_StrengthShortTrueWhenEnabled(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Enabled: true}
	if !a.HasVariant(activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(Strength, Short) = true when enabled")
	}
}

func TestContentLibraryAdapter_StrengthSimplifiedReadsConfig(t *testing.T) {
	cfg := fixtureConfig(t) // fixture has simplified.active = false
	a := ContentLibraryAdapter{Config: cfg, Enabled: true}
	if a.HasVariant(activity.Strength, cascade.VariantSimplified) {
		t.Error("expected HasVariant(Strength, Simplified) = false — the fixture config's simplified.active is false")
	}

	activeCfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"X": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": true},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		}
	}`))
	if err != nil {
		t.Fatalf("activeCfg: %v", err)
	}
	b := ContentLibraryAdapter{Config: activeCfg, Enabled: true}
	if !b.HasVariant(activity.Strength, cascade.VariantSimplified) {
		t.Error("expected HasVariant(Strength, Simplified) = true once the config says simplified.active = true — read from config, not hardcoded")
	}
}

func TestContentLibraryAdapter_NonStrengthAlwaysFalse(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Enabled: true}
	for _, id := range []activity.ID{activity.Pilates, activity.Yoga, activity.HIIT, activity.Running} {
		if a.HasVariant(id, cascade.VariantShort) {
			t.Errorf("%s: expected HasVariant(..., Short) = false — every non-Strength activity must return exactly what it returned before (always false)", id)
		}
		if a.HasVariant(id, cascade.VariantSimplified) {
			t.Errorf("%s: expected HasVariant(..., Simplified) = false", id)
		}
	}
}

func TestContentLibraryAdapter_DisabledBehavesLikeTheOldStubForStrengthToo(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Enabled: false}
	if a.HasVariant(activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(Strength, Short) = false when the feature flag is disabled — must match the old stub's always-false behavior")
	}
	if a.HasVariant(activity.Strength, cascade.VariantSimplified) {
		t.Error("expected HasVariant(Strength, Simplified) = false when disabled")
	}
}

// TestCascade_SimplifyComplexityLeverIsSkippedForStrength wires this
// adapter in as the REAL cascade.DefaultContentLibrary (exactly how
// main.go does it) and confirms the exact condition cascade/resolve.go's
// SimplifyComplexity case gates on (`if
// !DefaultContentLibrary.HasVariant(id, VariantSimplified) { continue }`)
// evaluates to "skip" for Strength — "Simplified falls through (lever
// skipped)." A fuller end-to-end run through ResolveSlotConflict would
// additionally need a goal/weeklytarget scenario engineered to reach
// SimplifyComplexity in the lever order, which is VIV-105's own
// well-tested territory, not this package's.
func TestCascade_SimplifyComplexityLeverIsSkippedForStrength(t *testing.T) {
	cfg := fixtureConfig(t)
	orig := cascade.DefaultContentLibrary
	defer func() { cascade.DefaultContentLibrary = orig }()
	cascade.DefaultContentLibrary = ContentLibraryAdapter{Config: cfg, Enabled: true}

	if cascade.DefaultContentLibrary.HasVariant(activity.Strength, cascade.VariantSimplified) {
		t.Fatal("expected the SimplifyComplexity lever's own skip condition to hold true for Strength — it does not yet have a reviewed Simplified variant")
	}
}
