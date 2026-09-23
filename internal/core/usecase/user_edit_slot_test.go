package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// TestUserEditSlot_ClearsCachedWeeklyNotes is a regression test for the
// weekly-note cache (see get_weekly_note_test.go): WeeklyNoteInput
// summarizes the whole week, not just one day, so a manual edit anywhere
// in the week can make every already-cached note stale — not just the one
// for the edited date. UserEditSlotUsecase persists through
// WeeklyPlanDraftRepository.UpdateDaySlot, which is where that
// invalidation actually happens (see fakeDraftRepo.UpdateDaySlot /
// FirestoreWeeklyPlanDraftRepository.UpdateDaySlot).
func TestUserEditSlot_ClearsCachedWeeklyNotes(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)
	mondayTraining := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false)
	mondayTraining.Weekday = "monday"
	draft := seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		mondayTraining,
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	gen := &fakeWeeklyNoteGenerator{note: "Strong start to the week."}
	noteUC := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen))

	// Prime the cache for monday.
	if _, err := noteUC.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday}); err != nil {
		t.Fatalf("priming Execute returned error: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("generator calls after priming = %d, want 1", gen.calls)
	}
	if len(drafts.byUser["u1"][draft.ID].Notes) == 0 {
		t.Fatal("expected the note cache to be primed before the edit")
	}

	// Edit a *different* day (tuesday) — the cached monday note still
	// covers a week whose shape just changed.
	editUC := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	_, err := editUC.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   tuesday,
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.Yoga,
			Intensity:    activity.IntensityL,
			Impact:       activity.ImpactL,
			MuscleGroup:  activity.MuscleGroupFullBody,
		},
	})
	if err != nil {
		t.Fatalf("UserEditSlotUsecase.Execute returned error: %v", err)
	}

	if got := len(drafts.byUser["u1"][draft.ID].Notes); got != 0 {
		t.Errorf("cached notes after the edit = %d, want 0 — editing any day must invalidate the whole draft's note cache", got)
	}

	// Asking for monday's note again must regenerate, not reuse the
	// (correctly discarded) stale cache entry.
	if _, err := noteUC.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday}); err != nil {
		t.Fatalf("post-edit Execute returned error: %v", err)
	}
	if gen.calls != 2 {
		t.Errorf("generator calls after the post-edit request = %d, want 2 — the cache was invalidated, so this must hit the LLM again", gen.calls)
	}
}
