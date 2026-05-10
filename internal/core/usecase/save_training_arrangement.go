package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"viv/internal/core/domain"
	"viv/internal/core/nutrition"
	"viv/internal/core/training"
)

type SaveTrainingArrangementInput struct {
	UserID         string
	TrainingPlanID string
	DaysJson       string //validated arrnagement JSON from frontend
}

type SaveTrainingArrangementUseCase struct {
	plans    PlanRepository
	users    UserRepository
	checkins CheckinRepository
	library  *training.Library
}

func NewSaveTrainingArrangementUseCase(
	plans PlanRepository,
	users UserRepository,
	checkins CheckinRepository,
	library *training.Library,
) *SaveTrainingArrangementUseCase {
	return &SaveTrainingArrangementUseCase{
		plans:    plans,
		users:    users,
		checkins: checkins,
		library:  library,
	}
}

func (uc *SaveTrainingArrangementUseCase) Execute(ctx context.Context, in SaveTrainingArrangementInput) error {
	if strings.TrimSpace(in.UserID) == "" || strings.TrimSpace(in.TrainingPlanID) == "" {
		return fmt.Errorf("user_id and training plan id are required")
	}

	plan, err := uc.plans.GetByID(ctx, in.UserID, in.TrainingPlanID)
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("plan not found: %s", in.TrainingPlanID)
	}

	//Parse the existing training plan
	var existingTrainingPlan domain.TrainingWeekPlan
	if err := json.Unmarshal(plan.TrainingJSON, &existingTrainingPlan); err != nil {
		return fmt.Errorf("parsing exsiting training plan: %w", err)
	}

	// Load latest checkin and compute real dimnesions
	dimensions := domain.CheckinDimensions{
		Recovery:  domain.RecoveryModerate,
		Bandwidth: domain.BandwidthModerate,
		Build:     domain.ReadinessMaintain,
	}

	latestCheckin, err := uc.checkins.GetLatestByUser(ctx, in.UserID)
	if err == nil && latestCheckin != nil {
		dimensions = training.TranslateCheckin(*latestCheckin)
	}

	//Parse the new days arrangement
	var newDays []struct {
		Weekday   string `json:"weekday"`
		IsRestDay bool   `json:"is_rest_day"`
		Session   *struct {
			Modality        string `json:"modality"`
			MuscleGroup     string `json:"muscle_group"`
			Intensity       string `json:"intensity"`
			DurationMinutes int    `json:"duration_minutes"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(in.DaysJson), &newDays); err != nil {
		return fmt.Errorf("parsing new arrangement: %w", err)
	}

	if len(newDays) != 7 {
		return fmt.Errorf("expected 7 days, got %d", len(newDays))
	}

	// Re-assemble: build a WeekArrangement and run it through AssemblePlan
	var arrangement domain.WeekArrangement
	arrangement.WeekStart = plan.StartDate

	weekdayMap := map[string]int{
		"monday": 0, "tuesday": 1, "wednesday": 2, "thursday": 3, "friday": 4, "saturday": 5, "sunday": 6,
	}

	for _, d := range newDays {
		idx, ok := weekdayMap[strings.ToLower(d.Weekday)]
		if !ok {
			return fmt.Errorf("invalid weekday: %q", d.Weekday)
		}
		slot := domain.TrainingDaySlot{}
		if !d.IsRestDay && d.Session != nil {
			slot.Session = &domain.TrainingSession{
				Modality:        domain.Modality(strings.ToUpper(d.Session.Modality)),
				MuscleGroup:     domain.MuscleGroup(strings.ToLower(d.Session.MuscleGroup)),
				Intensity:       domain.Intensity(strings.ToLower(d.Session.Intensity)),
				DurationMinutes: d.Session.DurationMinutes,
			}
		}
		arrangement.Days[idx] = slot
	}

	// Re-assemble with content from library + real dimensions
	updatedPlan := training.AssemblePlan(
		arrangement,
		existingTrainingPlan.Phase,
		uc.library,
		dimensions,
		existingTrainingPlan.RestWeekOffered,
	)

	// Serialize
	updatedJSON, err := json.Marshal(updatedPlan)
	if err != nil {
		return fmt.Errorf("serializing updated plan: %w", err)
	}

	if err := uc.plans.UpdatedTrainingJSON(ctx, in.UserID, in.TrainingPlanID, updatedJSON); err != nil {
		log.Printf("[save_arrangement] training update failed plan=%s err=%v", in.TrainingPlanID, err)
		return fmt.Errorf("training update failed plan=%s err=%v", in.TrainingPlanID, err)
	}

	// Re-assemble nutrition to reflect the updated training/rest day layout.
	// Meals are preserved — only macros, hydration, and is_training_day are recalculated.
	if len(plan.NutritionJSON) > 0 {
		user, err := uc.users.GetByID(ctx, in.UserID)
		if err != nil || user == nil {
			log.Printf("[save_arrangement] could not load user for nutrition update user=%s err=%v", in.UserID, err)
		} else {
			var existingNutrition domain.NutritionWeekPlan
			if err := json.Unmarshal(plan.NutritionJSON, &existingNutrition); err == nil {
				updatedNutrition := nutrition.ReassembleDays(existingNutrition, updatedPlan, user.WeightKg)
				if nutritionJSON, err := json.Marshal(updatedNutrition); err == nil {
					if err := uc.plans.UpdateNutritionJSON(ctx, in.UserID, in.TrainingPlanID, nutritionJSON); err != nil {
						log.Printf("[save_arrangement] nutrition update failed plan=%s err=%v", in.TrainingPlanID, err)
					}
				}
			}
		}
	}
	return nil
}
