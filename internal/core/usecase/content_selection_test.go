package usecase_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
	"viv/internal/core/mesocycle"
	"viv/internal/core/usecase"
)

// ============================================================================
// Fixtures
// ============================================================================

func writeContentFixture(t *testing.T, dir, filename string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		t.Fatalf("writing fixture %s: %v", filename, err)
	}
}

func buildSessionLibrary(t *testing.T, sessions ...content.Session) *content.SessionLibrary {
	t.Helper()
	dir := t.TempDir()
	for i, s := range sessions {
		writeContentFixture(t, dir, "session_"+string(rune('a'+i))+".json", s)
	}
	lib, err := content.LoadSessionLibrary(dir)
	if err != nil {
		t.Fatalf("LoadSessionLibrary: %v", err)
	}
	return lib
}

func buildExerciseLibrary(t *testing.T, exercises ...content.Exercise) *content.ExerciseLibrary {
	t.Helper()
	dir := t.TempDir()
	writeContentFixture(t, dir, "exercises.json", exercises)
	lib, err := content.LoadExerciseLibrary(dir)
	if err != nil {
		t.Fatalf("LoadExerciseLibrary: %v", err)
	}
	return lib
}

func buildWarmupCooldownLibrary(t *testing.T, pairs ...content.MesocycleWarmupCooldown) *content.MesocycleWarmupCooldownLibrary {
	t.Helper()
	dir := t.TempDir()
	for i, p := range pairs {
		writeContentFixture(t, dir, "wc_"+string(rune('a'+i))+".json", p)
	}
	lib, err := content.LoadMesocycleWarmupCooldownLibrary(dir)
	if err != nil {
		t.Fatalf("LoadMesocycleWarmupCooldownLibrary: %v", err)
	}
	return lib
}

func lowerWarmupCooldown() content.MesocycleWarmupCooldown {
	return content.MesocycleWarmupCooldown{
		MuscleGroup: activity.MuscleGroupLower,
		Warmup:      content.ContentBlock{DurationMinutes: 5, Description: "Leg prep.", Movements: []string{"Leg swings"}},
		Cooldown:    content.ContentBlock{DurationMinutes: 5, Description: "Leg stretch.", Movements: []string{"Quad stretch"}},
	}
}

// fakePinResolver is a fake usecase.ExercisePinResolver.
type fakePinResolver struct {
	ids       []mesocycle.ExerciseID
	err       error
	callCount int
	lastUser  string
	lastType  activity.ID
	lastGroup activity.MuscleGroup
}

func (f *fakePinResolver) ResolvePinnedExercises(_ context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup) ([]mesocycle.ExerciseID, error) {
	f.callCount++
	f.lastUser, f.lastType, f.lastGroup = userID, activityType, muscleGroup
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}

// oneDayDraft builds a WeekDraft with day 0 set to day0 and every other
// day explicitly marked as a rest day — the zero-value DayPlan is NOT a
// rest day (IsRestDay defaults to false), so leaving days 1-6
// unspecified would make SelectContent try to hydrate them too.
func oneDayDraft(userID string, day0 usecase.DayPlan) usecase.WeekDraft {
	draft := usecase.WeekDraft{UserID: userID}
	draft.Days[0] = day0
	for i := 1; i < 7; i++ {
		draft.Days[i] = usecase.DayPlan{IsRestDay: true}
	}
	return draft
}

func standardYogaSession() content.Session {
	return content.Session{
		ID: "yoga_std", ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
		DurationTier: cascade.DurationStandard, ComplexityTier: cascade.ComplexityStandard, LoadTier: cascade.LoadStandard,
		Content: content.SessionContent{MainExercises: []content.ExerciseDetail{{Name: "Flow"}}},
	}
}

// ============================================================================
// A standard-tier slot selects normally
// ============================================================================

