package http

import (
	"encoding/json"
	"log"
	"net/http"
	"viv/internal/core/usecase"
)

type onboardingRequest struct {
	Name             string  `json:"name"`
	DOB              string  `json:"dob"`
	WeightKg         float64 `json:"weight_kg"`
	HeightCm         float64 `json:"height_cm"`
	CycleType        string  `json:"cycle_type"`
	CycleDay         string  `json:"cycle_day"`
	CycleDuration    string  `json:"cycle_duration"`
	CyclePhase       string  `json:"cycle_phase"`
	PeriodDuration   string  `json:"period_duration"`
	TrainingOften    string  `json:"training_often"`
	TrainingDuration string  `json:"training_duration"`
	TrainingType     string  `json:"training_type"`
	TrainingTime     string  `json:"training_time"`
	TrainingGoals    string  `json:"training_goals"`

	DietRestrictions     string `json:"diet_restrictions"`
	DietProteinResources string `json:"diet_protein_resources"`
	MealsPerDay          string `json:"meals_per_day"`
	MealsTimingStability string `json:"meals_timing_stability"`
	DigestionConditions  string `json:"digestion_conditions"`

	SleepWindow        string `json:"sleep_window"`
	RecoveryAfterSleep string `json:"recovery_after_sleep"`
	SleepContinuity    string `json:"sleep_continuity"`
	DailyActivityLevel string `json:"daily_activity_level"`
	LingeringMarker    string `json:"lingering_marker"`
	StressReactivity   string `json:"stress_reactivity"`
	StressLevel        string `json:"stress_level"`
	Priority           string `json:"priority"`
}

type onboardingResponse struct {
	ID                  string  `json:"id"`
	OnboardingCompleted bool    `json:"onboarding_completed"`
	DOB                 string  `json:"dob"`
	WeightKg            float64 `json:"weight_kg"`
	HeightCm            float64 `json:"height_cm"`
	TrainingOften       string  `json:"training_often"`
	TrainingType        string  `json:"training_type"`
	TrainingTime        string  `json:"training_time"`
	TrainingGoals       string  `json:"training_goals"`
	SleepWindow         string  `json:"sleep_window"`
	StressLevel         string  `json:"stress_level"`
	Priority            string  `json:"priority"`
}

type OnboardingHandler struct {
	UC *usecase.CompleteOnboardingUseCase
}

func NewOnboardingHandler(uc *usecase.CompleteOnboardingUseCase) *OnboardingHandler {
	return &OnboardingHandler{UC: uc}
}

func (h *OnboardingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Obtener userID del contexto
	ctx := r.Context()
	userIDValue := ctx.Value(userIDContextKey)
	userID, ok := userIDValue.(string)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parsear JSON del body
	var req onboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[onboarding] decode error: %v", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// 3. Mapear a input del use case
	in := usecase.CompleteOnboardingInput{
		UserID:           userID,
		Name:             req.Name,
		DOB:              req.DOB,
		WeightKg:         req.WeightKg,
		HeightCm:         req.HeightCm,
		CycleType:        req.CycleType,
		CycleDay:         req.CycleDay,
		CyclePhase:       req.CyclePhase,
		CycleDuration:    req.CycleDuration,
		PeriodDuration:   req.PeriodDuration,
		TrainingOften:    req.TrainingOften,
		TrainingDuration: req.TrainingDuration,
		TrainingType:     req.TrainingType,
		TrainingTime:     req.TrainingTime,
		TrainingGoals:    req.TrainingGoals,

		DietRestrictions:     req.DietRestrictions,
		DietProteinResources: req.DietProteinResources,
		MealsPerDay:          req.MealsPerDay,
		MealsTimingStability: req.MealsTimingStability,
		DigestionConditions:  req.DigestionConditions,

		SleepWindow:        req.SleepWindow,
		RecoveryAfterSleep: req.RecoveryAfterSleep,
		SleepContinuity:    req.SleepContinuity,
		LingeringMarker:    req.LingeringMarker,
		DailyActivityLevel: req.DailyActivityLevel,
		StressReactivity:   req.StressReactivity,
		StressLevel:        req.StressLevel,
		Priority:           req.Priority,
	}

	out, err := h.UC.Execute(ctx, in)
	if err != nil {
		http.Error(w, "failed to complete onboarding", http.StatusInternalServerError)
		return
	}

	// 5. Mapear a response
	resp := onboardingResponse{
		ID:                  out.User.ID,
		OnboardingCompleted: out.User.OnboardingCompleted,
		DOB:                 out.User.DOB,
		WeightKg:            out.User.WeightKg,
		HeightCm:            out.User.HeightCm,
		TrainingOften:       out.User.TrainingOften,
		TrainingType:        out.User.TrainingType,
		TrainingTime:        out.User.TrainingTime,
		SleepWindow:         out.User.SleepWindow,
		StressLevel:         out.User.StressLevel,
		Priority:            out.User.Priority,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}
