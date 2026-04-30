package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"viv/internal/core/usecase"
)

type createCheckinRequest struct {
	Sleep           string  `json:"sleep_quality"`
	Body            string  `json:"body_status"`
	AppetiteV2      string  `json:"appetite"`
	Demand          string  `json:"demand"`
	LastWeekFeel    string  `json:"last_week_feeling"`
	CycleStart      *string `json:"cycle_start"`  // opcional, "2025-11-30"
	PMSSymptoms     *string `json:"pms_symptoms"` // opcional, just in luteal and menstrual phase
	Predictability  string  `json:"predictability"`
	Readiness       string  `json:"readiness"`
	AdditionalNotes string  `json:"additional_notes"`
}

type createCheckinResponse struct {
	ID              string  `json:"id"`
	WeekStart       string  `json:"week_start"`
	CreatedAt       string  `json:"created_at"`
	Sleep           string  `json:"sleep_quality"`
	Body            string  `json:"body_status"`
	AppetiteV2      string  `json:"appetite"`
	Demand          string  `json:"demand"`
	LastWeekFeel    string  `json:"last_week_feeling"`
	CycleStart      *string `json:"cycle_start"`  // opcional, "2025-11-30"
	PMSSymptoms     *string `json:"pms_symptoms"` // opcional, just in luteal and menstrual phase
	Predictability  string  `json:"predictability"`
	Readiness       string  `json:"readiness"`
	AdditionalNotes string  `json:"additional_notes"`
}

type CheckinHandler struct {
	CreateUC *usecase.CreateCheckinUseCase
	LatestUC *usecase.GetLatestCheckinUseCase
	StatusUC *usecase.GetCheckinStatusUseCase
}

type checkinStatusResponse struct {
	CanCheckin      bool    `json:"can_checkin"`
	NextAvailableAt *string `json:"next_available_at,omitempty"`
	NeedCheckin     bool    `json:"need_checkin"`
}

func NewCheckinHandler(createUC *usecase.CreateCheckinUseCase, latestUC *usecase.GetLatestCheckinUseCase, statusUC *usecase.GetCheckinStatusUseCase) *CheckinHandler {
	return &CheckinHandler{CreateUC: createUC, LatestUC: latestUC, StatusUC: statusUC}
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
		UserID:          userID,
		WeekStart:       weekStart,
		Sleep:           req.Sleep,
		Body:            req.Body,
		AppetiteV2:      req.AppetiteV2,
		Demand:          req.Demand,
		LastWeekFeel:    req.LastWeekFeel,
		CycleStart:      cycleStart,
		PMSSymptoms:     req.PMSSymptoms,
		Predictability:  req.Predictability,
		Readiness:       req.Readiness,
		AdditionalNotes: req.AdditionalNotes,
	}

	out, err := h.CreateUC.Execute(ctx, in)
	if err != nil {
		var lockedErr *usecase.CheckinLockedError
		if errors.As(err, &lockedErr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "checkin_not_available_yet",
				"next_available_at": lockedErr.NextAvailableAt.UTC().Format("2006-01-02"),
			})
			return
		}
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
		ID:              out.Checkin.ID,
		WeekStart:       weekStartStr,
		CreatedAt:       createdAtStr,
		Sleep:           string(out.Checkin.Sleep),
		Body:            string(out.Checkin.Body),
		AppetiteV2:      string(out.Checkin.AppetiteV2),
		Demand:          string(out.Checkin.Demand),
		LastWeekFeel:    string(out.Checkin.LastWeekFeel),
		CycleStart:      cycleStartStr,
		PMSSymptoms:     (*string)(out.Checkin.PMSSymptoms),
		Predictability:  string(out.Checkin.Predictability),
		Readiness:       string(out.Checkin.Readiness),
		AdditionalNotes: out.Checkin.AdditionalNotes,
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

	if out.Checkin == nil {
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
		ID:              ch.ID,
		WeekStart:       weekStartStr,
		CreatedAt:       createdAtStr,
		Sleep:           string(ch.Sleep),
		Body:            string(ch.Body),
		AppetiteV2:      string(ch.AppetiteV2),
		Demand:          string(ch.Demand),
		LastWeekFeel:    string(ch.LastWeekFeel),
		CycleStart:      cycleStartStr,
		PMSSymptoms:     (*string)(ch.PMSSymptoms),
		Predictability:  string(ch.Predictability),
		Readiness:       string(ch.Readiness),
		AdditionalNotes: string(ch.AdditionalNotes),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *CheckinHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	in := usecase.GetCheckinStatusInput{
		UserID: userID,
	}

	out, err := h.StatusUC.Execute(ctx, in)
	if err != nil {
		http.Error(w, "failed to get checkin status", http.StatusInternalServerError)
		return
	}

	var nextAvailableAt *string
	if out.NextAvailableAt != nil {
		s := out.NextAvailableAt.Format("2006-01-02")
		nextAvailableAt = &s
	}

	resp := checkinStatusResponse{
		CanCheckin:      out.CanCheckin,
		NextAvailableAt: nextAvailableAt,
		NeedCheckin:     out.NeedCheckin,
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
	if wd == 0 {
		wd = 7
	}
	// lunes = 1 → restar wd-1 días
	return d.AddDate(0, 0, -(wd - 1))
}
