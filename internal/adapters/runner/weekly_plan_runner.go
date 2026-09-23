package runner

import (
	"context"
	"log"
	"strings"
	"time"

	"viv/internal/core/usecase"
)

// LocalWeeklyPlanRunner runs the new weekly-plan pipeline (VIV-106..113)
// asynchronously — same in-process-goroutine-per-job pattern as
// LocalTrainingPlanRunner, sized down because GenerateWeeklyPlanUsecase
// is a single orchestrated call rather than a training+nutrition pair.
type LocalWeeklyPlanRunner struct {
	jobsRepo   usecase.PlanJobsRepository
	generateUC *usecase.GenerateWeeklyPlanUsecase
	timeout    time.Duration
}

func NewLocalWeeklyPlanRunner(
	jobsRepo usecase.PlanJobsRepository,
	generateUC *usecase.GenerateWeeklyPlanUsecase,
	timeout time.Duration,
) *LocalWeeklyPlanRunner {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &LocalWeeklyPlanRunner{jobsRepo: jobsRepo, generateUC: generateUC, timeout: timeout}
}

// Run starts background weekly-plan generation. Must return immediately
// (non-blocking) — implements usecase.WeeklyPlanGenerationRunner. input
// carries whatever the caller already knows (StartWeeklyPlanGenerationUseCase
// leaves GoalID/Catalog/Readiness unset, letting GenerateWeeklyPlanUsecase
// fall back to its documented defaults; CompleteOnboardingUseCase sets them
// from the just-saved user instead).
func (r *LocalWeeklyPlanRunner) Run(jobID string, input usecase.GenerateWeeklyPlanInput) {
	jobID = strings.TrimSpace(jobID)
	userID := strings.TrimSpace(input.UserID)
	input.UserID = userID

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		if err := r.jobsRepo.MarkRunning(ctx, userID, jobID); err != nil {
			log.Printf("[weeklyplan.runner] mark running failed user=%s job=%s err=%v", userID, jobID, err)
		}

		out, err := r.generateUC.Execute(ctx, input)
		if err != nil {
			log.Printf("[weeklyplan.runner] generation failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, err.Error())
			return
		}

		if err := r.jobsRepo.MarkDone(ctx, userID, jobID, out.Draft.ID); err != nil {
			log.Printf("[weeklyplan.runner] mark done failed user=%s job=%s draft=%s err=%v", userID, jobID, out.Draft.ID, err)
		}
	}()
}
