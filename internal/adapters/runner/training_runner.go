package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/training"
	"viv/internal/core/usecase"
)

// CopyEnricher enriches meal-option copy in the background, after the plan
// is already saved and the job marked done — see
// mealgen.AsyncCopyEnricher. Optional: leave nil to skip this step entirely
// (e.g. no OpenAI key configured, tests) without touching the rest of the
// pipeline.
type CopyEnricher interface {
	EnrichAsync(userID, planID string, plan domain.NutritionWeekPlan)
}

// LocalTrainingPlanRunner runs training plan generation asynchronously.
// Same pattern as LocalPlanGenerationRunner but uses the new training pipeline.
type LocalTrainingPlanRunner struct {
	jobsRepo     usecase.PlanJobsRepository
	traningUC    *usecase.GenerateTrainingPlanUsecase
	nutritionUC  *usecase.GenerateNutritionPlanUsecase
	userRepo     usecase.UserRepository
	checkinRepo  usecase.CheckinRepository
	cycleSync    usecase.CyclePhaseLookup
	planRepo     usecase.PlanRepository
	copyEnricher CopyEnricher
	timeout      time.Duration
}

func NewLocalTrainingPlanRunner(
	jobsRepo usecase.PlanJobsRepository,
	traningUC *usecase.GenerateTrainingPlanUsecase,
	nutritionUC *usecase.GenerateNutritionPlanUsecase,
	userRepo usecase.UserRepository,
	checkinRepo usecase.CheckinRepository,
	cycleSync usecase.CyclePhaseLookup,
	planRepo usecase.PlanRepository,
	copyEnricher CopyEnricher,
	timeout time.Duration,
) *LocalTrainingPlanRunner {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	return &LocalTrainingPlanRunner{
		jobsRepo:     jobsRepo,
		traningUC:    traningUC,
		nutritionUC:  nutritionUC,
		userRepo:     userRepo,
		checkinRepo:  checkinRepo,
		cycleSync:    cycleSync,
		planRepo:     planRepo,
		copyEnricher: copyEnricher,
		timeout:      timeout,
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

		startTotal := time.Now()

		// Initialize the generation log
		gl := generationLog{
			Event:     "plan_generation",
			UserID:    userID,
			JobID:     jobID,
			CheckinID: checkinID,
		}

		// Helper for failure logging
		fail := func(reason string) {
			gl.Success = false
			gl.Error = reason
			gl.TotalDurationMs = time.Since(startTotal).Milliseconds()
			logGeneration(gl)
			_ = r.jobsRepo.MarkFailed(context.Background(), userID, jobID, reason)
		}

		if err := r.jobsRepo.MarkRunning(ctx, userID, jobID); err != nil {
			log.Printf("[plan.runner] mark running failed user=%s job=%s err=%v", userID, jobID, err)
		}

		// ── Load dependencies ────────────────────────────────────
		user, err := r.userRepo.GetByID(ctx, userID)
		if err != nil || user == nil {
			fail("user not found")
			return
		}

		// 2. Load checkin (optional)
		var checkin domain.Checkin
		if checkinID != "" {
			c, err := r.checkinRepo.GetByID(ctx, userID, checkinID)
			if err != nil {
				fail("failed to load checkin: " + err.Error())
				return
			}
			if c != nil {
				checkin = *c
			}
		}

		// 3. Get current cycle phase
		phase, err := r.cycleSync.CurrentPhase(ctx, userID)
		if err != nil {
			fail("failed to determine cycle phase: " + err.Error())
			return
		}
		gl.Phase = string(phase)

		// 4. Execute training pipeline
		startTraining := time.Now()
		trainingOutput, err := r.traningUC.Execute(ctx, usecase.GenerateTrainingPlanInput{
			User:    *user,
			Checkin: checkin,
			Phase:   phase,
		})
		trainingDuration := time.Since(startTraining)
		if err != nil {
			gl.Training = &stepLog{
				DurationMs: int(trainingDuration.Milliseconds()),
				Failed:     true,
			}
			fail("training generation failed: " + err.Error())
			return
		}

		// Log training step
		trainingTokens := trainingOutput.TokensUsed.PromptTokens + trainingOutput.TokensUsed.CompletionTokens
		gl.Training = &stepLog{
			DurationMs: int(trainingDuration.Milliseconds()),
			Tokens:     trainingTokens,
			Retried:    trainingOutput.RetriedOnce,
		}

		// Log checkin dimensions
		dims := training.TranslateCheckin(checkin)
		gl.CheckinDimensions = &dimensionsLog{
			Recovery:  string(dims.Recovery),
			Bandwidth: string(dims.Bandwidth),
			Build:     string(dims.Build),
			RestWeek:  trainingOutput.Budget.RestWeekOffered,
		}

		// Log budget
		var modNames []string
		for _, m := range trainingOutput.Budget.Modalities {
			modNames = append(modNames, fmt.Sprintf("%s×%d", m.Modality, m.Count))
		}
		gl.Budget = &budgetLog{
			TotalSessions: trainingOutput.Budget.TotalSessions,
			Modalities:    modNames,
		}

		// Log violations if any
		for _, v := range trainingOutput.Decision.Violations {
			gl.Violations = append(gl.Violations, violationLog{
				RuleID:   v.RuleID,
				Severity: string(v.Severity),
			})
		}

		// Serialize the plan to JSON and store
		trainingJSON, err := json.Marshal(trainingOutput.Plan)
		if err != nil {
			fail("failed to serialize training plan")
			return
		}

		// 5. Generate nutrition plan
		startNutrition := time.Now()
		nutritionOutput, err := r.nutritionUC.Execute(ctx, usecase.GenerateNutritionPlanInput{
			User:         *user,
			Checkin:      checkin,
			Phase:        phase,
			TrainingPlan: trainingOutput.Plan,
		})
		nutritionDuration := time.Since(startNutrition)
		nutritionSucceeded := err == nil // captured before err gets reassigned below (marshal, save, ...)

		var nutritionJSON []byte
		if err != nil {
			gl.Nutrition = &stepLog{
				DurationMs: int(nutritionDuration.Milliseconds()),
				Failed:     true,
			}
			log.Printf("[plan.runner] nutrition generation failed (non-fatal): %v", err)
		} else {
			nutritionTokens := nutritionOutput.TokensUsed.PromptTokens + nutritionOutput.TokensUsed.CompletionTokens
			gl.Nutrition = &stepLog{
				DurationMs: int(nutritionDuration.Milliseconds()),
				Tokens:     nutritionTokens,
			}

			nutritionJSON, err = json.Marshal(nutritionOutput.Plan)
			if err != nil {
				log.Printf("[plan.runner] nutrition marshal failed user=%s err=%v", userID, err)
				nutritionJSON = nil
			}
		}

		// 6.Save as Plan with JSON populated
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer saveCancel()

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

		if err := r.planRepo.Create(saveCtx, plan); err != nil {
			fail("failed to save plan")
			return
		}

		// Update user's active plan pointer
		user.LastActivePlanID = &plan.ID
		if err := r.userRepo.Save(ctx, user); err != nil {
			log.Printf("[plan.runner] update LastActivePlanID failed user=%s plan=%s err=%v", userID, plan.ID, err)
		}

		// Kick off copy enrichment in the background — the plan above is
		// already correct and saved, this only improves Name/Summary text
		// later and must never delay job completion below.
		if r.copyEnricher != nil && nutritionSucceeded {
			r.copyEnricher.EnrichAsync(userID, plan.ID, nutritionOutput.Plan)
		}

		// ── Finalize ─────────────────────────────────────
		gl.PlanID = plan.ID
		gl.Success = true
		gl.TotalTokens = trainingTokens
		if gl.Nutrition != nil {
			gl.TotalTokens += gl.Nutrition.Tokens
		}
		gl.TotalDurationMs = time.Since(startTotal).Milliseconds()

		logGeneration(gl)

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

// ============================================================================
// OBSERVABILITY
// ============================================================================

type generationLog struct {
	Event             string         `json:"event"`
	UserID            string         `json:"user_id"`
	JobID             string         `json:"job_id"`
	PlanID            string         `json:"plan_id,omitempty"`
	Phase             string         `json:"phase"`
	CheckinID         string         `json:"checkin_id,omitempty"`
	CheckinDimensions *dimensionsLog `json:"checkin_dimensions,omitempty"`
	Budget            *budgetLog     `json:"budget,omitempty"`
	Violations        []violationLog `json:"violations,omitempty"`
	Training          *stepLog       `json:"training,omitempty"`
	Nutrition         *stepLog       `json:"nutrition,omitempty"`
	TotalTokens       int            `json:"total_tokens"`
	TotalDurationMs   int64          `json:"total_duration_ms"`
	Success           bool           `json:"success"`
	Error             string         `json:"error,omitempty"`
}

type dimensionsLog struct {
	Recovery  string `json:"recovery"`
	Bandwidth string `json:"bandwidth"`
	Build     string `json:"build"`
	RestWeek  bool   `json:"rest_week_offered"`
}

type budgetLog struct {
	TotalSessions int      `json:"total_sessions"`
	Modalities    []string `json:"modalities"`
}

type stepLog struct {
	DurationMs int  `json:"duration_ms"`
	Tokens     int  `json:"tokens"`
	Retried    bool `json:"retried,omitempty"`
	Failed     bool `json:"failed,omitempty"`
}

type violationLog struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
}

func logGeneration(gl generationLog) {
	data, err := json.Marshal(gl)
	if err != nil {
		log.Printf("[plan.runner] failed to marshal generation log: %v", err)
		return
	}
	log.Printf("[plan.generation] %s", string(data))
}
