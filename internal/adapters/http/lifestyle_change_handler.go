package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"viv/internal/core/usecase"
)

type LifestyleHandler struct {
	ReportUC *usecase.ReportLifestyleChangeUseCase
	ListUC   *usecase.ListLifestyleChangesUseCase
}

func NewLifestyleHandler(reportUC *usecase.ReportLifestyleChangeUseCase, listUC *usecase.ListLifestyleChangesUseCase) *LifestyleHandler {
	return &LifestyleHandler{ReportUC: reportUC, ListUC: listUC}
}

// Request from frontend
type reportLifestyleChangeRequest struct {
	Type         string  `json:"type"`          // illness | travel | injury | ...
	SpaceTrain   string  `json:"space_train"`   // normal | limited | none
	PossibleDiet string  `json:"possible_diet"` // normal | irregular | etc.
	Energy       string  `json:"energy"`        // 1–5
	AppliesTo    *string `json:"applies_to"`    // duration
}

// Response to frontend
type reportLifestyleChangeResponse struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	SpaceTrain      string  `json:"space_train"`
	PossibleDiet    string  `json:"possible_diet"`
	Energy          string  `json:"energy"`
	AppliesTo       *string `json:"applies_to,omitempty"`
	PlanID          *string `json:"plan_id,omitempty"`
	CreatedAt       string  `json:"created_at"`
	TrainingPaused  bool    `json:"training_paused"`
	HasActiveInjury bool    `json:"has_active_injury"`
}

type lifestyleChangeItemResponse struct {
	ID           string  `json:"id"`
	Type         string  `json:"type"`
	SpaceTrain   string  `json:"space_train"`
	PossibleDiet string  `json:"possible_diet"`
	Energy       string  `json:"energy"`
	AppliesTo    *string `json:"applies_to,omitempty"`
	PlanID       *string `json:"plan_id,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

func (h *LifestyleHandler) Report(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req reportLifestyleChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var appliesTo *time.Time
	if req.AppliesTo != nil && *req.AppliesTo != "" {
		t, err := time.Parse("2006-01-02", *req.AppliesTo)
		if err != nil {
			http.Error(w, "invalid applies_to format, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		appliesTo = &t
	}

	in := usecase.ReportLifestyleChangeInput{
		UserID:       userID,
		Type:         req.Type,
		SpaceTrain:   req.SpaceTrain,
		PossibleDiet: req.PossibleDiet,
		Energy:       req.Energy,
		AppliesTo:    appliesTo,
	}

	out, err := h.ReportUC.Execute(ctx, in)
	if err != nil {
		// aquí podrías checar si es ErrUserNotFound para devolver 404
		http.Error(w, "failed to report lifestyle change", http.StatusInternalServerError)
		return
	}

	change := out.Change

	createdAtStr := change.CreatedAt.Format(time.RFC3339)
	var appliesToStr *string
	if change.AppliesTo != nil {
		s := change.AppliesTo.Format("2006-01-02")
		appliesToStr = &s
	}

	resp := reportLifestyleChangeResponse{
		ID:              change.ID,
		Type:            change.Type,
		SpaceTrain:      change.SpaceTrain,
		PossibleDiet:    change.PossibleDiet,
		Energy:          change.Energy,
		AppliesTo:       appliesToStr,
		PlanID:          change.PlanID,
		CreatedAt:       createdAtStr,
		TrainingPaused:  out.TrainingPaused,
		HasActiveInjury: out.HasActiveInjury,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *LifestyleHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Query param ?limit=10
	limit := 10
	if q := r.URL.Query().Get("limit"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			limit = v
		}
		if limit > 50 {
			limit = 50
		}
	}

	in := usecase.ListLifestyleChangesInput{
		UserID: userID,
		Limit:  limit,
	}

	out, err := h.ListUC.Execute(ctx, in)
	if err != nil {
		http.Error(w, "failed to list lifestyle changes", http.StatusInternalServerError)
		return
	}

	items := make([]lifestyleChangeItemResponse, 0, len(out.Items))

	for _, e := range out.Items {
		createdStr := e.CreatedAt.UTC().Format(time.RFC3339)

		var appliesToStr *string
		if e.AppliesTo != nil {
			s := e.AppliesTo.Format("2006-01-02")
			appliesToStr = &s
		}

		item := lifestyleChangeItemResponse{
			ID:           e.ID,
			Type:         e.Type,
			SpaceTrain:   e.SpaceTrain,
			PossibleDiet: e.PossibleDiet,
			Energy:       e.Energy,
			AppliesTo:    appliesToStr,
			PlanID:       e.PlanID,
			CreatedAt:    createdStr,
		}

		items = append(items, item)
	}

	// Puedes retornar directamente el array o envolverlo en un objeto
	// Para ser simple: devolvemos { "items": [...] }

	resp := struct {
		Items []lifestyleChangeItemResponse `json:"items"`
	}{
		Items: items,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
