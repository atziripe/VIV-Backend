package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/goal"
	"viv/internal/core/usecase"
)

// spyWeeklyPlanGenerator is a usecase.WeeklyPlanGenerator fake that records
// every call it receives (no real generation logic) — used to verify
// CompleteOnboardingUseCase and SubmitDailyCheckinUseCase trigger exactly
// one call with the expected input, and that a simulated failure never
// surfaces back where it shouldn't. draft, when set, is returned as-is —
// callers that need dayIndexForDate to resolve (e.g. SubmitDailyCheckinUseCase)
// set it to a WeekDraft actually covering the date under test.
type spyWeeklyPlanGenerator struct {
	calls []usecase.GenerateWeeklyPlanInput
	draft usecase.WeekDraft
	err   error
}

func (s *spyWeeklyPlanGenerator) Execute(_ context.Context, input usecase.GenerateWeeklyPlanInput) (usecase.GenerateWeeklyPlanOutput, error) {
	s.calls = append(s.calls, input)
	if s.err != nil {
		return usecase.GenerateWeeklyPlanOutput{}, s.err
	}
	return usecase.GenerateWeeklyPlanOutput{Draft: s.draft}, nil
}

func TestCompleteOnboarding_ValidCatalogAndGoalPersist(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{}
	uc := usecase.NewCompleteOnboardingUseCase(repo, gen)

	out, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength", "yoga"},
		GoalID:     "consistency_wellbeing",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(out.User.UserCatalog) != 2 || out.User.UserCatalog[0] != "strength" || out.User.UserCatalog[1] != "yoga" {
		t.Errorf("UserCatalog = %v, want [strength yoga]", out.User.UserCatalog)
	}
	if out.User.GoalID != goal.ConsistencyWellbeing {
		t.Errorf("GoalID = %q, want %q", out.User.GoalID, goal.ConsistencyWellbeing)
	}

	saved := repo.users["u1"]
	if saved == nil || len(saved.UserCatalog) != 2 || saved.GoalID != goal.ConsistencyWellbeing {
		t.Errorf("persisted user catalog/goal mismatch: %+v", saved)
	}
	if !saved.OnboardingCompleted {
		t.Error("expected OnboardingCompleted = true")
	}
}

func TestCompleteOnboarding_InvalidActivityIDRejected(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{}
	uc := usecase.NewCompleteOnboardingUseCase(repo, gen)

	_, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength", "not_a_real_activity"},
		GoalID:     "consistency_wellbeing",
	})
	if err == nil {
		t.Fatal("expected an error for an unrecognized activity id")
	}
	var invalidActivity usecase.InvalidActivityError
	if !errors.As(err, &invalidActivity) {
		t.Fatalf("error = %v (%T), want usecase.InvalidActivityError", err, err)
	}
	if invalidActivity.ActivityID != "not_a_real_activity" {
		t.Errorf("ActivityID = %q, want %q", invalidActivity.ActivityID, "not_a_real_activity")
	}

	if _, ok := repo.users["u1"]; ok {
		t.Error("user should not have been persisted when validation fails")
	}
	if len(gen.calls) != 0 {
		t.Error("weekly-plan generation should not be triggered when onboarding fails validation")
	}
}

func TestCompleteOnboarding_InvalidGoalIDRejected(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{}
	uc := usecase.NewCompleteOnboardingUseCase(repo, gen)

	_, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID: "u1",
		GoalID: "not_a_real_goal",
	})
	if err == nil {
		t.Fatal("expected an error for an unrecognized goal id")
	}
	var invalidGoal usecase.InvalidGoalError
	if !errors.As(err, &invalidGoal) {
		t.Fatalf("error = %v (%T), want usecase.InvalidGoalError", err, err)
	}
}

func TestCompleteOnboarding_TriggersExactlyOneWeeklyPlanGenerationWithDefaultReadiness(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{}
	uc := usecase.NewCompleteOnboardingUseCase(repo, gen)

	_, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength"},
		GoalID:     "consistency_wellbeing",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(gen.calls) != 1 {
		t.Fatalf("weekly-plan generation calls = %d, want exactly 1", len(gen.calls))
	}
	call := gen.calls[0]
	if call.UserID != "u1" {
		t.Errorf("call.UserID = %q, want %q", call.UserID, "u1")
	}
	if call.GoalID != goal.ConsistencyWellbeing {
		t.Errorf("call.GoalID = %q, want %q", call.GoalID, goal.ConsistencyWellbeing)
	}
	if len(call.Catalog.Activities) != 1 || call.Catalog.Activities[0] != "strength" {
		t.Errorf("call.Catalog.Activities = %v, want [strength]", call.Catalog.Activities)
	}
	if call.Readiness == nil {
		t.Fatal("expected an explicit Readiness override, got nil")
	}
	if *call.Readiness != usecase.DefaultOnboardingReadiness {
		t.Errorf("call.Readiness = %+v, want DefaultOnboardingReadiness %+v", *call.Readiness, usecase.DefaultOnboardingReadiness)
	}
	if usecase.DefaultOnboardingReadiness.RecoveryCapacity != checkin.RecoveryModerate ||
		usecase.DefaultOnboardingReadiness.LifeBandwidth != checkin.BandwidthModerate ||
		usecase.DefaultOnboardingReadiness.BuildReadiness != checkin.BuildMaintain {
		t.Errorf("DefaultOnboardingReadiness = %+v, want Moderate/Moderate/Maintain", usecase.DefaultOnboardingReadiness)
	}
}

func TestCompleteOnboarding_GenerationFailureDoesNotBlockOrRollBackOnboarding(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{err: fmt.Errorf("weekly plan: simulated failure")}
	uc := usecase.NewCompleteOnboardingUseCase(repo, gen)

	out, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength"},
		GoalID:     "consistency_wellbeing",
	})
	if err != nil {
		t.Fatalf("Execute returned error even though only plan generation failed: %v", err)
	}
	if !out.User.OnboardingCompleted {
		t.Error("expected onboarding to still be marked completed")
	}

	saved := repo.users["u1"]
	if saved == nil || !saved.OnboardingCompleted {
		t.Error("expected the user to still be persisted with onboarding completed")
	}
	if len(gen.calls) != 1 {
		t.Errorf("weekly-plan generation calls = %d, want exactly 1 (attempted, even though it failed)", len(gen.calls))
	}
}
