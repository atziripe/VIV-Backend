package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// fakeDailyCheckinRepo is an in-memory usecase.DailyCheckinRepository,
// keyed by (userID, date) exactly like the real Firestore doc-ID scheme —
// Upsert overwrites the same key rather than appending, which is what
// makes idempotency observable in tests (map length stays 1 across
// repeated submissions for the same date).
type fakeDailyCheckinRepo struct {
	byKey map[string]*domain.DailyCheckin
}

func (f *fakeDailyCheckinRepo) Upsert(_ context.Context, c *domain.DailyCheckin) error {
	if f.byKey == nil {
		f.byKey = map[string]*domain.DailyCheckin{}
	}
	key := c.UserID + "|" + c.Date.Format("2006-01-02")
	cp := *c
	f.byKey[key] = &cp
	return nil
}

// spyDailyAdapter is a usecase.DailyAdapter fake that records every call
// it receives and returns a configurable, fixed output — used to verify
// SubmitDailyCheckinUseCase's dispatch logic and data threading without
// re-exercising AdaptDailySlotUsecase's own cascade math (covered by
// adapt_daily_slot_test.go).
type spyDailyAdapter struct {
	calls []usecase.AdaptDailySlotInput
	out   usecase.AdaptDailySlotOutput
	err   error
}

func (s *spyDailyAdapter) Execute(_ context.Context, input usecase.AdaptDailySlotInput) (usecase.AdaptDailySlotOutput, error) {
	s.calls = append(s.calls, input)
	if s.err != nil {
		return usecase.AdaptDailySlotOutput{}, s.err
	}
	return s.out, nil
}

// weekDraftCovering builds a 7-day WeekDraft (VIV-106 shape) whose
// StartDate is weekStart and whose day 0 carries dayZero — enough to
// exercise dayIndexForDate/GetByDate lookups keyed on weekStart itself.
func weekDraftCovering(weekStart time.Time, g goal.ID, dayZero usecase.DayPlan) usecase.WeekDraft {
	dayZero.Date = weekStart
	days := [7]usecase.DayPlan{dayZero}
	for i := 1; i < 7; i++ {
		days[i] = usecase.DayPlan{Date: weekStart.AddDate(0, 0, i), IsRestDay: true}
	}
	return usecase.WeekDraft{
		ID:        "draft-1",
		UserID:    "u1",
		StartDate: weekStart,
		EndDate:   weekStart.AddDate(0, 0, 6),
		Status:    "draft",
		GoalID:    g,
		Days:      days,
	}
}

func newSubmitCheckinUC(
	checkins *fakeDailyCheckinRepo,
	drafts *fakeDraftRepo,
	users *fakeUserRepo,
	generator *spyWeeklyPlanGenerator,
	adapter *spyDailyAdapter,
) *usecase.SubmitDailyCheckinUseCase {
	return usecase.NewSubmitDailyCheckinUseCase(checkins, drafts, users, generator, adapter, nil)
}

// spyNutritionResync is a usecase.NutritionResyncer fake that records every
// call it receives — used to verify SubmitDailyCheckinUseCase only triggers
// a resync when the training week actually changed (a fresh generation, or
// an adaptation with Changed=true), not on every check-in.
type spyNutritionResync struct {
	calls []usecase.WeekDraft
}

func (s *spyNutritionResync) Execute(_ context.Context, _ string, draft usecase.WeekDraft) error {
	s.calls = append(s.calls, draft)
	return nil
}

var okCheckin = checkin.DailyCheckin{
	Sleep:  checkin.SleepNormal,
	Body:   checkin.BodyNormal,
	Demand: checkin.DemandNormal,
	Need:   checkin.NeedMeetMeWhereImAt,
}

// ============================================================================
// No plan covers today → generation, not adaptation
// ============================================================================

func TestSubmitDailyCheckin_NoExistingPlan_TriggersGenerationNotAdaptation(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	checkins := &fakeDailyCheckinRepo{}
	drafts := &fakeDraftRepo{} // empty — no plan covers `date`
	users := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", GoalID: goal.ConsistencyWellbeing, UserCatalog: []activity.ID{activity.Strength}},
	}}
	generator := &spyWeeklyPlanGenerator{
		draft: weekDraftCovering(date, goal.ConsistencyWellbeing, trainingDay(activity.Strength, activity.IntensityM, activity.ImpactM, false)),
	}
	adapter := &spyDailyAdapter{}
	uc := newSubmitCheckinUC(checkins, drafts, users, generator, adapter)

	out, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{
		UserID: "u1", Date: date, Answers: okCheckin,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(generator.calls) != 1 {
		t.Errorf("generator calls = %d, want 1", len(generator.calls))
	}
	if len(adapter.calls) != 0 {
		t.Errorf("adapter calls = %d, want 0 (should not run daily adaptation)", len(adapter.calls))
	}
	if !out.Regenerated {
		t.Error("expected Regenerated = true")
	}

	// The just-computed readiness was passed as an explicit override, not
	// left for GenerateWeeklyPlanUsecase to (re-)derive from a checkin.
	call := generator.calls[0]
	if call.Readiness == nil {
		t.Fatal("expected an explicit Readiness override on the generator call")
	}
	wantReadiness, _ := checkin.Derive(okCheckin)
	if *call.Readiness != wantReadiness {
		t.Errorf("call.Readiness = %+v, want %+v", *call.Readiness, wantReadiness)
	}
	// The user's real catalog/goal — not the pipeline's own defaults —
	// flow into a rollover/first-ever generation too.
	if call.GoalID != goal.ConsistencyWellbeing {
		t.Errorf("call.GoalID = %q, want %q", call.GoalID, goal.ConsistencyWellbeing)
	}
	if len(call.Catalog.Activities) != 1 || call.Catalog.Activities[0] != activity.Strength {
		t.Errorf("call.Catalog.Activities = %v, want [strength]", call.Catalog.Activities)
	}
}

