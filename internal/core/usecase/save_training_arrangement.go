package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"viv/internal/core/domain"
	"viv/internal/core/training"
)

type SaveTrainingArrangementInput struct {
	UserID         string
	TrainingPlanID string
	DaysJson       string //validated arrnagement JSON from frontend
}

type SaveTrainingArrangementUseCase struct {
	plans    PlanRepository
	checkins CheckinRepository
	library  *training.Library
}

func NewSaveTrainingArrangementUseCase(
	plans PlanRepository,
	checkins CheckinRepository,
	library *training.Library,
) *SaveTrainingArrangementUseCase {
	return &SaveTrainingArrangementUseCase{
		plans:    plans,
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
		Weekday string                  `json:"weekday"`
		Session *domain.TrainingSession `json:"session"`
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
			return fmt.Errorf("invlaida weekday: %q", d.Weekday)
		}
		arrangement.Days[idx] = domain.TrainingDaySlot{
			Session: d.Session,
		}
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

	return uc.plans.UpdatedTrainingJSON(ctx, in.UserID, in.TrainingPlanID, updatedJSON)
}
