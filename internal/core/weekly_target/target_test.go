package weeklytarget_test

import (
	"fmt"
	"reflect"
	"testing"

	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	weeklytarget "viv/internal/core/weekly_target"
)

func TestTypeCompositionPrioritiesIsAPassthrough(t *testing.T) {
	g, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("goal.ByID(StrengthMuscle) not found")
	}
	got := weeklytarget.TypeCompositionPriorities(g)
	if !reflect.DeepEqual(got, g.CompositionPriorities) {
		t.Errorf("TypeCompositionPriorities = %+v, want exactly g.CompositionPriorities = %+v", got, g.CompositionPriorities)
	}
}

// TestEachGoalProducesDifferentOutputForIdenticalReadinessInput proves the
// goal-specific curve is actually wired in and not a no-op: given the
// exact same readiness input, all four goals must not collapse to the
// same WeeklyTarget shape.
func TestEachGoalProducesDifferentOutputForIdenticalReadinessInput(t *testing.T) {
	ids := []goal.ID{goal.StrengthMuscle, goal.EndurancePerformance, goal.BodyComposition, goal.ConsistencyWellbeing}

	seen := map[string]goal.ID{}
	for _, id := range ids {
		g, ok := goal.ByID(id)
		if !ok {
			t.Fatalf("goal.ByID(%s) not found", id)
		}

		wt, err := weeklytarget.BuildWeeklyTarget(g, checkin.RecoveryHigh, checkin.BandwidthModerate, checkin.BuildPullBack, domain.PhaseFollicular)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", id, err)
		}

		key := fmt.Sprintf("intensity=%s impact=%s budget=%d direction=%s", wt.IntensityCap, wt.ImpactCap, wt.SessionBudget, wt.ProgressionDirection)
		if other, dup := seen[key]; dup {
			t.Errorf("%s and %s produced the identical WeeklyTarget (%s) for the same readiness input — the goal-specific curve isn't actually wired in",
				id, other, key)
		}
		seen[key] = id
	}

	if len(seen) != len(ids) {
		t.Errorf("expected %d distinct WeeklyTarget shapes across the 4 goals, got %d", len(ids), len(seen))
	}
}

// TestPullBackBehaviorMatchesEachGoalsDesignDocRationale pins down the
// specific, named difference the task called out: strength_muscle
// protects session count and relaxes intensity only, while
// consistency_wellbeing protects session count too but relaxes BOTH
// intensity and impact — same signal, different magnitude, per goal.
func TestPullBackBehaviorMatchesEachGoalsDesignDocRationale(t *testing.T) {
	const recovery = checkin.RecoveryHigh   // baseline H/H
	const bandwidth = checkin.BandwidthHigh // baseline budget 5
	const build = checkin.BuildPullBack

	tests := []struct {
		id            goal.ID
		wantIntensity string
		wantImpact    string
		wantBudget    int
	}{
		// Drops intensity by 1 step (H→M), impact and budget untouched —
		// "reduce load or volume while preserving a useful strength stimulus".
		{goal.StrengthMuscle, "M", "H", 5},
		// Drops intensity by 2 steps (H→L), impact and budget untouched —
		// "replace high intensity with easier aerobic work".
		{goal.EndurancePerformance, "L", "H", 5},
		// Drops impact by 1 step (H→M), budget trimmed by 1 —
		// "maintain the muscle stimulus without adding excessive fatigue".
		{goal.BodyComposition, "H", "M", 4},
		// Drops BOTH intensity and impact by 1 step, budget untouched —
		// "shorten, simplify or reduce intensity rather than skip".
		{goal.ConsistencyWellbeing, "M", "M", 5},
	}

	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			g, ok := goal.ByID(tt.id)
			if !ok {
				t.Fatalf("goal.ByID(%s) not found", tt.id)
			}
			wt, err := weeklytarget.BuildWeeklyTarget(g, recovery, bandwidth, build, domain.PhaseFollicular)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(wt.IntensityCap) != tt.wantIntensity {
				t.Errorf("IntensityCap = %s, want %s", wt.IntensityCap, tt.wantIntensity)
			}
			if string(wt.ImpactCap) != tt.wantImpact {
				t.Errorf("ImpactCap = %s, want %s", wt.ImpactCap, tt.wantImpact)
			}
			if wt.SessionBudget != tt.wantBudget {
				t.Errorf("SessionBudget = %d, want %d", wt.SessionBudget, tt.wantBudget)
			}
			if wt.ProgressionDirection != weeklytarget.ProgressionPullBack {
				t.Errorf("ProgressionDirection = %s, want %s", wt.ProgressionDirection, weeklytarget.ProgressionPullBack)
			}
		})
	}
}

