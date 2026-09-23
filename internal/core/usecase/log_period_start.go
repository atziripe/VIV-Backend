package usecase

import (
	"context"
	"log"
	"strings"
	"time"

	"viv/internal/core/cascade"
	"viv/internal/core/domain"
)

// LogPeriodStartInput reports that a period started, independent of the
// weekly check-in — always available so it can be logged the moment it
// happens instead of waiting for the next check-in window. Mom Test P0
// fix: I04/I07/I08 each named "had to wait until Sunday to log it" as an
// active abandonment trigger.
type LogPeriodStartInput struct {
	UserID string
	// Date defaults to today (UTC) when zero — the common case is logging
	// the day it happens, but reporting a day or two late (or early) is
	// allowed.
	Date time.Time
}

// MovedDay is one day of the current week whose training assignment
// changed because this period-start report landed on a different date
// than VIV had predicted — see LogPeriodStartUseCase's rebuildWeek.
type MovedDay struct {
	Weekday         string
	Date            time.Time
	IsRestDay       bool
	Title           string // "" for a rest day
	DurationMinutes int    // 0 for a rest day
}

type LogPeriodStartOutput struct {
	User               *domain.User
	CycleDay           int
	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int

	// PreviousCycleDuration/CycleDurationChanged describe whether this
	// report recalibrated the learned cycle length (see
	// ApplyCycleStartOverride) — "" / false when there was no prior
	// anchor to measure an interval against (the first-ever report).
	PreviousCycleDuration int
	CycleDurationChanged  bool

	// NextEstimatedPeriodDate is the zero time when it can't be predicted
	// yet (shouldn't happen right after a successful report, since this
	// call itself just set CycleAnchorAt).
	NextEstimatedPeriodDate time.Time

	// WeekRebuilt is true when this report's date differed enough from
	// what VIV had predicted that the current week's shape was
	// regenerated around the real anchor — false when the report matched
	// the prediction (nothing to rebuild) or Drafts/Generator weren't
	// wired (best-effort, never blocks the core cycle update above).
	WeekRebuilt bool
	Moved       []MovedDay
}

type LogPeriodStartUseCase struct {
	Users UserRepository

	// Drafts/Generator/NutritionResync are all optional, nil-safe — they
	// enable the week-rebuild side effect (see rebuildWeek). Leaving them
	// nil still fully supports the core action (recording the report and
	// recalibrating the cycle), just without touching the training week —
	// same optional-collaborator convention as NutritionResyncer elsewhere
	// in this pipeline.
	Drafts          WeeklyPlanDraftRepository
	Generator       WeeklyPlanGenerator
	NutritionResync NutritionResyncer
}

func NewLogPeriodStartUseCase(
	users UserRepository,
	drafts WeeklyPlanDraftRepository,
	generator WeeklyPlanGenerator,
	nutritionResync NutritionResyncer,
) *LogPeriodStartUseCase {
	return &LogPeriodStartUseCase{
		Users:           users,
		Drafts:          drafts,
		Generator:       generator,
		NutritionResync: nutritionResync,
	}
}

func (uc *LogPeriodStartUseCase) Execute(ctx context.Context, in LogPeriodStartInput) (*LogPeriodStartOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return nil, ErrUserNotFound("")
	}

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound(userID)
	}

	now := time.Now().UTC()
	date := in.Date
	if date.IsZero() {
		date = now
	}
	date = localMidnightUTC(date)

	// Snapshot what VIV predicted BEFORE applying this report — this is
	// what decides whether the current week's premise needs to change,
	// and what the recalibration-changed comparison below is measured
	// against.
	prevExpected := ExpectedPeriodDate(user)
	prevDuration := parseIntDefault(user.CycleDuration, 28)

	// Reuses the exact recalibration the weekly check-in's optional
	// cycle_start field and onboarding already use — one source of truth
	// for what "reporting a period start" does to the cycle, instead of
	// duplicating that math here.
	ApplyCycleStartOverride(user, date, now)
	user.UpdatedAt = now

	if err := uc.Users.Save(ctx, user); err != nil {
		return nil, err
	}

	newDuration := parseIntDefault(user.CycleDuration, 28)
	phase := mapCyclePhase(user.CyclePhase)

	out := &LogPeriodStartOutput{
		User:                    user,
		CycleDay:                user.CycleDay,
		CurrentPhase:            string(phase),
		NextPhase:               NextPhaseName(phase),
		DaysUntilNextPhase:      DaysUntilNextPhase(user),
		PreviousCycleDuration:   prevDuration,
		CycleDurationChanged:    newDuration != prevDuration,
		NextEstimatedPeriodDate: ExpectedPeriodDate(user),
	}

	// Only reshuffle the current week when there was an actual prediction
	// to be wrong about, and this report landed on a different calendar
	// day than that prediction — an on-time report means the week was
	// already built on a correct premise, so there's nothing to rebuild.
	if !prevExpected.IsZero() && !sameDay(prevExpected, date) {
		uc.rebuildWeek(ctx, user, date, out)
	}

	return out, nil
}

