package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type fakeUserRepo struct {
	users map[string]*domain.User
}

// GetByID returns a copy, not the map's own pointer — matching a real
// repository (which deserializes a fresh object per call) closely enough
// that a caller mutating the returned user and then failing before Save
// can't accidentally leave a partial edit visible through repo.users.
func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (f *fakeUserRepo) Save(_ context.Context, user *domain.User) error {
	f.users[user.ID] = user
	return nil
}

// fakeWeeklyPlanGenerator is a minimal WeeklyPlanGenerator recording every
// call it received — shared shape with fakeDraftRepo's package (usecase_test).
type fakeWeeklyPlanGenerator struct {
	out   usecase.GenerateWeeklyPlanOutput
	err   error
	calls []usecase.GenerateWeeklyPlanInput
}

func (f *fakeWeeklyPlanGenerator) Execute(_ context.Context, in usecase.GenerateWeeklyPlanInput) (usecase.GenerateWeeklyPlanOutput, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return usecase.GenerateWeeklyPlanOutput{}, f.err
	}
	return f.out, nil
}

// weekDraftStartingMonday builds a 7-day WeekDraft starting at start
// (assumed Monday), every day a rest day by default — tests override
// specific days via the returned draft's Days array.
func weekDraftStartingMonday(id string, start time.Time) usecase.WeekDraft {
	draft := usecase.WeekDraft{
		ID:        id,
		StartDate: start,
		EndDate:   start.AddDate(0, 0, 6),
	}
	weekdays := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	for i := range draft.Days {
		draft.Days[i] = usecase.DayPlan{
			Date:      start.AddDate(0, 0, i),
			Weekday:   weekdays[i],
			IsRestDay: true,
		}
	}
	return draft
}

func TestLogPeriodStart_RecalibratesCycleWithoutCheckinGate(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {
			ID:             "u1",
			CycleDuration:  "28",
			PeriodDuration: "5",
			CycleDay:       20, // was deep into late luteal per the old anchor
			CyclePhase:     "late_luteal",
		},
	}}
	uc := usecase.NewLogPeriodStartUseCase(repo, nil, nil, nil)

	// Report the period starting today — no Date set, defaults to now.
	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.CycleDay != 1 {
		t.Errorf("CycleDay = %d, want 1 (today is the new anchor)", out.CycleDay)
	}
	if out.CurrentPhase != string(domain.PhaseMenstrual) {
		t.Errorf("CurrentPhase = %q, want %q", out.CurrentPhase, domain.PhaseMenstrual)
	}

	saved := repo.users["u1"]
	if saved.CycleAnchorAt == nil {
		t.Fatal("expected CycleAnchorAt to be set")
	}
	if saved.CycleDay != 1 {
		t.Errorf("persisted CycleDay = %d, want 1", saved.CycleDay)
	}
}

func TestLogPeriodStart_UnknownUserErrors(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewLogPeriodStartUseCase(repo, nil, nil, nil)

	_, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "ghost"})
	if err == nil {
		t.Fatal("expected an error for an unknown user")
	}
}

func TestLogPeriodStart_BackdatedReport(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleDay: 1},
	}}
	uc := usecase.NewLogPeriodStartUseCase(repo, nil, nil, nil)

	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1", Date: yesterday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.CycleDay != 2 {
		t.Errorf("CycleDay = %d, want 2 (period started yesterday)", out.CycleDay)
	}
}

