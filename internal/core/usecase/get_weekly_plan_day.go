package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/activity"
)

// ============================================================================
// GET WEEKLY PLAN DAY — the "session detail" read for one day of an
// already-generated week: the warmup/main-work/cooldown breakdown a
// session-detail screen needs. Pure read of what VIV-106/VIV-112 already
// persisted on the draft (DayPlan.Content) — this does NOT re-run content
// selection or exercise pinning.
//
// This is read-only, and deliberately only that: it does not implement
// set-by-set logging, rest timers, or session completion/feedback — a
// real-time logging flow for mesocycle-pinned (Loggable) days needs its
// own, separate write endpoints that don't exist yet.
// ============================================================================

// assumedSecondsPerSet is a first-draft estimate used ONLY to derive a
// rough total duration for mesocycle-pinned (exercise-list) days, which
// have no author-specified duration the way session-library content
// does (content.Session.DurationMinutes). Not sourced from any
// literature review — flagged via WeeklyPlanDayDetail.DurationIsEstimated
// rather than silently presented as exact.
const assumedSecondsPerSet = 45

type GetWeeklyPlanDayInput struct {
	UserID string
	Date   time.Time // local calendar date, client-sent — same convention as every other date-scoped endpoint here
}

type WeeklyPlanDayExercise struct {
	ID           string // "" for session-library exercises — no stable per-exercise ID exists for that path today
	Name         string
	Sets         int
	Reps         string
	RestSeconds  int
	LoadGuidance string
	FormCue      string
}

type WeeklyPlanDayBlock struct {
	DurationMinutes int
	Description     string
	Movements       []string
}

type WeeklyPlanDayDetail struct {
	Date      time.Time
	Weekday   string
	IsRestDay bool

	ActivityType string
	MuscleGroup  string
	Intensity    string
	Impact       string
	Substituted  bool
	Warning      string

	// Loggable is true for a mesocycle-pinned day (Strength, and
	// Functional if ever enabled) — the only days with discrete
	// sets/reps/weight to log in real time today. Derived from which
	// half of Content was actually populated at generation time (never
	// both), so it always matches what MainExercises actually is.
	Loggable bool

	// Title and LoadLabel are mechanical taxonomy labels ("Lower" +
	// "Strength" -> "Lower Strength"; Intensity M -> "Moderate load") —
	// NOT the weekly-note copy/tone system (VIV-113, LLM-generated
	// narrative). No coaching-note/description text is generated
	// anywhere in this pipeline yet, so there is deliberately no such
	// field here.
	Title     string
	LoadLabel string

	DurationMinutes int
	// DurationIsEstimated is true when DurationMinutes came from
	// assumedSecondsPerSet rather than author-specified content.
	DurationIsEstimated bool

	// Warmup/Cooldown come from the session library for session-library
	// days, or from a generic per-muscle-group pair for mesocycle-pinned
	// days (see content.MesocycleWarmupCooldownLibrary) — nil only if
	// Content was never hydrated at all (shouldn't happen once
	// generation completes, but never fabricated to fill the gap).
	Warmup        *WeeklyPlanDayBlock
	MainExercises []WeeklyPlanDayExercise
	Cooldown      *WeeklyPlanDayBlock
}

type GetWeeklyPlanDayOutput struct {
	// Found is false when no generated week covers Date at all.
	Found bool
	Day   WeeklyPlanDayDetail
}

type GetWeeklyPlanDayUseCase struct {
	Drafts WeeklyPlanDraftRepository
}

func NewGetWeeklyPlanDayUseCase(drafts WeeklyPlanDraftRepository) *GetWeeklyPlanDayUseCase {
	return &GetWeeklyPlanDayUseCase{Drafts: drafts}
}

