package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"

	"github.com/go-chi/chi/v5"
)

type PlansHandler struct {
	GenerateUC       *usecase.GeneratePlanUseCase
	CurrentUC        *usecase.GetCurrentPlanUseCase
	GetByIDUC        *usecase.GetPlanByIDUseCase
	AdjustUC         *usecase.AdjustPlanUseCase
	GetByWeekStartUC *usecase.GetPlanByWeekStartUseCase
	StartGenUC       *usecase.StartPlanGenerationUseCase
	JobStatusUC      *usecase.GetPlanGenerationStatusUseCase
}

type adjustPlanRequest struct {
	LifestyleChangeID string `json:"lifestyle_change_id"`
}

func NewPlansHandler(
	generateUC *usecase.GeneratePlanUseCase,
	currentUC *usecase.GetCurrentPlanUseCase,
	getByIDUC *usecase.GetPlanByIDUseCase,
	adjustUC *usecase.AdjustPlanUseCase,
	getByWeekStartUC *usecase.GetPlanByWeekStartUseCase,
	startGenUC *usecase.StartPlanGenerationUseCase,
	jobStatusUC *usecase.GetPlanGenerationStatusUseCase,
) *PlansHandler {
	return &PlansHandler{
		GenerateUC:       generateUC,
		CurrentUC:        currentUC,
		GetByIDUC:        getByIDUC,
		AdjustUC:         adjustUC,
		GetByWeekStartUC: getByWeekStartUC,
		StartGenUC:       startGenUC,
		JobStatusUC:      jobStatusUC,
	}
}

// ---------- REQUESTS ----------

type generatePlanRequest struct {
	CheckinID *string `json:"checkin_id,omitempty"`
}

// ---------- RESPONSES ----------

