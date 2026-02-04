package http

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"viv/internal/core/usecase"
)

type MeHandler struct {
	UC *usecase.GetCurrentUserUseCase
}

func NewMeHandler(uc *usecase.GetCurrentUserUseCase) *MeHandler {
	return &MeHandler{UC: uc}
}

// Lo que devolvemos al frontend (puedes ajustar campos)
type meResponse struct {
	ID                     string `json:"id"`
	Username               string `json:"username"`
	Name                   string `json:"name"`
	OnboardingCompleted    bool   `json:"onboarding_completed"`
	OnboardingCompletedInt int    `json:"onboarding_completed_int"`

	DOB            string  `json:"dob"`
	WeightKg       float64 `json:"weight_kg"`
	HeightCm       float64 `json:"height_cm"`
	CycleType      string  `json:"cycle_type"`
	CycleDay       string  `json:"cycle_day"`
	CyclePhase     string  `json:"cycle_phase"`
	CycleLength    string  `json:"cycle_duration"`
	PeriodDuration string  `json:"period_duration"`

	TrainingOften         string `json:"training_often"`
	TrainingDuration      string `json:"training_duration"`
	TrainingType          string `json:"training_type"`
	TrainingTime          string `json:"training_time"`
	TrainingGoals         string `json:"training_goals"`
	TrainingGuidanceLevel string `json:"training_guidance_level"`

	DietRestrictions       string `json:"diet_restrictions"`
	DietProteinResources   string `json:"diet_protein_resources"`
	MealsPerDay            string `json:"meals_per_day"`
	MealsTimingStability   string `json:"meals_timing_stability"`
	DigestionConditions    string `json:"digestion_conditions"`
	NutritionIntent        string `json:"nutrition_intent"`
	NutritionGuidanceLevel string `json:"nutrition_guidance_level"`

	SleepWindow        string `json:"sleep_window"`
	RecoveryAfterSleep string `json:"recovery_after_sleep"`
	SleepContinuity    string `json:"sleep_continuity"`
	LingeringMarker    string `json:"lingering_marker"`
	StressReactivity   string `json:"stress_reactivity"`
	StressLevel        string `json:"stress_level"`
	Priority           string `json:"priority"`

	HasActiveInjury      bool    `json:"has_active_injury"`
	TrainingPaused       bool    `json:"training_paused"`
	LastActivePlanID     *string `json:"last_active_plan_id,omitempty"`
	LastInjuryReportedAt *string `json:"last_injury_reported_at,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (h *MeHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	in := usecase.GetCurrentUserInput{
		UserID: userID,
	}

	out, err := h.UC.Execute(ctx, in)
	if err != nil {
		// MVP error handling, check it later
		log.Printf("http server error: %v", err)
		http.Error(w, "failed to get current user", http.StatusInternalServerError)
		return
	}

	u := out.User

	var lastInjuryStr *string
	if u.LastInjuryReportedAt != nil {
		s := u.LastInjuryReportedAt.UTC().Format("2006-01-02")
		lastInjuryStr = &s
	}

	createdStr := u.CreatedAt.UTC().Format(time.RFC3339)
	updatedStr := u.UpdatedAt.UTC().Format(time.RFC3339)
	onboardingInt := 0
	if u.OnboardingCompleted {
		onboardingInt = 1
	}

	resp := meResponse{
		ID:                     u.ID,
		Username:               u.Username,
		Name:                   u.Name,
		OnboardingCompleted:    u.OnboardingCompleted,
		OnboardingCompletedInt: onboardingInt,

		DOB:            u.DOB,
		WeightKg:       u.WeightKg,
		HeightCm:       u.HeightCm,
		CycleType:      u.CycleType,
		CycleDay:       u.CycleDay,
		CyclePhase:     u.CyclePhase,
		CycleLength:    u.CycleDuration,
		PeriodDuration: u.PeriodDuration,

		TrainingOften:         u.TrainingOften,
		TrainingDuration:      u.TrainingDuration,
		TrainingType:          u.TrainingType,
		TrainingTime:          u.TrainingTime,
		TrainingGoals:         u.TrainingGoals,
		TrainingGuidanceLevel: u.TrainingGuidanceLevel,

		DietRestrictions:       u.DietRestrictions,
		DietProteinResources:   u.DietProteinResources,
		MealsPerDay:            u.MealsPerDay,
		MealsTimingStability:   u.MealsTimingStability,
		DigestionConditions:    u.DigestionConditions,
		NutritionIntent:        u.NutritionIntent,
		NutritionGuidanceLevel: u.NutritionGuidanceLevel,

		SleepWindow:        u.SleepWindow,
		RecoveryAfterSleep: u.RecoveryAfterSleep,
		SleepContinuity:    u.SleepContinuity,
		LingeringMarker:    u.LingeringMarker,
		StressReactivity:   u.StressReactivity,
		StressLevel:        u.StressLevel,
		Priority:           u.Priority,

		HasActiveInjury:      u.HasActiveInjury,
		TrainingPaused:       u.TrainingPaused,
		LastActivePlanID:     u.LastActivePlanID,
		LastInjuryReportedAt: lastInjuryStr,

		CreatedAt: createdStr,
		UpdatedAt: updatedStr,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
