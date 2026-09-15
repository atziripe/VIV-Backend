package cascade_test

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	weeklytarget "viv/internal/core/weekly_target"
)

func mustGoal(t *testing.T, id goal.ID) goal.GoalProfile {
	t.Helper()
	g, ok := goal.ByID(id)
	if !ok {
		t.Fatalf("goal.ByID(%s) not found", id)
	}
	return g
}

func mustTarget(t *testing.T, g goal.GoalProfile, recovery checkin.RecoveryCapacity, bandwidth checkin.LifeBandwidth, build checkin.BuildReadiness) weeklytarget.WeeklyTarget {
	t.Helper()
	wt, err := weeklytarget.BuildWeeklyTarget(g, recovery, bandwidth, build, domain.PhaseFollicular)
	if err != nil {
		t.Fatalf("BuildWeeklyTarget(%s) returned error: %v", g.ID, err)
	}
	return wt
}

// ============================================================================
// Worked example #1 — literal design doc §8.2: "A HIIT session is planned.
// Today's check-in shows low energy and accumulated fatigue."
//
// HONEST NOTE ON A REAL DESIGN-DOC INCONSISTENCY: §8.2's prose describes
// Strength/Endurance resolving this via "lower intensity — same session,
// medium instead of high effort." But VIV-101's own taxonomy (built from
// this same design doc's §3.1 table) classifies HIIT's intensity as FIXED
// at H — "true HIIT means repeated Z4-5 or near-maximal efforts; 'medium
// HIIT' is really circuit/interval training." The narrative §8.2 example
// and the formal §3.1 table don't actually agree with each other. Given
// VIV-101 is the source of truth this cascade is built on, RelaxIntensity
// is correctly "not applicable" for HIIT here, and — since low-energy
// conditions cap intensity at L while HIIT can never go below H — every
// goal is genuinely forced all the way to SubstituteType. That's the
// correct behavior given the actual taxonomy, not a test bug.
func TestWorkedExample_HIIT_LowEnergy_AllGoalsForcedToSubstitute(t *testing.T) {
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.HIIT, activity.Running, activity.Strength, activity.Pilates, activity.Swimming}}
	candidate := cascade.Activity{ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupFullBody}

	for _, id := range []goal.ID{goal.StrengthMuscle, goal.EndurancePerformance, goal.BodyComposition, goal.ConsistencyWellbeing} {
		t.Run(string(id), func(t *testing.T) {
			g := mustGoal(t, id)
			target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack)

			sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if sa.AdjustmentLever != cascade.AdjustmentSubstituteType {
				t.Errorf("AdjustmentLever = %s, want %s (Running excluded by impact; Strength@L/mobility is the only catalog fit)",
					sa.AdjustmentLever, cascade.AdjustmentSubstituteType)
			}
			if !sa.Substituted {
				t.Error("expected Substituted = true")
			}
			if sa.OriginalType != activity.HIIT {
				t.Errorf("OriginalType = %s, want %s", sa.OriginalType, activity.HIIT)
			}
			// Running would match on effect (aerobic_base) but its impact
			// range is fixed at H, which never fits an L cap — must be
			// skipped in favor of Strength.
			if sa.ActivityType != activity.Strength {
				t.Errorf("ActivityType = %s, want %s (Running must be excluded by its fixed H impact)", sa.ActivityType, activity.Strength)
			}
			if sa.Intensity != activity.IntensityL || sa.Impact != activity.ImpactL {
				t.Errorf("substitute intensity/impact = %s/%s, want L/L", sa.Intensity, sa.Impact)
			}
		})
	}
}

