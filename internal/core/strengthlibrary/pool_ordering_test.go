package strengthlibrary

import "testing"

func demandExercise(id string, technical, balance DemandLevel) Exercise {
	return Exercise{ID: id, TechnicalDemand: technical, BalanceDemand: balance, ReviewStatus: ReviewApproved}
}

func idsOf(exercises []Exercise) []string {
	ids := make([]string, len(exercises))
	for i, e := range exercises {
		ids[i] = e.ID
	}
	return ids
}

func assertOrder(t *testing.T, got []Exercise, want ...string) {
	t.Helper()
	gotIDs := idsOf(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("pool = %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("pool = %v, want %v", gotIDs, want)
		}
	}
}

func TestBuildSlotPool_SingleGroupSortsAscendingByDemand(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Squat"}}
	approved := map[MovementGroup][]Exercise{
		"Squat": {
			demandExercise("high_high", DemandHigh, DemandHigh),
			demandExercise("low_low", DemandLow, DemandLow),
			demandExercise("moderate_low", DemandModerate, DemandLow),
		},
	}
	assertOrder(t, BuildSlotPool(slot, approved), "low_low", "moderate_low", "high_high")
}

func TestBuildSlotPool_TiebreakIsBalanceAscending(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Squat"}}
	approved := map[MovementGroup][]Exercise{
		"Squat": {
			demandExercise("mod_high", DemandModerate, DemandHigh),
			demandExercise("mod_low", DemandModerate, DemandLow),
			demandExercise("mod_moderate", DemandModerate, DemandModerate),
		},
	}
	assertOrder(t, BuildSlotPool(slot, approved), "mod_low", "mod_moderate", "mod_high")
}

// TestBuildSlotPool_TiesInterleaveAcrossSourceGroups is the behavior
// the real ordering_rule exists for: a slot merging two source groups
// must not list all of one group's tied exercises before the other's.
func TestBuildSlotPool_TiesInterleaveAcrossSourceGroups(t *testing.T) {
	slot := Slot{Name: "Pull", MovementGroups: []MovementGroup{"Vertical pull", "Horizontal pull"}}
	approved := map[MovementGroup][]Exercise{
		"Vertical pull": {
			demandExercise("vp_1", DemandLow, DemandLow),
			demandExercise("vp_2", DemandLow, DemandLow),
		},
		"Horizontal pull": {
			demandExercise("hp_1", DemandLow, DemandLow),
			demandExercise("hp_2", DemandLow, DemandLow),
		},
	}
	// All 4 tie at (Low, Low) — must interleave by source group
	// (Vertical pull listed first in MovementGroups), not list both
	// Vertical pull exercises before either Horizontal pull one.
	assertOrder(t, BuildSlotPool(slot, approved), "vp_1", "hp_1", "vp_2", "hp_2")
}

func TestBuildSlotPool_UnequalGroupSizesInterleaveThenDrain(t *testing.T) {
	slot := Slot{Name: "Pull", MovementGroups: []MovementGroup{"A", "B"}}
	approved := map[MovementGroup][]Exercise{
		"A": {demandExercise("a1", DemandLow, DemandLow)},
		"B": {
			demandExercise("b1", DemandLow, DemandLow),
			demandExercise("b2", DemandLow, DemandLow),
			demandExercise("b3", DemandLow, DemandLow),
		},
	}
	assertOrder(t, BuildSlotPool(slot, approved), "a1", "b1", "b2", "b3")
}

func TestBuildSlotPool_DistinctRatingsAcrossGroupsStillBucketByRatingFirst(t *testing.T) {
	slot := Slot{Name: "Pull", MovementGroups: []MovementGroup{"A", "B"}}
	approved := map[MovementGroup][]Exercise{
		"A": {demandExercise("a_high", DemandHigh, DemandHigh)},
		"B": {demandExercise("b_low", DemandLow, DemandLow)},
	}
	// b_low's rating bucket sorts before a_high's, regardless of which
	// group is listed first in MovementGroups.
	assertOrder(t, BuildSlotPool(slot, approved), "b_low", "a_high")
}

func TestBuildSlotPool_PendingNeverIncluded(t *testing.T) {
	// BuildSlotPool trusts its approvedByGroup input completely — this
	// just documents that contract: callers (mesocycle's selector) must
	// pass Library.ApprovedExercises's output, never ExercisesForZone's
	// includePending=true variant.
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Squat"}}
	approvedOnlyOneEntry := map[MovementGroup][]Exercise{
		"Squat": {demandExercise("approved_one", DemandLow, DemandLow)},
	}
	assertOrder(t, BuildSlotPool(slot, approvedOnlyOneEntry), "approved_one")
}

func TestBuildSlotPool_EmptyPoolForUnknownGroup(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Nonexistent"}}
	pool := BuildSlotPool(slot, map[MovementGroup][]Exercise{})
	if len(pool) != 0 {
		t.Errorf("expected an empty pool, got %v", idsOf(pool))
	}
}