// ============================================================================
// A plan already covers today → adaptation, not regeneration
// ============================================================================

func TestSubmitDailyCheckin_ExistingPlan_TriggersAdaptationNotRegeneration(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	checkins := &fakeDailyCheckinRepo{}
	drafts := &fakeDraftRepo{}
	seedDraft(t, drafts, "u1", date, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, false),
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	generator := &spyWeeklyPlanGenerator{}
	adapter := &spyDailyAdapter{out: usecase.AdaptDailySlotOutput{
		Date:       date,
		Assignment: cascade.SlotAssignment{ActivityType: activity.Yoga, Intensity: activity.IntensityL, Impact: activity.ImpactL},
		Changed:    false,
	}}
	uc := newSubmitCheckinUC(checkins, drafts, users, generator, adapter)

	out, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{
		UserID: "u1", Date: date, Answers: okCheckin,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(adapter.calls) != 1 {
		t.Errorf("adapter calls = %d, want 1", len(adapter.calls))
	}
	if len(generator.calls) != 0 {
		t.Errorf("generator calls = %d, want 0 (should not regenerate the week)", len(generator.calls))
	}
	if out.Regenerated {
		t.Error("expected Regenerated = false")
	}
}

// ============================================================================
// Idempotency: a second submission for the same date updates, not
// duplicates — and downstream reflects the updated answers.
// ============================================================================

func TestSubmitDailyCheckin_SecondSubmissionSameDate_UpdatesAndDownstreamReflectsIt(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	checkins := &fakeDailyCheckinRepo{}
	drafts := &fakeDraftRepo{}
	seedDraft(t, drafts, "u1", date, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, false),
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	generator := &spyWeeklyPlanGenerator{}
	adapter := &spyDailyAdapter{}
	uc := newSubmitCheckinUC(checkins, drafts, users, generator, adapter)

	firstAnswers := checkin.DailyCheckin{
		Sleep: checkin.SleepDeepAndRestful, Body: checkin.BodyStrongAndResponsive,
		Demand: checkin.DemandLightAndOpen, Need: checkin.NeedPushMe,
	}
	secondAnswers := checkin.DailyCheckin{
		Sleep: checkin.SleepBarelySlept, Body: checkin.BodySensitiveOrReactive,
		Demand: checkin.DemandPacked, Need: checkin.NeedLetMeReset,
	}

	if _, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: firstAnswers}); err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if _, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: secondAnswers}); err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}

	if len(checkins.byKey) != 1 {
		t.Fatalf("stored check-in records = %d, want 1 (update, not duplicate)", len(checkins.byKey))
	}
	stored := checkins.byKey["u1|2026-05-04"]
	if stored == nil {
		t.Fatal("expected a stored record under the (userID, date) key")
	}
	if stored.Answers != secondAnswers {
		t.Errorf("stored answers = %+v, want the second submission's %+v", stored.Answers, secondAnswers)
	}
	wantReadiness, _ := checkin.Derive(secondAnswers)
	if stored.Readiness != wantReadiness {
		t.Errorf("stored readiness = %+v, want readiness derived from the second submission %+v", stored.Readiness, wantReadiness)
	}

	// Downstream (the adapter call) saw the second submission's answers,
	// not the first's.
	if len(adapter.calls) != 2 {
		t.Fatalf("adapter calls = %d, want 2", len(adapter.calls))
	}
	if adapter.calls[1].Checkin != secondAnswers {
		t.Errorf("second adapter call's Checkin = %+v, want the second submission %+v", adapter.calls[1].Checkin, secondAnswers)
	}
}

// ============================================================================
// Response reason: present when an adjustment applies, empty ("no
// change") when it doesn't — for both the adaptation and generation paths.
// ============================================================================

