package runner

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// LocalTrainingPlanRunner runs training plan generation asynchronously.
// Same pattern as LocalPlanGenerationRunner but uses the new training pipeline.
type LocalTrainingPlanRunner struct {
	jobsRepo    usecase.PlanJobsRepository
	generateUC  *usecase.GenerateTrainingPlanUsecase
	userRepo    usecase.UserRepository
	checkinRepo usecase.CheckinRepository
	cycleSync   usecase.CyclePhaseLookup
	timeout     time.Duration
}

func NewLocalTrainingPlanRunner(
	jobsRepo usecase.PlanJobsRepository,
	generateUC *usecase.GenerateTrainingPlanUsecase,
	userRepo usecase.UserRepository,
	checkinRepo usecase.CheckinRepository,
	cycleSync usecase.CyclePhaseLookup,
	timeout time.Duration,
) *LocalTrainingPlanRunner {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &LocalTrainingPlanRunner{
		jobsRepo:    jobsRepo,
		generateUC:  generateUC,
		userRepo:    userRepo,
		checkinRepo: checkinRepo,
		cycleSync:   cycleSync,
		timeout:     timeout,
	}
}

// Run starts background training plan generation.
// Must return immediately (non-blocking).
func (r *LocalTrainingPlanRunner) Run(userID, jobID, checkinID string) {
	userID = strings.TrimSpace(userID)
	jobID = strings.TrimSpace(jobID)
	checkinID = strings.TrimSpace(checkinID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		if err := r.jobsRepo.MarkRunning(ctx, userID, jobID); err != nil {
			log.Printf("[training.runner] mark running failed user=%s job=%s err=%v", userID, jobID, err)
		}

		// 1. Load user
		user, err := r.userRepo.GetByID(ctx, userID)
		if err != nil || user == nil {
			log.Printf("[training.runner] user not found user=%s err=%v", userID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "user not found")
			return
		}

		// 2. Load checkin (optional)
		var checkin domain.Checkin
		if checkinID != "" {
			c, err := r.checkinRepo.GetByID(ctx, userID, checkinID)
			if err != nil {
				log.Printf("[training.runner] checkin load failed user=%s checkin=%s err=%v", userID, checkinID, err)
				_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to load checkin")
				return
			}
			if c != nil {
				checkin = *c
			}
		}

		// 3. Get current cycle phase
		phase, err := r.cycleSync.CurrentPhase(ctx, userID)
		if err != nil {
			log.Printf("[training.runner] cycle phase lookup failed user=%s err=%v", userID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to determine cycle phase")
			return
		}

		// 4. Execute training pipeline
		output, err := r.generateUC.Execute(ctx, usecase.GenerateTrainingPlanInput{
			User:    *user,
			Checkin: checkin,
			Phase:   phase,
		})
		if err != nil {
			log.Printf("[training.runner] generation failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, err.Error())
			return
		}

		// 5. Serialize the plan to JSON and store
		planJSON, err := json.Marshal(output.Plan)
		if err != nil {
			log.Printf("[training.runner] marshal failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to serialize plan")
			return
		}

		// 6. Save plan (reuse existing plan infrastructure)
		planID, err := r.savePlan(ctx, userID, checkinID, planJSON, output)
		if err != nil {
			log.Printf("[training.runner] save failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to save plan")
			return
		}

		// 7. Log observability data
		log.Printf("[training.runner] success user=%s job=%s plan=%s tokens=%d retried=%v",
			userID, jobID, planID, output.TokensUsed.PromptTokens+output.TokensUsed.CompletionTokens, output.RetriedOnce)

		// 8. Mark job done
		if err := r.jobsRepo.MarkDone(ctx, userID, jobID, planID); err != nil {
			log.Printf("[training.runner] mark done failed user=%s job=%s plan=%s err=%v", userID, jobID, planID, err)
		}
	}()
}

// savePlan persists the training plan. For now, stores as TrainingJSON
// in the existing Plan struct. This bridges old and new systems.
func (r *LocalTrainingPlanRunner) savePlan(
	ctx context.Context,
	userID, checkinID string,
	trainingJSON []byte,
	output usecase.GenerateTrainingPlanOutput,
) (string, error) {
	// TODO: implement plan persistence
	// This should create a domain.Plan with TrainingJSON populated
	// and save it via the plan repository.
	// For now, return a placeholder — you'll wire this to your existing
	// plan_firestore.go repository.
	_ = ctx
	_ = userID
	_ = checkinID
	_ = trainingJSON
	_ = output
	return "plan_placeholder_id", nil
}
