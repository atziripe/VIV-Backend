package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/checkin"
	"viv/internal/core/content"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// fakeRecoveryCardProvider lets these tests fix the recovery card a rest
// day resolves to without exercising GetRecoveryCardUseCase's own derivation
// logic (covered by get_recovery_card_test.go).
type fakeRecoveryCardProvider struct {
	out usecase.GetRecoveryCardOutput
	err error
}

func (f fakeRecoveryCardProvider) Execute(_ context.Context, _ usecase.GetRecoveryCardInput) (usecase.GetRecoveryCardOutput, error) {
	return f.out, f.err
}

// loggableTrainingDay is a mesocycle-pinned (Loggable) day — the only
// shape with a real completion signal (a SessionLog).
func loggableTrainingDay() usecase.DayPlan {
	d := trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false)
	d.Content = &usecase.SelectedContent{
		Exercises: []usecase.PrescribedExercise{
			{
				Exercise:     content.Exercise{Name: "Squat"},
				Prescription: content.ExercisePrescription{Sets: 3, RestSeconds: 60},
			},
		},
	}
	return d
}

func newHomeTimelineUseCase(
	users *fakeUserRepo, drafts *fakeDraftRepo, checkins *fakeDailyCheckinRepo,
	logs *fakeSessionLogRepo, nutrition *fakeNutritionPlanRepo, recovery usecase.RecoveryCardProvider,
) *usecase.GetHomeTimelineUseCase {
	return usecase.NewGetHomeTimelineUseCase(users, drafts, checkins, logs, nutrition, recovery)
}

func TestGetHomeTimeline_NowCard_ChecksInFirst(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", TrainingTime: "Evening"}}}
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		loggableTrainingDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := newHomeTimelineUseCase(users, drafts, &fakeDailyCheckinRepo{}, &fakeSessionLogRepo{}, &fakeNutritionPlanRepo{}, fakeRecoveryCardProvider{})

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !out.Found {
		t.Fatalf("expected Found=true")
	}
	if out.NowCard == nil || out.NowCard.Kind != "checkin" {
		t.Fatalf("expected now_card=checkin when no checkin was submitted, got %+v", out.NowCard)
	}
	// No checkin submitted yet => no "checkin" row in the timeline itself.
	for _, item := range out.Timeline {
		if item.Kind == "checkin" {
			t.Fatalf("did not expect a checkin row before one was submitted: %+v", item)
		}
	}
}

func TestGetHomeTimeline_NowCard_SessionAfterCheckin(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", TrainingTime: "Evening"}}}
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		loggableTrainingDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	checkins := &fakeDailyCheckinRepo{}
	_ = checkins.Upsert(context.Background(), &domain.DailyCheckin{
		UserID: "u1", Date: monday, SubmittedAt: monday.Add(7*time.Hour + 40*time.Minute),
		Answers: checkin.DailyCheckin{Sleep: checkin.SleepNormal, Body: checkin.BodyStrongAndResponsive, Demand: checkin.DemandNormal, Need: checkin.NeedPushMe},
	})

	uc := newHomeTimelineUseCase(users, drafts, checkins, &fakeSessionLogRepo{}, &fakeNutritionPlanRepo{}, fakeRecoveryCardProvider{})

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.NowCard == nil || out.NowCard.Kind != "session" {
		t.Fatalf("expected now_card=session once checked in with an incomplete session, got %+v", out.NowCard)
	}

	foundCheckinRow := false
	for _, item := range out.Timeline {
		if item.Kind == "checkin" {
			foundCheckinRow = true
			if item.Status != "done" || item.Time != "07:40" {
				t.Errorf("checkin row = %+v, want status=done time=07:40", item)
			}
			if item.Detail != "Normal · Strong & responsive · Normal · Push me" {
				t.Errorf("checkin summary = %q", item.Detail)
			}
		}
	}
	if !foundCheckinRow {
		t.Fatalf("expected a done checkin row in the timeline")
	}
}

