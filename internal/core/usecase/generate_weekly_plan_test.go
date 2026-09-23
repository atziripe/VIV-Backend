package usecase_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// ============================================================================
// Fakes
// ============================================================================

type fakePhaseLookup struct {
	phase domain.CyclePhase
	err   error
}

func (f fakePhaseLookup) CurrentPhase(_ context.Context, _ string) (domain.CyclePhase, error) {
	return f.phase, f.err
}

// fakeDraftRepo is an in-memory WeeklyPlanDraftRepository shared by both
// the VIV-106 (generation) and VIV-107 (adaptation/manual edit) tests in
// this package.
type fakeDraftRepo struct {
	saved      *usecase.WeekDraft // last SaveDraft result — kept for the pre-existing VIV-106 tests
	err        error              // SaveDraft error
	getErr     error
	updateErr  error
	setNoteErr error

	setNoteCalls int

	byUser map[string]map[string]*usecase.WeekDraft // userID -> draftID -> draft
}

func (f *fakeDraftRepo) SaveDraft(_ context.Context, draft *usecase.WeekDraft) error {
	if f.err != nil {
		return f.err
	}
	if draft.ID == "" {
		draft.ID = "draft-1"
	}
	cp := *draft
	f.saved = &cp

	if f.byUser == nil {
		f.byUser = map[string]map[string]*usecase.WeekDraft{}
	}
	if f.byUser[draft.UserID] == nil {
		f.byUser[draft.UserID] = map[string]*usecase.WeekDraft{}
	}
	stored := *draft
	f.byUser[draft.UserID][draft.ID] = &stored
	return nil
}

