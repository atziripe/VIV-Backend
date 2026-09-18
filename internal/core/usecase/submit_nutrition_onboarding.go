package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// ============================================================================
// SUBMIT NUTRITION ONBOARDING — nutrition is no longer asked upfront during
// account onboarding (see CompleteOnboardingUseCase). It's deferred until
// the user opts into the nutrition module from the app, at which point the
// client asks the diet-preference questions and submits them here. This
// edits the same domain.User record onboarding already created, then
// generates the nutrition plan synchronously — safe inline because meal
// content is a deterministic ingredient/template solve (mealgen), not a
// live LLM call.
// ============================================================================

type SubmitNutritionOnboardingInput struct {
	UserID string
	Date   time.Time // local calendar date, client-sent, same convention as every other date-scoped endpoint here; zero value defaults to now (UTC)

	DietRestrictions     string
	DietProteinResources string
	MealsPerDay          string
	MealsTimingStability string
	DigestionConditions  string
	EatingStyle          string
}

type SubmitNutritionOnboardingOutput struct {
	Nutrition domain.NutritionWeekPlan
}

type SubmitNutritionOnboardingUseCase struct {
	Users          UserRepository
	Drafts         WeeklyPlanDraftRepository
	Cycle          CyclePhaseLookup
	NutritionGen   *GenerateNutritionPlanUsecase
	NutritionPlans NutritionPlanRepository
	CopyEnricher   NutritionPlanCopyEnricher // optional, nil-safe
}

func NewSubmitNutritionOnboardingUseCase(
	users UserRepository,
	drafts WeeklyPlanDraftRepository,
	cycle CyclePhaseLookup,
	nutritionGen *GenerateNutritionPlanUsecase,
	nutritionPlans NutritionPlanRepository,
	copyEnricher NutritionPlanCopyEnricher,
) *SubmitNutritionOnboardingUseCase {
	return &SubmitNutritionOnboardingUseCase{
		Users:          users,
		Drafts:         drafts,
		Cycle:          cycle,
		NutritionGen:   nutritionGen,
		NutritionPlans: nutritionPlans,
		CopyEnricher:   copyEnricher,
	}
}

func (uc *SubmitNutritionOnboardingUseCase) Execute(ctx context.Context, in SubmitNutritionOnboardingInput) (SubmitNutritionOnboardingOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: userID is required")
	}

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: loading user: %w", err)
	}
	if user == nil {
		return SubmitNutritionOnboardingOutput{}, ErrUserNotFound(userID)
	}
	if !user.OnboardingCompleted {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: user %s hasn't completed onboarding yet", userID)
	}

	date := in.Date
	if date.IsZero() {
		date = time.Now().UTC()
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	draft, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: loading current week: %w", err)
	}
	if draft == nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: no generated training week found for user %s — training must be generated before nutrition", userID)
	}

	user.DietRestrictions = in.DietRestrictions
	user.DietProteinResources = in.DietProteinResources
	user.MealsPerDay = in.MealsPerDay
	user.MealsTimingStability = in.MealsTimingStability
	user.DigestionConditions = in.DigestionConditions
	user.EatingStyle = in.EatingStyle
	user.UpdatedAt = time.Now().UTC()

	if err := uc.Users.Save(ctx, user); err != nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: saving user: %w", err)
	}

	phase, err := uc.Cycle.CurrentPhase(ctx, userID)
	if err != nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: getting cycle phase: %w", err)
	}

	out, err := uc.NutritionGen.Execute(ctx, GenerateNutritionPlanInput{
		User:         *user,
		Checkin:      domain.Checkin{}, // no old-pipeline checkin exists for a new-pipeline user; AppetiteV2 is best-effort/optional downstream, same as update_profile.go's recomputeNutritionSync
		Phase:        phase,
		TrainingPlan: trainingWeekPlanFromDraft(*draft),
	})
	if err != nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: generating nutrition plan: %w", err)
	}

	if err := uc.NutritionPlans.Save(ctx, userID, out.Plan); err != nil {
		return SubmitNutritionOnboardingOutput{}, fmt.Errorf("submit nutrition onboarding: saving nutrition plan: %w", err)
	}

	if uc.CopyEnricher != nil {
		uc.CopyEnricher.EnrichAsync(userID, out.Plan)
	}

	return SubmitNutritionOnboardingOutput{Nutrition: out.Plan}, nil
}

// trainingWeekPlanFromDraft adapts a new-pipeline WeekDraft into the
// old-pipeline domain.TrainingWeekPlan shape GenerateNutritionPlanUsecase
// expects. Deliberately minimal: AssembleNutritionPlan/buildMealInput only
// ever read Days[i].IsRestDay and Days[i].Session.DurationMinutes (to know
// training-vs-rest and derive hydration) — Modality/MuscleGroup/Intensity
// are never read, so they're left zero-value rather than guessed at from
// the new taxonomy's activity.ID enums.
func trainingWeekPlanFromDraft(draft WeekDraft) domain.TrainingWeekPlan {
	var days [7]domain.TrainingPlanDay
	for i, day := range draft.Days {
		days[i] = domain.TrainingPlanDay{
			Weekday:   day.Weekday,
			IsRestDay: day.IsRestDay,
		}
		if !day.IsRestDay {
			days[i].Session = &domain.TrainingSession{
				DurationMinutes: durationMinutesForDay(day),
			}
		}
	}
	return domain.TrainingWeekPlan{Days: days}
}

// durationMinutesForDay mirrors GetWeeklyPlanDayUseCase.Execute's duration
// derivation (session-library days have an author-specified duration;
// mesocycle-pinned/exercise-list days don't, so it's estimated the same
// way via assumedSecondsPerSet) — kept small and separate rather than
// exporting/reusing that method's internals across an unrelated read path.
func durationMinutesForDay(day DayPlan) int {
	if day.Content == nil {
		return 0
	}
	if day.Content.Session != nil {
		return day.Content.Session.DurationMinutes
	}
	if day.Content.Exercises != nil {
		totalSeconds := 0
		for _, pe := range day.Content.Exercises {
			totalSeconds += pe.Prescription.Sets * (pe.Prescription.RestSeconds + assumedSecondsPerSet)
		}
		return (totalSeconds + 30) / 60
	}
	return 0
}
