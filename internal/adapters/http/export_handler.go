package http

import (
	"encoding/json"
	"log"
	"net/http"

	"viv/internal/core/usecase"
)

type ExportHandler struct {
	UC *usecase.ExportUserDataUseCase
}

func NewExportHandler(uc *usecase.ExportUserDataUseCase) *ExportHandler {
	return &ExportHandler{UC: uc}
}

// exportResponse is a deliberately raw data dump, not a polished API
// shape — see ExportUserDataUseCase's doc comment for scope. Nested
// domain values (Profile, LifestyleChanges, CurrentWeekDraft, LegacyPlan)
// serialize with their own Go field names (most have no json tags, since
// they were never meant to be an HTTP response body directly) rather than
// being re-mapped into a pretty snake_case shape the way GetMe/GetConsents
// do — completeness and fidelity to the stored record matter more here
// than API consistency, which is the point of a portability export.
type exportResponse struct {
	ExportedAt       string      `json:"exported_at"`
	Profile          interface{} `json:"profile"`
	Consents         interface{} `json:"consents"`
	LifestyleChanges interface{} `json:"lifestyle_changes"`
	NutritionPlan    interface{} `json:"nutrition_plan,omitempty"`
	CurrentWeekDraft interface{} `json:"current_week_draft,omitempty"`
	LegacyPlan       interface{} `json:"legacy_plan,omitempty"`
}

// GET /me/export
//
// Data portability (GDPR Art. 20) — a JSON copy of the authenticated
// user's current data. See ExportUserDataUseCase's doc comment for exactly
// what's included and what a v1 known gap leaves out (full historical
// daily check-ins/session logs/recovery actions).
func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	out, err := h.UC.Execute(ctx, usecase.ExportUserDataInput{UserID: userID})
	if err != nil {
		log.Printf("[export] error: %v", err)
		http.Error(w, "failed to export data", http.StatusInternalServerError)
		return
	}

	resp := exportResponse{
		ExportedAt:       out.ExportedAt.Format("2006-01-02T15:04:05Z07:00"),
		Profile:          out.Profile,
		Consents:         out.Consents,
		LifestyleChanges: out.LifestyleChanges,
		NutritionPlan:    out.NutritionPlan,
		CurrentWeekDraft: out.CurrentWeekDraft,
		LegacyPlan:       out.LegacyPlan,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="viv-data-export.json"`)
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}
