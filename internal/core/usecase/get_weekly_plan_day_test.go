package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
	"viv/internal/core/goal"
	"viv/internal/core/mesocycle"
	"viv/internal/core/usecase"
)

func TestGetWeeklyPlanDay_SessionLibraryPath(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	yogaDay := trainingDay(activity.Yoga, activity.IntensityM, activity.ImpactL, false)
	yogaDay.Content = &usecase.SelectedContent{Session: &content.Session{
		DurationMinutes: 35,
		Content: content.SessionContent{
			Warmup:        content.ContentBlock{DurationMinutes: 5, Description: "Breath-led warmup.", Movements: []string{"Seated breathing", "Cat-cow"}},
			MainExercises: []content.ExerciseDetail{{Name: "Power flow", Sets: 3, Reps: "1 flow", RestSeconds: 20, LoadGuidance: "Bodyweight", FormCue: "Breathe with the movement"}},
			Cooldown:      content.ContentBlock{DurationMinutes: 5, Description: "Seated stretch.", Movements: []string{"Forward fold", "Savasana"}},
		},
	}}
	seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		yogaDay, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewGetWeeklyPlanDayUseCase(drafts, &fakeSessionLogRepo{})
	out, err := uc.Execute(context.Background(), usecase.GetWeeklyPlanDayInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Found {
		t.Fatal("expected Found = true")
	}

	d := out.Day
	if d.IsRestDay {
		t.Fatal("expected a training day")
	}
	if d.Title != "Full Body Yoga" {
		t.Errorf("Title = %q, want %q", d.Title, "Full Body Yoga")
	}
	if d.LoadLabel != "Moderate load" {
		t.Errorf("LoadLabel = %q, want %q", d.LoadLabel, "Moderate load")
	}
	if d.Loggable {
		t.Error("session-library days should not be Loggable")
	}
	if d.DurationIsEstimated {
		t.Error("session-library duration should not be marked as estimated")
	}
	if d.DurationMinutes != 35 {
		t.Errorf("DurationMinutes = %d, want 35", d.DurationMinutes)
	}
	if d.Warmup == nil || len(d.Warmup.Movements) != 2 {
		t.Fatalf("Warmup = %+v, want 2 movements", d.Warmup)
	}
	if d.Cooldown == nil || d.Cooldown.Description != "Seated stretch." {
		t.Fatalf("Cooldown = %+v", d.Cooldown)
	}
	if len(d.MainExercises) != 1 || d.MainExercises[0].Name != "Power flow" {
		t.Fatalf("MainExercises = %+v", d.MainExercises)
	}
	if d.MainExercises[0].ID != "" {
		t.Errorf("session-library exercises shouldn't have a stable ID, got %q", d.MainExercises[0].ID)
	}
}

func TestGetWeeklyPlanDay_MesocyclePinnedPath(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	strengthDay := usecase.DayPlan{
		Assignment: cascade.SlotAssignment{
			ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactM,
			MuscleGroup: activity.MuscleGroupLower,
		},
	}
	strengthWarmup := content.ContentBlock{DurationMinutes: 5, Description: "Leg prep.", Movements: []string{"Leg swings", "Walking lunges"}}
	strengthCooldown := content.ContentBlock{DurationMinutes: 5, Description: "Leg stretch.", Movements: []string{"Quad stretch"}}
	strengthDay.Content = &usecase.SelectedContent{
		Exercises: []usecase.PrescribedExercise{
			{
				Exercise:     content.Exercise{ID: mesocycle.ExerciseID("lower_exercise_1"), Name: "Front foot elevated split squat", MuscleGroup: activity.MuscleGroupLower},
				Prescription: content.ExercisePrescription{Sets: 3, Reps: "8-10 /leg", RestSeconds: 90, LoadGuidance: "RPE 7", FormCue: "Control the descent"},
			},
			{
				Exercise:     content.Exercise{ID: mesocycle.ExerciseID("lower_exercise_2"), Name: "Single-leg Romanian deadlift", MuscleGroup: activity.MuscleGroupLower},
				Prescription: content.ExercisePrescription{Sets: 3, Reps: "8-10 /leg", RestSeconds: 90, LoadGuidance: "RPE 7", FormCue: "Hinge at the hips"},
			},
		},
		// A real generation run populates these from
		// content.MesocycleWarmupCooldownLibrary (generic per muscle
		// group) — seeded directly here since this test builds the draft
		// by hand rather than running SessionContentSelector.
		Warmup:   &strengthWarmup,
		Cooldown: &strengthCooldown,
	}
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		strengthDay, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewGetWeeklyPlanDayUseCase(drafts, &fakeSessionLogRepo{})
	out, err := uc.Execute(context.Background(), usecase.GetWeeklyPlanDayInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	d := out.Day
	if d.Title != "Lower Strength" {
		t.Errorf("Title = %q, want %q", d.Title, "Lower Strength")
	}
	if !d.Loggable {
		t.Error("mesocycle-pinned days should be Loggable")
	}
	if d.Warmup == nil || len(d.Warmup.Movements) != 2 {
		t.Errorf("expected the generic warmup to come through, got %+v", d.Warmup)
	}
	if d.Cooldown == nil || d.Cooldown.Description != "Leg stretch." {
		t.Errorf("expected the generic cooldown to come through, got %+v", d.Cooldown)
	}
	if !d.DurationIsEstimated {
		t.Error("mesocycle-pinned duration should be marked as estimated")
	}
	if d.DurationMinutes <= 0 {
		t.Errorf("DurationMinutes = %d, want a positive estimate", d.DurationMinutes)
	}
	if len(d.MainExercises) != 2 {
		t.Fatalf("MainExercises = %+v, want 2", d.MainExercises)
	}
	if d.MainExercises[0].ID != "lower_exercise_1" {
		t.Errorf("MainExercises[0].ID = %q, want %q", d.MainExercises[0].ID, "lower_exercise_1")
	}
	if d.MainExercises[0].Sets != 3 || d.MainExercises[0].Reps != "8-10 /leg" {
		t.Errorf("MainExercises[0] prescription mismatch: %+v", d.MainExercises[0])
	}
}

func TestGetWeeklyPlanDay_RestDay(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewGetWeeklyPlanDayUseCase(drafts, &fakeSessionLogRepo{})
	out, err := uc.Execute(context.Background(), usecase.GetWeeklyPlanDayInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Found {
		t.Fatal("expected Found = true")
	}
	if !out.Day.IsRestDay {
		t.Error("expected IsRestDay = true")
	}
	if out.Day.Title != "" || len(out.Day.MainExercises) != 0 {
		t.Errorf("rest day should carry no session detail, got %+v", out.Day)
	}
}

func TestGetWeeklyPlanDay_NoPlanCoversDate(t *testing.T) {
	drafts := &fakeDraftRepo{}
	uc := usecase.NewGetWeeklyPlanDayUseCase(drafts, &fakeSessionLogRepo{})

	out, err := uc.Execute(context.Background(), usecase.GetWeeklyPlanDayInput{
		UserID: "u1", Date: time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Found {
		t.Error("expected Found = false when no plan covers the date")
	}
}