func TestGetHomeTimeline_NowCard_SessionCompleted_FallsThroughToLightsOut(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", TrainingTime: "Morning"}}}
	drafts := &fakeDraftRepo{}
	// "Today" (real wall-clock date), not an arbitrary fixed date — the
	// now_card's lights_out branch compares against time.Now(), so the
	// fixture needs lights_out (23:00) to still be ahead of it.
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", today, goal.StrengthMuscle, [7]usecase.DayPlan{
		loggableTrainingDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	checkins := &fakeDailyCheckinRepo{}
	_ = checkins.Upsert(context.Background(), &domain.DailyCheckin{UserID: "u1", Date: today, SubmittedAt: today.Add(7 * time.Hour)})

	logs := &fakeSessionLogRepo{}
	_ = logs.Upsert(context.Background(), &domain.SessionLog{UserID: "u1", Date: today, Status: domain.SessionLogDone})

	uc := newHomeTimelineUseCase(users, drafts, checkins, logs, &fakeNutritionPlanRepo{}, fakeRecoveryCardProvider{})

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: today})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.NowCard == nil || out.NowCard.Kind != "lights_out" {
		t.Fatalf("expected now_card=lights_out once the session is done, got %+v", out.NowCard)
	}

	for _, item := range out.Timeline {
		if item.Kind == "session" && item.Status != "done" {
			t.Errorf("session row status = %q, want done", item.Status)
		}
	}
}

func TestGetHomeTimeline_RestDay_RecoveryActionPending(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(), loggableTrainingDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	checkins := &fakeDailyCheckinRepo{}
	_ = checkins.Upsert(context.Background(), &domain.DailyCheckin{UserID: "u1", Date: monday, SubmittedAt: monday.Add(7 * time.Hour)})

	recovery := fakeRecoveryCardProvider{out: usecase.GetRecoveryCardOutput{
		Found: true,
		Card: domain.RecoveryCard{
			IsRestDay: true,
			Primary:   &domain.RecoveryActionItem{Title: "Walk 20 minutes", Detail: "After last night's lower strength."},
			AvailableActions: []domain.RecoveryActionOption{
				{Kind: domain.RecoveryActionDone, Label: "Done"},
			},
		},
	}}

	uc := newHomeTimelineUseCase(users, drafts, checkins, &fakeSessionLogRepo{}, &fakeNutritionPlanRepo{}, recovery)

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.NowCard == nil || out.NowCard.Kind != "recovery_action" {
		t.Fatalf("expected now_card=recovery_action on a rest day with nothing logged, got %+v", out.NowCard)
	}
	if out.NowCard.Title != "Walk 20 minutes" || out.NowCard.CTALabel != "Done" {
		t.Errorf("now_card = %+v", out.NowCard)
	}

	// A rest day's recovery action is never a plain timeline row — only
	// ever surfaced as the now_card.
	for _, item := range out.Timeline {
		if item.Kind == "recovery_action" {
			t.Fatalf("did not expect a recovery_action row in the timeline: %+v", item)
		}
	}
}

func TestGetHomeTimeline_NonLoggableSession_TimeBasedStatus(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", TrainingTime: "Morning"}}} // 08:00 estimated start
	drafts := &fakeDraftRepo{}
	// Yesterday, relative to "now" (time.Now()), so the 08:00 estimate is
	// unambiguously in the past regardless of what time this test runs.
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	weekStart := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	// Content left nil => not Loggable (no SessionLog is ever written for it) —
	// exactly the session-library gap this usecase works around.
	seedDraft(t, drafts, "u1", weekStart, goal.StrengthMuscle,
		[7]usecase.DayPlan{trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false), restDay(), restDay(), restDay(), restDay(), restDay(), restDay()})

	uc := newHomeTimelineUseCase(users, drafts, &fakeDailyCheckinRepo{}, &fakeSessionLogRepo{}, &fakeNutritionPlanRepo{}, fakeRecoveryCardProvider{})

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: weekStart})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	found := false
	for _, item := range out.Timeline {
		if item.Kind == "session" {
			found = true
			if item.Status != "done" {
				t.Errorf("non-loggable, no-signal session from yesterday should read done (time-based fallback), got %q", item.Status)
			}
		}
	}
	if !found {
		t.Fatalf("expected a session row")
	}
	// now_card should have moved past the session to lights_out, not be
	// stuck waiting on a completion signal that will never arrive.
	if out.NowCard != nil && out.NowCard.Kind == "session" {
		t.Errorf("now_card should not still be pointing at yesterday's non-loggable session: %+v", out.NowCard)
	}
}

