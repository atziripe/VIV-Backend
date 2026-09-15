package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
)

// ============================================================================
// SESSION LOGGING — real-time set-by-set logging for Loggable
// (mesocycle-pinned) days only, per product decision: session-library
// days (Yoga, Pilates, Running, ...) have no completion/logging support
// yet — a smaller, separate feature if ever needed. Three actions, one
// per screen of the logging flow: start a session, log one set, complete
// the session with optional feedback. A rest timer between sets needs no
// endpoint at all — it's purely derived client-side from the current
// exercise's RestSeconds, already returned by GET /training/weekly-plan/day.
// ============================================================================

type SessionLogRepository interface {
	GetByDate(ctx context.Context, userID string, date time.Time) (*domain.SessionLog, error)
	// Upsert creates or replaces the log for (log.UserID, log.Date) —
	// idempotent by construction, same convention as DailyCheckinRepository.
	Upsert(ctx context.Context, log *domain.SessionLog) error
}

// loggableDayExercises is the shared precondition every session-logging
// usecase below needs: a plan must cover the date, the day must not be a
// rest day, and it must be Loggable (mesocycle-pinned) — errors clearly
// otherwise rather than letting a non-loggable day silently accept logs.
func loggableDayExercises(ctx context.Context, drafts WeeklyPlanDraftRepository, userID string, date time.Time) (DayPlan, []PrescribedExercise, error) {
	draft, err := drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return DayPlan{}, nil, fmt.Errorf("session logging: %w", err)
	}
	if draft == nil {
		return DayPlan{}, nil, errSessionLogging("session logging: no plan covers %s", date.Format("2006-01-02"))
	}
	idx, ok := dayIndexForDate(*draft, date)
	if !ok {
		return DayPlan{}, nil, errSessionLogging("session logging: %s is not part of the loaded week", date.Format("2006-01-02"))
	}
	day := draft.Days[idx]
	if day.IsRestDay {
		return DayPlan{}, nil, errSessionLogging("session logging: %s is a rest day", date.Format("2006-01-02"))
	}
	if day.Content == nil || day.Content.Exercises == nil {
		return DayPlan{}, nil, errSessionLogging("session logging: %s is not a loggable day", date.Format("2006-01-02"))
	}
	return day, day.Content.Exercises, nil
}

func normalizeLogDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ============================================================================
// START SESSION
// ============================================================================

type StartSessionInput struct {
	UserID string
	Date   time.Time
}

type StartSessionOutput struct {
	Log domain.SessionLog
}

type StartSessionUseCase struct {
	Drafts WeeklyPlanDraftRepository
	Logs   SessionLogRepository
}

func NewStartSessionUseCase(drafts WeeklyPlanDraftRepository, logs SessionLogRepository) *StartSessionUseCase {
	return &StartSessionUseCase{Drafts: drafts, Logs: logs}
}

func (uc *StartSessionUseCase) Execute(ctx context.Context, in StartSessionInput) (StartSessionOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return StartSessionOutput{}, errSessionLogging("start session: userID is required")
	}
	date := normalizeLogDate(in.Date)

	day, _, err := loggableDayExercises(ctx, uc.Drafts, userID, date)
	if err != nil {
		return StartSessionOutput{}, err
	}

	existing, err := uc.Logs.GetByDate(ctx, userID, date)
	if err != nil {
		return StartSessionOutput{}, fmt.Errorf("start session: %w", err)
	}
	if existing != nil {
		// Resuming, not restarting: never reset StartedAt/Sets/Feedback a
		// user has already logged — whether still in progress or already
		// done, hand back exactly what's there.
		return StartSessionOutput{Log: *existing}, nil
	}

	log := domain.SessionLog{
		UserID:       userID,
		Date:         date,
		ActivityType: string(day.Assignment.ActivityType),
		Status:       domain.SessionLogInProgress,
		StartedAt:    time.Now().UTC(),
	}
	if err := uc.Logs.Upsert(ctx, &log); err != nil {
		return StartSessionOutput{}, fmt.Errorf("start session: %w", err)
	}
	return StartSessionOutput{Log: log}, nil
}

// ============================================================================
// LOG SET
// ============================================================================

type LogSetInput struct {
	UserID     string
	Date       time.Time
	ExerciseID string
	SetNumber  int
	WeightKg   float64
	Reps       int
}

type LogSetOutput struct {
	Log domain.SessionLog
}

type LogSetUseCase struct {
	Drafts WeeklyPlanDraftRepository
	Logs   SessionLogRepository
}

func NewLogSetUseCase(drafts WeeklyPlanDraftRepository, logs SessionLogRepository) *LogSetUseCase {
	return &LogSetUseCase{Drafts: drafts, Logs: logs}
}