func (uc *GetWeeklyPlanDayUseCase) Execute(ctx context.Context, in GetWeeklyPlanDayInput) (GetWeeklyPlanDayOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return GetWeeklyPlanDayOutput{}, fmt.Errorf("get weekly plan day: userID is required")
	}
	date := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)

	draft, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return GetWeeklyPlanDayOutput{}, fmt.Errorf("get weekly plan day: %w", err)
	}
	if draft == nil {
		return GetWeeklyPlanDayOutput{Found: false}, nil
	}

	idx, ok := dayIndexForDate(*draft, date)
	if !ok {
		return GetWeeklyPlanDayOutput{}, fmt.Errorf("get weekly plan day: %s is not part of the loaded week", date.Format("2006-01-02"))
	}
	day := draft.Days[idx]

	detail := WeeklyPlanDayDetail{
		Date:      day.Date,
		Weekday:   day.Weekday,
		IsRestDay: day.IsRestDay,
	}
	if day.IsRestDay {
		return GetWeeklyPlanDayOutput{Found: true, Day: detail}, nil
	}

	detail.ActivityType = string(day.Assignment.ActivityType)
	detail.MuscleGroup = string(day.Assignment.MuscleGroup)
	detail.Intensity = string(day.Assignment.Intensity)
	detail.Impact = string(day.Assignment.Impact)
	detail.Substituted = day.Assignment.Substituted
	detail.Warning = day.Warning
	detail.Title = titleFor(day.Assignment.MuscleGroup, day.Assignment.ActivityType)
	detail.LoadLabel = loadLabelFor(day.Assignment.Intensity)

	switch {
	case day.Content != nil && day.Content.Session != nil:
		s := day.Content.Session
		detail.DurationMinutes = s.DurationMinutes
		detail.Warmup = &WeeklyPlanDayBlock{
			DurationMinutes: s.Content.Warmup.DurationMinutes,
			Description:     s.Content.Warmup.Description,
			Movements:       s.Content.Warmup.Movements,
		}
		detail.Cooldown = &WeeklyPlanDayBlock{
			DurationMinutes: s.Content.Cooldown.DurationMinutes,
			Description:     s.Content.Cooldown.Description,
			Movements:       s.Content.Cooldown.Movements,
		}
		for _, e := range s.Content.MainExercises {
			detail.MainExercises = append(detail.MainExercises, WeeklyPlanDayExercise{
				Name: e.Name, Sets: e.Sets, Reps: e.Reps, RestSeconds: e.RestSeconds,
				LoadGuidance: e.LoadGuidance, FormCue: e.FormCue,
			})
		}

	case day.Content != nil && day.Content.Exercises != nil:
		detail.Loggable = true
		totalSeconds := 0
		for _, pe := range day.Content.Exercises {
			detail.MainExercises = append(detail.MainExercises, WeeklyPlanDayExercise{
				ID: string(pe.Exercise.ID), Name: pe.Exercise.Name,
				Sets: pe.Prescription.Sets, Reps: pe.Prescription.Reps, RestSeconds: pe.Prescription.RestSeconds,
				LoadGuidance: pe.Prescription.LoadGuidance, FormCue: pe.Prescription.FormCue,
			})
			totalSeconds += pe.Prescription.Sets * (pe.Prescription.RestSeconds + assumedSecondsPerSet)
		}
		detail.DurationMinutes = (totalSeconds + 30) / 60 // rounded to the nearest minute
		detail.DurationIsEstimated = true
		if day.Content.Warmup != nil {
			detail.Warmup = &WeeklyPlanDayBlock{
				DurationMinutes: day.Content.Warmup.DurationMinutes,
				Description:     day.Content.Warmup.Description,
				Movements:       day.Content.Warmup.Movements,
			}
		}
		if day.Content.Cooldown != nil {
			detail.Cooldown = &WeeklyPlanDayBlock{
				DurationMinutes: day.Content.Cooldown.DurationMinutes,
				Description:     day.Content.Cooldown.Description,
				Movements:       day.Content.Cooldown.Movements,
			}
		}
	}

	return GetWeeklyPlanDayOutput{Found: true, Day: detail}, nil
}

// titleFor and loadLabelFor are simple, mechanical taxonomy-to-label
// mappings — see WeeklyPlanDayDetail's doc comment for why these are not
// the weekly-note copy system.
func titleFor(mg activity.MuscleGroup, at activity.ID) string {
	return strings.TrimSpace(titleCaseWords(string(mg)) + " " + titleCaseWords(string(at)))
}

func titleCaseWords(s string) string {
	words := strings.Fields(strings.ReplaceAll(s, "_", " "))
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func loadLabelFor(intensity activity.IntensityLevel) string {
	switch intensity {
	case activity.IntensityAR:
		return "Recovery load"
	case activity.IntensityL:
		return "Light load"
	case activity.IntensityM:
		return "Moderate load"
	case activity.IntensityH:
		return "High load"
	default:
		return ""
	}
}
