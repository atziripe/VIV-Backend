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

type fakeRecoveryActionRepo struct {
	byKey map[string]*domain.RecoveryAction
}

func (f *fakeRecoveryActionRepo) Upsert(_ context.Context, a *domain.RecoveryAction) error {
	if f.byKey == nil {
		f.byKey = map[string]*domain.RecoveryAction{}
	}
	cp := *a
	f.byKey[a.UserID+"|"+a.Date.Format("2006-01-02")] = &cp
	return nil
}

func (f *fakeRecoveryActionRepo) GetByDate(_ context.Context, userID string, date time.Time) (*domain.RecoveryAction, error) {
	if f.byKey == nil {
		return nil, nil
	}
	return f.byKey[userID+"|"+date.Format("2006-01-02")], nil
}

func TestGetRecoveryCard_TrainingDay(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(),
		trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false), // tuesday
		restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewGetRecoveryCardUseCase(drafts, &fakeDailyCheckinRepo{}, &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}, &fakeRecoveryActionRepo{}, nil)

	tuesday := monday.AddDate(0, 0, 1)
	out, err := uc.Execute(context.Background(), usecase.GetRecoveryCardInput{UserID: "u1", Date: tuesday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Found {
		t.Fatal("expected Found = true")
	}
	if out.Card.IsRestDay {
		t.Error("expected a training-day card")
	}
	if out.Card.CostTier != "" {
		t.Errorf("CostTier = %q, want empty on a training day", out.Card.CostTier)
	}
	if out.Card.Context != "One rest day behind you · full body strength" {
		t.Errorf("Context = %q", out.Card.Context)
	}
	if len(out.Card.AvailableActions) != 0 {
		t.Error("expected no action buttons on a training day")
	}
}

func TestGetRecoveryCard_RestDayAfterLowCostSession(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		trainingDay(activity.Strength, activity.IntensityL, activity.ImpactL, false), // monday, RecoveryCost=1 -> low, muscle group full_body from trainingDay() helper
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewGetRecoveryCardUseCase(drafts, &fakeDailyCheckinRepo{}, &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}, &fakeRecoveryActionRepo{}, nil)

	tuesday := monday.AddDate(0, 0, 1)
	out, err := uc.Execute(context.Background(), usecase.GetRecoveryCardInput{UserID: "u1", Date: tuesday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Found || !out.Card.IsRestDay {
		t.Fatalf("expected a found rest-day card, got %+v", out.Card)
	}
	// trainingDay() helper sets MuscleGroupFullBody, which bumps
	// Strength@L's base RecoveryCost (1) up to medium.
	if out.Card.CostTier != domain.RecoveryCostMedium {
		t.Errorf("CostTier = %s, want medium (full-body bump)", out.Card.CostTier)
	}
	if len(out.Card.AvailableActions) != 2 || out.Card.AvailableActions[0].Kind != domain.RecoveryActionDone {
		t.Errorf("AvailableActions = %+v, want Done/Not-today", out.Card.AvailableActions)
	}
}

func TestGetRecoveryCard_SleepDebtBumpsTier(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	lowDay := usecase.DayPlan{}
	lowDay.Assignment.ActivityType = activity.Strength
	lowDay.Assignment.Intensity = activity.IntensityL
	lowDay.Assignment.MuscleGroup = activity.MuscleGroupUpper // isolated, not full-body -> stays low absent sleep debt
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		lowDay, restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	tuesday := monday.AddDate(0, 0, 1)
	checkins := &fakeDailyCheckinRepo{byKey: map[string]*domain.DailyCheckin{
		"u1|" + tuesday.Format("2006-01-02"): {
			UserID: "u1", Date: tuesday,
			Answers: checkin.DailyCheckin{Sleep: checkin.SleepBarelySlept},
		},
	}}

	uc := usecase.NewGetRecoveryCardUseCase(drafts, checkins, &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}, &fakeRecoveryActionRepo{}, nil)

	out, err := uc.Execute(context.Background(), usecase.GetRecoveryCardInput{UserID: "u1", Date: tuesday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Card.CostTier != domain.RecoveryCostMedium {
		t.Errorf("CostTier = %s, want medium (sleep-debt bump from low)", out.Card.CostTier)
	}
}

func TestGetRecoveryCard_NoSessionYesterday_NeutralCard(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	uc := usecase.NewGetRecoveryCardUseCase(drafts, &fakeDailyCheckinRepo{}, &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}, &fakeRecoveryActionRepo{}, nil)

	tuesday := monday.AddDate(0, 0, 1)
	out, err := uc.Execute(context.Background(), usecase.GetRecoveryCardInput{UserID: "u1", Date: tuesday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.Found || !out.Card.IsRestDay {
		t.Fatalf("expected a found rest-day card, got %+v", out.Card)
	}
	if out.Card.CostTier != domain.RecoveryCostLow {
		t.Errorf("CostTier = %s, want low", out.Card.CostTier)
	}
}

func TestGetRecoveryCard_HighCostReschedulesTomorrow(t *testing.T) {
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	// Tuesday: HIIT@H, full body, long -> high cost (base 3, already capped)
	// -> this is "yesterday" for Wednesday's (today's) card.
	hiitDay := usecase.DayPlan{}
	hiitDay.Assignment.ActivityType = activity.HIIT
	hiitDay.Assignment.Intensity = activity.IntensityH
	hiitDay.Assignment.MuscleGroup = activity.MuscleGroupFullBody
	hiitDay.Content = &usecase.SelectedContent{Session: &content.Session{DurationMinutes: 90}}

	// Thursday: another high-intensity day — "tomorrow" relative to
	// Wednesday (today), so it should get pushed out once today's card
	// lands on High.
	tomorrowHigh := usecase.DayPlan{}
	tomorrowHigh.Assignment.ActivityType = activity.Strength
	tomorrowHigh.Assignment.Intensity = activity.IntensityH
	tomorrowHigh.Assignment.MuscleGroup = activity.MuscleGroupLower

	seedDraft(t, drafts, "u1", monday, goal.StrengthMuscle, [7]usecase.DayPlan{
		restDay(),    // monday
		hiitDay,      // tuesday - yesterday, drives today's high cost
		restDay(),    // wednesday - today
		tomorrowHigh, // thursday - tomorrow, should get rescheduled
		restDay(),    // friday - the swap target
		restDay(), restDay(),
	})

	selector := &fakeContentSelector{content: usecase.SelectedContent{Session: &content.Session{DurationMinutes: 30}}}
	uc := usecase.NewGetRecoveryCardUseCase(drafts, &fakeDailyCheckinRepo{}, &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1"}}}, &fakeRecoveryActionRepo{}, selector)

	wednesday := monday.AddDate(0, 0, 2)
	out, err := uc.Execute(context.Background(), usecase.GetRecoveryCardInput{UserID: "u1", Date: wednesday})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Card.CostTier != domain.RecoveryCostHigh {
		t.Fatalf("CostTier = %s, want high", out.Card.CostTier)
	}
	if out.Card.RescheduleNote == "" {
		t.Fatal("expected a non-empty RescheduleNote when tomorrow was high-intensity and got moved")
	}

	stored, err := drafts.GetByDate(context.Background(), "u1", monday)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if !stored.Days[3].IsRestDay {
		t.Error("expected Thursday (tomorrow) to become a rest day after reschedule")
	}
	if stored.Days[4].IsRestDay {
		t.Error("expected Friday (the swap target) to now carry the moved session")
	}
	if stored.Days[4].Assignment.ActivityType != activity.Strength {
		t.Errorf("Friday's assignment = %+v, want the moved Strength session", stored.Days[4].Assignment)
	}
}

func TestSaveRecoveryAction_ValidKindPersists(t *testing.T) {
	actions := &fakeRecoveryActionRepo{}
	uc := usecase.NewSaveRecoveryActionUseCase(actions)
	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	if err := uc.Execute(context.Background(), usecase.SaveRecoveryActionInput{UserID: "u1", Date: date, Kind: domain.RecoveryActionDone}); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	stored, err := actions.GetByDate(context.Background(), "u1", date)
	if err != nil {
		t.Fatalf("GetByDate returned error: %v", err)
	}
	if stored == nil || stored.Kind != domain.RecoveryActionDone {
		t.Errorf("stored = %+v, want Kind=done", stored)
	}
}

func TestSaveRecoveryAction_RejectsUnknownKind(t *testing.T) {
	uc := usecase.NewSaveRecoveryActionUseCase(&fakeRecoveryActionRepo{})
	err := uc.Execute(context.Background(), usecase.SaveRecoveryActionInput{
		UserID: "u1", Date: time.Now(), Kind: domain.RecoveryActionKind("bogus"),
	})
	if err == nil {
		t.Fatal("expected an error for an unknown action kind")
	}
}
