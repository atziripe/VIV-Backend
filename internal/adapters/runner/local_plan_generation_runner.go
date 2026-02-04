package runner

import (
	"context"
	"log"
	"strings"
	"time"

	"viv/internal/core/usecase"
)

// LocalPlanGenerationRunner runs plan generation asynchronously using a goroutine.
// This is suitable for local development (and long-lived servers).
//
// IMPORTANT: In serverless environments, goroutines may be interrupted if the
// process is frozen or restarted. In that case, implement another runner that
// enqueues a task (e.g., Cloud Tasks / PubSub) instead.
type LocalPlanGenerationRunner struct {
	jobsRepo   usecase.PlanJobsRepository
	generateUC *usecase.GeneratePlanUseCase

	// Controls max time allowed for a single plan generation.
	timeout time.Duration
}

func NewLocalPlanGenerationRunner(
	jobsRepo usecase.PlanJobsRepository,
	generateUC *usecase.GeneratePlanUseCase,
	timeout time.Duration,
) *LocalPlanGenerationRunner {
	if timeout <= 0 {
		timeout = 4 * time.Minute
	}
	return &LocalPlanGenerationRunner{
		jobsRepo:   jobsRepo,
		generateUC: generateUC,
		timeout:    timeout,
	}
}

// Run starts the background plan generation for an already-created job.
// This method must return immediately (non-blocking).
func (r *LocalPlanGenerationRunner) Run(userID, jobID, checkinID string) {
	userID = strings.TrimSpace(userID)
	jobID = strings.TrimSpace(jobID)
	checkinID = strings.TrimSpace(checkinID)

	// Fire-and-forget goroutine
	go func() {
		// Never use request contexts here. Create a fresh background context.
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		if err := r.jobsRepo.MarkRunning(ctx, userID, jobID); err != nil {
			log.Printf("[plan.runner] mark running failed user=%s job=%s err=%v", userID, jobID, err)
			// Continue anyway; the job can still complete.
		}

		result, err := r.generateUC.Generate(ctx, userID, checkinID)
		if err != nil {
			log.Printf("[plan.runner] generation failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, err.Error())
			return
		}
		if result.Plan == nil || strings.TrimSpace(result.Plan.ID) == "" {
			log.Printf("[plan.runner] generation returned empty plan user=%s job=%s", userID, jobID)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "plan generation returned empty plan")
			return
		}

		planID := result.Plan.ID
		if err := r.jobsRepo.MarkDone(ctx, userID, jobID, planID); err != nil {
			log.Printf("[plan.runner] mark done failed user=%s job=%s plan=%s err=%v", userID, jobID, planID, err)
			// No return: plan is already created, job status update failed only.
		}
	}()
}
