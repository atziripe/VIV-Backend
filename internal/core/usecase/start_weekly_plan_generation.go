package usecase

import (
	"context"
	"strings"
	"time"
)

// WeeklyPlanGenerationRunner starts background generation for the new
// pipeline (VIV-106..113) — same async shape as PlanGenerationRunner (the
// old system's equivalent). It takes the full GenerateWeeklyPlanInput
// (rather than just a generationDate) so a caller that already knows the
// real goal/catalog/readiness to use — like CompleteOnboardingUseCase —
// can pass them through instead of falling back to
// GenerateWeeklyPlanUsecase's documented defaults, which is what happens
// when a caller — like StartWeeklyPlanGenerationUseCase — leaves them
// unset.
type WeeklyPlanGenerationRunner interface {
	// Run starts the background generation process for a previously
	// created job. Must return immediately (non-blocking).
	Run(jobID string, input GenerateWeeklyPlanInput)
}

// StartWeeklyPlanGenerationUseCase is the new pipeline's counterpart to
// StartPlanGenerationUseCase: creates a queued job and hands it to the
// runner, returning immediately with the jobID the client polls.
type StartWeeklyPlanGenerationUseCase struct {
	jobsRepo PlanJobsRepository
	runner   WeeklyPlanGenerationRunner
}

func NewStartWeeklyPlanGenerationUseCase(
	jobsRepo PlanJobsRepository,
	runner WeeklyPlanGenerationRunner,
) *StartWeeklyPlanGenerationUseCase {
	return &StartWeeklyPlanGenerationUseCase{jobsRepo: jobsRepo, runner: runner}
}

// Execute creates a queued job and triggers the runner. Returns the jobID
// the client can poll via GetPlanGenerationStatusUseCase — the same
// PlanJobsRepository/domain.PlanJob shape the old pipeline already uses,
// reused as-is rather than duplicated: PlanJob.PlanID holds the resulting
// WeekDraft.ID for this job type.
func (uc *StartWeeklyPlanGenerationUseCase) Execute(ctx context.Context, userID string, generationDate time.Time) (string, error) {
	userID = strings.TrimSpace(userID)

	jobID, err := uc.jobsRepo.CreateQueued(ctx, userID, "")
	if err != nil {
		return "", err
	}

	// Non-blocking: runner must return immediately.
	uc.runner.Run(jobID, GenerateWeeklyPlanInput{UserID: userID, GenerationDate: generationDate})

	return jobID, nil
}
