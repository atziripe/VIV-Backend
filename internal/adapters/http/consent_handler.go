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

type ConsentHandler struct {
	GetUC *usecase.GetConsentsUseCase
	SetUC *usecase.SetConsentUseCase
}

func NewConsentHandler(getUC *usecase.GetConsentsUseCase, setUC *usecase.SetConsentUseCase) *ConsentHandler {
	return &ConsentHandler{GetUC: getUC, SetUC: setUC}
}

type consentResponse struct {
	Purpose       string `json:"purpose"`
	Granted       bool   `json:"granted"`
	PolicyVersion string `json:"policy_version"`
	GrantedAt     string `json:"granted_at"`
}

func (h *ConsentHandler) GetConsents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	out, err := h.GetUC.Execute(ctx, usecase.GetConsentsInput{UserID: userID})
	if err != nil {
		log.Printf("[consents.get] error: %v", err)
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		http.Error(w, "failed to get consents", http.StatusInternalServerError)
		return
	}

	resp := make([]consentResponse, 0, len(out.Consents))
	for _, c := range out.Consents {
		resp = append(resp, consentResponse{
			Purpose:       string(c.Purpose),
			Granted:       c.Granted,
			PolicyVersion: c.PolicyVersion,
			GrantedAt:     c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type setConsentRequest struct {
	Purpose string `json:"purpose"`
	Granted bool   `json:"granted"`
}

type setConsentResponse struct {
	// AccountDeleted is true only for a health_data_processing revoke —
	// see SetConsentUseCase's doc comment. The client should treat this
	// exactly like a successful DELETE /me (log out, clear local state).
	AccountDeleted bool `json:"account_deleted,omitempty"`
}

func (h *ConsentHandler) SetConsent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req setConsentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	out, err := h.SetUC.Execute(ctx, usecase.SetConsentInput{
		UserID:  userID,
		Purpose: domain.ConsentPurpose(req.Purpose),
		Granted: req.Granted,
	})
	if err != nil {
		var invalidPurpose usecase.InvalidConsentPurposeError
		if errors.As(err, &invalidPurpose) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("[consents.set] error: %v", err)
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		http.Error(w, "failed to set consent", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(setConsentResponse{AccountDeleted: out.AccountDeleted})
}
