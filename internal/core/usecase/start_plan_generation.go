package usecase

import (
	"context"
	"strings"
)

// StartPlanGenerationUseCase creates a plan generation job and triggers
// background execution via a PlanGenerationRunner.
//
// It must return quickly (no long-running work here).
type StartPlanGenerationUseCase struct {
	jobsRepo PlanJobsRepository
	runner   PlanGenerationRunner
}

func NewStartPlanGenerationUseCase(
	jobsRepo PlanJobsRepository,
	runner PlanGenerationRunner,
) *StartPlanGenerationUseCase {
	return &StartPlanGenerationUseCase{
		jobsRepo: jobsRepo,
		runner:   runner,
	}
}

// Execute creates a queued job and triggers the runner.
// Returns the jobID that the client can poll.
func (uc *StartPlanGenerationUseCase) Execute(ctx context.Context, userID, checkinID string) (string, error) {
	userID = strings.TrimSpace(userID)
	checkinID = strings.TrimSpace(checkinID)

	jobID, err := uc.jobsRepo.CreateQueued(ctx, userID, checkinID)
	if err != nil {
		return "", err
	}

	// Non-blocking: runner must return immediately.
	uc.runner.Run(userID, jobID, checkinID)

	return jobID, nil
}
