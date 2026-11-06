package strengthlibrary

import (
	"encoding/json"
	"testing"
)

func exJSON(id, name string, zone Zone, group MovementGroup, status ReviewStatus) Exercise {
	return Exercise{
		ID:            id,
		ActivityType:  "strength",
		Name:          name,
		Zone:          zone,
		MovementGroup: group,
		ReviewStatus:  status,
	}
}

func TestLoad_EmbeddedCatalogIsValid(t *testing.T) {
	// Guards against the real embedded data/exercises.json ever shipping
	// broken — same intent as every other content library's boot-time
	// Load call, just exercised directly in a test instead of only at
	// server startup.
	if _, err := Load(); err != nil {
		t.Fatalf("Load() on embedded catalog: %v", err)
	}
}

func TestLoadFromJSON_RejectsInvalidJSON(t *testing.T) {
	if _, err := loadFromJSON([]byte("{not valid json")); err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

func TestLoadFromJSON_RejectsDuplicateID(t *testing.T) {
	exercises := []Exercise{
		exJSON("lower_goblet_squat", "Goblet Squat", ZoneLower, "Squat", ReviewApproved),
		exJSON("lower_goblet_squat", "Goblet Squat Variant", ZoneLower, "Squat", ReviewPending),
	}
	data := mustMarshal(t, exercises)
	if _, err := loadFromJSON(data); err == nil {
		t.Fatal("expected an error for duplicate id, got nil")
	}
}

func TestLoadFromJSON_RejectsMissingZoneOrGroup(t *testing.T) {
	missingZone := []Exercise{{ID: "x", Name: "X", MovementGroup: "Squat"}}
	if _, err := loadFromJSON(mustMarshal(t, missingZone)); err == nil {
		t.Fatal("expected an error for missing zone, got nil")
	}

	missingGroup := []Exercise{{ID: "x", Name: "X", Zone: ZoneLower}}
	if _, err := loadFromJSON(mustMarshal(t, missingGroup)); err == nil {
		t.Fatal("expected an error for missing movement group, got nil")
	}
}

func TestApprovedExercises_NeverReturnsPending(t *testing.T) {
	exercises := []Exercise{
		exJSON("lower_goblet_squat", "Goblet Squat", ZoneLower, "Squat", ReviewApproved),
		exJSON("lower_pistol_squat", "Pistol Squat", ZoneLower, "Squat", ReviewPending),
		exJSON("lower_rdl", "RDL", ZoneLower, "Hip hinge", ReviewApproved),
	}
	lib, err := loadFromJSON(mustMarshal(t, exercises))
	if err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}

	got := lib.ApprovedExercises(ZoneLower)
	for _, list := range got {
		for _, e := range list {
			if e.ReviewStatus != ReviewApproved {
				t.Fatalf("ApprovedExercises returned a %s exercise: %+v", e.ReviewStatus, e)
			}
		}
	}
	if len(got["Squat"]) != 1 || got["Squat"][0].ID != "lower_goblet_squat" {
		t.Fatalf("expected exactly the approved squat, got %+v", got["Squat"])
	}
	if len(got["Hip hinge"]) != 1 {
		t.Fatalf("expected the approved hip hinge exercise, got %+v", got["Hip hinge"])
	}
}

func TestExercisesForZone_IncludePendingExposesPendingToo(t *testing.T) {
	exercises := []Exercise{
		exJSON("lower_goblet_squat", "Goblet Squat", ZoneLower, "Squat", ReviewApproved),
		exJSON("lower_pistol_squat", "Pistol Squat", ZoneLower, "Squat", ReviewPending),
	}
	lib, err := loadFromJSON(mustMarshal(t, exercises))
	if err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}

	approvedOnly := lib.ExercisesForZone(ZoneLower, false)
	if len(approvedOnly["Squat"]) != 1 {
		t.Fatalf("includePending=false: expected 1 exercise, got %d", len(approvedOnly["Squat"]))
	}

	withPending := lib.ExercisesForZone(ZoneLower, true)
	if len(withPending["Squat"]) != 2 {
		t.Fatalf("includePending=true: expected 2 exercises, got %d", len(withPending["Squat"]))
	}
}

func TestExercisesForZone_ZoneScopedNoCrossZoneLeak(t *testing.T) {
	exercises := []Exercise{
		exJSON("lower_bench_press", "Barbell Bench Press", ZoneLower, "Push", ReviewApproved),
		exJSON("upper_bench_press", "Barbell Bench Press", ZoneUpper, "Horizontal push", ReviewApproved),
	}
	lib, err := loadFromJSON(mustMarshal(t, exercises))
	if err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}
	if got := lib.ApprovedExercises(ZoneLower); len(got) != 1 || len(got["Push"]) != 1 {
		t.Fatalf("expected exactly one lower-zone entry, got %+v", got)
	}
	if got := lib.ApprovedExercises(ZoneUpper); len(got) != 1 || len(got["Horizontal push"]) != 1 {
		t.Fatalf("expected exactly one upper-zone entry, got %+v", got)
	}
}

func TestExerciseID_SameNameDifferentZoneDistinctIDs(t *testing.T) {
	lower := ExerciseID(ZoneLower, "Barbell Bench Press")
	upper := ExerciseID(ZoneUpper, "Barbell Bench Press")
	if lower == upper {
		t.Fatalf("expected distinct IDs across zones, got %q for both", lower)
	}
	if lower != "lower_barbell_bench_press" || upper != "upper_barbell_bench_press" {
		t.Fatalf("unexpected slugs: lower=%q upper=%q", lower, upper)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
