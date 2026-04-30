package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"

	"github.com/go-chi/chi/v5"
)

type PlansHandler struct {
	CurrentUC *usecase.GetCurrentPlanUseCase
	GetByIDUC *usecase.GetPlanByIDUseCase
	//AdjustUC         *usecase.AdjustPlanUseCase
	GetByWeekStartUC *usecase.GetPlanByWeekStartUseCase
	//StartGenUC       *usecase.StartPlanGenerationUseCase
	JobStatusUC     *usecase.GetPlanGenerationStatusUseCase
	PhaseFeedbackUC *usecase.SavePhaseFeedbackUseCase
}

//type adjustPlanRequest struct {
//	LifestyleChangeID string `json:"lifestyle_change_id"`
//}

func NewPlansHandler(
	currentUC *usecase.GetCurrentPlanUseCase,
	getByIDUC *usecase.GetPlanByIDUseCase,
	getByWeekStartUC *usecase.GetPlanByWeekStartUseCase,
	//startGenUC *usecase.StartPlanGenerationUseCase,
	jobStatusUC *usecase.GetPlanGenerationStatusUseCase,
	phaseFeedbackUC *usecase.SavePhaseFeedbackUseCase,
) *PlansHandler {
	return &PlansHandler{
		CurrentUC:        currentUC,
		GetByIDUC:        getByIDUC,
		GetByWeekStartUC: getByWeekStartUC,
		//StartGenUC:       startGenUC,
		JobStatusUC:     jobStatusUC,
		PhaseFeedbackUC: phaseFeedbackUC,
	}
}

// ---------- REQUESTS ----------

type generatePlanRequest struct {
	CheckinID *string `json:"checkin_id,omitempty"`
}
type phaseFeedbackRequest struct {
	PlanID                    string `json:"plan_id"`
	Phase                     string `json:"phase"`
	HormonalBriefingResonates bool   `json:"hormonal_briefing_resonates"`
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

	CycleDayRange      string `json:"cycle_day_range,omitempty"`
	CurrentPhase       string `json:"current_phase,omitempty"`
	NextPhase          string `json:"next_phase,omitempty"`
	DaysUntilNextPhase int    `json:"days_until_next_phase,omitempty"`

	Training  json.RawMessage `json:"training,omitempty"`
	Nutrition json.RawMessage `json:"nutrition,omitempty"`
	Recovery  json.RawMessage `json:"recovery,omitempty"`

	TrainingCompleted map[string]bool                      `json:"training_completed,omitempty"`
	MealSelections    map[string]map[string]int            `json:"meal_selections,omitempty"`
	PhaseFeedback     map[string]domain.PhaseFeedbackEntry `json:"phase_feedback,omitempty"`

	// Meta solo cuando viene (ej. /generate)
	ModelName     string `json:"model_name,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	TokensInput   int    `json:"tokens_input,omitempty"`
	TokensOutput  int    `json:"tokens_output,omitempty"`
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
/*func (h *PlansHandler) Generate(w http.ResponseWriter, r *http.Request) {
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
	if checkinID == "null" {
		checkinID = ""
	}
	if strings.EqualFold(checkinID, "null") {
		checkinID = ""
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
}*/

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

	cycle := &cycleInfo{
		CurrentPhase:       out.CurrentPhase,
		NextPhase:          out.NextPhase,
		DaysUntilNextPhase: out.DaysUntilNextPhase,
	}

	if out == nil || out.Plan == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// meta usually not available here
	resp := mapPlanToResponse(out.Plan, nil, cycle)

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

	cycle := &cycleInfo{
		CurrentPhase:       out.CurrentPhase,
		NextPhase:          out.NextPhase,
		DaysUntilNextPhase: out.DaysUntilNextPhase,
	}

	if out == nil || out.Plan == nil {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}

	resp := mapPlanToResponse(out.Plan, nil, cycle)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// POST /plans/adjust
/*func (h *PlansHandler) Adjust(w http.ResponseWriter, r *http.Request) {
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
	cycle := &cycleInfo{
		CurrentPhase:       out.CurrentPhase,
		NextPhase:          out.NextPhase,
		DaysUntilNextPhase: out.DaysUntilNextPhase,
	}
	if out == nil || out.Plan == nil {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}

	resp := mapPlanToResponse(out.Plan, nil, cycle)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}*/

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
	cycle := &cycleInfo{
		CurrentPhase:       out.CurrentPhase,
		NextPhase:          out.NextPhase,
		DaysUntilNextPhase: out.DaysUntilNextPhase,
	}

	resp := mapPlanToResponse(out.Plan, nil, cycle)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------- MAPPERS ----------

type cycleInfo struct {
	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int
}

func mapPlanToResponse(p *domain.Plan, meta *domain.PlanGenerationMeta, cycle *cycleInfo) planResponse {
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

	resp := planResponse{
		ID:            p.ID,
		UserID:        p.UserID,
		Status:        p.Status,
		CheckinID:     checkinID,
		StartDate:     start,
		EndDate:       end,
		CreatedAt:     created,
		CycleDayRange: p.CycleDayRange,

		Training:  json.RawMessage(p.TrainingJSON),
		Nutrition: json.RawMessage(p.NutritionJSON),
		Recovery:  json.RawMessage(p.RecoveryJSON),

		TrainingCompleted: p.TrainingCompleted,
		MealSelections:    p.MealSelections,
		PhaseFeedback:     p.PhaseFeedback,
	}

	if cycle != nil {
		resp.CurrentPhase = cycle.CurrentPhase
		resp.NextPhase = cycle.NextPhase
		resp.DaysUntilNextPhase = cycle.DaysUntilNextPhase
	}

	if meta != nil {
		resp.ModelName = meta.ModelName
		resp.PromptVersion = meta.PromptVersion
		resp.TokensInput = meta.TokensInput
		resp.TokensOutput = meta.TokensOutput
	}

	return resp
}

// POST /plans/phase-feedback
func (h *PlansHandler) SavePhaseFeedback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req phaseFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if req.PlanID == "" || req.Phase == "" {
		http.Error(w, "plan_id and phase are required", http.StatusBadRequest)
		return
	}

	err := h.PhaseFeedbackUC.Execute(ctx, usecase.SavePhaseFeedbackInput{
		UserID:                    userID,
		PlanID:                    req.PlanID,
		Phase:                     req.Phase,
		HormonalBriefingResonates: req.HormonalBriefingResonates,
	})
	if err != nil {
		log.Printf("[plans.phase-feedback] error: %+v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved"})
}
