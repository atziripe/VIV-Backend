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
	noteUC := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen), grantedConsentRepo("u1"))

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

// ============================================================================
// Manual edits are validated against the activity's own taxonomy entry
// ============================================================================
//
// Regression coverage for the gap that surfaced alongside the Running/
// Cycling/Swimming LoadTier=Reduced content gap: before this validation
// existed, a syntactically valid-looking edit (a known activity ID with
// an Intensity/Impact/MuscleGroup that activity never actually takes)
// sailed past this usecase's only check and failed later, opaquely,
// inside content selection's library lookup.

func seedSimpleDraft(t *testing.T, drafts *fakeDraftRepo, weekStart time.Time) {
	t.Helper()
	monday := trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, false)
	seedDraft(t, drafts, "u1", weekStart, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		monday, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})
}

func TestUserEditSlot_RejectsIntensityOutsideTaxonomyRange(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedSimpleDraft(t, drafts, weekStart)

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	// HIIT's IntensityRange is fixed to [H] — Low is never valid for it.
	_, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   weekStart,
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.HIIT, Intensity: activity.IntensityL, Impact: activity.ImpactH,
			MuscleGroup: activity.MuscleGroupFullBody,
		},
	})
	if err == nil {
		t.Fatal("expected an error for an intensity outside HIIT's taxonomy range, got nil")
	}
}

func TestUserEditSlot_RejectsImpactOutsideTaxonomyRange(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedSimpleDraft(t, drafts, weekStart)

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	// Running's ImpactRange is fixed to [H] — Low is never valid for it.
	_, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   weekStart,
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.Running, Intensity: activity.IntensityM, Impact: activity.ImpactL,
			MuscleGroup: activity.MuscleGroupLower,
		},
	})
	if err == nil {
		t.Fatal("expected an error for an impact outside Running's taxonomy range, got nil")
	}
}

func TestUserEditSlot_RejectsMuscleGroupOutsideTaxonomyDefault(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedSimpleDraft(t, drafts, weekStart)

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	// HIIT is UserSelectable in the taxonomy, but no selection mechanism
	// exists yet and content.SessionLibrary has no content for it outside
	// full_body — so a non-default muscle group must still be rejected.
	_, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   weekStart,
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH,
			MuscleGroup: activity.MuscleGroupLower,
		},
	})
	if err == nil {
		t.Fatal("expected an error for a non-default muscle group on a UserSelectable activity with no selection mechanism yet, got nil")
	}
}

func TestUserEditSlot_AcceptsAssignmentMatchingTaxonomy(t *testing.T) {
	drafts := &fakeDraftRepo{}
	weekStart := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedSimpleDraft(t, drafts, weekStart)

	uc := usecase.NewUserEditSlotUsecase(drafts, usecase.NoopContentSelectionLayer{})
	_, err := uc.Execute(context.Background(), usecase.UserEditSlotInput{
		UserID: "u1",
		Date:   weekStart,
		NewAssignment: cascade.SlotAssignment{
			ActivityType: activity.HIIT, Intensity: activity.IntensityH, Impact: activity.ImpactH,
			MuscleGroup: activity.MuscleGroupFullBody,
		},
	})
	if err != nil {
		t.Fatalf("expected a taxonomy-valid HIIT edit to succeed, got error: %v", err)
	}
}
