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
	"viv/internal/core/strengthlibrary"
	"viv/internal/core/strengthsession"
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

func TestGetWeeklyPlanDay_StrengthBlockPath(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	strengthDay := usecase.DayPlan{
		Assignment: cascade.SlotAssignment{
			ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactM,
			MuscleGroup: activity.MuscleGroupLower,
		},
	}
	assembled := &strengthsession.AssembledSession{
		Exercises: []strengthsession.AssembledExercise{
			{ExerciseID: "lower_back_squat", Slot: "Squat", Priority: strengthlibrary.PriorityMain, ExerciseName: "Back Squat", Equipment: []string{"Barbell", "Rack"}, PrescriptionText: "4 × 6-8 @ RIR 2-3", Sets: 4},
			{ExerciseID: "lower_hip_thrust_a", Slot: "Hip Thrust", Priority: strengthlibrary.PriorityAccessory, ExerciseName: "Hip Thrust A", Equipment: []string{"Barbell"}, PrescriptionText: "3 × 10-12 @ RIR 2-3", Sets: 3},
		},
		Week:                 3,
		BlockNumber:          2,
		ConfigStatus:         "draft_not_reviewed_by_sport_scientist",
		TierChanges:          []strengthsession.TierChange{{Lever: strengthsession.LeverLoadReduced, Detail: "using week 1's prescription for every exercise, regardless of the actual current week"}},
		Warmup:               strengthsession.AssembledWarmupCooldown{DurationMinutes: 7, Movements: []string{"Glute bridges", "Bodyweight squats"}},
		Cooldown:             strengthsession.AssembledWarmupCooldown{DurationMinutes: 5, Movements: []string{"Hamstring stretch"}},
		RampUpNote:           "Before the first Main exercise, do 1-2 light sets at about half the working load.",
		WarmupCooldownStatus: "draft_llm_generated_unreviewed",
	}
	strengthDay.Content = &usecase.SelectedContent{
		StrengthBlock: assembled,
		// Mirrors exactly what usecase.selectStrengthBlock derives from
		// the same AssembledSession — this test builds the draft by hand
		// rather than running SessionContentSelector.
		Warmup:   &content.ContentBlock{DurationMinutes: assembled.Warmup.DurationMinutes, Movements: assembled.Warmup.Movements},
		Cooldown: &content.ContentBlock{DurationMinutes: assembled.Cooldown.DurationMinutes, Movements: assembled.Cooldown.Movements},
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
	if !d.Loggable {
		t.Error("expected a real VIV-112 Strength assembly day to be Loggable")
	}
	if d.StrengthBlockNumber != 2 {
		t.Errorf("StrengthBlockNumber = %d, want 2", d.StrengthBlockNumber)
	}
	if d.StrengthWeek != 3 {
		t.Errorf("StrengthWeek = %d, want 3", d.StrengthWeek)
	}
	if d.StrengthConfigStatus != "draft_not_reviewed_by_sport_scientist" {
		t.Errorf("StrengthConfigStatus = %q, want the config's provisional status to come through verbatim", d.StrengthConfigStatus)
	}
	if len(d.StrengthTierChanges) != 1 || d.StrengthTierChanges[0].Lever != strengthsession.LeverLoadReduced {
		t.Fatalf("StrengthTierChanges = %+v, want the single LoadReduced change", d.StrengthTierChanges)
	}
	if len(d.MainExercises) != 2 {
		t.Fatalf("MainExercises = %+v, want 2", d.MainExercises)
	}
	if d.MainExercises[0].Name != "Back Squat" || d.MainExercises[0].Slot != "Squat" || d.MainExercises[0].Priority != "Main" {
		t.Errorf("MainExercises[0] = %+v, want Back Squat/Squat/Main", d.MainExercises[0])
	}
	if d.MainExercises[0].PrescriptionText != "4 × 6-8 @ RIR 2-3" {
		t.Errorf("PrescriptionText = %q, want the config-authored text kept as written", d.MainExercises[0].PrescriptionText)
	}
	if len(d.MainExercises[0].Equipment) != 2 {
		t.Errorf("Equipment = %+v, want 2 entries", d.MainExercises[0].Equipment)
	}
	// This path never decomposes the prescription text itself, but ID
	// and Sets ARE populated (needed to log sets against this day) —
	// Reps/RestSeconds/LoadGuidance/FormCue stay zero/empty.
	if d.MainExercises[0].ID != "lower_back_squat" {
		t.Errorf("ID = %q, want the stable exercise ID so a client can log sets against it", d.MainExercises[0].ID)
	}
	if d.MainExercises[0].Sets != 4 {
		t.Errorf("Sets = %d, want 4 (parsed off the prescription text's leading \"N ×\")", d.MainExercises[0].Sets)
	}
	if d.MainExercises[0].Reps != "" {
		t.Errorf("expected Reps to stay empty for the Strength assembly path, got %+v", d.MainExercises[0])
	}
	if d.Warmup == nil || len(d.Warmup.Movements) != 2 || d.Warmup.DurationMinutes != 7 {
		t.Errorf("Warmup = %+v, want the zone's own warm-up (2 movements, 7 minutes)", d.Warmup)
	}
	if d.Cooldown == nil || len(d.Cooldown.Movements) != 1 || d.Cooldown.DurationMinutes != 5 {
		t.Errorf("Cooldown = %+v, want the zone's own cool-down (1 movement, 5 minutes)", d.Cooldown)
	}
	if d.StrengthRampUpNote != "Before the first Main exercise, do 1-2 light sets at about half the working load." {
		t.Errorf("StrengthRampUpNote = %q, want the config's ramp_up_note verbatim", d.StrengthRampUpNote)
	}
	if d.StrengthWarmupCooldownStatus != "draft_llm_generated_unreviewed" {
		t.Errorf("StrengthWarmupCooldownStatus = %q, want warmup_cooldown._status verbatim", d.StrengthWarmupCooldownStatus)
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
