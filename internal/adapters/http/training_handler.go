package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/rules"
	"viv/internal/core/usecase"
)

type TrainingHandler struct {
	StartGenUC  *usecase.StartPlanGenerationUseCase
	JobStatusUC *usecase.GetPlanGenerationStatusUseCase
	Engine      *rules.Engine
	ResumeUC    *usecase.ResumeTrainingUseCase
}

func NewTrainingHandler(
	startGenUC *usecase.StartPlanGenerationUseCase,
	jobStatusUC *usecase.GetPlanGenerationStatusUseCase,
	engine *rules.Engine,
	resumeUC *usecase.ResumeTrainingUseCase,
) *TrainingHandler {
	return &TrainingHandler{
		StartGenUC:  startGenUC,
		JobStatusUC: jobStatusUC,
		Engine:      engine,
		ResumeUC:    resumeUC,
	}
}

// Requests

type generateTrainingRequest struct {
	CheckinID *string `json:"checkin_id,omitempty"`
}

type validateArrangementRequest struct {
	Phase string                   `json:"phase"`
	Days  []validateArrangementDay `json:"days"`
}

type validateArrangementDay struct {
	Weekday string                      `json:"weekday"`
	Session *validateArrangementSession `json:"session"`
}

type validateArrangementSession struct {
	Modality        string `json:"modality"`
	MuscleGroup     string `json:"muscle_group"`
	Intensity       string `json:"intensity"`
	DurationMinutes int    `json:"duration_minutes"`
}

type resumeTrainingRequest struct {
	ClearInjury bool `json:"clear_injury"`
}

// Responses

type generateTrainingResponse struct {
	JobID string `json:"job_id"`
}

type trainingJobStatusResponse struct {
	Status string  `json:"status"`
	PlanID *string `json:"plan_id,omitempty"`
	Error  *string `json:"error,omitempty"`
}

type validateArrangementResponse struct {
	Status     string              `json:"status"`
	Violations []violationResponse `json:"violations"`
}

type violationResponse struct {
	RuleID       string `json:"rule_id"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	AffectedDays []int  `json:"affected_days"`
}

type resumeTrainingResponse struct {
	TrainingPaused   bool    `json:"training_paused"`
	HasActiveInjury  bool    `json:"has_active_injury"`
	LastActivePlanID *string `json:"last_active_plan_id,omitempty"`
}

// POST /training/generate
func (h *TrainingHandler) Generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req generateTrainingRequest
	decErr := json.NewDecoder(r.Body).Decode(&req)
	if decErr != nil && decErr.Error() != "EOF" {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	checkinID := ""
	if req.CheckinID != nil {
		checkinID = strings.TrimSpace(*req.CheckinID)
	}

	jobID, err := h.StartGenUC.Execute(ctx, userID, checkinID)
	if err != nil {
		log.Printf("[training.generate] start job error: %+v\n", err)
		http.Error(w, "failed to start training generation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(generateTrainingResponse{JobID: jobID})
}

// GET /training/generate/status?job_id=...
func (h *TrainingHandler) GenerateStatus(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("[training.generate.status] error: %+v\n", err)
		http.Error(w, "failed to get job status", http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	resp := trainingJobStatusResponse{
		Status: string(job.Status),
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
	_ = json.NewEncoder(w).Encode(resp)
}

// POST /training/validate-arrangement
func (h *TrainingHandler) ValidateArrangement(w http.ResponseWriter, r *http.Request) {
	_, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req validateArrangementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	// Parse phase
	phase := domain.CyclePhase(strings.ToLower(strings.TrimSpace(req.Phase)))
	if !isValidPhase(phase) {
		http.Error(w, "invalid phase", http.StatusBadRequest)
		return
	}

	// Parse arrangement
	arrangement, err := parseArrangementRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Run engine validation
	decision := h.Engine.Evaluate(phase, arrangement)

	// Build response
	violations := make([]violationResponse, 0, len(decision.Violations))
	for _, v := range decision.Violations {
		violations = append(violations, violationResponse{
			RuleID:       v.RuleID,
			Severity:     string(v.Severity),
			Message:      v.Message,
			AffectedDays: v.AffectedDays,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(validateArrangementResponse{
		Status:     string(decision.Status),
		Violations: violations,
	})
}

func (h *TrainingHandler) Resume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req resumeTrainingRequest
	// body puede venir vacío, así que si da error pero no hay body, ignore:
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

// HELPERS

func isValidPhase(p domain.CyclePhase) bool {
	switch p {
	case domain.PhaseMenstrual, domain.PhaseFollicular, domain.PhaseOvulatory,
		domain.PhaseEarlyLuteal, domain.PhaseLateLuteal:
		return true
	}
	return false
}

var weekdayMap = map[string]time.Weekday{
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
	"sunday":    time.Sunday,
}

func parseArrangementRequest(req validateArrangementRequest) (domain.WeekArrangement, error) {
	if len(req.Days) != 7 {
		return domain.WeekArrangement{}, fmt.Errorf("expected 7 days, got %d", len(req.Days))
	}

	var arr domain.WeekArrangement
	now := time.Now()
	wd := now.Weekday()
	if wd == time.Sunday {
		wd = 7
	}
	monday := now.AddDate(0, 0, -int(wd-time.Monday))
	arr.WeekStart = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)

	for i, d := range req.Days {
		weekday, ok := weekdayMap[strings.ToLower(d.Weekday)]
		if !ok {
			return domain.WeekArrangement{}, fmt.Errorf("invalid weekday: %q", d.Weekday)
		}

		slot := domain.TrainingDaySlot{Weekday: weekday}

		if d.Session != nil {
			slot.Session = &domain.TrainingSession{
				Modality:        domain.Modality(strings.ToUpper(d.Session.Modality)),
				MuscleGroup:     domain.MuscleGroup(strings.ToLower(d.Session.MuscleGroup)),
				Intensity:       domain.Intensity(strings.ToLower(d.Session.Intensity)),
				DurationMinutes: d.Session.DurationMinutes,
			}
		}

		arr.Days[i] = slot
	}

	return arr, nil
}
