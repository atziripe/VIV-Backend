package usecase_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// ============================================================================
// The required architectural test: GenerateNote's input type structurally
// cannot carry mutable plan-state. This checks the type system, not any
// LLM behavior.
// ============================================================================

func TestWeeklyNoteGenerator_InputTypeCannotCarryMutablePlanState(t *testing.T) {
	ifaceType := reflect.TypeOf((*usecase.WeeklyNoteGenerator)(nil)).Elem()
	method, ok := ifaceType.MethodByName("GenerateNote")
	if !ok {
		t.Fatal("usecase.WeeklyNoteGenerator has no GenerateNote method")
	}
	if method.Type.NumIn() != 2 {
		t.Fatalf("GenerateNote has %d parameter(s), want 2 (context, input)", method.Type.NumIn())
	}

	inputType := method.Type.In(1)
	wantType := reflect.TypeOf(usecase.WeeklyNoteInput{})
	if inputType != wantType {
		t.Fatalf("GenerateNote's input type = %s, want %s — it must be the distinct read-only DTO, not something else",
			inputType, wantType)
	}

	// The actual guarantee this task asks for: the input type must not
	// BE, or contain anywhere in its fields, the live mutable plan types
	// — WeekDraft, DayPlan, or the repository that can persist changes to
	// them. If this fails, someone widened WeeklyNoteInput (or
	// GenerateNote's signature) to let the note generator see or hold a
	// reference to the real plan.
	forbidden := map[string]reflect.Type{
		"WeekDraft":                 reflect.TypeOf(usecase.WeekDraft{}),
		"*WeekDraft":                reflect.TypeOf(&usecase.WeekDraft{}),
		"DayPlan":                   reflect.TypeOf(usecase.DayPlan{}),
		"cascade.SlotAssignment":    reflect.TypeOf(cascade.SlotAssignment{}),
		"WeeklyPlanDraftRepository": reflect.TypeOf((*usecase.WeeklyPlanDraftRepository)(nil)).Elem(),
	}

	if inputType == forbidden["WeekDraft"] || inputType == forbidden["*WeekDraft"] {
		t.Fatalf("GenerateNote's input type must not itself be WeekDraft/*WeekDraft, got %s", inputType)
	}

	for name, bad := range forbidden {
		if containsType(inputType, bad, map[reflect.Type]bool{}) {
			t.Errorf("WeeklyNoteInput contains %s (%s) somewhere in its fields — the note generator must never be able to reach the live mutable plan", name, bad)
		}
	}

	// And every field on WeeklyNoteInput/WeeklyNoteDaySummary must be a
	// plain value (string/bool/array-of-those) — no pointers, no
	// interfaces, no funcs. A pointer field would be a channel back to
	// shared, mutable state even if it doesn't literally point at
	// WeekDraft today.
	assertOnlyPlainValueFields(t, inputType)
}

// containsType recursively checks whether t (or any field/element/key
// type reachable from it) equals target.
func containsType(t, target reflect.Type, seen map[reflect.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	if t == target {
		return true
	}
	switch t.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Chan:
		return containsType(t.Elem(), target, seen)
	case reflect.Map:
		return containsType(t.Key(), target, seen) || containsType(t.Elem(), target, seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if containsType(t.Field(i).Type, target, seen) {
				return true
			}
		}
	}
	return false
}

func assertOnlyPlainValueFields(t *testing.T, typ reflect.Type) {
	t.Helper()
	switch typ.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		t.Errorf("field type %s is not a plain value (kind=%s) — WeeklyNoteInput must only ever carry copied values", typ, typ.Kind())
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			assertOnlyPlainValueFields(t, typ.Field(i).Type)
		}
	case reflect.Array, reflect.Slice:
		assertOnlyPlainValueFields(t, typ.Elem())
	}
}

// ============================================================================
// SummarizeWeekForNote
// ============================================================================

