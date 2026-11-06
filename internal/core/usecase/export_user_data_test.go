package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// fakeLifestyleChangeRepo is a minimal usecase.LifestyleChangeRepository
// fake — in-memory, keyed by userID, matching FirestoreLifestyleChangeRepository's
// "most recent first" ListByUser ordering closely enough for assertions
// that don't depend on exact order.
type fakeLifestyleChangeRepo struct {
	byUser map[string][]*domain.LifestyleChange
}

func (f *fakeLifestyleChangeRepo) Create(_ context.Context, e *domain.LifestyleChange) error {
	if f.byUser == nil {
		f.byUser = map[string][]*domain.LifestyleChange{}
	}
	f.byUser[e.UserID] = append(f.byUser[e.UserID], e)
	return nil
}

func (f *fakeLifestyleChangeRepo) ListByUser(_ context.Context, userID string, limit int) ([]*domain.LifestyleChange, error) {
	all := f.byUser[userID]
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (f *fakeLifestyleChangeRepo) GetByID(_ context.Context, userID, changeID string) (*domain.LifestyleChange, error) {
	for _, c := range f.byUser[userID] {
		if c.ID == changeID {
			return c, nil
		}
	}
	return nil, nil
}

func (f *fakeLifestyleChangeRepo) SetPlanID(_ context.Context, userID, changeID, planID string) error {
	for _, c := range f.byUser[userID] {
		if c.ID == changeID {
			c.PlanID = &planID
		}
	}
	return nil
}

// fakePlanRepo is a minimal usecase.PlanRepository (legacy pipeline) fake —
// only GetLatest is exercised by ExportUserDataUseCase; the rest are
// no-ops, same "implement the interface, not the feature" convention as
// this package's other narrow fakes.
type fakePlanRepo struct {
	latest map[string]*domain.Plan
}

func (f *fakePlanRepo) Create(_ context.Context, p *domain.Plan) error {
	if f.latest == nil {
		f.latest = map[string]*domain.Plan{}
	}
	f.latest[p.UserID] = p
	return nil
}
func (f *fakePlanRepo) GetByID(_ context.Context, userID, planID string) (*domain.Plan, error) {
	return f.latest[userID], nil
}
func (f *fakePlanRepo) GetLatestByWeekStart(_ context.Context, userID string, weekStart time.Time) (*domain.Plan, error) {
	return f.latest[userID], nil
}
func (f *fakePlanRepo) GetLatest(_ context.Context, userID string) (*domain.Plan, error) {
	return f.latest[userID], nil
}
func (f *fakePlanRepo) UpdateTrainingCompleted(_ context.Context, userID, planID string, completed map[string]bool) error {
	return nil
}
func (f *fakePlanRepo) UpdatedTrainingJSON(_ context.Context, userID, trainingPlanID string, trainingJson []byte) error {
	return nil
}
func (f *fakePlanRepo) UpdateNutritionJSON(_ context.Context, userID, planID string, nutritionJSON []byte) error {
	return nil
}
func (f *fakePlanRepo) UpdatePhaseFeedback(_ context.Context, userID, planID string, feedback map[string]domain.PhaseFeedbackEntry) error {
	return nil
}
func (f *fakePlanRepo) UpdateMealSelections(_ context.Context, userID, planID string, selections map[string]map[string]int) error {
	return nil
}
func (f *fakePlanRepo) IsTrainingDay(_ context.Context, userID, weekday string) (bool, error) {
	return false, nil
}

func TestExportUserData_NothingGeneratedYet_MostlyNil(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", Name: "Ana"}}}
	uc := usecase.NewExportUserDataUseCase(users, &fakeConsentRepo{}, &fakeLifestyleChangeRepo{}, &fakeDailyCheckinRepo{}, &fakeSessionLogRepo{}, &fakeRecoveryActionRepo{}, &fakeNutritionPlanRepo{}, &fakeDraftRepo{}, &fakePlanRepo{})

	out, err := uc.Execute(context.Background(), usecase.ExportUserDataInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if out.Profile == nil || out.Profile.Name != "Ana" {
		t.Errorf("Profile = %+v, want the saved user", out.Profile)
	}
	if out.NutritionPlan != nil {
		t.Errorf("NutritionPlan = %+v, want nil — never generated", out.NutritionPlan)
	}
	if out.CurrentWeekDraft != nil {
		t.Errorf("CurrentWeekDraft = %+v, want nil — never generated", out.CurrentWeekDraft)
	}
	if out.LegacyPlan != nil {
		t.Errorf("LegacyPlan = %+v, want nil — never generated", out.LegacyPlan)
	}
	if len(out.LifestyleChanges) != 0 {
		t.Errorf("LifestyleChanges = %v, want empty", out.LifestyleChanges)
	}
	if len(out.DailyCheckins) != 0 {
		t.Errorf("DailyCheckins = %v, want empty", out.DailyCheckins)
	}
	if len(out.SessionLogs) != 0 {
		t.Errorf("SessionLogs = %v, want empty", out.SessionLogs)
	}
	if len(out.RecoveryActions) != 0 {
		t.Errorf("RecoveryActions = %v, want empty", out.RecoveryActions)
	}
}

func TestExportUserData_UnknownUser_ReturnsNotFound(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewExportUserDataUseCase(users, &fakeConsentRepo{}, &fakeLifestyleChangeRepo{}, &fakeDailyCheckinRepo{}, &fakeSessionLogRepo{}, &fakeRecoveryActionRepo{}, &fakeNutritionPlanRepo{}, &fakeDraftRepo{}, &fakePlanRepo{})

	_, err := uc.Execute(context.Background(), usecase.ExportUserDataInput{UserID: "ghost"})
	if err == nil {
		t.Fatal("expected an error for a user that doesn't exist")
	}
}

func TestExportUserData_IncludesEverythingThatExists(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{"u1": {ID: "u1", Name: "Ana"}}}

	consents := grantedConsentRepo("u1")

	lifestyle := &fakeLifestyleChangeRepo{}
	_ = lifestyle.Create(context.Background(), &domain.LifestyleChange{ID: "lc1", UserID: "u1", Type: "travel"})
	_ = lifestyle.Create(context.Background(), &domain.LifestyleChange{ID: "lc2", UserID: "u1", Type: "injury"})

	dailyCheckins := &fakeDailyCheckinRepo{}
	_ = dailyCheckins.Upsert(context.Background(), &domain.DailyCheckin{UserID: "u1", Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})
	_ = dailyCheckins.Upsert(context.Background(), &domain.DailyCheckin{UserID: "u1", Date: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)})

	sessionLogs := &fakeSessionLogRepo{}
	_ = sessionLogs.Upsert(context.Background(), &domain.SessionLog{UserID: "u1", Date: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})

	recoveryActions := &fakeRecoveryActionRepo{}
	_ = recoveryActions.Upsert(context.Background(), &domain.RecoveryAction{UserID: "u1", Date: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), Kind: domain.RecoveryActionDone})

	nutrition := &fakeNutritionPlanRepo{}
	_ = nutrition.Save(context.Background(), "u1", domain.NutritionWeekPlan{Phase: domain.PhaseFollicular})

	drafts := &fakeDraftRepo{}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := today.AddDate(0, 0, -int(today.Weekday()))
	seedDraft(t, drafts, "u1", weekStart, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	plans := &fakePlanRepo{}
	_ = plans.Create(context.Background(), &domain.Plan{ID: "p1", UserID: "u1", Status: "active"})

	uc := usecase.NewExportUserDataUseCase(users, consents, lifestyle, dailyCheckins, sessionLogs, recoveryActions, nutrition, drafts, plans)

	out, err := uc.Execute(context.Background(), usecase.ExportUserDataInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(out.Consents) != 3 {
		t.Errorf("Consents = %v, want 3 entries", out.Consents)
	}
	if len(out.LifestyleChanges) != 2 {
		t.Errorf("LifestyleChanges = %d, want 2", len(out.LifestyleChanges))
	}
	if len(out.DailyCheckins) != 2 {
		t.Errorf("DailyCheckins = %d, want 2", len(out.DailyCheckins))
	}
	if len(out.SessionLogs) != 1 {
		t.Errorf("SessionLogs = %d, want 1", len(out.SessionLogs))
	}
	if len(out.RecoveryActions) != 1 {
		t.Errorf("RecoveryActions = %d, want 1", len(out.RecoveryActions))
	}
	if out.NutritionPlan == nil || out.NutritionPlan.Plan.Phase != domain.PhaseFollicular {
		t.Errorf("NutritionPlan = %+v, want the saved plan", out.NutritionPlan)
	}
	if out.CurrentWeekDraft == nil {
		t.Error("CurrentWeekDraft = nil, want the seeded draft covering today")
	}
	if out.LegacyPlan == nil || out.LegacyPlan.ID != "p1" {
		t.Errorf("LegacyPlan = %+v, want p1", out.LegacyPlan)
	}
}