// ============================================================================
// Worked example #2 — same shape as design doc §8.2 ("same starting
// conflict, four different resolutions") but using Functional training,
// which (unlike HIIT) genuinely has a variable intensity AND impact range
// per VIV-101 — so this is where the goal-specific ORDER actually produces
// different resolutions, not just a shared forced substitution.
func TestWorkedExample_Functional_HighReadinessPullBack_DifferentResolutionsPerGoal(t *testing.T) {
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.Functional, activity.Strength, activity.Mobility}}
	// "Intense CrossFit-style/metcon" — the natural high-effort default.
	candidate := cascade.Activity{ActivityType: activity.Functional, Intensity: activity.IntensityH, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupFullBody}

	t.Run("strength_muscle: resolves via RelaxIntensity, keeps full impact cap", func(t *testing.T) {
		g := mustGoal(t, goal.StrengthMuscle)
		target := mustTarget(t, g, checkin.RecoveryHigh, checkin.BandwidthHigh, checkin.BuildPullBack) // target: intensity cap M, impact cap H
		sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sa.Substituted {
			t.Fatal("expected no substitution — Functional itself should resolve this")
		}
		if sa.AdjustmentLever != cascade.AdjustmentRelaxIntensity {
			t.Errorf("AdjustmentLever = %s, want %s", sa.AdjustmentLever, cascade.AdjustmentRelaxIntensity)
		}
		if sa.Intensity != activity.IntensityM {
			t.Errorf("Intensity = %s, want M (highest value at or below the M cap)", sa.Intensity)
		}
		if sa.Impact != activity.ImpactH {
			t.Errorf("Impact = %s, want H unchanged (impact was never the blocking dimension)", sa.Impact)
		}
	})

	t.Run("endurance_performance: resolves via RelaxIntensity too, but drops further (cap is L not M)", func(t *testing.T) {
		g := mustGoal(t, goal.EndurancePerformance)
		target := mustTarget(t, g, checkin.RecoveryHigh, checkin.BandwidthHigh, checkin.BuildPullBack) // target: intensity cap L, impact cap H
		sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sa.Substituted {
			t.Fatal("expected no substitution")
		}
		if sa.AdjustmentLever != cascade.AdjustmentRelaxIntensity {
			t.Errorf("AdjustmentLever = %s, want %s", sa.AdjustmentLever, cascade.AdjustmentRelaxIntensity)
		}
		if sa.Intensity != activity.IntensityL {
			t.Errorf("Intensity = %s, want L — Endurance's intensity cap is a full 2 steps lower than Strength's here", sa.Intensity)
		}
	})

	t.Run("body_composition: resolves via RelaxImpact FIRST, preserving full intensity — matches design doc's own wording", func(t *testing.T) {
		g := mustGoal(t, goal.BodyComposition)
		target := mustTarget(t, g, checkin.RecoveryHigh, checkin.BandwidthHigh, checkin.BuildPullBack) // target: intensity cap H, impact cap M
		sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sa.Substituted {
			t.Fatal("expected no substitution")
		}
		// Body composition's own ConflictLeverOrder tries RelaxImpact
		// before RelaxIntensity (VIV-102) — design doc §8.2: "starts by
		// lowering impact — removes jump-based/explosive elements, keeps
		// intensity where possible."
		if sa.AdjustmentLever != cascade.AdjustmentRelaxImpact {
			t.Errorf("AdjustmentLever = %s, want %s", sa.AdjustmentLever, cascade.AdjustmentRelaxImpact)
		}
		if sa.Intensity != activity.IntensityH {
			t.Errorf("Intensity = %s, want H unchanged — intensity is never the blocking dimension for this goal here", sa.Intensity)
		}
		if sa.Impact != activity.ImpactM {
			t.Errorf("Impact = %s, want M", sa.Impact)
		}
	})

	t.Run("consistency_wellbeing: Functional never trains mobility/recovery at any intensity, so it must substitute even after relaxing both caps", func(t *testing.T) {
		g := mustGoal(t, goal.ConsistencyWellbeing)
		target := mustTarget(t, g, checkin.RecoveryHigh, checkin.BandwidthHigh, checkin.BuildPullBack) // target: intensity cap M, impact cap M
		sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Functional only ever produces strength/aerobic_base/anaerobic_capacity
		// (VIV-101) — never mobility or recovery, which are Consistency &
		// wellbeing's only two priorities (VIV-102). No amount of
		// relaxing intensity/impact on Functional itself can satisfy
		// that, so this MUST substitute — a genuinely different
		// resolution mechanism from the other three goals, for a
		// well-motivated reason (not an artifact of catalog order).
		if !sa.Substituted {
			t.Fatalf("expected substitution — Functional's effects never overlap Consistency & wellbeing's priorities; got AdjustmentLever=%s, ActivityType=%s",
				sa.AdjustmentLever, sa.ActivityType)
		}
		if sa.AdjustmentLever != cascade.AdjustmentSubstituteType {
			t.Errorf("AdjustmentLever = %s, want %s", sa.AdjustmentLever, cascade.AdjustmentSubstituteType)
		}
		// Strength@L (mobility) comes before Mobility in this catalog's
		// order and also produces a prioritized effect, so it correctly
		// wins the catalog-order-first-match search (see
		// TestSubstituteType_PrefersCatalogOverOutsideCatalog for that
		// behavior tested directly) — the point here is that SOME
		// substitution happened at all, not which specific one.
		if sa.ActivityType != activity.Strength {
			t.Errorf("ActivityType = %s, want %s", sa.ActivityType, activity.Strength)
		}
		if sa.Intensity != activity.IntensityL {
			t.Errorf("Intensity = %s, want L (the only Strength intensity producing mobility)", sa.Intensity)
		}
	})
}

