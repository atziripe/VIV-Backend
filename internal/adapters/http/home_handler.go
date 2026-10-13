package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/usecase"
)

// HomeHandler exposes the Home screen's single aggregated read — see
// usecase.GetHomeTimelineUseCase's doc comment for what it composes and
// the two data-model gaps its estimates work around.
type HomeHandler struct {
	UC *usecase.GetHomeTimelineUseCase
}

func NewHomeHandler(uc *usecase.GetHomeTimelineUseCase) *HomeHandler {
	return &HomeHandler{UC: uc}
}

type timelineItemResponse struct {
	Time        string `json:"time,omitempty"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Title       string `json:"title"`
	Detail      string `json:"detail,omitempty"`
	IsEstimated bool   `json:"is_estimated,omitempty"`
}

type timelineMomentResponse struct {
	timelineItemResponse
	Label     string `json:"label"`
	CTALabel  string `json:"cta_label"`
	CTAAction string `json:"cta_action,omitempty"`
}

type dayDotResponse struct {
	Weekday string `json:"weekday"`
	Status  string `json:"status"`
	IsToday bool   `json:"is_today"`
}

type weekProgressResponse struct {
	CompletedSessions int              `json:"completed_sessions"`
	TotalSessions     int              `json:"total_sessions"`
	Days              []dayDotResponse `json:"days"`
}

type homeTimelineResponse struct {
	Date       string `json:"date"`
	Weekday    string `json:"weekday"`
	WeekNumber int    `json:"week_number"`

	CurrentPhase       string `json:"current_phase"`
	NextPhase          string `json:"next_phase"`
	DaysUntilNextPhase int    `json:"days_until_next_phase"`

	NowCard      *timelineMomentResponse `json:"now_card,omitempty"`
	Timeline     []timelineItemResponse  `json:"timeline"`
	WeekProgress weekProgressResponse    `json:"week_progress"`
}

func toTimelineItemResponse(i usecase.TimelineItem) timelineItemResponse {
	return timelineItemResponse{
		Time: i.Time, Kind: i.Kind, Status: i.Status,
		Title: i.Title, Detail: i.Detail, IsEstimated: i.IsEstimated,
	}
}

// GET /home/today?date=YYYY-MM-DD
//
// date is required and trusted as-is from the client, same reasoning as
// every other date-scoped endpoint in this pipeline. 204 when no
// generated week covers date.
func (h *HomeHandler) GetToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dateStr := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateStr == "" {
		http.Error(w, "date is required", http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return
	}

	out, err := h.UC.Execute(ctx, usecase.GetHomeTimelineInput{UserID: userID, Date: date})
	if err != nil {
		log.Printf("[home.today] error: %+v\n", err)
		http.Error(w, "failed to get home timeline", http.StatusInternalServerError)
		return
	}
	if !out.Found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	timeline := make([]timelineItemResponse, 0, len(out.Timeline))
	for _, item := range out.Timeline {
		timeline = append(timeline, toTimelineItemResponse(item))
	}

	days := make([]dayDotResponse, 0, len(out.WeekProgress.Days))
	for _, d := range out.WeekProgress.Days {
		days = append(days, dayDotResponse{Weekday: d.Weekday, Status: d.Status, IsToday: d.IsToday})
	}

	resp := homeTimelineResponse{
		Date:       out.Date,
		Weekday:    out.Weekday,
		WeekNumber: out.WeekNumber,

		CurrentPhase:       out.CurrentPhase,
		NextPhase:          out.NextPhase,
		DaysUntilNextPhase: out.DaysUntilNextPhase,

		Timeline: timeline,
		WeekProgress: weekProgressResponse{
			CompletedSessions: out.WeekProgress.CompletedSessions,
			TotalSessions:     out.WeekProgress.TotalSessions,
			Days:              days,
		},
	}
	if out.NowCard != nil {
		resp.NowCard = &timelineMomentResponse{
			timelineItemResponse: toTimelineItemResponse(out.NowCard.TimelineItem),
			Label:                out.NowCard.Label,
			CTALabel:             out.NowCard.CTALabel,
			CTAAction:            out.NowCard.CTAAction,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