func TestSessionContentSelector_StandardTierSlotSelectsNormally(t *testing.T) {
	sessions := buildSessionLibrary(t, standardYogaSession())
	exercises := buildExerciseLibrary(t)
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t), &fakePinResolver{})

	draft := oneDayDraft("u1", usecase.DayPlan{
		Assignment: cascade.SlotAssignment{ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody},
	})

	got, err := selector.SelectContent(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Days[0].Content == nil || got.Days[0].Content.Session == nil {
		t.Fatal("expected Content.Session to be set")
	}
	if got.Days[0].Content.Session.ID != "yoga_std" {
		t.Errorf("selected session ID = %q, want yoga_std", got.Days[0].Content.Session.ID)
	}
	if got.Days[0].Content.Exercises != nil {
		t.Error("expected Content.Exercises to stay nil for a non-mesocycle activity")
	}
}

// ============================================================================
// A slot requesting a variant that doesn't exist errors clearly rather
// than silently degrading to the standard version.
// ============================================================================

func TestSessionContentSelector_MissingVariantErrorsLoudly(t *testing.T) {
	// Library has ONLY the standard yoga session — no "short" variant.
	sessions := buildSessionLibrary(t, standardYogaSession())
	exercises := buildExerciseLibrary(t)
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t), &fakePinResolver{})

	draft := oneDayDraft("u1", usecase.DayPlan{
		Assignment: cascade.SlotAssignment{
			ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody,
			DurationTier: cascade.DurationShort, // requested variant doesn't exist in the library
		},
	})

	_, err := selector.SelectContent(context.Background(), draft)
	if err == nil {
		t.Fatal("expected an error for a requested variant that doesn't exist in the library")
	}
	if !strings.Contains(err.Error(), "variant") {
		t.Errorf("expected the error to explicitly call out a missing variant, got: %v", err)
	}
}

func TestSessionContentSelector_MissingStandardContentErrors(t *testing.T) {
	sessions := buildSessionLibrary(t) // empty library
	exercises := buildExerciseLibrary(t)
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t), &fakePinResolver{})

	draft := oneDayDraft("u1", usecase.DayPlan{
		Assignment: cascade.SlotAssignment{ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody},
	})

	_, err := selector.SelectContent(context.Background(), draft)
	if err == nil {
		t.Fatal("expected an error when no standard content exists either — never a silently empty session")
	}
}

// ============================================================================
// A Strength slot pulls from the pinned exercise set, not the general library
// ============================================================================

func lowerExercises() []content.Exercise {
	return []content.Exercise{
		{ID: "lower_exercise_1", Name: "Back Squat", MuscleGroup: activity.MuscleGroupLower,
			Standard: content.ExercisePrescription{Sets: 4, Reps: "6-8"}, Reduced: content.ExercisePrescription{Sets: 3, Reps: "8-10"}},
		{ID: "lower_exercise_2", Name: "RDL", MuscleGroup: activity.MuscleGroupLower,
			Standard: content.ExercisePrescription{Sets: 3, Reps: "8-10"}, Reduced: content.ExercisePrescription{Sets: 3, Reps: "10-12"}},
	}
}

func TestSessionContentSelector_StrengthSlotPullsFromPinnedSet(t *testing.T) {
	sessions := buildSessionLibrary(t) // deliberately empty — Strength must never touch this
	exercises := buildExerciseLibrary(t, lowerExercises()...)
	pins := &fakePinResolver{ids: []mesocycle.ExerciseID{"lower_exercise_1", "lower_exercise_2"}}
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t, lowerWarmupCooldown()), pins)

	draft := oneDayDraft("u1", usecase.DayPlan{
		Assignment: cascade.SlotAssignment{ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM, MuscleGroup: activity.MuscleGroupLower, LoadTier: cascade.LoadStandard},
	})

	got, err := selector.SelectContent(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pins.callCount != 1 {
		t.Fatalf("pin resolver called %d times, want 1", pins.callCount)
	}
	if pins.lastUser != "u1" || pins.lastType != activity.Strength || pins.lastGroup != activity.MuscleGroupLower {
		t.Errorf("pin resolver called with (%s, %s, %s), want (u1, strength, lower)", pins.lastUser, pins.lastType, pins.lastGroup)
	}

	if got.Days[0].Content == nil {
		t.Fatal("expected Content to be set")
	}
	if got.Days[0].Content.Session != nil {
		t.Error("expected Content.Session to stay nil for a mesocycle-eligible activity — must never free-select from the session library")
	}
	if len(got.Days[0].Content.Exercises) != 2 {
		t.Fatalf("Content.Exercises has %d entries, want 2 (exactly the pinned set)", len(got.Days[0].Content.Exercises))
	}
	if got.Days[0].Content.Exercises[0].Exercise.ID != "lower_exercise_1" || got.Days[0].Content.Exercises[1].Exercise.ID != "lower_exercise_2" {
		t.Errorf("exercises = %+v, want the pinned IDs in order", got.Days[0].Content.Exercises)
	}
	if got.Days[0].Content.Exercises[0].Prescription != (content.ExercisePrescription{Sets: 4, Reps: "6-8"}) {
		t.Errorf("expected the Standard prescription for LoadTier=standard, got %+v", got.Days[0].Content.Exercises[0].Prescription)
	}
	if got.Days[0].Content.Warmup == nil || len(got.Days[0].Content.Warmup.Movements) == 0 {
		t.Errorf("expected a generic warmup for the pinned muscle group, got %+v", got.Days[0].Content.Warmup)
	}
	if got.Days[0].Content.Cooldown == nil || len(got.Days[0].Content.Cooldown.Movements) == 0 {
		t.Errorf("expected a generic cooldown for the pinned muscle group, got %+v", got.Days[0].Content.Cooldown)
	}
}

