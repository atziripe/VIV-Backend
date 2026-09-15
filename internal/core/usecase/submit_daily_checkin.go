package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
)

// ============================================================================
// SUBMIT DAILY CHECK-IN — the single entry point the client calls every
// day, tying together VIV-103 (check-in scoring), VIV-106 (weekly
// generation) and VIV-107 (daily adaptation).
// ============================================================================
//
// Date is the user's *local* calendar date, supplied by the client as-is.
// This codebase has no existing source of truth for "a user's timezone" —
// domain.User has no timezone field, and the only timezone data that
// exists (domain.DeviceToken.Timezone) is per-device and only ever used
// for push-notification scheduling, never for date-sensitive business
// logic. Rather than bolt on a guessed mechanism here, the client (which
// always knows its own local date) resolves and sends it directly; the
// server trusts it rather than deriving one from server time.

// DailyAdapter runs VIV-107's daily adaptation over an already-generated
// week. Satisfied directly by *AdaptDailySlotUsecase — declared narrowly
// here, same reasoning as WeeklyPlanGenerator (see complete_onboarding.go):
// this usecase's own tests don't need AdaptDailySlotUsecase's collaborators.
type DailyAdapter interface {
	Execute(ctx context.Context, input AdaptDailySlotInput) (AdaptDailySlotOutput, error)
}

// DailyCheckinRepository persists VIV-103's daily check-ins — one record
// per (userID, date). Distinct from CheckinRepository, which backs the
// older weekly check-in (domain.Checkin).
type DailyCheckinRepository interface {
	// Upsert creates or replaces the check-in for (c.UserID, c.Date) —
	// idempotent by construction: a second submission for the same date
	// updates the existing record instead of creating a duplicate.
	Upsert(ctx context.Context, c *domain.DailyCheckin) error
}

type SubmitDailyCheckinInput struct {
	UserID  string
	Date    time.Time
	Answers checkin.DailyCheckin
}

type SubmitDailyCheckinOutput struct {
	Date      time.Time
	Readiness checkin.ReadinessDimensions

	// Regenerated is true when no plan covered Date and this triggered a
	// fresh weekly generation (VIV-106) instead of daily adaptation
	// (VIV-107).
	Regenerated bool

	IsRestDay  bool
	Assignment cascade.SlotAssignment // zero value when IsRestDay

	// Reason explains an adjustment in plain language — "" when the
	// resolved session needed no adjustment (either it was fresh from
	// generation with nothing to relax, or it already fit and daily
	// adaptation left it untouched).
	Reason string

	// Suggestion carries a non-applied safety-relevant adjustment for a
	// user-overridden slot — see AdaptDailySlotOutput.Suggestion. Always
	// nil on the Regenerated path (a fresh week has no overrides yet).
	Suggestion *AdaptationSuggestion
}

type SubmitDailyCheckinUseCase struct {
	Checkins  DailyCheckinRepository
	Drafts    WeeklyPlanDraftRepository
	Users     UserRepository
	Generator WeeklyPlanGenerator
	Adapter   DailyAdapter
}

func NewSubmitDailyCheckinUseCase(
	checkins DailyCheckinRepository,
	drafts WeeklyPlanDraftRepository,
	users UserRepository,
	generator WeeklyPlanGenerator,
	adapter DailyAdapter,
) *SubmitDailyCheckinUseCase {
	return &SubmitDailyCheckinUseCase{
		Checkins:  checkins,
		Drafts:    drafts,
		Users:     users,
		Generator: generator,
		Adapter:   adapter,
	}
}

func (uc *SubmitDailyCheckinUseCase) Execute(ctx context.Context, in SubmitDailyCheckinInput) (SubmitDailyCheckinOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: userID is required")
	}
	date := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)

	// ── VIV-103: score today's answers ───────────────────────────
	readiness, err := checkin.Derive(in.Answers)
	if err != nil {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: deriving readiness: %w", err)
	}

	// ── Persist — idempotent per (userID, date); a second submission
	// for the same date overwrites rather than duplicates, so downstream
	// always recomputes from these (possibly just-updated) values ──────
	record := &domain.DailyCheckin{
		UserID:      userID,
		Date:        date,
		Answers:     in.Answers,
		Readiness:   readiness,
		SubmittedAt: time.Now().UTC(),
	}
	if err := uc.Checkins.Upsert(ctx, record); err != nil {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: saving: %w", err)
	}

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return SubmitDailyCheckinOutput{}, err
	}
	if user == nil {
		return SubmitDailyCheckinOutput{}, ErrUserNotFound(userID)
	}

	// ── Does a plan already cover today? ─────────────────────────
	existing, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: checking for an existing week: %w", err)
	}

	if existing == nil {
		// No plan covers today: either this is her very first check-in
		// ever (onboarding's own generation, VIV-115, somehow didn't
		// run), or — the expected common case — last week's plan rolled
		// over and today starts a new cycle. Either way: generate a
		// fresh week, using today's just-computed check-in as the
		// generation-day readiness (not onboarding's neutral default —
		// that's only for the very first plan).
		return uc.generateWeek(ctx, userID, date, readiness, user)
	}

	idx, ok := dayIndexForDate(*existing, date)
	if !ok {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: %s is not part of the loaded week", date.Format("2006-01-02"))
	}

	return uc.adaptDay(ctx, userID, date, in.Answers, readiness, existing.Days[idx].IsRestDay)
}

func (uc *SubmitDailyCheckinUseCase) generateWeek(
	ctx context.Context,
	userID string,
	date time.Time,
	readiness checkin.ReadinessDimensions,
	user *domain.User,
) (SubmitDailyCheckinOutput, error) {
	out, err := uc.Generator.Execute(ctx, GenerateWeeklyPlanInput{
		UserID:         userID,
		GenerationDate: date,
		GoalID:         user.GoalID,
		Catalog:        cascade.UserCatalog{Activities: user.UserCatalog},
		Readiness:      &readiness,
	})
	if err != nil {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: generating week: %w", err)
	}

	idx, ok := dayIndexForDate(out.Draft, date)
	if !ok {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: freshly generated week doesn't cover %s", date.Format("2006-01-02"))
	}
	day := out.Draft.Days[idx]

	return SubmitDailyCheckinOutput{
		Date:        date,
		Readiness:   readiness,
		Regenerated: true,
		IsRestDay:   day.IsRestDay,
		Assignment:  day.Assignment,
		Reason:      AdjustmentReason(day.Assignment.AdjustmentLever),
	}, nil
}

func (uc *SubmitDailyCheckinUseCase) adaptDay(
	ctx context.Context,
	userID string,
	date time.Time,
	answers checkin.DailyCheckin,
	readiness checkin.ReadinessDimensions,
	isRestDay bool,
) (SubmitDailyCheckinOutput, error) {
	out, err := uc.Adapter.Execute(ctx, AdaptDailySlotInput{
		UserID:  userID,
		Date:    date,
		Checkin: answers,
	})
	if err != nil {
		return SubmitDailyCheckinOutput{}, fmt.Errorf("submit daily checkin: adapting today: %w", err)
	}

	return SubmitDailyCheckinOutput{
		Date:        out.Date,
		Readiness:   readiness,
		Regenerated: false,
		IsRestDay:   isRestDay,
		Assignment:  out.Assignment,
		Reason:      out.Reason,
		Suggestion:  out.Suggestion,
	}, nil
}
