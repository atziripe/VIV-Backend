package content_test

import (
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
	"viv/internal/core/mesocycle"
)

const realExercisesDir = "../../content/training_v2/exercises"

func TestLoadExerciseLibrary_LoadsRealDummyContent(t *testing.T) {
	lib, err := content.LoadExerciseLibrary(realExercisesDir)
	if err != nil {
		t.Fatalf("LoadExerciseLibrary(%s) returned error: %v", realExercisesDir, err)
	}

	for _, mg := range []activity.MuscleGroup{
		activity.MuscleGroupLower, activity.MuscleGroupUpper, activity.MuscleGroupFullBody, activity.MuscleGroupCore,
	} {
		for _, n := range []string{"1", "2", "3"} {
			id := mesocycle.ExerciseID(string(mg) + "_exercise_" + n)
			if _, ok := lib.Lookup(id); !ok {
				t.Errorf("expected to find %s in the real dummy content", id)
			}
		}
	}

	ex, ok := lib.Lookup("lower_exercise_1")
	if !ok {
		t.Fatal("expected to find lower_exercise_1 in the real dummy content")
	}
	if ex.Name == "" {
		t.Error("expected a non-empty exercise Name")
	}
	if ex.Standard.Sets == 0 {
		t.Error("expected a non-zero Standard.Sets")
	}
	if ex.Reduced.Sets == 0 {
		t.Error("expected a non-zero Reduced.Sets")
	}
}

// ============================================================================
// PrescriptionFor
// ============================================================================

func TestPrescriptionFor(t *testing.T) {
	ex := content.Exercise{
		ID:       "x",
		Standard: content.ExercisePrescription{Sets: 4, Reps: "6-8"},
		Reduced:  content.ExercisePrescription{Sets: 3, Reps: "8-10"},
	}

	got, err := content.PrescriptionFor(ex, cascade.LoadStandard)
	if err != nil || got != ex.Standard {
		t.Errorf("PrescriptionFor(Standard) = %+v, %v, want %+v, nil", got, err, ex.Standard)
	}

	got, err = content.PrescriptionFor(ex, cascade.LoadReduced)
	if err != nil || got != ex.Reduced {
		t.Errorf("PrescriptionFor(Reduced) = %+v, %v, want %+v, nil", got, err, ex.Reduced)
	}

	if _, err := content.PrescriptionFor(ex, "extreme"); err == nil {
		t.Error("expected an error for an unrecognized LoadTier")
	}
}

// ============================================================================
// In-memory fixtures
// ============================================================================

func TestExerciseLibrary_DuplicateIDErrors(t *testing.T) {
	dir := t.TempDir()
	writeJSONFixture(t, dir, "a.json", []content.Exercise{
		{ID: "dup", Name: "A", MuscleGroup: activity.MuscleGroupCore},
	})
	writeJSONFixture(t, dir, "b.json", []content.Exercise{
		{ID: "dup", Name: "B", MuscleGroup: activity.MuscleGroupCore},
	})

	if _, err := content.LoadExerciseLibrary(dir); err == nil {
		t.Fatal("expected an error for two exercises claiming the same ID")
	}
}

func TestExerciseLibrary_LookupMissingIDReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	writeJSONFixture(t, dir, "a.json", []content.Exercise{
		{ID: "real_one", Name: "A", MuscleGroup: activity.MuscleGroupCore},
	})

	lib, err := content.LoadExerciseLibrary(dir)
	if err != nil {
		t.Fatalf("LoadExerciseLibrary returned error: %v", err)
	}

	if _, ok := lib.Lookup(mesocycle.ExerciseID("not_real")); ok {
		t.Error("expected Lookup to return ok=false for an ID that isn't in the library")
	}
}
