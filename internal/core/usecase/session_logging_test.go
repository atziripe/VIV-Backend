package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/content"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/mesocycle"
	"viv/internal/core/usecase"
)

// fakeSessionLogRepo is an in-memory usecase.SessionLogRepository, keyed
// by (userID, date) exactly like the real Firestore doc-ID scheme —
// Upsert overwrites the same key, matching DailyCheckinRepository's
// idempotent-by-date convention.
type fakeSessionLogRepo struct {
	logs map[string]*domain.SessionLog
}

func (f *fakeSessionLogRepo) key(userID string, date time.Time) string {
	return userID + "|" + date.Format("2006-01-02")
}

func (f *fakeSessionLogRepo) GetByDate(_ context.Context, userID string, date time.Time) (*domain.SessionLog, error) {
	if f.logs == nil {
		return nil, nil
	}
	l, ok := f.logs[f.key(userID, date)]
	if !ok {
		return nil, nil
	}
	cp := *l
	return &cp, nil
}

func (f *fakeSessionLogRepo) Upsert(_ context.Context, log *domain.SessionLog) error {
	if f.logs == nil {
		f.logs = map[string]*domain.SessionLog{}
	}
	cp := *log
	f.logs[f.key(log.UserID, log.Date)] = &cp
	return nil
}

// seedLoggableDay seeds a draft with a single mesocycle-pinned (Strength)
// day at sessionMonday, with two prescribed exercises (3 sets and 2 sets), and
// returns the draft's repo for reuse across Start/LogSet/Complete.
func seedLoggableDay(t *testing.T, sessionMonday time.Time) *fakeDraftRepo {
	t.Helper()
	drafts := &fakeDraftRepo{}
	day := usecase.DayPlan{
		Assignment: cascade.SlotAssignment{
			ActivityType: activity.Strength, Intensity: activity.IntensityM, Impact: activity.ImpactL,
			MuscleGroup: activity.MuscleGroupLower,
		},
		Content: &usecase.SelectedContent{Exercises: []usecase.PrescribedExercise{
			{
				Exercise:     content.Exercise{ID: mesocycle.ExerciseID("lower_exercise_1"), Name: "Back Squat"},
				Prescription: content.ExercisePrescription{Sets: 3, Reps: "6-8"},
			},
			{
				Exercise:     content.Exercise{ID: mesocycle.ExerciseID("lower_exercise_2"), Name: "RDL"},
				Prescription: content.ExercisePrescription{Sets: 2, Reps: "8-10"},
			},
		}},
	}
	seedDraft(t, drafts, "u1", sessionMonday, goal.StrengthMuscle, [7]usecase.DayPlan{
		day, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})
	return drafts
}

var sessionMonday = time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

// ============================================================================
// Start
// ============================================================================

func TestStartSession_CreatesInProgressLog(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	uc := usecase.NewStartSessionUseCase(drafts, logs)

	out, err := uc.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Log.Status != domain.SessionLogInProgress {
		t.Errorf("Status = %q, want %q", out.Log.Status, domain.SessionLogInProgress)
	}
	if out.Log.StartedAt.IsZero() {
		t.Error("expected a non-zero StartedAt")
	}
	if out.Log.ActivityType != string(activity.Strength) {
		t.Errorf("ActivityType = %q, want %q", out.Log.ActivityType, activity.Strength)
	}

	stored, err := logs.GetByDate(context.Background(), "u1", sessionMonday)
	if err != nil || stored == nil {
		t.Fatalf("expected the log to be persisted, got %v, err=%v", stored, err)
	}
}

func TestStartSession_CalledTwice_ResumesWithoutResettingStartedAt(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	uc := usecase.NewStartSessionUseCase(drafts, logs)

	first, err := uc.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}

	second, err := uc.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if !second.Log.StartedAt.Equal(first.Log.StartedAt) {
		t.Errorf("StartedAt changed on resume: first=%v second=%v", first.Log.StartedAt, second.Log.StartedAt)
	}
}

func TestStartSession_RestDayRejected(t *testing.T) {
	drafts := &fakeDraftRepo{}
	seedDraft(t, drafts, "u1", sessionMonday, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})
	uc := usecase.NewStartSessionUseCase(drafts, &fakeSessionLogRepo{})

	_, err := uc.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	if err == nil {
		t.Fatal("expected an error for a rest day")
	}
	var validationErr usecase.SessionLoggingValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("error = %v (%T), want usecase.SessionLoggingValidationError", err, err)
	}
}

func TestStartSession_NonLoggableDayRejected(t *testing.T) {
	drafts := &fakeDraftRepo{}
	yogaDay := usecase.DayPlan{
		Assignment: cascade.SlotAssignment{ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL, MuscleGroup: activity.MuscleGroupFullBody},
		Content:    &usecase.SelectedContent{Session: &content.Session{}},
	}
	seedDraft(t, drafts, "u1", sessionMonday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		yogaDay, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})
	uc := usecase.NewStartSessionUseCase(drafts, &fakeSessionLogRepo{})

	_, err := uc.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	if err == nil {
		t.Fatal("expected an error for a non-loggable (session-library) day")
	}
}

// ============================================================================
// LogSet
// ============================================================================

