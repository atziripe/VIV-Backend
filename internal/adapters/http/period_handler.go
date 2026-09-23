package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"viv/internal/core/usecase"
)

type PeriodHandler struct {
	LogPeriodStartUC *usecase.LogPeriodStartUseCase
}

func NewPeriodHandler(logPeriodStartUC *usecase.LogPeriodStartUseCase) *PeriodHandler {
	return &PeriodHandler{LogPeriodStartUC: logPeriodStartUC}
}

type logPeriodStartRequest struct {
	// Date is optional — "YYYY-MM-DD". Omit it to log today. Always
	// available, unlike the weekly check-in: reporting a period the
	// moment it happens (not waiting for Sunday) is the whole point.
	Date string `json:"date"`
}

type logPeriodStartResponse struct {
	CycleDay       int          `json:"cycle_day"`
	CycleSummary   cycleSummary `json:"cycle_summary"`
	CycleUpdatedAt string       `json:"cycle_updated_at"`
	CycleAnchorAt  string       `json:"cycle_anchor_at,omitempty"`

	// CycleDurationChanged/PreviousCycleDuration/CycleDuration describe a
	// recalibration — "VIV now has you at a 33-day cycle" in the client's
	// copy — omitted (zero values) when this report matched the prior
	// estimate closely enough not to change it.
	CycleDurationChanged  bool `json:"cycle_duration_changed"`
	PreviousCycleDuration int  `json:"previous_cycle_duration,omitempty"`
	CycleDuration         int  `json:"cycle_duration,omitempty"`

	NextEstimatedPeriodDate string `json:"next_estimated_period_date,omitempty"`

	// WeekRebuilt/Moved describe whether — and how — this report's date
	// differed enough from the prediction to regenerate the current
	// week's shape around the real anchor. See MovedDay.
	WeekRebuilt bool          `json:"week_rebuilt"`
	Moved       []movedDayDTO `json:"moved,omitempty"`
}

type movedDayDTO struct {
	Weekday         string `json:"weekday"`
	Date            string `json:"date"`
	IsRestDay       bool   `json:"is_rest_day"`
	Title           string `json:"title,omitempty"`
	DurationMinutes int    `json:"duration_minutes,omitempty"`
}

// POST /cycle/period-start
func (h *PeriodHandler) LogStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req logPeriodStartRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body means "today" — not an error
	}

	in := usecase.LogPeriodStartInput{UserID: userID}
	if d := strings.TrimSpace(req.Date); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			http.Error(w, "invalid date, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		in.Date = t
	}

	out, err := h.LogPeriodStartUC.Execute(ctx, in)
	if err != nil {
		log.Printf("[period.log-start] error user=%s err=%v", userID, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := logPeriodStartResponse{
		CycleDay:             out.CycleDay,
		CycleSummary:         buildCycleSummary(out.User, out.CurrentPhase, out.NextPhase, out.DaysUntilNextPhase),
		CycleUpdatedAt:       time.Now().UTC().Format("2006-01-02"),
		CycleDurationChanged: out.CycleDurationChanged,
		WeekRebuilt:          out.WeekRebuilt,
	}
	if out.User.CycleAnchorAt != nil {
		resp.CycleAnchorAt = out.User.CycleAnchorAt.Format("2006-01-02")
	}
	if out.CycleDurationChanged {
		resp.PreviousCycleDuration = out.PreviousCycleDuration
		if d, err := strconv.Atoi(out.User.CycleDuration); err == nil {
			resp.CycleDuration = d
		}
	}
	if !out.NextEstimatedPeriodDate.IsZero() {
		resp.NextEstimatedPeriodDate = out.NextEstimatedPeriodDate.Format("2006-01-02")
	}
	for _, m := range out.Moved {
		resp.Moved = append(resp.Moved, movedDayDTO{
			Weekday:         m.Weekday,
			Date:            m.Date.Format("2006-01-02"),
			IsRestDay:       m.IsRestDay,
			Title:           m.Title,
			DurationMinutes: m.DurationMinutes,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
