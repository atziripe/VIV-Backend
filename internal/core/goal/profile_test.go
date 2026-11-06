package goal_test

import (
	"testing"

	"viv/internal/core/goal"
)

var validLevers = map[goal.ConflictLever]bool{
	goal.ProtectType:        true,
	goal.RelaxIntensity:     true,
	goal.RelaxImpact:        true,
	goal.ReduceDuration:     true,
	goal.SimplifyComplexity: true,
	goal.SubstituteType:     true,
}

func TestEveryGoalHasNonEmptyValidConflictLeverOrder(t *testing.T) {
	for _, p := range goal.Profiles {
		if len(p.ConflictLeverOrder) == 0 {
			t.Errorf("%s: ConflictLeverOrder is empty", p.ID)
			continue
		}
		for _, lever := range p.ConflictLeverOrder {
			if !validLevers[lever] {
				t.Errorf("%s: ConflictLeverOrder contains an unknown lever %q", p.ID, lever)
			}
		}
	}
}

// TestSubstituteTypeIsAlwaysLast is the structural invariant the rest of
// the system depends on — the cascade needs a guaranteed-applicable
// fallback at the end of every goal's order (design doc §8: "Substituting
// the activity itself is always the last resort across all four goals").
func TestSubstituteTypeIsAlwaysLast(t *testing.T) {
	for _, p := range goal.Profiles {
		if len(p.ConflictLeverOrder) == 0 {
			t.Errorf("%s: ConflictLeverOrder is empty, can't check last element", p.ID)
			continue
		}
		last := p.ConflictLeverOrder[len(p.ConflictLeverOrder)-1]
		if last != goal.SubstituteType {
			t.Errorf("%s: last lever = %q, want %q", p.ID, last, goal.SubstituteType)
		}

		// SubstituteType must also appear exactly once, not just at the
		// end — a duplicate earlier in the order would break the "stop
		// at the first lever that resolves the mismatch" semantics.
		count := 0
		for _, lever := range p.ConflictLeverOrder {
			if lever == goal.SubstituteType {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%s: SubstituteType appears %d times, want exactly 1", p.ID, count)
		}
	}
}

func TestOnlyStrengthAndEnduranceHaveProtectedActivityType(t *testing.T) {
	wantProtected := map[goal.ID]bool{
		goal.StrengthMuscle:       true,
		goal.EndurancePerformance: true,
		goal.BodyComposition:      false,
		goal.ConsistencyWellbeing: false,
	}

	for _, p := range goal.Profiles {
		want := wantProtected[p.ID]
		got := p.ProtectedActivityType != nil
		if got != want {
			t.Errorf("%s: ProtectedActivityType set = %v, want %v", p.ID, got, want)
		}

		// A goal with a protected type must also actually try ProtectType
		// — and a goal without one must not, since there'd be nothing to
		// protect.
		hasProtectLever := false
		for _, lever := range p.ConflictLeverOrder {
			if lever == goal.ProtectType {
				hasProtectLever = true
				break
			}
		}
		if hasProtectLever != want {
			t.Errorf("%s: ConflictLeverOrder contains ProtectType = %v, want %v (must match whether ProtectedActivityType is set)",
				p.ID, hasProtectLever, want)
		}
	}
}

// TestEveryGoalHasAtLeastOnePrimaryPriority: a goal with zero primary
// composition priorities would be a data bug, not a valid "no preference"
// state — VIV-104's cascade always needs somewhere to start filling slots.
func TestEveryGoalHasAtLeastOnePrimaryPriority(t *testing.T) {
	for _, p := range goal.Profiles {
		if len(p.CompositionPriorities) == 0 {
			t.Errorf("%s: CompositionPriorities is empty", p.ID)
			continue
		}
		hasPrimary := false
		for _, cp := range p.CompositionPriorities {
			if cp.Tier == goal.PriorityPrimary {
				hasPrimary = true
				break
			}
		}
		if !hasPrimary {
			t.Errorf("%s: CompositionPriorities has no \"primary\" tier entry", p.ID)
		}
	}
}

func TestCompositionPrioritiesUseValidTiersAndNoDuplicateEffects(t *testing.T) {
	validTiers := map[goal.PriorityTier]bool{
		goal.PriorityPrimary:    true,
		goal.PrioritySupporting: true,
		goal.PriorityOccasional: true,
		goal.PriorityOptional:   true,
	}

	for _, p := range goal.Profiles {
		seen := map[string]bool{}
		for _, cp := range p.CompositionPriorities {
			if !validTiers[cp.Tier] {
				t.Errorf("%s: CompositionPriority for %q has an unknown tier %q", p.ID, cp.Effect, cp.Tier)
			}
			key := string(cp.Effect)
			if seen[key] {
				t.Errorf("%s: TrainingEffect %q appears more than once in CompositionPriorities", p.ID, cp.Effect)
			}
			seen[key] = true
		}
	}
}

func TestAllFourGoalProfilesArePresent(t *testing.T) {
	want := []goal.ID{
		goal.StrengthMuscle, goal.EndurancePerformance, goal.BodyComposition, goal.ConsistencyWellbeing,
	}
	if len(goal.Profiles) != len(want) {
		t.Fatalf("len(Profiles) = %d, want %d", len(goal.Profiles), len(want))
	}
	for _, id := range want {
		if _, ok := goal.ByID(id); !ok {
			t.Errorf("missing goal profile %q", id)
		}
	}
}

func TestByID(t *testing.T) {
	p, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("ByID(StrengthMuscle) not found")
	}
	if p.ID != goal.StrengthMuscle {
		t.Errorf("ByID(StrengthMuscle).ID = %q, want %q", p.ID, goal.StrengthMuscle)
	}

	if _, ok := goal.ByID("not_a_real_goal"); ok {
		t.Error("ByID of an unknown ID should return ok=false")
	}
}

func TestEveryGoalHasACopyTone(t *testing.T) {
	for _, p := range goal.Profiles {
		if p.CopyTone == "" {
			t.Errorf("%s: CopyTone is empty", p.ID)
		}
	}
}