type planResponse struct {
	ID        string  `json:"id"`
	UserID    string  `json:"user_id"`
	Status    string  `json:"status"`
	CheckinID *string `json:"checkin_id,omitempty"`

	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	CreatedAt string `json:"created_at"`

	WeeklyHeadline    string `json:"weekly_headline,omitempty"`
	CyclePhaseSummary string `json:"cycle_phase_summary,omitempty"`
	CycleDayRange     string `json:"cycle_day_range,omitempty"`

	Training  json.RawMessage `json:"training,omitempty"`
	Nutrition json.RawMessage `json:"nutrition,omitempty"`
	Recovery  json.RawMessage `json:"recovery,omitempty"`

	Recommendations []recommendationResponse `json:"recommendations,omitempty"`

	// Meta solo cuando viene (ej. /generate)
	ModelName     string `json:"model_name,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	TokensInput   int    `json:"tokens_input,omitempty"`
	TokensOutput  int    `json:"tokens_output,omitempty"`
}

type recommendationResponse struct {
	Title  string `json:"title"`
	Action string `json:"action,omitempty"`
	Why    string `json:"why,omitempty"`
}

type generatePlanAsyncResponse struct {
	JobID string `json:"job_id"`
}

type planJobStatusResponse struct {
	Status string  `json:"status"`
	PlanID *string `json:"plan_id,omitempty"`
	Error  *string `json:"error,omitempty"`
}

// ---------- HANDLERS ----------

// POST /plans/generate
func (h *PlansHandler) Generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req generatePlanRequest
	decErr := json.NewDecoder(r.Body).Decode(&req)
	if decErr != nil && decErr != io.EOF {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	checkinID := ""
	if req.CheckinID != nil {
		checkinID = strings.TrimSpace(*req.CheckinID)
	}

	jobID, err := h.StartGenUC.Execute(ctx, userID, checkinID)
	if err != nil {
		log.Printf("[plans.generate] start job error: %+v\n", err)
		http.Error(w, "failed to start plan generation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202
	_ = json.NewEncoder(w).Encode(generatePlanAsyncResponse{JobID: jobID})
}

// GET /plans/generate/status?job_id=...
func (h *PlansHandler) GenerateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID == "" {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}

	job, err := h.JobStatusUC.Execute(ctx, userID, jobID)
	if err != nil {
		log.Printf("[plans.generate.status] error: %+v\n", err)
		http.Error(w, "failed to get job status", http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	resp := planJobStatusResponse{
		Status: string(job.Status), // IMPORTANT: cast to string
	}
	if job.PlanID != "" {
		v := job.PlanID
		resp.PlanID = &v
	}
	if job.Error != "" {
		e := job.Error
		resp.Error = &e
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// GET /plans/current
func (h *PlansHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	out, err := h.CurrentUC.Execute(ctx, usecase.GetCurrentPlanInput{UserID: userID})
	if err != nil {
		if _, ok := err.(usecase.NoActivePlanError); ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if _, ok := err.(usecase.PlanNotFoundError); ok {
			http.Error(w, "plan not found", http.StatusNotFound)
			return
		}
		log.Printf("[plans.current] error: %+v\n", err)
		http.Error(w, "failed to get current plan", http.StatusInternalServerError)
		return
	}

	if out == nil || out.Plan == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// meta usually not available here
	resp := mapPlanToResponse(out.Plan, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// GET /plans/{id}
func (h *PlansHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	planID := chi.URLParam(r, "id")
	if planID == "" {
		http.Error(w, "plan id is required", http.StatusBadRequest)
		return
	}

	out, err := h.GetByIDUC.Execute(ctx, usecase.GetPlanByIDInput{
		UserID: userID,
		PlanID: planID,
	})
	if err != nil {
		log.Printf("[plans.getByID] error: %+v\n", err)
		http.Error(w, "failed to get plan", http.StatusInternalServerError)
		return
	}

	if out == nil || out.Plan == nil {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}

	resp := mapPlanToResponse(out.Plan, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// POST /plans/adjust
func (h *PlansHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req adjustPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.LifestyleChangeID == "" {
		http.Error(w, "lifestyle_change_id is required", http.StatusBadRequest)
		return
	}

	out, err := h.AdjustUC.Execute(ctx, usecase.AdjustPlanInput{
		UserID:            userID,
		LifestyleChangeID: req.LifestyleChangeID,
	})
	if err != nil {
		log.Printf("[plans.adjust] error: %+v\n", err)
		http.Error(w, "failed to adjust plan", http.StatusInternalServerError)
		return
	}
	if out == nil || out.Plan == nil {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}

	resp := mapPlanToResponse(out.Plan, nil)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GET /plans/week/{week_start}
func (h *PlansHandler) GetByWeekStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ws := chi.URLParam(r, "week_start")
	if ws == "" {
		http.Error(w, "week_start is required", http.StatusBadRequest)
		return
	}

	weekStart, err := time.Parse("2006-01-02", ws)
	if err != nil {
		http.Error(w, "week_start must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	out, err := h.GetByWeekStartUC.Execute(ctx, usecase.GetPlanByWeekStartInput{
		UserID:    userID,
		WeekStart: weekStart,
	})
	if err != nil {
		log.Printf("[plans.getByWeekStart] error: %+v\n", err)
		http.Error(w, "failed to get plan", http.StatusInternalServerError)
		return
	}

	if out == nil || out.Plan == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := mapPlanToResponse(out.Plan, nil)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------- MAPPERS ----------

func mapPlanToResponse(p *domain.Plan, meta *domain.PlanGenerationMeta) planResponse {
	var checkinID *string
	if p.CheckinID != "" {
		c := p.CheckinID
		checkinID = &c
	}

	start := ""
	if !p.StartDate.IsZero() {
		start = p.StartDate.Format("2006-01-02")
	}

	end := ""
	if !p.EndDate.IsZero() {
		end = p.EndDate.Format("2006-01-02")
	}

	created := ""
	if !p.CreatedAt.IsZero() {
		created = p.CreatedAt.UTC().Format(time.RFC3339)
	}

	recs := make([]recommendationResponse, 0, len(p.Recommendations))
	for _, r := range p.Recommendations {
		recs = append(recs, recommendationResponse{
			Title:  r.Title,
			Action: r.Action,
			Why:    r.Why,
		})
	}

	resp := planResponse{
		ID:                p.ID,
		UserID:            p.UserID,
		Status:            p.Status,
		CheckinID:         checkinID,
		StartDate:         start,
		EndDate:           end,
		CreatedAt:         created,
		WeeklyHeadline:    p.WeeklyHeadline,
		CyclePhaseSummary: p.CyclePhaseSummary,
		CycleDayRange:     p.CycleDayRange,

		Training:        json.RawMessage(p.TrainingJSON),
		Nutrition:       json.RawMessage(p.NutritionJSON),
		Recovery:        json.RawMessage(p.RecoveryJSON),
		Recommendations: recs,
	}

	if meta != nil {
		resp.ModelName = meta.ModelName
		resp.PromptVersion = meta.PromptVersion
		resp.TokensInput = meta.TokensInput
		resp.TokensOutput = meta.TokensOutput
	}

	return resp
}