func TestSessionContentSelector_StrengthSlotUsesReducedPrescriptionForLoadTierReduced(t *testing.T) {
	sessions := buildSessionLibrary(t)
	exercises := buildExerciseLibrary(t, lowerExercises()...)
	pins := &fakePinResolver{ids: []mesocycle.ExerciseID{"lower_exercise_1", "lower_exercise_2"}}
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t, lowerWarmupCooldown()), pins)

	draft := oneDayDraft("u1", usecase.DayPlan{
		Assignment: cascade.SlotAssignment{ActivityType: activity.Strength, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupLower, LoadTier: cascade.LoadReduced, AdjustmentLever: cascade.AdjustmentProtectType},
	})

	got, err := selector.SelectContent(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Days[0].Content.Exercises) != 2 {
		t.Fatalf("expected the same 2 pinned exercises regardless of LoadTier, got %d", len(got.Days[0].Content.Exercises))
	}
	if got.Days[0].Content.Exercises[0].Exercise.ID != "lower_exercise_1" {
		t.Error("expected the exercise SELECTION to stay the pinned set even when LoadTier changes")
	}
	if got.Days[0].Content.Exercises[0].Prescription != (content.ExercisePrescription{Sets: 3, Reps: "8-10"}) {
		t.Errorf("expected the Reduced prescription for LoadTier=reduced, got %+v", got.Days[0].Content.Exercises[0].Prescription)
	}
}

func TestSessionContentSelector_PinnedExerciseNotInLibraryErrors(t *testing.T) {
	sessions := buildSessionLibrary(t)
	exercises := buildExerciseLibrary(t) // empty — the pinned ID won't be found
	pins := &fakePinResolver{ids: []mesocycle.ExerciseID{"ghost_exercise"}}
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t), pins)

	draft := oneDayDraft("u1", usecase.DayPlan{
		Assignment: cascade.SlotAssignment{ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM, MuscleGroup: activity.MuscleGroupLower, LoadTier: cascade.LoadStandard},
	})

	_, err := selector.SelectContent(context.Background(), draft)
	if err == nil {
		t.Fatal("expected an error when a pinned exercise ID isn't in the exercise library")
	}
}

// ============================================================================
// Rest days are skipped
// ============================================================================

func TestSessionContentSelector_RestDaysSkipped(t *testing.T) {
	sessions := buildSessionLibrary(t)
	exercises := buildExerciseLibrary(t)
	pins := &fakePinResolver{}
	selector := usecase.NewSessionContentSelector(sessions, exercises, buildWarmupCooldownLibrary(t), pins)

	draft := oneDayDraft("u1", usecase.DayPlan{IsRestDay: true})

	got, err := selector.SelectContent(context.Background(), draft)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Days[0].Content != nil {
		t.Error("expected no Content for a rest day")
	}
	if pins.callCount != 0 {
		t.Error("expected the pin resolver never to be called for a rest day")
	}
}
