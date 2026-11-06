package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/getsentry/sentry-go"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

type IneligibilityHandler struct {
	UC *usecase.ReportIneligibilityUseCase
}

func NewIneligibilityHandler(uc *usecase.ReportIneligibilityUseCase) *IneligibilityHandler {
	return &IneligibilityHandler{UC: uc}
}

type reportIneligibilityRequest struct {
	Reason string `json:"reason"`
}

// Report handles someone turned away at the pre-onboarding eligibility
// screen — see usecase.ReportIneligibilityUseCase for what this does
// (records an anonymous statistic, then deletes the account outright).
func (h *IneligibilityHandler) Report(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req reportIneligibilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.UC.Execute(ctx, userID, domain.IneligibilityReason(req.Reason)); err != nil {
		var invalidReason usecase.InvalidIneligibilityReasonError
		if errors.As(err, &invalidReason) {
			http.Error(w, invalidReason.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("[report-ineligibility] execute error: %v", err)
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		http.Error(w, "failed to process ineligibility", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