// ============================================================================
// Individual required scenarios
// ============================================================================

func TestNoConflict_CascadeNeverRuns(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := weeklytarget.WeeklyTarget{
		CompositionPriorities: g.CompositionPriorities,
		IntensityCap:          activity.IntensityH,
		ImpactCap:             activity.ImpactH,
		SessionBudget:         5,
		ProgressionDirection:  weeklytarget.ProgressionMaintain,
	}
	// Strength @ H produces "strength" — StrengthMuscle's own primary
	// priority — and is well within an H/H cap, so this should already
	// satisfy without touching the cascade at all.
	candidate := cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactM, MuscleGroup: activity.MuscleGroupFullBody}
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.Strength}}

	sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa.AdjustmentLever != cascade.AdjustmentNone {
		t.Errorf("AdjustmentLever = %s, want %s (zero value) — candidate already satisfied the target, cascade must not run", sa.AdjustmentLever, cascade.AdjustmentNone)
	}
	if sa.Substituted {
		t.Error("expected Substituted = false")
	}
	if sa.ActivityType != activity.Strength || sa.Intensity != activity.IntensityH || sa.Impact != activity.ImpactM {
		t.Errorf("candidate must come back unchanged, got %+v", sa)
	}
}

// TestProtectType_SkippedWhenNotProtectedType proves ProtectType is
// correctly skipped (not applied, doesn't consume a "turn" in a way that
// breaks the rest of the cascade) when the candidate isn't the goal's
// protected activity type.
func TestProtectType_SkippedWhenNotProtectedType(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle) // protected type: "strength"
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthModerate, checkin.BuildPullBack)

	// Pilates is NOT strength_muscle's protected type.
	candidate := cascade.Activity{ActivityType: activity.Pilates, Intensity: activity.IntensityM, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody}
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.Pilates}}

	sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa.AdjustmentLever == cascade.AdjustmentProtectType {
		t.Error("ProtectType must not be the resolving lever for a candidate that isn't the protected activity type")
	}
	if sa.LoadTier == cascade.LoadReduced {
		t.Error("LoadTier must not be Reduced — ProtectType never applied here")
	}
	// It should instead resolve via RelaxIntensity (Pilates has a ranged
	// intensity, L-M).
	if sa.AdjustmentLever != cascade.AdjustmentRelaxIntensity {
		t.Errorf("AdjustmentLever = %s, want %s", sa.AdjustmentLever, cascade.AdjustmentRelaxIntensity)
	}
}

