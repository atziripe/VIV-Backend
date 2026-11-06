package main

import "testing"

// TestFindHeaderRow_OldTableRepeatsExerciseHeader guards a real bug the
// actual reviewer workbook exposed: the old left-side table and the
// real table share the word "Exercise" as a header in the SAME row
// (not just "Movement group" being absent from the old table, which is
// all the task description called out). A naive first-occurrence
// lookup resolves "Exercise" to the OLD table's column and then pairs
// those exercise names with the REAL table's equipment/demand/review
// columns — wrong data, not a loud failure. Column resolution must
// anchor on "Movement group" (unique to the real table) and only
// resolve every other header from that column onward.
func TestFindHeaderRow_OldTableRepeatsExerciseHeader(t *testing.T) {
	sh := sheet{tabName: "Lower body", rows: [][]string{
		{"Lower body: mark each exercise (Keep / Change / Remove)"},
		{},
		{"Step 1 DONE", "", "", "", "", "Step 2"},
		{"Exercise", "How it's prescribed now", "Your call", "Comment (optional)", "", "Movement group", "Exercise", "Equipment required", "Technical demand", "Balance demand", "Reviewed by Lina"},
		{"Barbell Back Squat", "5 x 4-6", "Keep", "", "", "Squat", "Bodyweight Box Squat", "Stable box or bench", "Low", "Low", "Yes"},
	}}

	headerRow, cols, err := findHeaderRow(sh)
	if err != nil {
		t.Fatalf("findHeaderRow: %v", err)
	}
	if headerRow != 3 {
		t.Fatalf("expected header row index 3, got %d", headerRow)
	}
	if cols.movementGroup != 5 {
		t.Fatalf("movementGroup: expected column 5, got %d", cols.movementGroup)
	}
	if cols.exercise != 6 {
		t.Fatalf("exercise: expected the REAL table's column 6, got %d (the old table's column 0)", cols.exercise)
	}
	if cols.equipment != 7 || cols.technicalDemand != 8 || cols.balanceDemand != 9 || cols.reviewedByLina != 10 {
		t.Fatalf("unexpected columns: %+v", cols)
	}

	// End-to-end: the parsed row's exercise must be "Bodyweight Box
	// Squat" (real table), never "Barbell Back Squat" (old table) —
	// and it must NOT inherit "Keep"/"How it's prescribed now" from
	// the old table at all.
	row := sh.rows[4]
	if got := cellAt(row, cols.exercise); got != "Bodyweight Box Squat" {
		t.Fatalf("exercise cell: got %q, want %q", got, "Bodyweight Box Squat")
	}
	if got := cellAt(row, cols.movementGroup); got != "Squat" {
		t.Fatalf("movement group cell: got %q, want %q", got, "Squat")
	}
}