func TestSubmitDailyCheckin_AdaptationPath_ReasonPresentOnlyWhenChanged(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	t.Run("no change", func(t *testing.T) {
		drafts := &fakeDraftRepo{}
		seedDraft(t, drafts, "u1", date, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
			trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, false),
			restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
		})
		adapter := &spyDailyAdapter{out: usecase.AdaptDailySlotOutput{Changed: false, Reason: ""}}
		uc := newSubmitCheckinUC(&fakeDailyCheckinRepo{}, drafts, users, &spyWeeklyPlanGenerator{}, adapter)

		out, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: okCheckin})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out.Reason != "" {
			t.Errorf("Reason = %q, want empty (no change)", out.Reason)
		}
	})

	t.Run("adjusted", func(t *testing.T) {
		drafts := &fakeDraftRepo{}
		seedDraft(t, drafts, "u1", date, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
			trainingDay(activity.Yoga, activity.IntensityH, activity.ImpactH, false),
			restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
		})
		wantReason := usecase.AdjustmentReason(cascade.AdjustmentRelaxIntensity)
		adapter := &spyDailyAdapter{out: usecase.AdaptDailySlotOutput{Changed: true, Reason: wantReason}}
		uc := newSubmitCheckinUC(&fakeDailyCheckinRepo{}, drafts, users, &spyWeeklyPlanGenerator{}, adapter)

		out, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: okCheckin})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if out.Reason == "" {
			t.Error("expected a non-empty Reason when an adjustment applies")
		}
		if out.Reason != wantReason {
			t.Errorf("Reason = %q, want %q", out.Reason, wantReason)
		}
	})
}

func TestSubmitDailyCheckin_GenerationPath_ReasonFromAdjustmentLever(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	adjustedDay := trainingDay(activity.Strength, activity.IntensityL, activity.ImpactL, false)
	adjustedDay.Assignment.AdjustmentLever = cascade.AdjustmentRelaxIntensity
	generator := &spyWeeklyPlanGenerator{draft: weekDraftCovering(date, goal.ConsistencyWellbeing, adjustedDay)}

	uc := newSubmitCheckinUC(&fakeDailyCheckinRepo{}, &fakeDraftRepo{}, users, generator, &spyDailyAdapter{})

	out, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: okCheckin})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	want := usecase.AdjustmentReason(cascade.AdjustmentRelaxIntensity)
	if out.Reason != want {
		t.Errorf("Reason = %q, want %q", out.Reason, want)
	}
}

// ============================================================================
// Nutrition resync: only triggered when the training week actually changed.
// ============================================================================

func TestSubmitDailyCheckin_GenerationPath_TriggersNutritionResync(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	generator := &spyWeeklyPlanGenerator{
		draft: weekDraftCovering(date, goal.ConsistencyWellbeing, trainingDay(activity.Strength, activity.IntensityM, activity.ImpactM, false)),
	}
	resync := &spyNutritionResync{}
	uc := usecase.NewSubmitDailyCheckinUseCase(&fakeDailyCheckinRepo{}, &fakeDraftRepo{}, users, generator, &spyDailyAdapter{}, resync)

	if _, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: okCheckin}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(resync.calls) != 1 {
		t.Fatalf("resync calls = %d, want 1 (a fresh week always resyncs nutrition)", len(resync.calls))
	}
}

func TestSubmitDailyCheckin_AdaptationPath_ResyncsOnlyWhenChanged(t *testing.T) {
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}

	t.Run("not changed — no resync", func(t *testing.T) {
		drafts := &fakeDraftRepo{}
		seedDraft(t, drafts, "u1", date, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
			trainingDay(activity.Yoga, activity.IntensityL, activity.ImpactL, false),
			restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
		})
		adapter := &spyDailyAdapter{out: usecase.AdaptDailySlotOutput{Changed: false}}
		resync := &spyNutritionResync{}
		uc := usecase.NewSubmitDailyCheckinUseCase(&fakeDailyCheckinRepo{}, drafts, users, &spyWeeklyPlanGenerator{}, adapter, resync)

		if _, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: okCheckin}); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if len(resync.calls) != 0 {
			t.Errorf("resync calls = %d, want 0 (nothing about the week changed)", len(resync.calls))
		}
	})

	t.Run("changed — resyncs", func(t *testing.T) {
		drafts := &fakeDraftRepo{}
		seedDraft(t, drafts, "u1", date, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
			trainingDay(activity.Yoga, activity.IntensityH, activity.ImpactH, false),
			restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
		})
		adapter := &spyDailyAdapter{out: usecase.AdaptDailySlotOutput{Changed: true, Reason: usecase.AdjustmentReason(cascade.AdjustmentRelaxIntensity)}}
		resync := &spyNutritionResync{}
		uc := usecase.NewSubmitDailyCheckinUseCase(&fakeDailyCheckinRepo{}, drafts, users, &spyWeeklyPlanGenerator{}, adapter, resync)

		if _, err := uc.Execute(context.Background(), usecase.SubmitDailyCheckinInput{UserID: "u1", Date: date, Answers: okCheckin}); err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if len(resync.calls) != 1 {
			t.Errorf("resync calls = %d, want 1 (the adaptation changed today's slot)", len(resync.calls))
		}
	})
}
