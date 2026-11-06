package strengthsession

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/strengthlibrary"
)

// fakeRollout is a minimal UserRollout for tests that don't care about
// userID (true/false for everyone) — fakeAllowlistRollout below covers
// the per-user cases.
type fakeRollout bool

func (f fakeRollout) Enabled(string) bool { return bool(f) }

// fakeAllowlistRollout mirrors usecase.StrengthSessionsRollout's
// allowlist shape, without importing usecase (which would cycle back
// to this package).
type fakeAllowlistRollout map[string]bool

func (f fakeAllowlistRollout) Enabled(userID string) bool { return f[userID] }

func TestContentLibraryAdapter_StrengthShortTrueWhenEnabled(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Rollout: fakeRollout(true)}
	if !a.HasVariant("u1", activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(Strength, Short) = true when enabled")
	}
}

func TestContentLibraryAdapter_StrengthSimplifiedReadsConfig(t *testing.T) {
	cfg := fixtureConfig(t) // fixture has simplified.active = false
	a := ContentLibraryAdapter{Config: cfg, Rollout: fakeRollout(true)}
	if a.HasVariant("u1", activity.Strength, cascade.VariantSimplified) {
		t.Error("expected HasVariant(Strength, Simplified) = false — the fixture config's simplified.active is false")
	}

	activeCfg, err := strengthlibrary.NewBlockConfigFromJSON([]byte(`{
		"status": "x",
		"zones": {"Lower body": [{"slot": "Squat", "priority": "Main", "source_groups": ["Squat"], "pool_order": ["X"]}]},
		"rotation_policy": {"Main": "hold", "Accessory": 1, "Optional": 1},
		"exercise_types": {"X": {"type": "C", "per_side": false}},
		"weekly_prescription_by_type": {"C": ["w1", "w2", "w3", "w4"]},
		"variant_rules": {
			"short": {"drop_order": ["Optional"], "never_drop": ["Main"], "min_exercises": 1},
			"reduced_load": {"rule": "use_week_1_prescription_of_each_exercise_type"},
			"simplified": {"active": true},
			"intensity_mapping": {"Low": {"extra_rir": 2, "sets_delta": -1, "min_sets": 2}, "Medium": "as_written", "High": "treated_as_Medium_until_reviewed"}
		},
		"warmup_cooldown": {
			"_status": "test_fixture",
			"applies_to_all_tiers": true,
			"ramp_up_note": "Before the first Main exercise, do 1-2 light sets.",
			"zones": {
				"Lower body": {"warmup_minutes": 5, "cooldown_minutes": 5, "warmup": ["Leg swings"], "cooldown": ["Quad stretch"]}
			}
		},
		"rest_seconds_by_type": {
			"_status": "test_fixture",
			"C": [90, 90, 120, 120]
		}
	}`))
	if err != nil {
		t.Fatalf("activeCfg: %v", err)
	}
	b := ContentLibraryAdapter{Config: activeCfg, Rollout: fakeRollout(true)}
	if !b.HasVariant("u1", activity.Strength, cascade.VariantSimplified) {
		t.Error("expected HasVariant(Strength, Simplified) = true once the config says simplified.active = true — read from config, not hardcoded")
	}
}

func TestContentLibraryAdapter_NonStrengthAlwaysFalse(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Rollout: fakeRollout(true)}
	for _, id := range []activity.ID{activity.Pilates, activity.Yoga, activity.HIIT, activity.Running} {
		if a.HasVariant("u1", id, cascade.VariantShort) {
			t.Errorf("%s: expected HasVariant(..., Short) = false — every non-Strength activity must return exactly what it returned before (always false)", id)
		}
		if a.HasVariant("u1", id, cascade.VariantSimplified) {
			t.Errorf("%s: expected HasVariant(..., Simplified) = false", id)
		}
	}
}

func TestContentLibraryAdapter_DisabledBehavesLikeTheOldStubForStrengthToo(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Rollout: fakeRollout(false)}
	if a.HasVariant("u1", activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(Strength, Short) = false when the rollout is disabled — must match the old stub's always-false behavior")
	}
	if a.HasVariant("u1", activity.Strength, cascade.VariantSimplified) {
		t.Error("expected HasVariant(Strength, Simplified) = false when disabled")
	}
}

// TestContentLibraryAdapter_PerUserRollout_OnlyAllowlistedUserGetsVariant
// is the root-cause fix's own test: during a partial rollout, HasVariant
// must say yes for an allowlisted user and no for everyone else, for the
// SAME Config/zone/slot — proving the per-user decision really reaches
// this adapter (via cascade.UserCatalog.UserID -> ResolveSlotConflict ->
// HasVariant's userID parameter), not just the old all-or-nothing flag.
func TestContentLibraryAdapter_PerUserRollout_OnlyAllowlistedUserGetsVariant(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg, Rollout: fakeAllowlistRollout{"allowed_user": true}}

	if !a.HasVariant("allowed_user", activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(Strength, Short) = true for the allowlisted user")
	}
	if a.HasVariant("someone_else", activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(Strength, Short) = false for a user not on the allowlist")
	}
}

func TestContentLibraryAdapter_NilRolloutAlwaysFalse(t *testing.T) {
	cfg := fixtureConfig(t)
	a := ContentLibraryAdapter{Config: cfg}
	if a.HasVariant("u1", activity.Strength, cascade.VariantShort) {
		t.Error("expected HasVariant(...) = false when Rollout is nil — never panic, never default to enabled")
	}
}

// TestCascade_SimplifyComplexityLeverIsSkippedForStrength wires this
// adapter in as the REAL cascade.DefaultContentLibrary (exactly how
// main.go does it) and confirms the exact condition cascade/resolve.go's
// SimplifyComplexity case gates on (`if
// !DefaultContentLibrary.HasVariant(userID, id, VariantSimplified) {
// continue }`) evaluates to "skip" for Strength — "Simplified falls
// through (lever skipped)." A fuller end-to-end run through
// ResolveSlotConflict would additionally need a goal/weeklytarget
// scenario engineered to reach SimplifyComplexity in the lever order,
// which is VIV-105's own well-tested territory, not this package's.
func TestCascade_SimplifyComplexityLeverIsSkippedForStrength(t *testing.T) {
	cfg := fixtureConfig(t)
	orig := cascade.DefaultContentLibrary
	defer func() { cascade.DefaultContentLibrary = orig }()
	cascade.DefaultContentLibrary = ContentLibraryAdapter{Config: cfg, Rollout: fakeRollout(true)}

	if cascade.DefaultContentLibrary.HasVariant("u1", activity.Strength, cascade.VariantSimplified) {
		t.Fatal("expected the SimplifyComplexity lever's own skip condition to hold true for Strength — it does not yet have a reviewed Simplified variant")
	}
}
