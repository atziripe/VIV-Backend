package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

// RecoveryCardHandler exposes the session-driven recovery card (VIV
// Recovery Engine Decision Table spec) — the sole recovery read/write
// surface now that the old paragraph-banner /recovery/today was removed.
type RecoveryCardHandler struct {
	CardUC   *usecase.GetRecoveryCardUseCase
	ActionUC *usecase.SaveRecoveryActionUseCase
}

func NewRecoveryCardHandler(cardUC *usecase.GetRecoveryCardUseCase, actionUC *usecase.SaveRecoveryActionUseCase) *RecoveryCardHandler {
	return &RecoveryCardHandler{CardUC: cardUC, ActionUC: actionUC}
}

// GET /recovery/card?date=YYYY-MM-DD
//
// date is required and trusted as-is from the client, same reasoning as
// every other date-scoped endpoint in this pipeline. 204 when no
// generated week covers date.
func (h *RecoveryCardHandler) GetCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dateStr := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateStr == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return
	}

	out, err := h.CardUC.Execute(ctx, usecase.GetRecoveryCardInput{UserID: userID, Date: date})
	if err != nil {
		log.Printf("[recovery.card] error: %+v\n", err)
		http.Error(w, "failed to get recovery card", http.StatusInternalServerError)
		return
	}
	if !out.Found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out.Card)
}

type saveRecoveryActionRequest struct {
	Date string `json:"date"`
	Kind string `json:"kind"`
}

// POST /recovery/card/action
//
// Persists which button the user tapped on a recovery card — kind is one
// of "done", "not_today" (Low/Medium-cost cards) or "acknowledged",
// "trained_anyway" (High-cost cards) — see the card's own
// available_actions for exactly which two apply to a given day.
func (h *RecoveryCardHandler) SaveAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req saveRecoveryActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	dateStr := strings.TrimSpace(req.Date)
	if dateStr == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.ActionUC.Execute(ctx, usecase.SaveRecoveryActionInput{
		UserID: userID,
		Date:   date,
		Kind:   domain.RecoveryActionKind(req.Kind),
	}); err != nil {
		log.Printf("[recovery.card.action] error: %+v\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}
