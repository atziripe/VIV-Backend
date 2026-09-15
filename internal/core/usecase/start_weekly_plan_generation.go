package usecase

import (
	"context"
	"strings"
	"time"
)

// WeeklyPlanGenerationRunner starts background generation for the new
// pipeline (VIV-106..113) — same async shape as PlanGenerationRunner (the
// old system's equivalent), but takes a generationDate instead of a
// checkinID: the new pipeline resolves readiness/goal/catalog on its own
// (see GenerateWeeklyPlanUsecase's documented defaults) rather than being
// handed a specific check-in to load.
type WeeklyPlanGenerationRunner interface {
	// Run starts the background generation process for a previously
	// created job. Must return immediately (non-blocking).
	Run(userID, jobID string, generationDate time.Time)
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
	uc.runner.Run(userID, jobID, generationDate)

	return jobID, nil
}