// TestProtectType_AppliesWhenCandidateIsProtectedType is the positive
// counterpart — confirms ProtectType actually fires (and is preferred
// over RelaxIntensity, since it's tried first) when the candidate really
// is the protected type.
func TestProtectType_AppliesWhenCandidateIsProtectedType(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildPullBack) // intensity cap L

	candidate := cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody}
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.Strength}}

	sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa.AdjustmentLever != cascade.AdjustmentProtectType {
		t.Errorf("AdjustmentLever = %s, want %s", sa.AdjustmentLever, cascade.AdjustmentProtectType)
	}
	if sa.LoadTier != cascade.LoadReduced {
		t.Errorf("LoadTier = %s, want %s", sa.LoadTier, cascade.LoadReduced)
	}
}

// TestRelaxImpact_NotApplicableForRunning: Running's impact range is
// FIXED at H (VIV-101) — RelaxImpact must correctly report "not
// applicable" rather than silently no-op-ing, and the cascade must move
// on to try something else instead of getting stuck.
func TestRelaxImpact_NotApplicableForRunning(t *testing.T) {
	g := mustGoal(t, goal.BodyComposition)                                                                 // order starts with RelaxImpact
	target := mustTarget(t, g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildPullBack) // impact cap L

	candidate := cascade.Activity{ActivityType: activity.Running, Intensity: activity.IntensityL, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupLower}
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.Running, activity.Swimming}}

	sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RelaxImpact cannot possibly be what resolved this, since Running's
	// impact is fixed — if the cascade is broken and silently "succeeds"
	// at relaxing a fixed range, this would wrongly report Running
	// unchanged instead of correctly falling through to substitution.
	if sa.AdjustmentLever == cascade.AdjustmentRelaxImpact {
		t.Error("RelaxImpact must not be reported as the resolving lever for Running — its impact range is fixed")
	}
	if !sa.Substituted {
		t.Errorf("expected a substitution once impact-relaxation was correctly skipped as not-applicable, got AdjustmentLever=%s ActivityType=%s",
			sa.AdjustmentLever, sa.ActivityType)
	}
	if sa.ActivityType != activity.Swimming {
		t.Errorf("ActivityType = %s, want %s (fixed-L-impact, matches aerobic_base)", sa.ActivityType, activity.Swimming)
	}
}

// TestSubstitutionCap_FourthConflictForcesRelaxInstead simulates 4
// conflicts for the same user in one week and confirms the 4th (once
// substitutionsUsedThisWeek reaches 3) forces relaxation instead of
// substituting again.
func TestSubstitutionCap_FourthConflictForcesRelaxInstead(t *testing.T) {
	g := mustGoal(t, goal.ConsistencyWellbeing)
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack) // intensity/impact cap L

	// HIIT can never satisfy an L cap (fixed H intensity) — every one of
	// these calls would normally substitute.
	candidate := cascade.Activity{ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupFullBody}
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.HIIT, activity.Mobility}}

	for i, substitutionsUsed := range []int{0, 1, 2, 3} {
		sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, substitutionsUsed)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}

		if substitutionsUsed < 3 {
			if !sa.Substituted || sa.AdjustmentLever != cascade.AdjustmentSubstituteType {
				t.Errorf("call %d (substitutionsUsed=%d): expected a normal substitution, got AdjustmentLever=%s Substituted=%v",
					i+1, substitutionsUsed, sa.AdjustmentLever, sa.Substituted)
			}
		} else {
			if sa.Substituted {
				t.Errorf("call %d (substitutionsUsed=%d): expected NO substitution once the cap is reached", i+1, substitutionsUsed)
			}
			if sa.AdjustmentLever != cascade.AdjustmentForcedRelax {
				t.Errorf("call %d: AdjustmentLever = %s, want %s", i+1, sa.AdjustmentLever, cascade.AdjustmentForcedRelax)
			}
			// Forced relax operates on the ORIGINAL candidate (HIIT), not
			// a substitute.
			if sa.ActivityType != activity.HIIT {
				t.Errorf("call %d: ActivityType = %s, want %s (forced relax keeps the original activity)", i+1, sa.ActivityType, activity.HIIT)
			}
			if sa.Impact != activity.ImpactM {
				t.Errorf("call %d: Impact = %s, want M (HIIT's impact range is M-H, relaxed one notch)", i+1, sa.Impact)
			}
			// HIIT's intensity is fixed — RelaxIntensity is not
			// applicable even in the forced path, so intensity stays H.
			if sa.Intensity != activity.IntensityH {
				t.Errorf("call %d: Intensity = %s, want H unchanged (HIIT's intensity is fixed, not even the forced path can relax it)", i+1, sa.Intensity)
			}
		}
	}
}

