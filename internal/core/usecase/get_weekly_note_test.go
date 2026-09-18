package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

type fakeWeeklyNoteGenerator struct {
	note string
	err  error
	got  usecase.WeeklyNoteInput
}

func (f *fakeWeeklyNoteGenerator) GenerateNote(_ context.Context, input usecase.WeeklyNoteInput) (string, error) {
	f.got = input
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