func TestGetHomeTimeline_WeekProgress(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", TrainingTime: "Evening"}}}
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		loggableTrainingDay(), // Monday — completed below
		restDay(),             // Tuesday
		loggableTrainingDay(), // Wednesday — not completed
		restDay(),             // Thursday
		restDay(),             // Friday
		restDay(),             // Saturday
		restDay(),             // Sunday
	})

	logs := &fakeSessionLogRepo{}
	_ = logs.Upsert(context.Background(), &domain.SessionLog{UserID: "u1", Date: monday, Status: domain.SessionLogDone})

	uc := newHomeTimelineUseCase(users, drafts, &fakeDailyCheckinRepo{}, logs, &fakeNutritionPlanRepo{}, fakeRecoveryCardProvider{})

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out.WeekProgress.TotalSessions != 2 || out.WeekProgress.CompletedSessions != 1 {
		t.Errorf("week progress = %+v, want total=2 completed=1", out.WeekProgress)
	}
	if len(out.WeekProgress.Days) != 7 {
		t.Fatalf("expected 7 day dots, got %d", len(out.WeekProgress.Days))
	}
	if !out.WeekProgress.Days[0].IsToday {
		t.Errorf("expected Monday's dot to be marked IsToday")
	}
	if out.WeekProgress.Days[0].Status != "done" {
		t.Errorf("Monday dot status = %q, want done", out.WeekProgress.Days[0].Status)
	}
	if out.WeekProgress.Days[2].Status != "pending" {
		t.Errorf("Wednesday dot status = %q, want pending", out.WeekProgress.Days[2].Status)
	}
}

func TestGetHomeTimeline_Meals_FixedAndSessionRelativeWindows(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", TrainingTime: "Evening"}}} // 18:00 estimated session start
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	mondayDay := loggableTrainingDay()
	mondayDay.Weekday = "monday"
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		mondayDay, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	nutrition := &fakeNutritionPlanRepo{}
	_ = nutrition.Save(context.Background(), "u1", domain.NutritionWeekPlan{
		Days: [7]domain.NutritionDayPlan{
			{Weekday: "monday", Meals: []domain.MealSlot{
				{MealName: "Breakfast", TimingWindow: "7–9am"},
				{MealName: "Pre-Training Meal", TimingWindow: "90min before session", IsPrePost: true},
				{MealName: "Unrecognized Slot", TimingWindow: "whenever works"},
			}},
		},
	})

	uc := newHomeTimelineUseCase(users, drafts, &fakeDailyCheckinRepo{}, &fakeSessionLogRepo{}, nutrition, fakeRecoveryCardProvider{})

	out, err := uc.Execute(context.Background(), usecase.GetHomeTimelineInput{UserID: "u1", Date: monday})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	byTitle := map[string]usecase.TimelineItem{}
	for _, item := range out.Timeline {
		if item.Kind == "meal" {
			byTitle[item.Title] = item
		}
	}

	if got := byTitle["Breakfast"]; got.Time != "07:00" || !got.IsEstimated {
		t.Errorf("Breakfast = %+v, want time=07:00 estimated", got)
	}
	if got := byTitle["Pre-Training Meal"]; got.Time != "16:30" { // 18:00 - 90min
		t.Errorf("Pre-Training Meal = %+v, want time=16:30", got)
	}
	if got, ok := byTitle["Unrecognized Slot"]; !ok || got.Time != "" || got.Status != "pending" {
		t.Errorf("Unrecognized Slot = %+v, want empty time and pending status (unknown window falls back gracefully)", got)
	}
}
