package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"viv/internal/core/usecase"
)

type NutritionHandler struct {
	GetPlanUC       *usecase.GetNutritionPlanUseCase
	MealSelectionUC *usecase.SaveMealSelectionUseCase
}

func NewNutritionHandler(
	getPlanUC *usecase.GetNutritionPlanUseCase,
	mealSelectionUC *usecase.SaveMealSelectionUseCase,
) *NutritionHandler {
	return &NutritionHandler{
		GetPlanUC:       getPlanUC,
		MealSelectionUC: mealSelectionUC,
	}
}

type mealSelectionRequest struct {
	PlanID      string `json:"plan_id"`
	Weekday     string `json:"weekday"`
	MealSlot    string `json:"meal_slot"`
	OptionIndex int    `json:"option_index"`
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

	if req.PlanID == "" || req.Weekday == "" || req.MealSlot == "" {
		http.Error(w, "plan_id, weekday, and meal_slot are required", http.StatusBadRequest)
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
