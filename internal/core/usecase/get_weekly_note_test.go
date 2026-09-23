package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

type fakeWeeklyNoteGenerator struct {
	note  string
	err   error
	got   usecase.WeeklyNoteInput
	calls int
}

func (f *fakeWeeklyNoteGenerator) GenerateNote(_ context.Context, input usecase.WeeklyNoteInput) (string, error) {
	f.got = input
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if f.note == "" {
		return "You've got this today.", nil
	}
	return f.note, nil
}

func TestGetWeeklyNote_HappyPath(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	mondayTraining := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false)
	mondayTraining.Weekday = "monday"
	seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		mondayTraining,
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	gen := &fakeWeeklyNoteGenerator{note: "Strong start to the week."}
	uc := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen))

	out, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Found {
		t.Fatal("expected Found = true")
	}
	if out.Note != "Strong start to the week." {
		t.Errorf("Note = %q, want %q", out.Note, "Strong start to the week.")
	}
	if gen.got.TodayWeekday != "monday" {
		t.Errorf("generator input TodayWeekday = %q, want %q", gen.got.TodayWeekday, "monday")
	}
}

func TestGetWeeklyNote_NoDraftCoveringDate_NotFound(t *testing.T) {
	drafts := &fakeDraftRepo{}
	gen := &fakeWeeklyNoteGenerator{}
	uc := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen))

	out, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: time.Now()})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Found {
		t.Error("expected Found = false when no week covers the date")
	}
}

func TestGetWeeklyNote_SecondCallForSameDateIsServedFromCacheNoLLMCall(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	mondayTraining := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false)
	mondayTraining.Weekday = "monday"
	draft := seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		mondayTraining,
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	gen := &fakeWeeklyNoteGenerator{note: "Strong start to the week."}
	uc := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen))

	first, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("generator calls after first request = %d, want 1", gen.calls)
	}
	if drafts.setNoteCalls != 1 {
		t.Fatalf("SetNote calls after first request = %d, want 1", drafts.setNoteCalls)
	}

	second, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if gen.calls != 1 {
		t.Errorf("generator calls after second request = %d, want still 1 — should be served from cache", gen.calls)
	}
	if second.Note != first.Note {
		t.Errorf("second.Note = %q, want %q (same cached note)", second.Note, first.Note)
	}

	if cached := drafts.byUser["u1"][draft.ID].Notes["2026-05-04"]; cached != "Strong start to the week." {
		t.Errorf("cached note = %q, want %q", cached, "Strong start to the week.")
	}
}

func TestGetWeeklyNote_DifferentDatesInSameWeekAreCachedSeparately(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)
	mondayTraining := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false)
	mondayTraining.Weekday = "monday"
	tuesdayTraining := trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, false)
	tuesdayTraining.Weekday = "tuesday"
	seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		mondayTraining, tuesdayTraining,
		restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	gen := &fakeWeeklyNoteGenerator{note: "Note text."}
	uc := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen))

	if _, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday}); err != nil {
		t.Fatalf("Execute(monday) returned error: %v", err)
	}
	if _, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: tuesday}); err != nil {
		t.Fatalf("Execute(tuesday) returned error: %v", err)
	}

	if gen.calls != 2 {
		t.Errorf("generator calls = %d, want 2 — a cache hit for monday must not satisfy a request for tuesday", gen.calls)
	}
}

func TestGetWeeklyNote_CacheWriteFailureStillReturnsTheGeneratedNote(t *testing.T) {
	drafts := &fakeDraftRepo{setNoteErr: fmt.Errorf("firestore: simulated outage")}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	mondayTraining := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false)
	mondayTraining.Weekday = "monday"
	seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		mondayTraining,
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	gen := &fakeWeeklyNoteGenerator{note: "Strong start to the week."}
	uc := usecase.NewGetWeeklyNoteUseCase(drafts, usecase.NewWeeklyNoteUsecase(gen))

	out, err := uc.Execute(context.Background(), usecase.GetWeeklyNoteInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute returned error even though only the cache write failed: %v", err)
	}
	if !out.Found || out.Note != "Strong start to the week." {
		t.Errorf("out = %+v, want the generated note returned despite the cache write failing", out)
	}
}
