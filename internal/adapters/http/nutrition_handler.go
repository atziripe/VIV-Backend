package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/usecase"
)

type NutritionHandler struct {
	GetPlanUC                    *usecase.GetNutritionPlanUseCase
	MealSelectionUC              *usecase.SaveMealSelectionUseCase
	SubmitOnboardingUC           *usecase.SubmitNutritionOnboardingUseCase
	SaveNutritionMealSelectionUC *usecase.SaveNutritionMealSelectionUseCase
}

func NewNutritionHandler(
	getPlanUC *usecase.GetNutritionPlanUseCase,
	mealSelectionUC *usecase.SaveMealSelectionUseCase,
	submitOnboardingUC *usecase.SubmitNutritionOnboardingUseCase,
	saveNutritionMealSelectionUC *usecase.SaveNutritionMealSelectionUseCase,
) *NutritionHandler {
	return &NutritionHandler{
		GetPlanUC:                    getPlanUC,
		MealSelectionUC:              mealSelectionUC,
		SubmitOnboardingUC:           submitOnboardingUC,
		SaveNutritionMealSelectionUC: saveNutritionMealSelectionUC,
	}
}

type mealSelectionRequest struct {
	PlanID      string `json:"plan_id"`
	Weekday     string `json:"weekday"`
	MealSlot    string `json:"meal_slot"`
	OptionIndex int    `json:"option_index"`
}

type submitNutritionOnboardingRequest struct {
	Date                 string `json:"date"` // "2006-01-02", optional — defaults to today (UTC)
	DietRestrictions     string `json:"diet_restrictions"`
	DietProteinResources string `json:"diet_protein_resources"`
	MealsPerDay          string `json:"meals_per_day"`
	MealsTimingStability string `json:"meals_timing_stability"`
	DigestionConditions  string `json:"digestion_conditions"`
	EatingStyle          string `json:"eating_style"`
}

// GET /nutrition/plan
func (h *NutritionHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	planID := strings.TrimSpace(r.URL.Query().Get("plan_id"))

	output, err := h.GetPlanUC.Execute(ctx, usecase.GetNutritionPlanInput{
		UserID: userID,
		PlanID: planID,
	})
	if err != nil {
		log.Printf("[nutrition.plan] error: %+v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(output.Nutrition)
}

func (h *NutritionHandler) SaveMealSelection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req mealSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if req.Weekday == "" || req.MealSlot == "" {
		http.Error(w, "weekday and meal_slot are required", http.StatusBadRequest)
		return
	}

	// New pipeline: one nutrition plan per user, no plan_id — same
	// no-plan_id-means-new-pipeline convention as GET /nutrition/plan.
	if strings.TrimSpace(req.PlanID) == "" {
		err := h.SaveNutritionMealSelectionUC.Execute(ctx, usecase.SaveNutritionMealSelectionInput{
			UserID:      userID,
			Weekday:     req.Weekday,
			MealSlot:    req.MealSlot,
			OptionIndex: req.OptionIndex,
		})
		if err != nil {
			log.Printf("[nutrition.meal-selection] error: %+v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
		return
	}

	err := h.MealSelectionUC.Execute(ctx, usecase.SaveMealSelectionInput{
		UserID:      userID,
		PlanID:      req.PlanID,
		Weekday:     req.Weekday,
		MealSlot:    req.MealSlot,
		OptionIndex: req.OptionIndex,
	})
	if err != nil {
		log.Printf("[nutrition.meal-selection] error: %+v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}

// POST /nutrition/onboarding — the diet-preference questions, asked only
// once the user opts into the nutrition module (not during account
// onboarding). Saves the answers onto the user's profile and generates
// their nutrition plan synchronously.
func (h *NutritionHandler) SubmitOnboarding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req submitNutritionOnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	var date time.Time
	if d := strings.TrimSpace(req.Date); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
			return
		}
		date = parsed
	}

	out, err := h.SubmitOnboardingUC.Execute(ctx, usecase.SubmitNutritionOnboardingInput{
		UserID:               userID,
		Date:                 date,
		DietRestrictions:     req.DietRestrictions,
		DietProteinResources: req.DietProteinResources,
		MealsPerDay:          req.MealsPerDay,
		MealsTimingStability: req.MealsTimingStability,
		DigestionConditions:  req.DigestionConditions,
		EatingStyle:          req.EatingStyle,
	})
	if err != nil {
		log.Printf("[nutrition.onboarding] error: %+v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out.Nutrition)
}
