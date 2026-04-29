package http

import (
	"encoding/json"
	"log"
	"net/http"

	"viv/internal/core/usecase"
)

type RecoveryHandler struct {
	RecoveryUC *usecase.GetRecoveryUseCase
}

func NewRecoveryHandler(recoveryUC *usecase.GetRecoveryUseCase) *RecoveryHandler {
	return &RecoveryHandler{RecoveryUC: recoveryUC}
}

// GET /recovery/today
func (h *RecoveryHandler) GetToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	result, err := h.RecoveryUC.Execute(ctx, usecase.GetRecoveryInput{
		UserID: userID,
	})
	if err != nil {
		log.Printf("[recovery.today] error: %+v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