// TestSubstituteType_PrefersCatalogOverOutsideCatalog confirms the
// catalog-first search order: an activity that fits sits in the catalog,
// so the out-of-catalog fallback must never be used even though a
// different, also-compatible activity exists outside the catalog too.
func TestSubstituteType_PrefersCatalogOverOutsideCatalog(t *testing.T) {
	g := mustGoal(t, goal.ConsistencyWellbeing)                                                  // priorities: mobility, recovery
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack) // intensity/impact cap L

	// Mobility (produces mobility/recovery) is IN the catalog. Yoga (also
	// produces mobility/recovery at low intensities) is NOT.
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.HIIT, activity.Mobility}}
	candidate := cascade.Activity{ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupFullBody}

	sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sa.Substituted {
		t.Fatal("expected a substitution")
	}
	if sa.ActivityType != activity.Mobility {
		t.Errorf("ActivityType = %s, want %s (in-catalog fit, must be preferred over Yoga which is outside the catalog)", sa.ActivityType, activity.Mobility)
	}
	if sa.SubstituteReason == "" || containsOutsideCatalogPhrase(sa.SubstituteReason) {
		t.Errorf("SubstituteReason = %q, must not read as an outside-catalog suggestion for an in-catalog match", sa.SubstituteReason)
	}
}

// TestSubstituteType_FallsBackOutsideCatalogWhenNothingFits confirms the
// out-of-catalog path actually works and is flagged, for the case where
// nothing in the catalog can satisfy the target at all.
func TestSubstituteType_FallsBackOutsideCatalogWhenNothingFits(t *testing.T) {
	g := mustGoal(t, goal.ConsistencyWellbeing) // priorities: mobility, recovery
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack)

	// Nothing else in this catalog produces mobility/recovery.
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.HIIT, activity.Running, activity.Swimming}}
	candidate := cascade.Activity{ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupFullBody}

	sa, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sa.Substituted {
		t.Fatal("expected a substitution")
	}
	if catalog.Has(sa.ActivityType) {
		t.Errorf("ActivityType = %s is in the catalog — expected an outside-catalog suggestion since nothing in it fits", sa.ActivityType)
	}
	if !containsOutsideCatalogPhrase(sa.SubstituteReason) {
		t.Errorf("SubstituteReason = %q, want it flagged as outside the catalog", sa.SubstituteReason)
	}
}

func containsOutsideCatalogPhrase(reason string) bool {
	for i := 0; i+len("outside") <= len(reason); i++ {
		if reason[i:i+len("outside")] == "outside" {
			return true
		}
	}
	return false
}

// ============================================================================
// Input validation
// ============================================================================