// rebuildWeek regenerates the current week from date forward, now that the
// cycle is anchored to a real (not predicted) period start — reusing the
// exact same generation pipeline the daily check-in rollover uses
// (GenerateWeeklyPlanUsecase) rather than a bespoke reshuffle algorithm.
// The old draft still covers every day before date, so past days are left
// as historical record; only today-forward comes from the fresh draft
// (WeeklyPlanDraftRepository.GetByDate resolves this automatically by
// start_date, newest first). Best-effort throughout: a failure here never
// fails the period-start report itself, which already succeeded and was
// saved by the time this runs.
func (uc *LogPeriodStartUseCase) rebuildWeek(ctx context.Context, user *domain.User, date time.Time, out *LogPeriodStartOutput) {
	if uc.Drafts == nil || uc.Generator == nil {
		return
	}
	userID := user.ID

	existing, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		log.Printf("[period.rebuild] loading current week failed user=%s err=%v", userID, err)
		return
	}
	if existing == nil {
		// No plan covers today at all — the normal check-in rollover
		// already handles generating one; nothing here for this report
		// to rebuild against.
		return
	}
	oldDraft := *existing

	genOut, err := uc.Generator.Execute(ctx, GenerateWeeklyPlanInput{
		UserID:         userID,
		GenerationDate: date,
		GoalID:         user.GoalID,
		Catalog:        cascade.UserCatalog{Activities: user.UserCatalog},
		// No daily check-in exists for this call — same neutral default
		// onboarding's first-ever generation uses. A same-day /checkin
		// call afterward still adapts today normally on top of this.
		Readiness: &DefaultOnboardingReadiness,
	})
	if err != nil {
		log.Printf("[period.rebuild] regenerating week failed user=%s err=%v", userID, err)
		return
	}

	uc.resyncNutrition(ctx, userID, genOut.Draft)

	out.WeekRebuilt = true
	out.Moved = diffWeeks(oldDraft, genOut.Draft, date)
}

func (uc *LogPeriodStartUseCase) resyncNutrition(ctx context.Context, userID string, draft WeekDraft) {
	if uc.NutritionResync == nil {
		return
	}
	if err := uc.NutritionResync.Execute(ctx, userID, draft); err != nil {
		log.Printf("[period.rebuild] nutrition resync failed user=%s err=%v", userID, err)
	}
}

// diffWeeks reports each day from >= from whose assignment changed between
// the old and new draft — the "what moved" list the client shows after a
// rebuild. Only days present in both drafts are compared; a day the new
// draft doesn't cover (shouldn't happen — both span 7 days from their own
// start) is silently skipped rather than treated as a false "moved".
func diffWeeks(oldDraft, newDraft WeekDraft, from time.Time) []MovedDay {
	var moved []MovedDay
	for _, oldDay := range oldDraft.Days {
		if oldDay.Date.Before(from) {
			continue
		}
		newIdx, ok := dayIndexForDate(newDraft, oldDay.Date)
		if !ok {
			continue
		}
		newDay := newDraft.Days[newIdx]
		if oldDay.IsRestDay == newDay.IsRestDay && oldDay.Assignment == newDay.Assignment {
			continue
		}
		moved = append(moved, MovedDay{
			Weekday:         newDay.Weekday,
			Date:            newDay.Date,
			IsRestDay:       newDay.IsRestDay,
			Title:           dayTitle(newDay),
			DurationMinutes: durationMinutesForDay(newDay),
		})
	}
	return moved
}

func dayTitle(day DayPlan) string {
	if day.IsRestDay {
		return ""
	}
	return titleFor(day.Assignment.MuscleGroup, day.Assignment.ActivityType)
}