func TestLogPeriodStart_LateReportRebuildsWeek(t *testing.T) {
	today := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) // a Wednesday
	anchor := today.AddDate(0, 0, -34)                    // expected Sep 12 on a 28-day cycle — 4 days late

	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleAnchorAt: &anchor},
	}}

	monday := today.AddDate(0, 0, -2)
	oldDraft := weekDraftStartingMonday("old-draft", monday)
	oldDraft.Days[2].IsRestDay = false // Wednesday (today) was a heavy day under the old prediction
	oldDraft.Days[2].Assignment = cascade.SlotAssignment{ActivityType: activity.Strength, Intensity: activity.IntensityH, MuscleGroup: activity.MuscleGroupFullBody}

	drafts := &fakeDraftRepo{byUser: map[string]map[string]*usecase.WeekDraft{
		"u1": {"old-draft": &oldDraft},
	}}

	newDraft := weekDraftStartingMonday("new-draft", today) // regenerated, anchored at today
	newDraft.Days[0].IsRestDay = true                       // today is now light/rest under the real anchor

	gen := &fakeWeeklyPlanGenerator{out: usecase.GenerateWeeklyPlanOutput{Draft: newDraft}}
	resync := &spyNutritionResync{}

	uc := usecase.NewLogPeriodStartUseCase(repo, drafts, gen, resync)

	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1", Date: today})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !out.WeekRebuilt {
		t.Fatal("expected WeekRebuilt to be true — the report landed 4 days off the prediction")
	}
	if len(gen.calls) != 1 {
		t.Fatalf("expected exactly 1 call to the generator, got %d", len(gen.calls))
	}
	if !gen.calls[0].GenerationDate.Equal(today) {
		t.Errorf("GenerationDate = %v, want %v", gen.calls[0].GenerationDate, today)
	}
	if len(out.Moved) == 0 {
		t.Fatal("expected at least one moved day (today's assignment changed from heavy Strength to rest)")
	}
	found := false
	for _, m := range out.Moved {
		if m.Date.Equal(today) && m.IsRestDay {
			found = true
		}
	}
	if !found {
		t.Errorf("expected today's rest-day change in Moved, got %+v", out.Moved)
	}
	if len(resync.calls) != 1 {
		t.Errorf("expected nutrition resync to be called once, got %d calls", len(resync.calls))
	}
}

func TestLogPeriodStart_OnTimeReportSkipsRebuild(t *testing.T) {
	today := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	anchor := today.AddDate(0, 0, -28) // expected exactly today on a 28-day cycle

	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleAnchorAt: &anchor},
	}}
	gen := &fakeWeeklyPlanGenerator{}
	uc := usecase.NewLogPeriodStartUseCase(repo, &fakeDraftRepo{}, gen, nil)

	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1", Date: today})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.WeekRebuilt {
		t.Error("expected WeekRebuilt to be false — the report matched the prediction exactly")
	}
	if len(gen.calls) != 0 {
		t.Errorf("expected the generator not to be called, got %d calls", len(gen.calls))
	}
}

func TestLogPeriodStart_FirstReportSkipsRebuild(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5"}, // no CycleAnchorAt yet
	}}
	gen := &fakeWeeklyPlanGenerator{}
	uc := usecase.NewLogPeriodStartUseCase(repo, &fakeDraftRepo{}, gen, nil)

	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.WeekRebuilt {
		t.Error("expected WeekRebuilt to be false — nothing was predicted yet to be wrong about")
	}
	if len(gen.calls) != 0 {
		t.Errorf("expected the generator not to be called, got %d calls", len(gen.calls))
	}
}

func TestLogPeriodStart_MissingCollaboratorsSkipRebuildGracefully(t *testing.T) {
	today := time.Now().UTC()          // must be real "now" — ApplyCycleStartOverride measures elapsed days against it
	anchor := today.AddDate(0, 0, -34) // 6 days late on a 28-day cycle — would normally rebuild

	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleAnchorAt: &anchor},
	}}
	uc := usecase.NewLogPeriodStartUseCase(repo, nil, nil, nil) // Drafts/Generator not wired

	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1", Date: today})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.WeekRebuilt {
		t.Error("expected WeekRebuilt to be false when Drafts/Generator aren't wired")
	}
	// The core cycle update must still have gone through.
	if out.CycleDay != 1 {
		t.Errorf("CycleDay = %d, want 1 — the core report shouldn't be blocked by a missing rebuild collaborator", out.CycleDay)
	}
}

func TestLogPeriodStart_RecalibratesCycleDuration(t *testing.T) {
	today := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	anchor := today.AddDate(0, 0, -33) // observed interval: 33 days, not the stored 28

	repo := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", CycleDuration: "28", PeriodDuration: "5", CycleAnchorAt: &anchor},
	}}
	uc := usecase.NewLogPeriodStartUseCase(repo, nil, nil, nil)

	out, err := uc.Execute(context.Background(), usecase.LogPeriodStartInput{UserID: "u1", Date: today})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !out.CycleDurationChanged {
		t.Error("expected CycleDurationChanged to be true")
	}
	if out.PreviousCycleDuration != 28 {
		t.Errorf("PreviousCycleDuration = %d, want 28", out.PreviousCycleDuration)
	}
	if repo.users["u1"].CycleDuration != "33" {
		t.Errorf("persisted CycleDuration = %q, want %q", repo.users["u1"].CycleDuration, "33")
	}
}