func (uc *LogSetUseCase) Execute(ctx context.Context, in LogSetInput) (LogSetOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return LogSetOutput{}, errSessionLogging("log set: userID is required")
	}
	exerciseID := strings.TrimSpace(in.ExerciseID)
	if exerciseID == "" {
		return LogSetOutput{}, errSessionLogging("log set: exerciseID is required")
	}
	if in.WeightKg < 0 {
		return LogSetOutput{}, errSessionLogging("log set: weight_kg cannot be negative")
	}
	if in.Reps < 0 {
		return LogSetOutput{}, errSessionLogging("log set: reps cannot be negative")
	}
	date := normalizeLogDate(in.Date)

	_, exercises, err := loggableDayExercises(ctx, uc.Drafts, userID, date)
	if err != nil {
		return LogSetOutput{}, err
	}

	var prescribed *PrescribedExercise
	for i := range exercises {
		if string(exercises[i].Exercise.ID) == exerciseID {
			prescribed = &exercises[i]
			break
		}
	}
	if prescribed == nil {
		return LogSetOutput{}, errSessionLogging("log set: exercise %q is not part of today's session", exerciseID)
	}
	if in.SetNumber < 1 || in.SetNumber > prescribed.Prescription.Sets {
		return LogSetOutput{}, errSessionLogging("log set: set %d is out of range for %q (prescribed %d sets)", in.SetNumber, exerciseID, prescribed.Prescription.Sets)
	}

	log, err := uc.Logs.GetByDate(ctx, userID, date)
	if err != nil {
		return LogSetOutput{}, fmt.Errorf("log set: %w", err)
	}
	if log == nil {
		return LogSetOutput{}, errSessionLogging("log set: no session in progress for %s — start the session first", date.Format("2006-01-02"))
	}
	if log.Status != domain.SessionLogInProgress {
		return LogSetOutput{}, errSessionLogging("log set: session for %s is already done", date.Format("2006-01-02"))
	}

	entry := domain.SetLog{
		ExerciseID:   exerciseID,
		ExerciseName: prescribed.Exercise.Name,
		SetNumber:    in.SetNumber,
		WeightKg:     in.WeightKg,
		Reps:         in.Reps,
		LoggedAt:     time.Now().UTC(),
	}
	replaced := false
	for i, s := range log.Sets {
		if s.ExerciseID == exerciseID && s.SetNumber == in.SetNumber {
			log.Sets[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		log.Sets = append(log.Sets, entry)
	}

	if err := uc.Logs.Upsert(ctx, log); err != nil {
		return LogSetOutput{}, fmt.Errorf("log set: %w", err)
	}
	return LogSetOutput{Log: *log}, nil
}

// ============================================================================
// COMPLETE SESSION
// ============================================================================

type CompleteSessionInput struct {
	UserID   string
	Date     time.Time
	Feedback string // "" | "easy" | "right" | "too_much"
}

type CompleteSessionOutput struct {
	Log            domain.SessionLog
	ElapsedMinutes int
	SetsCompleted  int
	SetsTotal      int
}

type CompleteSessionUseCase struct {
	Drafts WeeklyPlanDraftRepository
	Logs   SessionLogRepository
}

func NewCompleteSessionUseCase(drafts WeeklyPlanDraftRepository, logs SessionLogRepository) *CompleteSessionUseCase {
	return &CompleteSessionUseCase{Drafts: drafts, Logs: logs}
}

func (uc *CompleteSessionUseCase) Execute(ctx context.Context, in CompleteSessionInput) (CompleteSessionOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return CompleteSessionOutput{}, errSessionLogging("complete session: userID is required")
	}
	date := normalizeLogDate(in.Date)

	var feedback domain.SessionFeedback
	switch domain.SessionFeedback(in.Feedback) {
	case "", domain.FeedbackEasy, domain.FeedbackRight, domain.FeedbackTooMuch:
		feedback = domain.SessionFeedback(in.Feedback)
	default:
		return CompleteSessionOutput{}, errSessionLogging("complete session: unrecognized feedback %q", in.Feedback)
	}

	_, exercises, err := loggableDayExercises(ctx, uc.Drafts, userID, date)
	if err != nil {
		return CompleteSessionOutput{}, err
	}
	setsTotal := 0
	for _, pe := range exercises {
		setsTotal += pe.Prescription.Sets
	}

	log, err := uc.Logs.GetByDate(ctx, userID, date)
	if err != nil {
		return CompleteSessionOutput{}, fmt.Errorf("complete session: %w", err)
	}
	if log == nil {
		return CompleteSessionOutput{}, errSessionLogging("complete session: no session in progress for %s — start the session first", date.Format("2006-01-02"))
	}
	if log.Status == domain.SessionLogDone {
		return CompleteSessionOutput{}, errSessionLogging("complete session: session for %s is already done", date.Format("2006-01-02"))
	}

	now := time.Now().UTC()
	log.Status = domain.SessionLogDone
	log.EndedAt = &now
	if feedback != "" {
		log.Feedback = feedback
	}

	if err := uc.Logs.Upsert(ctx, log); err != nil {
		return CompleteSessionOutput{}, fmt.Errorf("complete session: %w", err)
	}

	return CompleteSessionOutput{
		Log:            *log,
		ElapsedMinutes: int(now.Sub(log.StartedAt).Minutes() + 0.5),
		SetsCompleted:  len(log.Sets),
		SetsTotal:      setsTotal,
	}, nil
}
