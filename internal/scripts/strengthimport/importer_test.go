package main

import (
	"encoding/json"
	"strings"
	"testing"

	"viv/internal/core/strengthlibrary"
)

func testEquipMapping() map[string]string {
	return map[string]string{
		"dumbbell":   "dumbbell",
		"barbell":    "barbell",
		"bench":      "bench",
		"machine":    "machine",
		"none":       "none",
		"bodyweight": "none",
	}
}

func layoutAHeader() []string {
	return []string{"Movement group", "Exercise", "Equipment required", "Technical demand", "Balance demand", "Reviewed by Lina"}
}

func sheetA(tabName string, dataRows ...[]string) sheet {
	rows := append([][]string{layoutAHeader()}, dataRows...)
	return sheet{tabName: tabName, rows: rows}
}

func findExercise(t *testing.T, exercises []strengthlibrary.Exercise, id string) strengthlibrary.Exercise {
	t.Helper()
	for _, e := range exercises {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("exercise %q not found in %+v", id, exercises)
	return strengthlibrary.Exercise{}
}

func TestImportWorkbook_FixtureDirectory_NormalRowsImportAndEquipmentMaps(t *testing.T) {
	wb, err := readCSVWorkbook("testdata")
	if err != nil {
		t.Fatalf("readCSVWorkbook: %v", err)
	}

	result := importWorkbook(wb, loadRealEquipmentMappingOrFatal(t))
	if len(result.errs) > 0 {
		t.Fatalf("unexpected import errors: %v", result.errs)
	}

	// lower_body_layout_a.csv: 4 real rows (2 Squat, 2 Hip hinge) once
	// the decoy header rows and the one blank row are skipped.
	// upper_body_layout_b.csv: 2 real rows (Lat Pulldown goes to
	// needs_review, not here).
	if len(result.exercises) != 6 {
		t.Fatalf("expected 6 exercises, got %d: %+v", len(result.exercises), result.exercises)
	}

	squat := findExercise(t, result.exercises, "lower_goblet_squat")
	if squat.ReviewStatus != strengthlibrary.ReviewApproved {
		t.Errorf("Goblet Squat: expected approved, got %s", squat.ReviewStatus)
	}
	if !squat.DemandReviewed {
		t.Error("Goblet Squat: expected DemandReviewed=true for an approved row")
	}
	if len(squat.Equipment) != 1 || squat.Equipment[0] != "dumbbell" {
		t.Errorf("Goblet Squat: expected equipment [dumbbell], got %v", squat.Equipment)
	}

	pistol := findExercise(t, result.exercises, "lower_pistol_squat")
	if pistol.ReviewStatus != strengthlibrary.ReviewPending {
		t.Errorf("Pistol Squat: expected pending (blank Reviewed cell), got %s", pistol.ReviewStatus)
	}
	if pistol.DemandReviewed {
		t.Error("Pistol Squat: expected DemandReviewed=false for a pending row")
	}

	singleLegRDL := findExercise(t, result.exercises, "lower_single_leg_rdl")
	if singleLegRDL.ReviewStatus != strengthlibrary.ReviewPending {
		t.Errorf("Single Leg RDL: expected pending (\"No\" is not \"Yes\"), got %s", singleLegRDL.ReviewStatus)
	}

	benchPress := findExercise(t, result.exercises, "upper_barbell_bench_press")
	wantEquip := map[string]bool{"barbell": true, "bench": true}
	if len(benchPress.Equipment) != 2 || !wantEquip[benchPress.Equipment[0]] || !wantEquip[benchPress.Equipment[1]] {
		t.Errorf("Barbell Bench Press: expected [barbell bench] (any order), got %v", benchPress.Equipment)
	}

	pushUp := findExercise(t, result.exercises, "upper_push_up")
	if pushUp.Flag != "Lina removed this" || pushUp.Note != "Near something Lina removed" {
		t.Errorf("Push-up: expected Flag/Note from the v1 layout columns, got flag=%q note=%q", pushUp.Flag, pushUp.Note)
	}

	// The "Doesn't belong" row must never show up as an exercise.
	for _, e := range result.exercises {
		if e.Name == "Lat Pulldown" {
			t.Fatal("Lat Pulldown was flagged \"Doesn't belong here?\" and must not be imported")
		}
	}
}

func TestImportWorkbook_DoesntBelongGoesToNeedsReviewWithComment(t *testing.T) {
	wb, err := readCSVWorkbook("testdata")
	if err != nil {
		t.Fatalf("readCSVWorkbook: %v", err)
	}
	result := importWorkbook(wb, loadRealEquipmentMappingOrFatal(t))
	if len(result.errs) > 0 {
		t.Fatalf("unexpected import errors: %v", result.errs)
	}

	if len(result.needsReview) != 1 {
		t.Fatalf("expected exactly 1 needs-review row, got %d: %+v", len(result.needsReview), result.needsReview)
	}
	nr := result.needsReview[0]
	if nr.Name != "Lat Pulldown" || nr.Zone != strengthlibrary.ZoneUpper || nr.MovementGroup != "Vertical pull" {
		t.Errorf("unexpected needs-review entry: %+v", nr)
	}
	if nr.Comment != "Better suited to a different zone" {
		t.Errorf("expected the Comment column's text, got %q", nr.Comment)
	}
}

func TestImportWorkbook_SameExerciseNameTwoZonesGetsDistinctIDs(t *testing.T) {
	lower := sheetA("Lower body", []string{"Push", "Barbell Bench Press", "Barbell", "Low", "Low", "Yes"})
	upper := sheetA("Upper body", []string{"Horizontal push", "Barbell Bench Press", "Barbell", "Low", "Low", "Yes"})

	result := importWorkbook(workbook{sheets: []sheet{lower, upper}}, testEquipMapping())
	if len(result.errs) > 0 {
		t.Fatalf("unexpected errors: %v", result.errs)
	}
	if len(result.exercises) != 2 {
		t.Fatalf("expected 2 exercises, got %d", len(result.exercises))
	}
	ids := map[string]bool{result.exercises[0].ID: true, result.exercises[1].ID: true}
	if !ids["lower_barbell_bench_press"] || !ids["upper_barbell_bench_press"] {
		t.Fatalf("expected distinct zone-prefixed IDs, got %v", ids)
	}
}

func TestImportWorkbook_DuplicateZoneGroupNameFails(t *testing.T) {
	sh := sheetA("Lower body",
		[]string{"Squat", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"},
		[]string{"Squat", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"},
	)
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) == 0 {
		t.Fatal("expected a duplicate-row error, got none")
	}
	if !containsSubstring(result.errs, "duplicate") {
		t.Errorf("expected an error mentioning \"duplicate\", got %v", result.errs)
	}
}

func TestImportWorkbook_SameNameTwoGroupsSameZoneFailsOnIDCollision(t *testing.T) {
	// Different movement group, same zone + name: not caught by the
	// literal (zone, group, name) check, but IS caught as an ID
	// collision, since ID is zone+name only (see id.go's doc comment).
	sh := sheetA("Lower body",
		[]string{"Squat", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"},
		[]string{"Hip hinge", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"},
	)
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) == 0 {
		t.Fatal("expected an ID-collision error, got none")
	}
	if !containsSubstring(result.errs, "same exercise id") {
		t.Errorf("expected an error mentioning the ID collision, got %v", result.errs)
	}
}

func TestImportWorkbook_UnknownZoneFails(t *testing.T) {
	sh := sheetA("Mystery zone", []string{"Squat", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"})
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) == 0 {
		t.Fatal("expected an unknown-zone error, got none")
	}
	if !containsSubstring(result.errs, "unknown zone/tab name") {
		t.Errorf("expected an unknown-zone error, got %v", result.errs)
	}
}

func TestImportWorkbook_EmptyMovementGroupFails(t *testing.T) {
	sh := sheetA("Lower body", []string{"", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"})
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) == 0 {
		t.Fatal("expected an empty-movement-group error, got none")
	}
	if !containsSubstring(result.errs, "empty movement group") {
		t.Errorf("expected an empty-movement-group error, got %v", result.errs)
	}
}

func TestImportWorkbook_UnmappedEquipmentFails(t *testing.T) {
	sh := sheetA("Lower body", []string{"Squat", "Goblet Squat", "Resistance Band", "Low", "Low", "Yes"})
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) == 0 {
		t.Fatal("expected an unmapped-equipment error, got none")
	}
	if !containsSubstring(result.errs, "no mapping") {
		t.Errorf("expected an unmapped-equipment error, got %v", result.errs)
	}
}

func TestImportWorkbook_InvalidDemandValueFails(t *testing.T) {
	sh := sheetA("Lower body", []string{"Squat", "Goblet Squat", "Dumbbell", "Medium", "Low", "Yes"})
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) == 0 {
		t.Fatal("expected an invalid-demand error, got none")
	}
	if !containsSubstring(result.errs, "not Low/Moderate/High") {
		t.Errorf("expected an invalid-demand error, got %v", result.errs)
	}
}

func TestImportWorkbook_BlankExerciseRowSkippedSilently(t *testing.T) {
	sh := sheetA("Lower body",
		[]string{"Squat", "", "Dumbbell", "Low", "Low", "Yes"}, // blank Exercise cell
		[]string{"Squat", "Goblet Squat", "Dumbbell", "Low", "Low", "Yes"},
	)
	result := importWorkbook(workbook{sheets: []sheet{sh}}, testEquipMapping())
	if len(result.errs) > 0 {
		t.Fatalf("unexpected errors: %v", result.errs)
	}
	if len(result.exercises) != 1 {
		t.Fatalf("expected the blank row to be skipped silently, got %d exercises: %+v", len(result.exercises), result.exercises)
	}
}

func TestFindHeaderRow_FailsWhenNeitherDocumentedLayoutMatches(t *testing.T) {
	sh := sheet{tabName: "Lower body", rows: [][]string{
		{"Movement group", "Exercise", "Equipment required"}, // missing demand/review columns
		{"Squat", "Goblet Squat", "Dumbbell"},
	}}
	if _, _, err := findHeaderRow(sh); err == nil {
		t.Fatal("expected an error for a header missing required columns, got nil")
	}
}

func TestImportWorkbook_Deterministic_ReRunIsByteIdentical(t *testing.T) {
	wb, err := readCSVWorkbook("testdata")
	if err != nil {
		t.Fatalf("readCSVWorkbook: %v", err)
	}
	mapping := loadRealEquipmentMappingOrFatal(t)

	first := importWorkbook(wb, mapping)
	second := importWorkbook(wb, mapping)
	if len(first.errs) > 0 || len(second.errs) > 0 {
		t.Fatalf("unexpected errors: first=%v second=%v", first.errs, second.errs)
	}

	firstJSON, err := json.MarshalIndent(first.exercises, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	secondJSON, err := json.MarshalIndent(second.exercises, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("re-running importWorkbook on the same input produced different output")
	}
}

func loadRealEquipmentMappingOrFatal(t *testing.T) map[string]string {
	t.Helper()
	mapping, err := loadEquipmentMapping("equipment_mapping.json")
	if err != nil {
		t.Fatalf("loadEquipmentMapping: %v", err)
	}
	return mapping
}

func containsSubstring(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}
