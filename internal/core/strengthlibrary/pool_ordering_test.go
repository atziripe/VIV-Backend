package strengthlibrary

import "testing"

func namedExercise(id, name string) Exercise {
	return Exercise{ID: id, Name: name, ReviewStatus: ReviewApproved}
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

// TestBuildSlotPool_FollowsExplicitPoolOrderVerbatim is the core
// contract: the pool is PoolOrder, resolved to Exercises, in that exact
// order — never re-sorted by anything this package computes itself.
func TestBuildSlotPool_FollowsExplicitPoolOrderVerbatim(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Squat"}, PoolOrder: []string{"Goblet Squat", "Box Squat", "Back Squat"}}
	approved := map[MovementGroup][]Exercise{
		"Squat": {
			namedExercise("back_squat", "Back Squat"),
			namedExercise("box_squat", "Box Squat"),
			namedExercise("goblet_squat", "Goblet Squat"),
		},
	}
	// approvedByGroup lists Back/Box/Goblet in that (file) order, but
	// PoolOrder says Goblet, Box, Back — the pool must follow PoolOrder,
	// proving this is never re-sorted by file order either.
	assertOrder(t, BuildSlotPool(slot, approved), "goblet_squat", "box_squat", "back_squat")
}

// TestBuildSlotPool_SkipsNamesNotApproved covers "skip any exercise
// that is not approved in the catalog" — a PoolOrder entry with no
// matching approved exercise (pending, rejected, or simply absent) is
// silently omitted, never an error.
func TestBuildSlotPool_SkipsNamesNotApproved(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Squat"}, PoolOrder: []string{"Goblet Squat", "Not Yet Reviewed Squat", "Back Squat"}}
	approved := map[MovementGroup][]Exercise{
		"Squat": {
			namedExercise("back_squat", "Back Squat"),
			namedExercise("goblet_squat", "Goblet Squat"),
			// "Not Yet Reviewed Squat" deliberately absent — e.g. still
			// pending, so Library.ApprovedExercises never included it.
		},
	}
	assertOrder(t, BuildSlotPool(slot, approved), "goblet_squat", "back_squat")
}

// TestBuildSlotPool_ResolvesAcrossAllOfSlotsMovementGroups confirms a
// slot merging several source groups (e.g. Full body's "Pull") still
// resolves PoolOrder names regardless of which group they came from.
func TestBuildSlotPool_ResolvesAcrossAllOfSlotsMovementGroups(t *testing.T) {
	slot := Slot{Name: "Pull", MovementGroups: []MovementGroup{"Vertical pull", "Horizontal pull"}, PoolOrder: []string{"Row", "Pulldown", "Pull-Up"}}
	approved := map[MovementGroup][]Exercise{
		"Vertical pull":   {namedExercise("pulldown", "Pulldown"), namedExercise("pullup", "Pull-Up")},
		"Horizontal pull": {namedExercise("row", "Row")},
	}
	assertOrder(t, BuildSlotPool(slot, approved), "row", "pulldown", "pullup")
}

func TestBuildSlotPool_EmptyWhenNoPoolOrderEntryResolves(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Squat"}, PoolOrder: []string{"Ghost Exercise"}}
	pool := BuildSlotPool(slot, map[MovementGroup][]Exercise{"Squat": {namedExercise("back_squat", "Back Squat")}})
	if len(pool) != 0 {
		t.Errorf("expected an empty pool, got %v", idsOf(pool))
	}
}

func TestBuildSlotPool_EmptyPoolForUnknownGroup(t *testing.T) {
	slot := Slot{Name: "Squat", MovementGroups: []MovementGroup{"Nonexistent"}, PoolOrder: []string{"Back Squat"}}
	pool := BuildSlotPool(slot, map[MovementGroup][]Exercise{})
	if len(pool) != 0 {
		t.Errorf("expected an empty pool, got %v", idsOf(pool))
	}
}
