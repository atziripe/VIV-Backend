package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"viv/internal/core/usecase"

	"github.com/getsentry/sentry-go"
)

// WeeklyPlanHandler exposes the new training-plan pipeline (VIV-106..113)
// as an async job, mirroring TrainingHandler's Generate/GenerateStatus
// shape for the old pipeline: start returns 202 + a job_id immediately,
// the client polls status until it's done/failed.
type WeeklyPlanHandler struct {
	StartGenUC  *usecase.StartWeeklyPlanGenerationUseCase
	JobStatusUC *usecase.GetPlanGenerationStatusUseCase
	CurrentUC   *usecase.GetCurrentWeeklyPlanUseCase
	DayUC       *usecase.GetWeeklyPlanDayUseCase
}

func NewWeeklyPlanHandler(
	startGenUC *usecase.StartWeeklyPlanGenerationUseCase,
	jobStatusUC *usecase.GetPlanGenerationStatusUseCase,
	currentUC *usecase.GetCurrentWeeklyPlanUseCase,
	dayUC *usecase.GetWeeklyPlanDayUseCase,
) *WeeklyPlanHandler {
	return &WeeklyPlanHandler{StartGenUC: startGenUC, JobStatusUC: jobStatusUC, CurrentUC: currentUC, DayUC: dayUC}
}

type generateWeeklyPlanResponse struct {
	JobID string `json:"job_id"`
}

type weeklyPlanJobStatusResponse struct {
	Status  string  `json:"status"`
	DraftID *string `json:"draft_id,omitempty"`
	Error   *string `json:"error,omitempty"`
}