func TestResolveSlotConflict_InvalidInputsError(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain)
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.Strength}}

	t.Run("empty goal profile", func(t *testing.T) {
		_, err := cascade.ResolveSlotConflict(target, cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactL}, catalog, goal.GoalProfile{}, 0)
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("unknown activity type", func(t *testing.T) {
		_, err := cascade.ResolveSlotConflict(target, cascade.Activity{ActivityType: "not_a_real_activity", Intensity: activity.IntensityM, Impact: activity.ImpactL}, catalog, g, 0)
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("intensity not valid for this activity type", func(t *testing.T) {
		// Cycling doesn't support AR intensity.
		_, err := cascade.ResolveSlotConflict(target, cascade.Activity{ActivityType: activity.Cycling, Intensity: activity.IntensityAR, Impact: activity.ImpactL}, catalog, g, 0)
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("impact not valid for this activity type", func(t *testing.T) {
		// Running's impact is fixed at H — M is not valid.
		_, err := cascade.ResolveSlotConflict(target, cascade.Activity{ActivityType: activity.Running, Intensity: activity.IntensityL, Impact: activity.ImpactM}, catalog, g, 0)
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

// ============================================================================
// SatisfiesTarget — VIV-107 support
// ============================================================================

func TestSatisfiesTarget_TrueWhenWithinCapsAndPrioritized(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain) // baseline M/M

	ok, err := cascade.SatisfiesTarget(cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM}, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected Strength@(M,M) to satisfy a (M,M) target")
	}
}

func TestSatisfiesTarget_FalseWhenOverCap(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack) // caps down at L/L

	ok, err := cascade.SatisfiesTarget(cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactM}, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected Strength@(H,M) NOT to satisfy an (L,L) target")
	}
}

func TestSatisfiesTarget_InvalidActivityErrors(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain)

	if _, err := cascade.SatisfiesTarget(cascade.Activity{ActivityType: "not_real", Intensity: activity.IntensityM, Impact: activity.ImpactM}, target); err == nil {
		t.Error("expected an error for an unknown activity type")
	}
	if _, err := cascade.SatisfiesTarget(cascade.Activity{ActivityType: activity.Cycling, Intensity: activity.IntensityAR, Impact: activity.ImpactL}, target); err == nil {
		t.Error("expected an error for an intensity Cycling doesn't support")
	}
}

// ============================================================================
// SuggestSafetyRelax — VIV-107 support
// ============================================================================

// TestSuggestSafetyRelax_MatchesFullCascadeWhenOnlyRelaxWasNeeded reuses
// the body_composition/Functional case from
// TestWorkedExample_Functional_HighReadinessPullBack_DifferentResolutionsPerGoal,
// where the full cascade resolves via RelaxImpact alone — body_composition
// has no ProtectType in its order at all, so the safety-only path should
// reach exactly the same result as the full cascade here.
func TestSuggestSafetyRelax_MatchesFullCascadeWhenOnlyRelaxWasNeeded(t *testing.T) {
	g := mustGoal(t, goal.BodyComposition)
	target := mustTarget(t, g, checkin.RecoveryHigh, checkin.BandwidthHigh, checkin.BuildPullBack) // (H,M,4)
	candidate := cascade.Activity{ActivityType: activity.Functional, Intensity: activity.IntensityH, Impact: activity.ImpactH}

	full, err := cascade.ResolveSlotConflict(target, candidate, cascade.UserCatalog{Activities: []activity.ID{activity.Functional}}, g, 0)
	if err != nil {
		t.Fatalf("unexpected error from the full cascade: %v", err)
	}

	suggestion, ok, err := cascade.SuggestSafetyRelax(target, candidate, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a suggestion")
	}
	if suggestion != full {
		t.Errorf("SuggestSafetyRelax = %+v, want it to match the full cascade's result %+v (both should only have needed RelaxImpact)", suggestion, full)
	}
}

// TestSuggestSafetyRelax_NeverAppliesProtectType proves ProtectType —
// explicitly named in the task as NOT safety-relevant — never fires here,
// even for a goal/candidate combination where it's the very first lever in
// ConflictLeverOrder and normally WOULD apply (TestProtectType_AppliesWhenCandidateIsProtectedType
// is the exact same setup, minus the override gate). The full cascade
// gets a LoadReduced tier out of ProtectType; the safety-only path must
// not, because it must never run ProtectType at all.
func TestSuggestSafetyRelax_NeverAppliesProtectType(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack) // caps down at L/L

	candidate := cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactM}

	// Sanity check: the full (ungated) cascade resolves this via
	// ProtectType + RelaxImpact, with LoadTier=LoadReduced — proving
	// ProtectType really would fire here if allowed to.
	full, err := cascade.ResolveSlotConflict(target, candidate, cascade.UserCatalog{Activities: []activity.ID{activity.Strength}}, g, 0)
	if err != nil {
		t.Fatalf("sanity check: unexpected error from the full cascade: %v", err)
	}
	if full.AdjustmentLever != cascade.AdjustmentRelaxImpact || full.LoadTier != cascade.LoadReduced {
		t.Fatalf("sanity check failed — expected the full cascade to resolve via RelaxImpact with LoadReduced (ProtectType having fired first), got lever=%s loadTier=%s",
			full.AdjustmentLever, full.LoadTier)
	}

	suggestion, ok, err := cascade.SuggestSafetyRelax(target, candidate, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected SuggestSafetyRelax to find a resolution via RelaxIntensity/RelaxImpact alone")
	}
	if suggestion.AdjustmentLever != cascade.AdjustmentRelaxImpact {
		t.Errorf("AdjustmentLever = %s, want %s", suggestion.AdjustmentLever, cascade.AdjustmentRelaxImpact)
	}
	if suggestion.LoadTier != cascade.LoadStandard {
		t.Errorf("LoadTier = %s, want %s — ProtectType must never have run here", suggestion.LoadTier, cascade.LoadStandard)
	}
	if suggestion.Intensity != activity.IntensityL || suggestion.Impact != activity.ImpactL {
		t.Errorf("result = (%s,%s), want (L,L)", suggestion.Intensity, suggestion.Impact)
	}
}