func TestMaintainAndPushForwardStayAtBaseline(t *testing.T) {
	g, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("goal.ByID(StrengthMuscle) not found")
	}

	for _, build := range []checkin.BuildReadiness{checkin.BuildMaintain, checkin.BuildPushForward} {
		wt, err := weeklytarget.BuildWeeklyTarget(g, checkin.RecoveryModerate, checkin.BandwidthLow, build, domain.PhaseLateLuteal)
		if err != nil {
			t.Fatalf("build=%s: unexpected error: %v", build, err)
		}
		if wt.IntensityCap != "M" || wt.ImpactCap != "M" {
			t.Errorf("build=%s: caps = (%s,%s), want baseline (M,M) for RecoveryModerate", build, wt.IntensityCap, wt.ImpactCap)
		}
		if wt.SessionBudget != 2 {
			t.Errorf("build=%s: SessionBudget = %d, want baseline 2 for BandwidthLow", build, wt.SessionBudget)
		}
	}
}

func TestCyclePhaseIsThreadedThroughWithoutChangingOutput(t *testing.T) {
	g, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("goal.ByID(StrengthMuscle) not found")
	}

	var results []weeklytarget.WeeklyTarget
	for _, phase := range []domain.CyclePhase{
		domain.PhaseMenstrual, domain.PhaseFollicular, domain.PhaseOvulatory, domain.PhaseEarlyLuteal, domain.PhaseLateLuteal,
	} {
		wt, err := weeklytarget.BuildWeeklyTarget(g, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain, phase)
		if err != nil {
			t.Fatalf("phase=%s: unexpected error: %v", phase, err)
		}
		results = append(results, wt)
	}

	for i := 1; i < len(results); i++ {
		if !reflect.DeepEqual(results[i], results[0]) {
			t.Errorf("WeeklyTarget differs across cycle phases with identical readiness input — phase must not branch logic here (design doc §6): %+v vs %+v",
				results[0], results[i])
		}
	}
}

func TestMissingGoalProfileReturnsError(t *testing.T) {
	_, err := weeklytarget.BuildWeeklyTarget(goal.GoalProfile{}, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain, domain.PhaseFollicular)
	if err == nil {
		t.Fatal("expected an error for an empty/missing GoalProfile from BuildWeeklyTarget")
	}

	_, _, _, _, err = weeklytarget.ReadinessResponseCurve(goal.GoalProfile{}, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain)
	if err == nil {
		t.Fatal("expected an error for an empty/missing GoalProfile from ReadinessResponseCurve too")
	}
}

func TestUnknownGoalIDReturnsError(t *testing.T) {
	unknown := goal.GoalProfile{ID: "not_a_real_goal"}
	_, err := weeklytarget.BuildWeeklyTarget(unknown, checkin.RecoveryModerate, checkin.BandwidthModerate, checkin.BuildMaintain, domain.PhaseFollicular)
	if err == nil {
		t.Fatal("expected an error for a GoalProfile with an unregistered ID")
	}
}

func TestReadinessResponseCurve_InvalidReadinessValuesError(t *testing.T) {
	g, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("goal.ByID(StrengthMuscle) not found")
	}

	tests := []struct {
		name      string
		recovery  checkin.RecoveryCapacity
		bandwidth checkin.LifeBandwidth
		build     checkin.BuildReadiness
	}{
		{"invalid recovery", "extreme", checkin.BandwidthModerate, checkin.BuildMaintain},
		{"invalid bandwidth", checkin.RecoveryModerate, "overloaded", checkin.BuildMaintain},
		{"invalid build", checkin.RecoveryModerate, checkin.BandwidthModerate, "sprint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, err := weeklytarget.ReadinessResponseCurve(g, tt.recovery, tt.bandwidth, tt.build)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