func weekDraftForNote(weekStart time.Time) usecase.WeekDraft {
	return usecase.WeekDraft{
		GoalID:    goal.StrengthMuscle,
		StartDate: weekStart,
		Days: [7]usecase.DayPlan{
			{Date: weekStart, Weekday: "monday", Assignment: cascade.SlotAssignment{
				ActivityType: activity.Strength, Intensity: activity.IntensityH, Impact: activity.ImpactM, MuscleGroup: activity.MuscleGroupLower,
			}},
			{Date: weekStart.AddDate(0, 0, 1), Weekday: "tuesday", IsRestDay: true},
			{Date: weekStart.AddDate(0, 0, 2), Weekday: "wednesday", Assignment: cascade.SlotAssignment{
				ActivityType: activity.Running, Intensity: activity.IntensityL, Impact: activity.ImpactH, MuscleGroup: activity.MuscleGroupLower,
				Substituted: true, AdjustmentLever: cascade.AdjustmentSubstituteType,
			}},
			{Date: weekStart.AddDate(0, 0, 3), Weekday: "thursday", IsRestDay: true},
			{Date: weekStart.AddDate(0, 0, 4), Weekday: "friday", IsRestDay: true},
			{Date: weekStart.AddDate(0, 0, 5), Weekday: "saturday", IsRestDay: true},
			{Date: weekStart.AddDate(0, 0, 6), Weekday: "sunday", IsRestDay: true},
		},
	}
}

func TestSummarizeWeekForNote_ExtractsCopyToneAndDaySummaries(t *testing.T) {
	weekStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC) // Monday
	draft := weekDraftForNote(weekStart)

	g, ok := goal.ByID(goal.StrengthMuscle)
	if !ok {
		t.Fatal("goal.ByID(StrengthMuscle) not found")
	}

	input, err := usecase.SummarizeWeekForNote(draft, weekStart.AddDate(0, 0, 2)) // Wednesday
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if input.CopyTone != g.CopyTone {
		t.Errorf("CopyTone = %q, want %q", input.CopyTone, g.CopyTone)
	}
	if input.TodayWeekday != "wednesday" {
		t.Errorf("TodayWeekday = %q, want wednesday", input.TodayWeekday)
	}
	if input.Days[2].ActivityType != string(activity.Running) || !input.Days[2].Substituted {
		t.Errorf("Days[2] = %+v, want Running/Substituted", input.Days[2])
	}
	if input.Days[0].ActivityType != string(activity.Strength) {
		t.Errorf("Days[0] = %+v, want Strength", input.Days[0])
	}
	if !input.Days[1].IsRestDay || input.Days[1].ActivityType != "" {
		t.Errorf("Days[1] = %+v, want an empty rest day", input.Days[1])
	}
}

func TestSummarizeWeekForNote_UnknownGoalErrors(t *testing.T) {
	weekStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	draft := weekDraftForNote(weekStart)
	draft.GoalID = "not_a_real_goal"

	_, err := usecase.SummarizeWeekForNote(draft, weekStart)
	if err == nil {
		t.Fatal("expected an error for an unknown goal id")
	}
}

func TestSummarizeWeekForNote_DateNotInWeekErrors(t *testing.T) {
	weekStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	draft := weekDraftForNote(weekStart)

	_, err := usecase.SummarizeWeekForNote(draft, weekStart.AddDate(0, 0, 30))
	if err == nil {
		t.Fatal("expected an error for a date outside the week")
	}
}

// ============================================================================
// WeeklyNoteUsecase
// ============================================================================

type fakeNoteGenerator struct {
	lastInput usecase.WeeklyNoteInput
	note      string
	err       error
}

func (f *fakeNoteGenerator) GenerateNote(_ context.Context, input usecase.WeeklyNoteInput) (string, error) {
	f.lastInput = input
	if f.err != nil {
		return "", f.err
	}
	return f.note, nil
}

func TestWeeklyNoteUsecase_CallsGeneratorWithSummary(t *testing.T) {
	weekStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	draft := weekDraftForNote(weekStart)
	gen := &fakeNoteGenerator{note: "You've got this today."}
	uc := usecase.NewWeeklyNoteUsecase(gen)

	note, err := uc.Execute(context.Background(), draft, weekStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note != "You've got this today." {
		t.Errorf("note = %q, want the generator's note", note)
	}
	if gen.lastInput.TodayWeekday != "monday" {
		t.Errorf("generator received TodayWeekday = %q, want monday", gen.lastInput.TodayWeekday)
	}
}

func TestWeeklyNoteUsecase_GeneratorErrorPropagates(t *testing.T) {
	weekStart := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	draft := weekDraftForNote(weekStart)
	gen := &fakeNoteGenerator{err: context.DeadlineExceeded}
	uc := usecase.NewWeeklyNoteUsecase(gen)

	_, err := uc.Execute(context.Background(), draft, weekStart)
	if err == nil {
		t.Fatal("expected the generator's error to propagate")
	}
}
