package http

import (
	"encoding/json"
	"net/http"

	"viv/internal/core/usecase"
)

type TrainingHandler struct {
	ResumeUC *usecase.ResumeTrainingUseCase
}

func NewTrainingHandler(resumeUC *usecase.ResumeTrainingUseCase) *TrainingHandler {
	return &TrainingHandler{ResumeUC: resumeUC}
}

// Request body (puedes mandarlo vacío si quieres el default)
type resumeTrainingRequest struct {
	ClearInjury bool `json:"clear_injury"`
}

// Response con flags actualizados
type resumeTrainingResponse struct {
	TrainingPaused   bool    `json:"training_paused"`
	HasActiveInjury  bool    `json:"has_active_injury"`
	LastActivePlanID *string `json:"last_active_plan_id,omitempty"`
}

func (h *TrainingHandler) Resume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req resumeTrainingRequest
	// body puede venir vacío, así que si da error pero no hay body, puedes ignorar:
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	in := usecase.ResumeTrainingInput{
		UserID:      userID,
		ClearInjury: req.ClearInjury,
	}

	out, err := h.ResumeUC.Execute(ctx, in)
	if err != nil {
		http.Error(w, "failed to resume training", http.StatusInternalServerError)
		return
	}

	u := out.User

	resp := resumeTrainingResponse{
		TrainingPaused:   u.TrainingPaused,
		HasActiveInjury:  u.HasActiveInjury,
		LastActivePlanID: u.LastActivePlanID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
