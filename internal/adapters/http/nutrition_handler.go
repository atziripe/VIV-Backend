package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"viv/internal/core/usecase"
)

type NutritionHandler struct {
	GetPlanUC *usecase.GetNutritionPlanUseCase
}

func NewNutritionHandler(
	getPlanUC *usecase.GetNutritionPlanUseCase,
) *NutritionHandler {
	return &NutritionHandler{
		GetPlanUC: getPlanUC,
	}
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