func (f *fakeDraftRepo) GetByDate(_ context.Context, userID string, date time.Time) (*usecase.WeekDraft, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, d := range f.byUser[userID] {
		if !date.Before(d.StartDate) && !date.After(d.EndDate) {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeDraftRepo) UpdateDaySlot(_ context.Context, userID, draftID string, dayIndex int, day usecase.DayPlan) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if dayIndex < 0 || dayIndex > 6 {
		return fmt.Errorf("fakeDraftRepo: dayIndex %d out of range", dayIndex)
	}
	d, ok := f.byUser[userID][draftID]
	if !ok {
		return fmt.Errorf("fakeDraftRepo: draft %s not found for user %s", draftID, userID)
	}
	d.Days[dayIndex] = day
	d.Notes = nil // mirrors FirestoreWeeklyPlanDraftRepository.UpdateDaySlot
	return nil
}

func (f *fakeDraftRepo) SetNote(_ context.Context, userID, draftID, dateKey, note string) error {
	if f.setNoteErr != nil {
		return f.setNoteErr
	}
	d, ok := f.byUser[userID][draftID]
	if !ok {
		return fmt.Errorf("fakeDraftRepo: draft %s not found for user %s", draftID, userID)
	}
	if d.Notes == nil {
		d.Notes = map[string]string{}
	}
	d.Notes[dateKey] = note
	f.setNoteCalls++
	return nil
}

// spyLayer records that it ran (and in what order, via the shared calls
// slice) and passes its input through unchanged — same behavior as the
// production Noop* stubs, just observable.
type spyLayer struct {
	name  string
	calls *[]string
}

func (s spyLayer) Schedule(_ context.Context, draft usecase.WeekDraft, _ string) (usecase.WeekDraft, error) {
	*s.calls = append(*s.calls, s.name)
	return draft, nil
}
func (s spyLayer) Validate(_ context.Context, draft usecase.WeekDraft) (usecase.WeekDraft, error) {
	*s.calls = append(*s.calls, s.name)
	return draft, nil
}
func (s spyLayer) Apply(_ context.Context, draft usecase.WeekDraft) (usecase.WeekDraft, error) {
	*s.calls = append(*s.calls, s.name)
	return draft, nil
}
func (s spyLayer) SelectContent(_ context.Context, draft usecase.WeekDraft) (usecase.WeekDraft, error) {
	*s.calls = append(*s.calls, s.name)
	return draft, nil
}

func newTestUsecase(phase fakePhaseLookup, drafts *fakeDraftRepo) *usecase.GenerateWeeklyPlanUsecase {
	return usecase.NewGenerateWeeklyPlanUsecase(
		phase,
		usecase.NoopSchedulingLayer{},
		usecase.NoopValidationEngine{},
		usecase.NoopWarningsOverridesLayer{},
		usecase.NoopContentSelectionLayer{},
		drafts,
	)
}

// ============================================================================
// TestGenerateWeeklyPlan_FullEndToEnd_StrengthMuscle_HighReadinessPullBack
//
// The required integration-style test: runs the full 0a→0b→stub-layers→
// persist sequence for one full goal × readiness combination and confirms
// a complete, valid 7-day plan comes out.
// ============================================================================

func TestGenerateWeeklyPlan_FullEndToEnd_StrengthMuscle_HighReadinessPullBack(t *testing.T) {
	drafts := &fakeDraftRepo{}
	uc := newTestUsecase(fakePhaseLookup{phase: domain.PhaseFollicular}, drafts)

	dailyCheckin := checkin.DailyCheckin{
		Sleep:  checkin.SleepDeepAndRestful,     // +2
		Body:   checkin.BodyStrongAndResponsive, // +2 => recovery=4 => High
		Demand: checkin.DemandLightAndOpen,      // => BandwidthHigh
		Need:   checkin.NeedLetMeReset,          // => BuildPullBack
	}

	catalog := cascade.UserCatalog{
		Activities: []activity.ID{
			activity.Strength, activity.Running, activity.Pilates, activity.Yoga, activity.Mobility,
		},
	}

	generationDate := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC) // a Monday

	out, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "user-1",
		GenerationDate: generationDate,
		Checkin:        &dailyCheckin,
		GoalID:         goal.StrengthMuscle,
		Catalog:        catalog,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	draft := out.Draft

	// Readiness/target sanity: this exact combination is independently
	// verified in weeklytarget's own test table (RecoveryHigh/BandwidthHigh/
	// PullBack for strength_muscle => IntensityCap=M, ImpactCap=H, budget=5).
	if draft.Readiness.RecoveryCapacity != checkin.RecoveryHigh {
		t.Errorf("RecoveryCapacity = %s, want High", draft.Readiness.RecoveryCapacity)
	}
	if draft.Target.SessionBudget != 5 {
		t.Fatalf("SessionBudget = %d, want 5", draft.Target.SessionBudget)
	}
	if draft.Target.IntensityCap != activity.IntensityM || draft.Target.ImpactCap != activity.ImpactH {
		t.Errorf("caps = (%s,%s), want (M,H)", draft.Target.IntensityCap, draft.Target.ImpactCap)
	}

	if draft.UserID != "user-1" {
		t.Errorf("UserID = %s, want user-1", draft.UserID)
	}
	if draft.GoalID != goal.StrengthMuscle {
		t.Errorf("GoalID = %s, want %s", draft.GoalID, goal.StrengthMuscle)
	}
	if !draft.StartDate.Equal(generationDate) {
		t.Errorf("StartDate = %v, want %v", draft.StartDate, generationDate)
	}
	if !draft.EndDate.Equal(generationDate.AddDate(0, 0, 6)) {
		t.Errorf("EndDate = %v, want start+6days", draft.EndDate)
	}
	if draft.Status != "draft" {
		t.Errorf("Status = %q, want %q", draft.Status, "draft")
	}

	trainingDays := 0
	restDays := 0
	for i, day := range draft.Days {
		if day.Date.IsZero() {
			t.Fatalf("day %d has zero Date — not all 7 days were populated", i)
		}
		if day.IsRestDay {
			restDays++
			if day.Assignment != (cascade.SlotAssignment{}) {
				t.Errorf("day %d is a rest day but has a non-zero Assignment: %+v", i, day.Assignment)
			}
			continue
		}
		trainingDays++
		if day.Assignment.ActivityType == "" {
			t.Errorf("day %d is a training day but has no ActivityType assigned", i)
		}
	}

	if trainingDays != 5 {
		t.Errorf("trainingDays = %d, want 5 (the target's SessionBudget)", trainingDays)
	}
	if restDays != 2 {
		t.Errorf("restDays = %d, want 2", restDays)
	}

	// The draft must actually have been persisted.
	if drafts.saved == nil {
		t.Fatal("expected SaveDraft to have been called")
	}
	if drafts.saved.ID != "draft-1" {
		t.Errorf("persisted draft ID = %q, want draft-1", drafts.saved.ID)
	}
	if draft.ID != "draft-1" {
		t.Errorf("returned draft ID = %q, want draft-1 (should reflect what SaveDraft assigned)", draft.ID)
	}
}

// ============================================================================
// Defaults
// ============================================================================

func TestGenerateWeeklyPlan_DefaultsAppliedWhenInputsOmitted(t *testing.T) {
	drafts := &fakeDraftRepo{}
	uc := newTestUsecase(fakePhaseLookup{phase: domain.PhaseFollicular}, drafts)

	out, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "new-user",
		GenerationDate: time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC),
		// Checkin, GoalID, Catalog all omitted — first-ever week for a new user.
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	draft := out.Draft
	if draft.GoalID != usecase.DefaultGoalID {
		t.Errorf("GoalID = %s, want default %s", draft.GoalID, usecase.DefaultGoalID)
	}

	wantReadiness, err := checkin.Derive(usecase.DefaultDailyCheckin)
	if err != nil {
		t.Fatalf("DefaultDailyCheckin itself failed to derive: %v", err)
	}
	if draft.Readiness != wantReadiness {
		t.Errorf("Readiness = %+v, want %+v (derived from DefaultDailyCheckin)", draft.Readiness, wantReadiness)
	}

	for i, day := range draft.Days {
		if day.IsRestDay {
			continue
		}
		if _, ok := activity.ByID(day.Assignment.ActivityType); !ok {
			t.Errorf("day %d assigned an activity type not in the full taxonomy default catalog: %q",
				i, day.Assignment.ActivityType)
		}
	}
}

