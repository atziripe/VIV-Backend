package http

import (
	"encoding/json"
	"log"
	"net/http"
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
		CycleDay: out.CycleDay,
		CycleSummary: cycleSummary{
			CurrentPhase:       out.CurrentPhase,
			NextPhase:          out.NextPhase,
			DaysUntilNextPhase: out.DaysUntilNextPhase,
		},
		CycleUpdatedAt: time.Now().UTC().Format("2006-01-02"),
	}
	if out.User.CycleAnchorAt != nil {
		resp.CycleAnchorAt = out.User.CycleAnchorAt.Format("2006-01-02")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
