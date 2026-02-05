package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"viv/internal/core/usecase"
)

type createCheckinRequest struct {
	SleepQuality           string  `json:"sleep_quality"`
	BodyStatus             string  `json:"body_status"`
	Appetite               string  `json:"appetite"`
	StressLevel            string  `json:"stress_level"`
	LastWeekFeeling        string  `json:"last_week_feeling"`
	CycleStart             *string `json:"cycle_start"` // opcional, "2025-11-30"
	WorkloadPrediction     string  `json:"workload_prediction"`
	MentalEnergy           string  `json:"mental_energy"`
	TrainingGuidanceLevel  *string `json:"training_guidance_level,omitempty"`
	NutritionGuidanceLevel *string `json:"nutrition_guidance_level,omitempty"`
}

type createCheckinResponse struct {
	ID                 string  `json:"id"`
	WeekStart          string  `json:"week_start"`
	CreatedAt          string  `json:"created_at"`
	SleepQuality       string  `json:"sleep_quality"`
	BodyStatus         string  `json:"body_status"`
	Appetite           string  `json:"appetite"`
	StressLevel        string  `json:"stress_level"`
	LastWeekFeeling    string  `json:"last_week_feeling"`
	CycleStart         *string `json:"cycle_start,omitempty"`
	WorkloadPrediction string  `json:"workload_prediction"`
	MentalEnergy       string  `json:"mental_energy"`
	PromptVersion      string  `json:"prompt_version"`
}

type CheckinHandler struct {
	CreateUC *usecase.CreateCheckinUseCase
	LatestUC *usecase.GetLatestCheckinUseCase
}

func NewCheckinHandler(createUC *usecase.CreateCheckinUseCase, latestUC *usecase.GetLatestCheckinUseCase) *CheckinHandler {
	return &CheckinHandler{CreateUC: createUC, LatestUC: latestUC}
}

// Método para registrar en el router: r.Post("/checkins", checkinHandler.Create)
func (h *CheckinHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	raw, _ := io.ReadAll(r.Body)
	log.Printf("[checkins] raw body: %s", string(raw))
	r.Body = io.NopCloser(bytes.NewBuffer(raw))

	var req createCheckinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON "+err.Error(), http.StatusBadRequest)
		return
	}

	// week_start
	now := time.Now().UTC()
	weekStart := startOfWeekMondayUTC(now)

	// Parsear cycle_start si viene
	var cycleStart *time.Time
	if req.CycleStart != nil && *req.CycleStart != "" {
		t, err := time.Parse("2006-01-02", *req.CycleStart)
		if err != nil {
			//http.Error(w, "invalid cycle_start format, expected YYYY-MM-DD", http.StatusBadRequest)
			http.Error(w, "invalid cycle_start: "+err.Error(), http.StatusBadRequest)
			return
		}
		cycleStart = &t
	}

	in := usecase.CreateCheckinInput{
		UserID:                 userID,
		WeekStart:              weekStart,
		SleepQuality:           req.SleepQuality,
		BodyStatus:             req.BodyStatus,
		Appetite:               req.Appetite,
		StressLevel:            req.StressLevel,
		LastWeekFeeling:        req.LastWeekFeeling,
		CycleStart:             cycleStart,
		WorkloadPrediction:     req.WorkloadPrediction,
		MentalEnergy:           req.MentalEnergy,
		TrainingGuidanceLevel:  req.TrainingGuidanceLevel,
		NutritionGuidanceLevel: req.NutritionGuidanceLevel,
	}

	out, err := h.CreateUC.Execute(ctx, in)
	if err != nil {
		http.Error(w, "failed to create checkin", http.StatusInternalServerError)
		return
	}

	// formatear fechas a string para la respuesta
	weekStartStr := out.Checkin.WeekStart.Format("2006-01-02")
	createdAtStr := out.Checkin.CreatedAt.Format(time.RFC3339)
	var cycleStartStr *string
	if out.Checkin.CycleStart != nil {
		s := out.Checkin.CycleStart.Format("2006-01-02")
		cycleStartStr = &s
	}

	resp := createCheckinResponse{
		ID:                 out.Checkin.ID,
		WeekStart:          weekStartStr,
		CreatedAt:          createdAtStr,
		SleepQuality:       out.Checkin.SleepQuality,
		BodyStatus:         out.Checkin.BodyStatus,
		Appetite:           out.Checkin.Appetite,
		StressLevel:        out.Checkin.StressLevel,
		LastWeekFeeling:    out.Checkin.LastWeekFeeling,
		CycleStart:         cycleStartStr,
		WorkloadPrediction: out.Checkin.WorkloadPrediction,
		MentalEnergy:       out.Checkin.MentalEnergy,
		PromptVersion:      out.Checkin.PromptVersion,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *CheckinHandler) Latest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	in := usecase.GetLatestCheckinInput{
		UserID: userID,
	}

	out, err := h.LatestUC.Execute(ctx, in)
	if err != nil {
		http.Error(w, "failed to get latest checkin", http.StatusInternalServerError)
		return
	}

	// Si no hay check-ins aún para esta user:
	if out.Checkin == nil {
		// puedes elegir:
		// - 204 No Content
		// - 200 con body { "has_checkin": false }
		// Para MVP, 204 está bien:
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ch := out.Checkin

	weekStartStr := ch.WeekStart.Format("2006-01-02")
	createdAtStr := ch.CreatedAt.Format(time.RFC3339)
	var cycleStartStr *string
	if ch.CycleStart != nil {
		s := ch.CycleStart.Format("2006-01-02")
		cycleStartStr = &s
	}

	resp := createCheckinResponse{
		ID:                 ch.ID,
		WeekStart:          weekStartStr,
		CreatedAt:          createdAtStr,
		SleepQuality:       ch.SleepQuality,
		BodyStatus:         ch.BodyStatus,
		Appetite:           ch.Appetite,
		StressLevel:        ch.StressLevel,
		LastWeekFeeling:    ch.LastWeekFeeling,
		CycleStart:         cycleStartStr,
		WorkloadPrediction: ch.WorkloadPrediction,
		MentalEnergy:       ch.MentalEnergy,
		PromptVersion:      ch.PromptVersion,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func startOfWeekMondayUTC(t time.Time) time.Time {
	// normaliza a medianoche
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

	// Go: Sunday=0 ... Saturday=6
	wd := int(d.Weekday())
	if wd == 0 { // domingo -> lo tratamos como 7 para retroceder a lunes
		wd = 7
	}
	// lunes = 1 → restar wd-1 días
	return d.AddDate(0, 0, -(wd - 1))
}
