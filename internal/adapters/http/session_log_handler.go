package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"

	"github.com/getsentry/sentry-go"
)

// SessionLogHandler is the write side of real-time set-by-set logging
// for Loggable (mesocycle-pinned) days — Strength today, and Functional
// if that's ever enabled. It has no read endpoint of its own: the
// current session state (if any) is already returned by
// GET /training/weekly-plan/day, so a client resumes by reading that,
// not by polling here. There is deliberately no rest-timer endpoint —
// the countdown between sets is derived client-side from the current
// exercise's rest_seconds, already present in that same response.
type SessionLogHandler struct {
	StartUC    *usecase.StartSessionUseCase
	LogSetUC   *usecase.LogSetUseCase
	CompleteUC *usecase.CompleteSessionUseCase
}

func NewSessionLogHandler(
	startUC *usecase.StartSessionUseCase,
	logSetUC *usecase.LogSetUseCase,
	completeUC *usecase.CompleteSessionUseCase,
) *SessionLogHandler {
	return &SessionLogHandler{StartUC: startUC, LogSetUC: logSetUC, CompleteUC: completeUC}
}

type setLogResponse struct {
	ExerciseID string  `json:"exercise_id"`
	SetNumber  int     `json:"set_number"`
	WeightKg   float64 `json:"weight_kg"`
	Reps       int     `json:"reps"`
}

type sessionLogResponse struct {
	Status    string           `json:"status"`
	StartedAt string           `json:"started_at"`
	Sets      []setLogResponse `json:"sets"`
	Feedback  string           `json:"feedback,omitempty"`
}

// handleSessionLoggingError maps usecase.SessionLoggingValidationError to
// 400 (the request doesn't make sense given the current state — wrong
// exercise, set out of range, already done, no plan for that date, ...)
// and anything else to 500, matching this codebase's convention of
// distinguishing client-correctable errors from real server faults via a
// typed error rather than string-matching.
func handleSessionLoggingError(w http.ResponseWriter, r *http.Request, endpoint string, err error) {
	var validationErr usecase.SessionLoggingValidationError
	if errors.As(err, &validationErr) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("[%s] error: %+v\n", endpoint, err)
	if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
		hub.Scope().SetTag("endpoint", r.URL.Path)
		hub.Scope().SetTag("method", r.Method)
		hub.CaptureException(err)
	}
	http.Error(w, "failed to process session logging request", http.StatusInternalServerError)
}

func parseRequiredDate(w http.ResponseWriter, dateStr string) (time.Time, bool) {
	if dateStr == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return time.Time{}, false
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return time.Time{}, false
	}
	return date, true
}

type startSessionRequest struct {
	Date string `json:"date"`
}

// POST /training/weekly-plan/day/start
func (h *SessionLogHandler) Start(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req startSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	date, ok := parseRequiredDate(w, req.Date)
	if !ok {
		return
	}

	out, err := h.StartUC.Execute(ctx, usecase.StartSessionInput{UserID: userID, Date: date})
	if err != nil {
		handleSessionLoggingError(w, r, "session.start", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toSessionLogResponse(out.Log))
}

type logSetRequest struct {
	Date       string  `json:"date"`
	ExerciseID string  `json:"exercise_id"`
	SetNumber  int     `json:"set_number"`
	WeightKg   float64 `json:"weight_kg"`
	Reps       int     `json:"reps"`
}

// POST /training/weekly-plan/day/log-set
//
// Idempotent per (exercise_id, set_number): logging the same set again
// (e.g. the client retries after a flaky connection) updates it in
// place rather than adding a duplicate.
func (h *SessionLogHandler) LogSet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req logSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	date, ok := parseRequiredDate(w, req.Date)
	if !ok {
		return
	}

	out, err := h.LogSetUC.Execute(ctx, usecase.LogSetInput{
		UserID: userID, Date: date, ExerciseID: req.ExerciseID,
		SetNumber: req.SetNumber, WeightKg: req.WeightKg, Reps: req.Reps,
	})
	if err != nil {
		handleSessionLoggingError(w, r, "session.log-set", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toSessionLogResponse(out.Log))
}

type completeSessionRequest struct {
	Date     string `json:"date"`
	Feedback string `json:"feedback"` // "" | "easy" | "right" | "too_much"
}

type completeSessionResponse struct {
	sessionLogResponse
	EndedAt        string `json:"ended_at"`
	ElapsedMinutes int    `json:"elapsed_minutes"`
	SetsCompleted  int    `json:"sets_completed"`
	SetsTotal      int    `json:"sets_total"`
}

// POST /training/weekly-plan/day/complete
func (h *SessionLogHandler) Complete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req completeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	date, ok := parseRequiredDate(w, req.Date)
	if !ok {
		return
	}

	out, err := h.CompleteUC.Execute(ctx, usecase.CompleteSessionInput{UserID: userID, Date: date, Feedback: req.Feedback})
	if err != nil {
		handleSessionLoggingError(w, r, "session.complete", err)
		return
	}

	resp := completeSessionResponse{
		sessionLogResponse: toSessionLogResponse(out.Log),
		ElapsedMinutes:     out.ElapsedMinutes,
		SetsCompleted:      out.SetsCompleted,
		SetsTotal:          out.SetsTotal,
	}
	if out.Log.EndedAt != nil {
		resp.EndedAt = out.Log.EndedAt.Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func toSessionLogResponse(log domain.SessionLog) sessionLogResponse {
	sets := make([]setLogResponse, len(log.Sets))
	for i, s := range log.Sets {
		sets[i] = setLogResponse{ExerciseID: s.ExerciseID, SetNumber: s.SetNumber, WeightKg: s.WeightKg, Reps: s.Reps}
	}
	return sessionLogResponse{
		Status:    string(log.Status),
		StartedAt: log.StartedAt.Format(time.RFC3339),
		Sets:      sets,
		Feedback:  string(log.Feedback),
	}
}
