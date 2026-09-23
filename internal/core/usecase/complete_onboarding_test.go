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

// fakePlanJobsRepo is a minimal usecase.PlanJobsRepository fake — in-memory,
// no concurrency guards needed since these tests always run the runner
// synchronously (see fakeWeeklyPlanGenerationRunner), never from a real
// goroutine.
type fakePlanJobsRepo struct {
	jobs      map[string]*domain.PlanJob
	nextID    int
	createErr error
}

func (f *fakePlanJobsRepo) CreateQueued(_ context.Context, userID, checkinID string) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	if f.jobs == nil {
		f.jobs = map[string]*domain.PlanJob{}
	}
	f.nextID++
	id := fmt.Sprintf("job-%d", f.nextID)
	f.jobs[id] = &domain.PlanJob{ID: id, UserID: userID, CheckinID: checkinID, Status: domain.PlanJobQueued}
	return id, nil
}

func (f *fakePlanJobsRepo) MarkRunning(_ context.Context, _, jobID string) error {
	if j, ok := f.jobs[jobID]; ok {
		j.Status = domain.PlanJobRunning
	}
	return nil
}

func (f *fakePlanJobsRepo) MarkDone(_ context.Context, _, jobID, planID string) error {
	if j, ok := f.jobs[jobID]; ok {
		j.Status = domain.PlanJobDone
		j.PlanID = planID
	}
	return nil
}

func (f *fakePlanJobsRepo) MarkFailed(_ context.Context, _, jobID, errMsg string) error {
	if j, ok := f.jobs[jobID]; ok {
		j.Status = domain.PlanJobFailed
		j.Error = errMsg
	}
	return nil
}

func (f *fakePlanJobsRepo) GetByID(_ context.Context, _, jobID string) (*domain.PlanJob, error) {
	return f.jobs[jobID], nil
}

// fakeWeeklyPlanGenerationRunner is a usecase.WeeklyPlanGenerationRunner
// fake that runs synchronously — no goroutine — so these tests can assert
// on the generator/job-repo state right after Execute returns, unlike the
// real runner.LocalWeeklyPlanRunner, which must return immediately and
// finishes on its own goroutine's schedule. Delegates the actual "work" to
// an embedded spyWeeklyPlanGenerator so calls are still recorded/
// controllable the same way the old fully-synchronous tests did.
type fakeWeeklyPlanGenerationRunner struct {
	gen  *spyWeeklyPlanGenerator
	jobs *fakePlanJobsRepo
}

func (r *fakeWeeklyPlanGenerationRunner) Run(jobID string, input usecase.GenerateWeeklyPlanInput) {
	out, err := r.gen.Execute(context.Background(), input)
	if err != nil {
		if r.jobs != nil {
			_ = r.jobs.MarkFailed(context.Background(), input.UserID, jobID, err.Error())
		}
		return
	}
	if r.jobs != nil {
		_ = r.jobs.MarkDone(context.Background(), input.UserID, jobID, out.Draft.ID)
	}
}

// newTestOnboardingUseCase wires a CompleteOnboardingUseCase whose weekly-
// plan trigger runs synchronously against gen, so assertions on gen.calls
// right after Execute returns are safe.
func newTestOnboardingUseCase(repo *fakeUserRepo, gen *spyWeeklyPlanGenerator) (*usecase.CompleteOnboardingUseCase, *fakePlanJobsRepo) {
	jobs := &fakePlanJobsRepo{}
	runner := &fakeWeeklyPlanGenerationRunner{gen: gen, jobs: jobs}
	return usecase.NewCompleteOnboardingUseCase(repo, jobs, runner), jobs
}

func TestCompleteOnboarding_ValidCatalogAndGoalPersist(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{}
	uc, _ := newTestOnboardingUseCase(repo, gen)

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
	uc, _ := newTestOnboardingUseCase(repo, gen)

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
	uc, _ := newTestOnboardingUseCase(repo, gen)

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
	uc, jobs := newTestOnboardingUseCase(repo, gen)

	out, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength"},
		GoalID:     "consistency_wellbeing",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if out.WeeklyPlanJobID == "" {
		t.Error("expected a non-empty WeeklyPlanJobID to poll")
	}
	if job := jobs.jobs[out.WeeklyPlanJobID]; job == nil || job.Status != domain.PlanJobDone {
		t.Errorf("job %q status = %+v, want done", out.WeeklyPlanJobID, job)
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
	uc, jobs := newTestOnboardingUseCase(repo, gen)

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
	if out.WeeklyPlanJobID == "" {
		t.Fatal("expected a job id even though generation later failed — the job was queued successfully")
	}
	if job := jobs.jobs[out.WeeklyPlanJobID]; job == nil || job.Status != domain.PlanJobFailed {
		t.Errorf("job %q status = %+v, want failed", out.WeeklyPlanJobID, job)
	}
}

func TestCompleteOnboarding_JobQueueFailureDoesNotBlockOnboarding(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	gen := &spyWeeklyPlanGenerator{}
	jobs := &fakePlanJobsRepo{createErr: fmt.Errorf("firestore: simulated outage")}
	runner := &fakeWeeklyPlanGenerationRunner{gen: gen, jobs: jobs}
	uc := usecase.NewCompleteOnboardingUseCase(repo, jobs, runner)

	out, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength"},
		GoalID:     "consistency_wellbeing",
	})
	if err != nil {
		t.Fatalf("Execute returned error even though only job queuing failed: %v", err)
	}
	if !out.User.OnboardingCompleted {
		t.Error("expected onboarding to still be marked completed")
	}
	if out.WeeklyPlanJobID != "" {
		t.Errorf("WeeklyPlanJobID = %q, want empty — queuing itself failed", out.WeeklyPlanJobID)
	}
	if len(gen.calls) != 0 {
		t.Error("the runner should never have been reached — CreateQueued failed first")
	}
}

func TestCompleteOnboarding_NilJobsRepoOrRunnerSkipsTriggerGracefully(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*domain.User{}}
	uc := usecase.NewCompleteOnboardingUseCase(repo, nil, nil)

	out, err := uc.Execute(context.Background(), usecase.CompleteOnboardingInput{
		UserID:     "u1",
		Activities: []string{"strength"},
		GoalID:     "consistency_wellbeing",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !out.User.OnboardingCompleted {
		t.Error("expected onboarding to still be marked completed")
	}
	if out.WeeklyPlanJobID != "" {
		t.Errorf("WeeklyPlanJobID = %q, want empty — JobsRepo/Runner aren't wired", out.WeeklyPlanJobID)
	}
}