func TestLogSet_RecordsAndIsIdempotentPerExerciseAndSetNumber(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	logSetUC := usecase.NewLogSetUseCase(drafts, logs)

	if _, err := startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday}); err != nil {
		t.Fatalf("start returned error: %v", err)
	}

	out, err := logSetUC.Execute(context.Background(), usecase.LogSetInput{
		UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_1", SetNumber: 1, WeightKg: 60, Reps: 8,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(out.Log.Sets) != 1 || out.Log.Sets[0].WeightKg != 60 || out.Log.Sets[0].Reps != 8 {
		t.Fatalf("Sets = %+v, want one entry at 60kg x 8", out.Log.Sets)
	}

	// Re-logging the same (exercise, set) updates in place, not a duplicate.
	out2, err := logSetUC.Execute(context.Background(), usecase.LogSetInput{
		UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_1", SetNumber: 1, WeightKg: 62.5, Reps: 7,
	})
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if len(out2.Log.Sets) != 1 {
		t.Fatalf("Sets = %d entries, want 1 (update, not duplicate)", len(out2.Log.Sets))
	}
	if out2.Log.Sets[0].WeightKg != 62.5 || out2.Log.Sets[0].Reps != 7 {
		t.Errorf("Sets[0] = %+v, want the updated 62.5kg x 7", out2.Log.Sets[0])
	}
}

func TestLogSet_UnknownExerciseRejected(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	logSetUC := usecase.NewLogSetUseCase(drafts, logs)
	_, _ = startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})

	_, err := logSetUC.Execute(context.Background(), usecase.LogSetInput{
		UserID: "u1", Date: sessionMonday, ExerciseID: "ghost_exercise", SetNumber: 1, WeightKg: 20, Reps: 10,
	})
	if err == nil {
		t.Fatal("expected an error for an exercise not part of today's session")
	}
}

func TestLogSet_SetNumberOutOfRangeRejected(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	logSetUC := usecase.NewLogSetUseCase(drafts, logs)
	_, _ = startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})

	// lower_exercise_2 is prescribed 2 sets — set 3 doesn't exist.
	_, err := logSetUC.Execute(context.Background(), usecase.LogSetInput{
		UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_2", SetNumber: 3, WeightKg: 20, Reps: 10,
	})
	if err == nil {
		t.Fatal("expected an error for a set number beyond the prescribed count")
	}
}

func TestLogSet_WithoutStartingFirstRejected(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logSetUC := usecase.NewLogSetUseCase(drafts, &fakeSessionLogRepo{})

	_, err := logSetUC.Execute(context.Background(), usecase.LogSetInput{
		UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_1", SetNumber: 1, WeightKg: 20, Reps: 10,
	})
	if err == nil {
		t.Fatal("expected an error when no session was started")
	}
}

func TestLogSet_NegativeWeightRejected(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	logSetUC := usecase.NewLogSetUseCase(drafts, logs)
	_, _ = startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})

	_, err := logSetUC.Execute(context.Background(), usecase.LogSetInput{
		UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_1", SetNumber: 1, WeightKg: -5, Reps: 10,
	})
	if err == nil {
		t.Fatal("expected an error for a negative weight_kg")
	}
}

// ============================================================================
// Complete
// ============================================================================

func TestCompleteSession_MarksDoneAndComputesStats(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	logSetUC := usecase.NewLogSetUseCase(drafts, logs)
	completeUC := usecase.NewCompleteSessionUseCase(drafts, logs)

	_, _ = startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	_, _ = logSetUC.Execute(context.Background(), usecase.LogSetInput{UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_1", SetNumber: 1, WeightKg: 60, Reps: 8})
	_, _ = logSetUC.Execute(context.Background(), usecase.LogSetInput{UserID: "u1", Date: sessionMonday, ExerciseID: "lower_exercise_1", SetNumber: 2, WeightKg: 60, Reps: 8})

	out, err := completeUC.Execute(context.Background(), usecase.CompleteSessionInput{UserID: "u1", Date: sessionMonday, Feedback: "right"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Log.Status != domain.SessionLogDone {
		t.Errorf("Status = %q, want %q", out.Log.Status, domain.SessionLogDone)
	}
	if out.Log.EndedAt == nil {
		t.Fatal("expected a non-nil EndedAt")
	}
	if out.Log.Feedback != domain.FeedbackRight {
		t.Errorf("Feedback = %q, want %q", out.Log.Feedback, domain.FeedbackRight)
	}
	if out.SetsCompleted != 2 {
		t.Errorf("SetsCompleted = %d, want 2", out.SetsCompleted)
	}
	if out.SetsTotal != 5 { // 3 (exercise 1) + 2 (exercise 2), prescribed
		t.Errorf("SetsTotal = %d, want 5", out.SetsTotal)
	}
}

func TestCompleteSession_InvalidFeedbackRejected(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	completeUC := usecase.NewCompleteSessionUseCase(drafts, logs)
	_, _ = startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})

	_, err := completeUC.Execute(context.Background(), usecase.CompleteSessionInput{UserID: "u1", Date: sessionMonday, Feedback: "meh"})
	if err == nil {
		t.Fatal("expected an error for an unrecognized feedback value")
	}
}

func TestCompleteSession_AlreadyDoneRejected(t *testing.T) {
	drafts := seedLoggableDay(t, sessionMonday)
	logs := &fakeSessionLogRepo{}
	startUC := usecase.NewStartSessionUseCase(drafts, logs)
	completeUC := usecase.NewCompleteSessionUseCase(drafts, logs)
	_, _ = startUC.Execute(context.Background(), usecase.StartSessionInput{UserID: "u1", Date: sessionMonday})
	if _, err := completeUC.Execute(context.Background(), usecase.CompleteSessionInput{UserID: "u1", Date: sessionMonday}); err != nil {
		t.Fatalf("first complete returned error: %v", err)
	}

	_, err := completeUC.Execute(context.Background(), usecase.CompleteSessionInput{UserID: "u1", Date: sessionMonday})
	if err == nil {
		t.Fatal("expected an error when completing an already-done session again")
	}
}