// TestSuggestSafetyRelax_NeverFallsThroughToSubstitute is the strongest
// negative proof the task asks for: HIIT's intensity is fixed (can't
// relax at all), and its impact alone can't bring it under an L cap
// either — so relaxing is genuinely insufficient here. The full,
// ungated cascade proves there IS a further resolution available
// (SubstituteType). SuggestSafetyRelax must refuse to reach it and report
// ok=false instead, rather than ever returning a SubstituteType (or any
// other non-relax) lever.
func TestSuggestSafetyRelax_NeverFallsThroughToSubstitute(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryLow, checkin.BandwidthLow, checkin.BuildPullBack) // caps down at L/L

	candidate := cascade.Activity{ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH}
	catalog := cascade.UserCatalog{Activities: []activity.ID{activity.HIIT, activity.Strength}}

	// Sanity check: the full cascade DOES resolve this, via substitution —
	// proving a resolution exists beyond what relax-only can reach.
	full, err := cascade.ResolveSlotConflict(target, candidate, catalog, g, 0)
	if err != nil {
		t.Fatalf("sanity check: unexpected error from the full cascade: %v", err)
	}
	if full.AdjustmentLever != cascade.AdjustmentSubstituteType {
		t.Fatalf("sanity check failed — expected the full cascade to resolve via SubstituteType, got %s", full.AdjustmentLever)
	}

	suggestion, ok, err := cascade.SuggestSafetyRelax(target, candidate, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false (no safe relax-only adjustment exists), got a suggestion: %+v", suggestion)
	}
	if suggestion != (cascade.SlotAssignment{}) {
		t.Errorf("expected the zero-value SlotAssignment when ok=false, got %+v", suggestion)
	}
}

func TestSuggestSafetyRelax_InvalidInputsError(t *testing.T) {
	g := mustGoal(t, goal.StrengthMuscle)
	target := mustTarget(t, g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain)

	t.Run("empty goal profile", func(t *testing.T) {
		_, _, err := cascade.SuggestSafetyRelax(target, cascade.Activity{ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM}, goal.GoalProfile{})
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("unknown activity type", func(t *testing.T) {
		_, _, err := cascade.SuggestSafetyRelax(target, cascade.Activity{ActivityType: "not_real", Intensity: activity.IntensityM, Impact: activity.ImpactM}, g)
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}
