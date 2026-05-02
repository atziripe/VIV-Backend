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
	traningUC   *usecase.GenerateTrainingPlanUsecase
	nutritionUC *usecase.GenerateNutritionPlanUsecase
	userRepo    usecase.UserRepository
	checkinRepo usecase.CheckinRepository
	cycleSync   usecase.CyclePhaseLookup
	planRepo    usecase.PlanRepository
	timeout     time.Duration
}

func NewLocalTrainingPlanRunner(
	jobsRepo usecase.PlanJobsRepository,
	traningUC *usecase.GenerateTrainingPlanUsecase,
	nutritionUC *usecase.GenerateNutritionPlanUsecase,
	userRepo usecase.UserRepository,
	checkinRepo usecase.CheckinRepository,
	cycleSync usecase.CyclePhaseLookup,
	planRepo usecase.PlanRepository,
	timeout time.Duration,
) *LocalTrainingPlanRunner {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &LocalTrainingPlanRunner{
		jobsRepo:    jobsRepo,
		traningUC:   traningUC,
		nutritionUC: nutritionUC,
		userRepo:    userRepo,
		checkinRepo: checkinRepo,
		cycleSync:   cycleSync,
		planRepo:    planRepo,
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
			log.Printf("[plan.runner] mark running failed user=%s job=%s err=%v", userID, jobID, err)
		}

		// 1. Load user
		user, err := r.userRepo.GetByID(ctx, userID)
		if err != nil || user == nil {
			log.Printf("[plan.runner] user not found user=%s err=%v", userID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "user not found")
			return
		}

		// 2. Load checkin (optional)
		var checkin domain.Checkin
		if checkinID != "" {
			c, err := r.checkinRepo.GetByID(ctx, userID, checkinID)
			if err != nil {
				log.Printf("[plan.runner] checkin load failed user=%s err=%v", userID, err)
				_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to load checkin")
				return
			}
			if c != nil {
				checkin = *c
				log.Printf("[plan.runner] checkin loaded id=%s sleep=%s body=%s demand=%s readiness=%s",
					checkin.ID, checkin.Sleep, checkin.Body, checkin.Demand, checkin.Readiness)
			} else {
				log.Printf("[plan.runner] checkin not found id=%s", checkinID)
			}
		} else {
			log.Printf("[plan.runner] no checkin_id provided, using defaults")
		}

		// 3. Get current cycle phase
		phase, err := r.cycleSync.CurrentPhase(ctx, userID)
		if err != nil {
			log.Printf("[training.runner] cycle phase lookup failed user=%s err=%v", userID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to determine cycle phase")
			return
		}

		// 4. Execute training pipeline
		trainingOutput, err := r.traningUC.Execute(ctx, usecase.GenerateTrainingPlanInput{
			User:    *user,
			Checkin: checkin,
			Phase:   phase,
		})
		if err != nil {
			log.Printf("[training.runner] training generation failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, err.Error())
			return
		}

		// Serialize the plan to JSON and store
		trainingJSON, err := json.Marshal(trainingOutput.Plan)
		if err != nil {
			log.Printf("[plan.runner] training marshal failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to serialize training plan")
			return
		}

		// 5. Generate nutrition plan
		nutritionOutput, err := r.nutritionUC.Execute(ctx, usecase.GenerateNutritionPlanInput{
			User:         *user,
			Checkin:      checkin,
			Phase:        phase,
			TrainingPlan: trainingOutput.Plan,
		})

		var nutritionJSON []byte
		if err != nil {
			log.Printf("[plan.runner] nutrition generation failed user=%s job=%s err=%v", userID, jobID, err)
			// Don't fail the whole job — save plan without nutrition
		} else {
			nutritionJSON, err = json.Marshal(nutritionOutput.Plan)
			if err != nil {
				log.Printf("[plan.runner] nutrition marshal failed user=%s err=%v", userID, err)
				nutritionJSON = nil
			}
		}

		// 6.Save as Plan with JSON populated
		now := time.Now().UTC()
		weekStart := trainingOutput.Plan.Days[0].Weekday // monday
		plan := &domain.Plan{
			UserID:    userID,
			Status:    "active",
			CheckinID: checkinID,
			CreatedAt: now,
			StartDate: mondayOfCurrentWeek(now),
			EndDate:   mondayOfCurrentWeek(now).AddDate(0, 0, 6),

			TrainingJSON:  trainingJSON,
			NutritionJSON: nutritionJSON,

			GeneratedFrom: trainingOutput.TokensUsed.Model,
			PlanVersion:   2, // v2 = rule-based training pipeline
		}
		_ = weekStart // used above conceptually

		if err := r.planRepo.Create(ctx, plan); err != nil {
			log.Printf("[plan.runner] save plan failed user=%s job=%s err=%v", userID, jobID, err)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, "failed to save plan")
			return
		}

		// Update user's active plan pointer
		user.LastActivePlanID = &plan.ID
		if err := r.userRepo.Save(ctx, user); err != nil {
			log.Printf("[plan.runner] update LastActivePlanID failed user=%s plan=%s err=%v", userID, plan.ID, err)
			// Don't fail the job — plan is already saved
		}

		// 7. Log observability data
		totalTokens := trainingOutput.TokensUsed.PromptTokens + trainingOutput.TokensUsed.CompletionTokens
		if nutritionOutput.TokensUsed.PromptTokens > 0 {
			totalTokens += nutritionOutput.TokensUsed.PromptTokens + nutritionOutput.TokensUsed.CompletionTokens
		}

		log.Printf("[plan.runner] success user=%s job=%s plan=%s phase=%s training_sessions=%d total_tokens=%d retried=%v",
			userID, jobID, plan.ID, phase,
			trainingOutput.Budget.TotalSessions,
			totalTokens,
			trainingOutput.RetriedOnce,
		)
		// 8. Mark job done
		if err := r.jobsRepo.MarkDone(ctx, userID, jobID, plan.ID); err != nil {
			log.Printf("[plan.runner] mark done failed user=%s job=%s plan=%s err=%v", userID, jobID, plan.ID, err)
		}
	}()
}

func mondayOfCurrentWeek(now time.Time) time.Time {
	wd := now.Weekday()
	if wd == time.Sunday {
		wd = 7
	}
	monday := now.AddDate(0, 0, -int(wd-time.Monday))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}
