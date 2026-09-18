package usecase_test

import (
	"context"
	"testing"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// fakeMealGenerator is a no-op MealContentGenerator: it returns no meal
// content, exercising GenerateNutritionPlanUsecase's "return the plan
// without meal content" fallback rather than needing the real
// ingredient/template solver here.
type fakeMealGenerator struct{}

func (fakeMealGenerator) GenerateWeekMeals(_ context.Context, _ usecase.MealGenerationInput) ([]usecase.DayMealsOutput, usecase.TokenUsage, error) {
	return nil, usecase.TokenUsage{}, nil
}

type fakeNutritionPlanRepo struct {
	saved   map[string]domain.NutritionWeekPlan
	saveErr error
}

func (f *fakeNutritionPlanRepo) GetByUserID(_ context.Context, userID string) (*domain.NutritionPlan, error) {
	if f.saved == nil {
		return nil, nil
	}
	plan, ok := f.saved[userID]
	if !ok {
		return nil, nil
	}
	return &domain.NutritionPlan{UserID: userID, Plan: plan}, nil
}

func (f *fakeNutritionPlanRepo) Save(_ context.Context, userID string, plan domain.NutritionWeekPlan) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if f.saved == nil {
		f.saved = map[string]domain.NutritionWeekPlan{}
	}
	f.saved[userID] = plan
	return nil
}

func TestSubmitNutritionOnboarding_HappyPath(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", OnboardingCompleted: true, WeightKg: 65},
	}}
	drafts := &fakeDraftRepo{}
	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	seedDraft(t, drafts, "u1", monday, goal.ConsistencyWellbeing, [7]usecase.DayPlan{
		trainingDay(activity.Strength, activity.IntensityM, activity.ImpactL, false),
		restDay(), restDay(), restDay(), restDay(), restDay(), restDay(),
	})

	nutritionGen := usecase.NewGenerateNutritionPlanUsecase(fakeMealGenerator{})
	plans := &fakeNutritionPlanRepo{}

	uc := usecase.NewSubmitNutritionOnboardingUseCase(users, drafts, fakePhaseLookup{phase: domain.PhaseFollicular}, nutritionGen, plans, nil)

	out, err := uc.Execute(context.Background(), usecase.SubmitNutritionOnboardingInput{
		UserID:               "u1",
		Date:                 monday,
		DietRestrictions:     "vegetarian",
		DietProteinResources: "Plant based",
		EatingStyle:          "3 meals",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !out.Nutrition.Days[0].IsTrainingDay {
		t.Error("expected day 0 (Strength) to be a training day in the assembled nutrition plan")
	}
	if out.Nutrition.Days[1].IsTrainingDay {
		t.Error("expected day 1 (rest) to not be a training day")
	}

	saved, ok := plans.saved["u1"]
	if !ok {
		t.Fatal("expected the generated plan to be saved via NutritionPlanRepository")
	}
	if saved.Days[0].Weekday != out.Nutrition.Days[0].Weekday {
		t.Errorf("saved plan doesn't match returned plan")
	}

	user, _ := users.GetByID(context.Background(), "u1")
	if user.DietRestrictions != "vegetarian" {
		t.Errorf("DietRestrictions = %q, want %q", user.DietRestrictions, "vegetarian")
	}
	if user.DietProteinResources != "Plant based" {
		t.Errorf("DietProteinResources = %q, want %q", user.DietProteinResources, "Plant based")
	}
}

func TestSubmitNutritionOnboarding_RequiresCompletedOnboarding(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", OnboardingCompleted: false},
	}}
	drafts := &fakeDraftRepo{}
	nutritionGen := usecase.NewGenerateNutritionPlanUsecase(fakeMealGenerator{})

	uc := usecase.NewSubmitNutritionOnboardingUseCase(users, drafts, fakePhaseLookup{phase: domain.PhaseFollicular}, nutritionGen, &fakeNutritionPlanRepo{}, nil)

	_, err := uc.Execute(context.Background(), usecase.SubmitNutritionOnboardingInput{UserID: "u1"})
	if err == nil {
		t.Fatal("expected an error for a user who hasn't completed onboarding")
	}
}

func TestSubmitNutritionOnboarding_RequiresGeneratedTrainingWeek(t *testing.T) {
	users := &fakeUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", OnboardingCompleted: true},
	}}
	drafts := &fakeDraftRepo{} // nothing seeded — no current week
	nutritionGen := usecase.NewGenerateNutritionPlanUsecase(fakeMealGenerator{})

	uc := usecase.NewSubmitNutritionOnboardingUseCase(users, drafts, fakePhaseLookup{phase: domain.PhaseFollicular}, nutritionGen, &fakeNutritionPlanRepo{}, nil)

	_, err := uc.Execute(context.Background(), usecase.SubmitNutritionOnboardingInput{UserID: "u1", Date: time.Now()})
	if err == nil {
		t.Fatal("expected an error when no training week has been generated yet")
	}
}