// POST /training/weekly-plan/generate
//
// No request body: the new pipeline resolves check-in/goal/catalog on its
// own (GenerateWeeklyPlanUsecase's documented defaults) rather than being
// handed a specific check-in ID like the old /training/generate. The
// week generated is always the current week (Monday-aligned, matching
// the weekday-index assumption VIV-109's scheduler makes).
func (h *WeeklyPlanHandler) Generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobID, err := h.StartGenUC.Execute(ctx, userID, mondayOfCurrentWeek(time.Now()))
	if err != nil {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		log.Printf("[weeklyplan.generate] start job error: %+v\n", err)
		http.Error(w, "failed to start weekly plan generation", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(generateWeeklyPlanResponse{JobID: jobID})
}

// GET /training/weekly-plan/generate/status?job_id=...
func (h *WeeklyPlanHandler) GenerateStatus(w http.ResponseWriter, r *http.Request) {
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
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		log.Printf("[weeklyplan.generate.status] error: %+v\n", err)
		http.Error(w, "failed to get job status", http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	resp := weeklyPlanJobStatusResponse{Status: string(job.Status)}
	if job.PlanID != "" {
		v := job.PlanID
		resp.DraftID = &v
	}
	if job.Error != "" {
		e := job.Error
		resp.Error = &e
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type weeklyPlanDayResponse struct {
	Weekday         string `json:"weekday"`
	Date            string `json:"date"`
	IsToday         bool   `json:"is_today"`
	IsRestDay       bool   `json:"is_rest_day"`
	ActivityType    string `json:"activity_type,omitempty"`
	MuscleGroup     string `json:"muscle_group,omitempty"`
	Intensity       string `json:"intensity,omitempty"`
	Impact          string `json:"impact,omitempty"`
	DurationMinutes *int   `json:"duration_minutes,omitempty"`
	Substituted     bool   `json:"substituted,omitempty"`
	Warning         string `json:"warning,omitempty"`
}

type currentWeeklyPlanResponse struct {
	DraftID   string                  `json:"draft_id"`
	StartDate string                  `json:"start_date"`
	EndDate   string                  `json:"end_date"`
	GoalID    string                  `json:"goal_id"`
	Days      []weeklyPlanDayResponse `json:"days"`
}

// GET /training/weekly-plan/current?date=YYYY-MM-DD
//
// Returns the already-generated week covering date, shaped for a "This
// week" list view: one entry per day with activity/muscle group/
// intensity/impact, rest-day and today flags, and duration when the
// content library has it (see the response's known gaps below). Doesn't
// hydrate full session content (warmup/exercises/cooldown) — that's a
// per-day detail concern for a future endpoint, not this list view's.
//
// date is required and trusted as-is from the client — same reasoning as
// POST /checkin: nothing server-side knows a user's local date.
//
// Known gaps in this response, since the underlying data doesn't exist
// yet: (1) no per-day completion state — WeekDraft has no "done" flag,
// unlike the old pipeline's domain.Plan.TrainingCompleted; a client
// wanting checkmarks needs that tracked separately (say the word and
// I'll add it — a DayPlan.Completed field plus a mark-complete
// endpoint). (2) duration_minutes is only populated for
// session-library days (Pilates/Yoga/Running/etc.) — mesocycle-pinned
// days (Strength, and Functional if ever enabled) resolve to a list of
// exercises with no single total-duration field yet, so it's omitted
// there.
func (h *WeeklyPlanHandler) CurrentWeek(w http.ResponseWriter, r *http.Request) {
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

	out, err := h.CurrentUC.Execute(ctx, usecase.GetCurrentWeeklyPlanInput{UserID: userID, Date: date})
	if err != nil {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		log.Printf("[weeklyplan.current] error: %+v\n", err)
		http.Error(w, "failed to get current weekly plan", http.StatusInternalServerError)
		return
	}
	if out.Draft == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	draft := out.Draft
	days := make([]weeklyPlanDayResponse, 0, len(draft.Days))
	for _, d := range draft.Days {
		day := weeklyPlanDayResponse{
			Weekday:   d.Weekday,
			Date:      d.Date.Format("2006-01-02"),
			IsToday:   sameCalendarDay(d.Date, date),
			IsRestDay: d.IsRestDay,
		}
		if !d.IsRestDay {
			day.ActivityType = string(d.Assignment.ActivityType)
			day.MuscleGroup = string(d.Assignment.MuscleGroup)
			day.Intensity = string(d.Assignment.Intensity)
			day.Impact = string(d.Assignment.Impact)
			day.Substituted = d.Assignment.Substituted
			day.Warning = d.Warning
			if d.Content != nil && d.Content.Session != nil {
				minutes := d.Content.Session.DurationMinutes
				day.DurationMinutes = &minutes
			}
		}
		days = append(days, day)
	}

	resp := currentWeeklyPlanResponse{
		DraftID:   draft.ID,
		StartDate: draft.StartDate.Format("2006-01-02"),
		EndDate:   draft.EndDate.Format("2006-01-02"),
		GoalID:    string(draft.GoalID),
		Days:      days,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

type weeklyPlanDayBlockResponse struct {
	DurationMinutes int      `json:"duration_minutes"`
	Description     string   `json:"description"`
	Movements       []string `json:"movements"`
}

type weeklyPlanDayExerciseResponse struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"`
	RestSeconds  int    `json:"rest_seconds"`
	LoadGuidance string `json:"load_guidance"`
	FormCue      string `json:"form_cue"`
}

type weeklyPlanDayDetailResponse struct {
	Date      string `json:"date"`
	Weekday   string `json:"weekday"`
	IsRestDay bool   `json:"is_rest_day"`

	ActivityType string `json:"activity_type,omitempty"`
	MuscleGroup  string `json:"muscle_group,omitempty"`
	Intensity    string `json:"intensity,omitempty"`
	Impact       string `json:"impact,omitempty"`
	Substituted  bool   `json:"substituted,omitempty"`
	Warning      string `json:"warning,omitempty"`

	// Loggable signals whether this day has discrete sets/reps to log in
	// real time — see WeeklyPlanDayDetail.Loggable. This endpoint itself
	// only reads; it does not implement the logging.
	Loggable bool `json:"loggable,omitempty"`

	Title               string `json:"title,omitempty"`
	LoadLabel           string `json:"load_label,omitempty"`
	DurationMinutes     int    `json:"duration_minutes,omitempty"`
	DurationIsEstimated bool   `json:"duration_is_estimated,omitempty"`
	ExerciseCount       int    `json:"exercise_count,omitempty"`

	Warmup        *weeklyPlanDayBlockResponse     `json:"warmup,omitempty"`
	MainExercises []weeklyPlanDayExerciseResponse `json:"main_exercises,omitempty"`
	Cooldown      *weeklyPlanDayBlockResponse     `json:"cooldown,omitempty"`

	// Session is the in-progress or completed real-time log for a
	// Loggable day, if one has been started — see SessionLogHandler
	// (POST .../day/start, .../log-set, .../complete) for how it's
	// written. nil means not started yet.
	Session *sessionLogResponse `json:"session,omitempty"`
}

// GET /training/weekly-plan/day?date=YYYY-MM-DD
//
// Session-detail read for one day of the already-generated week — the
// warmup/main-work/cooldown breakdown a session-detail screen needs,
// plus (for Loggable days) a stable id per exercise and whatever
// real-time session log already exists, so a client that reopens the
// app mid-workout can resume instead of losing state. This endpoint is
// read-only itself; see SessionLogHandler for the write side
// (start/log-set/complete). There is no rest-timer endpoint — the
// countdown between sets is derived client-side from each exercise's
// rest_seconds, already present below.
//
// date is required and trusted as-is from the client, same reasoning as
// every other date-scoped endpoint here.
//
// Known gaps, not fabricated: (1) no coaching-note/description copy is
// generated anywhere in this pipeline yet, so there's no such field —
// title/load_label are mechanical taxonomy labels, not a copy system.
// (2) mesocycle-pinned days' (Strength, Functional if ever enabled)
// warmup/cooldown is a GENERIC pair per muscle group (see
// content.MesocycleWarmupCooldownLibrary), not tailored to the specific
// pinned exercises — real per-exercise-aware warmup/cooldown is a
// separate, richer feature to build later if needed. (3) their
// duration_minutes is a rough estimate from sets/rest (see
// duration_is_estimated), not an author-specified value the way
// session-library content has.
func (h *WeeklyPlanHandler) Day(w http.ResponseWriter, r *http.Request) {
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

	out, err := h.DayUC.Execute(ctx, usecase.GetWeeklyPlanDayInput{UserID: userID, Date: date})
	if err != nil {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		log.Printf("[weeklyplan.day] error: %+v\n", err)
		http.Error(w, "failed to get session detail", http.StatusInternalServerError)
		return
	}
	if !out.Found {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	d := out.Day
	resp := weeklyPlanDayDetailResponse{
		Date:      d.Date.Format("2006-01-02"),
		Weekday:   d.Weekday,
		IsRestDay: d.IsRestDay,
	}
	if !d.IsRestDay {
		resp.ActivityType = d.ActivityType
		resp.MuscleGroup = d.MuscleGroup
		resp.Intensity = d.Intensity
		resp.Impact = d.Impact
		resp.Substituted = d.Substituted
		resp.Warning = d.Warning
		resp.Loggable = d.Loggable
		resp.Title = d.Title
		resp.LoadLabel = d.LoadLabel
		resp.DurationMinutes = d.DurationMinutes
		resp.DurationIsEstimated = d.DurationIsEstimated
		resp.ExerciseCount = len(d.MainExercises)
		if d.Session != nil {
			sr := toSessionLogResponse(*d.Session)
			resp.Session = &sr
		}

		if d.Warmup != nil {
			resp.Warmup = &weeklyPlanDayBlockResponse{
				DurationMinutes: d.Warmup.DurationMinutes,
				Description:     d.Warmup.Description,
				Movements:       d.Warmup.Movements,
			}
		}
		if d.Cooldown != nil {
			resp.Cooldown = &weeklyPlanDayBlockResponse{
				DurationMinutes: d.Cooldown.DurationMinutes,
				Description:     d.Cooldown.Description,
				Movements:       d.Cooldown.Movements,
			}
		}
		for _, e := range d.MainExercises {
			resp.MainExercises = append(resp.MainExercises, weeklyPlanDayExerciseResponse{
				ID: e.ID, Name: e.Name, Sets: e.Sets, Reps: e.Reps, RestSeconds: e.RestSeconds,
				LoadGuidance: e.LoadGuidance, FormCue: e.FormCue,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// mondayOfCurrentWeek mirrors the same helper duplicated in
// training_handler.go's parseArrangementRequest and
// runner.LocalTrainingPlanRunner — kept local rather than newly shared
// across packages for this change.
func mondayOfCurrentWeek(now time.Time) time.Time {
	wd := now.Weekday()
	if wd == time.Sunday {
		wd = 7
	}
	monday := now.AddDate(0, 0, -int(wd-time.Monday))
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}