// ============================================================================
// Error paths
// ============================================================================

func TestGenerateWeeklyPlan_UnknownGoalIDErrors(t *testing.T) {
	uc := newTestUsecase(fakePhaseLookup{phase: domain.PhaseFollicular}, &fakeDraftRepo{})

	_, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "u1",
		GenerationDate: time.Now(),
		GoalID:         "not_a_real_goal",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown GoalID")
	}
}

func TestGenerateWeeklyPlan_CatalogWithUnknownActivityErrors(t *testing.T) {
	uc := newTestUsecase(fakePhaseLookup{phase: domain.PhaseFollicular}, &fakeDraftRepo{})

	_, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "u1",
		GenerationDate: time.Now(),
		Catalog:        cascade.UserCatalog{Activities: []activity.ID{"not_a_real_activity"}},
	})
	if err == nil {
		t.Fatal("expected an error for a catalog referencing an unknown activity type")
	}
}

func TestGenerateWeeklyPlan_EmptyUserIDErrors(t *testing.T) {
	uc := newTestUsecase(fakePhaseLookup{phase: domain.PhaseFollicular}, &fakeDraftRepo{})

	_, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "   ",
		GenerationDate: time.Now(),
	})
	if err == nil {
		t.Fatal("expected an error for an empty userID")
	}
}

func TestGenerateWeeklyPlan_PhaseLookupErrorPropagates(t *testing.T) {
	uc := newTestUsecase(fakePhaseLookup{err: context.DeadlineExceeded}, &fakeDraftRepo{})

	_, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "u1",
		GenerationDate: time.Now(),
	})
	if err == nil {
		t.Fatal("expected the phase lookup error to propagate")
	}
}

func TestGenerateWeeklyPlan_SaveDraftErrorPropagates(t *testing.T) {
	uc := newTestUsecase(fakePhaseLookup{phase: domain.PhaseFollicular}, &fakeDraftRepo{err: context.DeadlineExceeded})

	_, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "u1",
		GenerationDate: time.Now(),
	})
	if err == nil {
		t.Fatal("expected the SaveDraft error to propagate")
	}
}

// ============================================================================
// Stage wiring
// ============================================================================

func TestGenerateWeeklyPlan_StubLayersCalledInOrder(t *testing.T) {
	var calls []string
	uc := usecase.NewGenerateWeeklyPlanUsecase(
		fakePhaseLookup{phase: domain.PhaseFollicular},
		spyLayer{name: "scheduling", calls: &calls},
		spyLayer{name: "validation", calls: &calls},
		spyLayer{name: "overrides", calls: &calls},
		spyLayer{name: "content", calls: &calls},
		&fakeDraftRepo{},
	)

	_, err := uc.Execute(context.Background(), usecase.GenerateWeeklyPlanInput{
		UserID:         "u1",
		GenerationDate: time.Now(),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	want := []string{"scheduling", "validation", "overrides", "content"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("calls[%d] = %s, want %s (order: %v)", i, calls[i], want[i], calls)
		}
	}
}

// TestNoopLayers_PassDataThroughUnchanged confirms the production stub
// implementations really are no-ops, as the task requires: "stub/no-op
// implementations that just pass the data through unchanged."
func TestNoopLayers_PassDataThroughUnchanged(t *testing.T) {
	draft := usecase.WeekDraft{UserID: "u1", Status: "draft"}
	ctx := context.Background()

	got, err := usecase.NoopSchedulingLayer{}.Schedule(ctx, draft, "")
	if err != nil || !reflect.DeepEqual(got, draft) {
		t.Errorf("NoopSchedulingLayer changed the draft or errored: %+v, %v", got, err)
	}
	got, err = usecase.NoopValidationEngine{}.Validate(ctx, draft)
	if err != nil || !reflect.DeepEqual(got, draft) {
		t.Errorf("NoopValidationEngine changed the draft or errored: %+v, %v", got, err)
	}
	got, err = usecase.NoopWarningsOverridesLayer{}.Apply(ctx, draft)
	if err != nil || !reflect.DeepEqual(got, draft) {
		t.Errorf("NoopWarningsOverridesLayer changed the draft or errored: %+v, %v", got, err)
	}
	got, err = usecase.NoopContentSelectionLayer{}.SelectContent(ctx, draft)
	if err != nil || !reflect.DeepEqual(got, draft) {
		t.Errorf("NoopContentSelectionLayer changed the draft or errored: %+v, %v", got, err)
	}
}
