package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"viv/internal/core/checkin"
	"viv/internal/core/usecase"

	"github.com/getsentry/sentry-go"
)

type submitDailyCheckinRequest struct {
	// Date is the user's local calendar date this check-in is for
	// ("2006-01-02"), resolved client-side. The server has no source of
	// truth for a user's timezone (see SubmitDailyCheckinInput's doc
	// comment), so it trusts this rather than deriving one from server
	// time.
	Date   string `json:"date"`
	Sleep  string `json:"sleep"`
	Body   string `json:"body"`
	Demand string `json:"demand"`
	Need   string `json:"need"`
}

type dailyCheckinAssignmentResponse struct {
	ActivityType string `json:"activity_type"`
	Intensity    string `json:"intensity"`
	Impact       string `json:"impact"`
}

type dailyCheckinSuggestionResponse struct {
	Assignment dailyCheckinAssignmentResponse `json:"assignment"`
	Reason     string                         `json:"reason"`
}

type submitDailyCheckinResponse struct {
	Date             string `json:"date"`
	RecoveryCapacity string `json:"recovery_capacity"`
	LifeBandwidth    string `json:"life_bandwidth"`
	BuildReadiness   string `json:"build_readiness"`

	// Regenerated is true when this check-in triggered a fresh weekly
	// generation (no plan covered today yet) instead of daily adaptation.
	Regenerated bool `json:"regenerated"`

	IsRestDay  bool                            `json:"is_rest_day"`
	Assignment *dailyCheckinAssignmentResponse `json:"assignment,omitempty"`
	Reason     string                          `json:"reason,omitempty"`
	Suggestion *dailyCheckinSuggestionResponse `json:"suggestion,omitempty"`
}

type DailyCheckinHandler struct {
	UC *usecase.SubmitDailyCheckinUseCase
}

func NewDailyCheckinHandler(uc *usecase.SubmitDailyCheckinUseCase) *DailyCheckinHandler {
	return &DailyCheckinHandler{UC: uc}
}

// POST /checkin
//
// Submits today's four check-in answers, persists them and their derived
// readiness (idempotent per date — a second submission for the same date
// updates rather than duplicates), and returns today's resolved session:
// either freshly generated (no plan covered today) or daily-adapted (a
// plan already did) — see SubmitDailyCheckinUseCase. The response is
// meant to be sufficient on its own for "what does today look like",
// without the client having to separately fetch the plan afterward.
func (h *DailyCheckinHandler) Submit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req submitDailyCheckinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return
	}

	out, err := h.UC.Execute(ctx, usecase.SubmitDailyCheckinInput{
		UserID: userID,
		Date:   date,
		Answers: checkin.DailyCheckin{
			Sleep:  checkin.SleepAnswer(req.Sleep),
			Body:   checkin.BodyAnswer(req.Body),
			Demand: checkin.DemandAnswer(req.Demand),
			Need:   checkin.NeedAnswer(req.Need),
		},
	})
	if err != nil {
		var userNotFound usecase.UserNotFoundError
		switch {
		case errors.As(err, &userNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			log.Printf("[checkin] submit error: %v", err)
			if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
				hub.Scope().SetTag("endpoint", r.URL.Path)
				hub.Scope().SetTag("method", r.Method)
				hub.CaptureException(err)
			}
			http.Error(w, "failed to submit checkin", http.StatusInternalServerError)
		}
		return
	}

	resp := submitDailyCheckinResponse{
		Date:             out.Date.Format("2006-01-02"),
		RecoveryCapacity: string(out.Readiness.RecoveryCapacity),
		LifeBandwidth:    string(out.Readiness.LifeBandwidth),
		BuildReadiness:   string(out.Readiness.BuildReadiness),
		Regenerated:      out.Regenerated,
		IsRestDay:        out.IsRestDay,
		Reason:           out.Reason,
	}
	if !out.IsRestDay {
		resp.Assignment = &dailyCheckinAssignmentResponse{
			ActivityType: string(out.Assignment.ActivityType),
			Intensity:    string(out.Assignment.Intensity),
			Impact:       string(out.Assignment.Impact),
		}
	}
	if out.Suggestion != nil {
		resp.Suggestion = &dailyCheckinSuggestionResponse{
			Assignment: dailyCheckinAssignmentResponse{
				ActivityType: string(out.Suggestion.Assignment.ActivityType),
				Intensity:    string(out.Suggestion.Assignment.Intensity),
				Impact:       string(out.Suggestion.Assignment.Impact),
			},
			Reason: out.Suggestion.Reason,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
