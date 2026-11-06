package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/activity"
	"viv/internal/core/cascade"
	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/recovery"
)

// ============================================================================
// GET RECOVERY CARD — the compact, session-driven recovery read (VIV
// Recovery Engine Decision Table spec) that replaces the old pipeline's
// paragraph-banner recovery read entirely. Read-only except for one
// deliberate side effect: a High-cost day pushes a too-soon
// high-intensity session out to a later rest day in the same week (see
// rescheduleIfNeeded) — everything else about this usecase is pure read.
// ============================================================================

// maxRescheduleLookaheadDays bounds how far into the week
// rescheduleIfNeeded will look for a rest day to swap into — the whole
// rest of the week after tomorrow. Not a spec value; a structural limit
// (a WeekDraft only has 7 days).
const maxRescheduleLookaheadDays = 6

type GetRecoveryCardInput struct {
	UserID string
	Date   time.Time // local calendar date, client-sent — same convention as every other date-scoped endpoint here
}

type GetRecoveryCardOutput struct {
	// Found is false when no generated week covers Date.
	Found bool
	Card  domain.RecoveryCard
}

type GetRecoveryCardUseCase struct {
	Drafts   WeeklyPlanDraftRepository
	Checkins DailyCheckinRepository
	Users    UserRepository
	Actions  RecoveryActionRepository
	Content  ContentSelectionLayer
}

func NewGetRecoveryCardUseCase(
	drafts WeeklyPlanDraftRepository,
	checkins DailyCheckinRepository,
	users UserRepository,
	actions RecoveryActionRepository,
	content ContentSelectionLayer,
) *GetRecoveryCardUseCase {
	return &GetRecoveryCardUseCase{
		Drafts: drafts, Checkins: checkins, Users: users, Actions: actions, Content: content,
	}
}

func (uc *GetRecoveryCardUseCase) Execute(ctx context.Context, in GetRecoveryCardInput) (GetRecoveryCardOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return GetRecoveryCardOutput{}, fmt.Errorf("get recovery card: userID is required")
	}
	date := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)

	draft, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return GetRecoveryCardOutput{}, fmt.Errorf("get recovery card: loading week: %w", err)
	}
	if draft == nil {
		return GetRecoveryCardOutput{}, nil
	}
	idx, ok := dayIndexForDate(*draft, date)
	if !ok {
		return GetRecoveryCardOutput{}, fmt.Errorf("get recovery card: %s is not part of the loaded week", date.Format("2006-01-02"))
	}
	today := draft.Days[idx]

	var card domain.RecoveryCard
	if !today.IsRestDay {
		card, err = uc.buildTrainingDayCard(date, draft.Days, idx)
	} else {
		card, err = uc.buildRestDayCard(ctx, userID, date, draft, idx)
	}
	if err != nil {
		return GetRecoveryCardOutput{}, err
	}

	if action, aerr := uc.Actions.GetByDate(ctx, userID, date); aerr == nil && action != nil {
		k := action.Kind
		card.LoggedAction = &k
	}

	return GetRecoveryCardOutput{Found: true, Card: card}, nil
}

func (uc *GetRecoveryCardUseCase) buildTrainingDayCard(date time.Time, weekDays [7]DayPlan, todayIdx int) (domain.RecoveryCard, error) {
	copy := recovery.TrainingDayCopy
	today := weekDays[todayIdx]

	// Counts backward from today within the SAME week's draft only — a
	// streak that started last week isn't carried over here.
	restStreak := 0
	for i := todayIdx - 1; i >= 0; i-- {
		if !weekDays[i].IsRestDay {
			break
		}
		restStreak++
	}

	title := titleFor(today.Assignment.MuscleGroup, today.Assignment.ActivityType)
	context := strings.TrimSpace(fmt.Sprintf("%s · %s", restDaysPhrase(restStreak), strings.ToLower(title)))

	primary := copy.Primary
	return domain.RecoveryCard{
		Date:      date.Format("2006-01-02"),
		Weekday:   today.Weekday,
		IsRestDay: false,
		Headline:  copy.Headline,
		Context:   context,
		Primary:   &primary,
		Secondary: copy.Secondary,
		Why:       copy.Why,
	}, nil
}

func restDaysPhrase(n int) string {
	switch n {
	case 0:
		return "No rest days behind you"
	case 1:
		return "One rest day behind you"
	default:
		return fmt.Sprintf("%d rest days behind you", n)
	}
}

func (uc *GetRecoveryCardUseCase) buildRestDayCard(ctx context.Context, userID string, date time.Time, draft *WeekDraft, todayIdx int) (domain.RecoveryCard, error) {
	yesterday := date.AddDate(0, 0, -1)

	yDraft := draft
	yIdx, ok := dayIndexForDate(*draft, yesterday)
	if !ok {
		fetched, err := uc.Drafts.GetByDate(ctx, userID, yesterday)
		if err != nil {
			return domain.RecoveryCard{}, fmt.Errorf("get recovery card: loading yesterday's week: %w", err)
		}
		if fetched == nil {
			return uc.neutralRestCard(date, draft.Days[todayIdx]), nil
		}
		yDraft = fetched
		yIdx, ok = dayIndexForDate(*yDraft, yesterday)
		if !ok {
			return uc.neutralRestCard(date, draft.Days[todayIdx]), nil
		}
	}
	yDay := yDraft.Days[yIdx]

	if yDay.IsRestDay || yDay.Assignment.ActivityType == "" {
		return uc.neutralRestCard(date, draft.Days[todayIdx]), nil
	}

	cat, ok := recovery.CategorizeSession(yDay.Assignment.ActivityType, yDay.Assignment.Intensity)
	if !ok {
		return uc.neutralRestCard(date, draft.Days[todayIdx]), nil
	}
	duration := durationMinutesForDay(yDay)
	tier, ok := recovery.DeriveCostTier(recovery.SessionInputs{
		ActivityType:    yDay.Assignment.ActivityType,
		Intensity:       yDay.Assignment.Intensity,
		MuscleGroup:     yDay.Assignment.MuscleGroup,
		DurationMinutes: duration,
	})
	if !ok {
		return uc.neutralRestCard(date, draft.Days[todayIdx]), nil
	}

	// Universal modifier: sleep debt logged for TODAY (asks "how did you
	// sleep last night") bumps the tier up regardless of session type —
	// spec §3.
	if c, err := uc.Checkins.GetByDate(ctx, userID, date); err == nil && c != nil {
		if c.Answers.Sleep == checkin.SleepBarelySlept || c.Answers.Sleep == checkin.SleepRestless {
			tier = recovery.BumpTierForSleepDebt(tier)
		}
	}

	copy, ok := recovery.CopyFor(cat, tier)
	if !ok {
		return uc.neutralRestCard(date, draft.Days[todayIdx]), nil
	}

	title := strings.ToLower(titleFor(yDay.Assignment.MuscleGroup, yDay.Assignment.ActivityType))
	context := fmt.Sprintf("After %s's %s · %d min", titleCaseWords(yDay.Weekday), title, duration)
	if yDay.Assignment.MuscleGroup == activity.MuscleGroupFullBody {
		context += ", full body"
	}

	actions := []domain.RecoveryActionOption{
		{Kind: domain.RecoveryActionDone, Label: "Done"},
		{Kind: domain.RecoveryActionNotToday, Label: "Not today"},
	}
	if tier == domain.RecoveryCostHigh {
		actions = []domain.RecoveryActionOption{
			{Kind: domain.RecoveryActionAcknowledged, Label: "Got it"},
			{Kind: domain.RecoveryActionTrainedAnyway, Label: "I trained"},
		}
	}

	var whyParts []string
	whyParts = append(whyParts, copy.Why)
	if user, err := uc.Users.GetByID(ctx, userID); err == nil && user != nil {
		if user.CycleType == "hormonal_contraception" {
			whyParts = append(whyParts, "Hormonal birth control can run baseline inflammation a little higher, so this may feel more pronounced than it looks on paper.")
		}
	}

	primary := copy.Primary
	card := domain.RecoveryCard{
		Date:             date.Format("2006-01-02"),
		Weekday:          draft.Days[todayIdx].Weekday,
		IsRestDay:        true,
		CostTier:         tier,
		Headline:         copy.Headline,
		Context:          context,
		Primary:          &primary,
		Secondary:        copy.Secondary,
		Why:              strings.Join(whyParts, " "),
		AvailableActions: actions,
	}

	if tier == domain.RecoveryCostHigh {
		note, err := uc.rescheduleIfNeeded(ctx, userID, draft, todayIdx)
		if err != nil {
			// Best-effort — a failed reschedule never blocks the card itself.
			note = ""
		}
		card.RescheduleNote = note
	}

	return card, nil
}

func (uc *GetRecoveryCardUseCase) neutralRestCard(date time.Time, today DayPlan) domain.RecoveryCard {
	primary := domain.RecoveryActionItem{
		Title:  "Hydrate and eat normally",
		Detail: "No session to recover from — today's focus is keeping the conditions right for your next one.",
	}
	return domain.RecoveryCard{
		Date:      date.Format("2006-01-02"),
		Weekday:   today.Weekday,
		IsRestDay: true,
		CostTier:  domain.RecoveryCostLow,
		Headline:  "One thing today",
		Context:   "No session yesterday to recover from",
		Primary:   &primary,
		Why:       "Multiple rest days allow deeper nervous-system recovery — there's nothing specific to manage today.",
		AvailableActions: []domain.RecoveryActionOption{
			{Kind: domain.RecoveryActionDone, Label: "Done"},
			{Kind: domain.RecoveryActionNotToday, Label: "Not today"},
		},
	}
}

// rescheduleIfNeeded implements the decision table spec's high-cost
// protocol line ("avoid stacking another high-intensity session too
// soon"): if tomorrow is itself a high-intensity day, it's swapped with
// the next rest day later in the same week — both days' content is
// re-hydrated together in one ContentSelectionLayer pass, same pattern
// UserEditSlotUsecase uses for a manual edit. Only ever looks within the
// SAME already-loaded draft (a week never spans two WeekDraft documents
// here) — if tomorrow falls in a different week's draft, this is skipped
// rather than loading and mutating a second document.
//
// first-draft, pending product sign-off: the spec says to "avoid
// stacking" a next high-intensity session but doesn't specify exactly
// which day counts as "too soon" — this treats tomorrow specifically as
// too soon, not later days in the week.
func (uc *GetRecoveryCardUseCase) rescheduleIfNeeded(ctx context.Context, userID string, draft *WeekDraft, todayIdx int) (string, error) {
	tomorrowIdx := todayIdx + 1
	if tomorrowIdx > 6 {
		return "", nil // tomorrow rolls into next week's draft — out of scope for this pass
	}
	tomorrow := draft.Days[tomorrowIdx]
	if tomorrow.IsRestDay || tomorrow.Assignment.Intensity != activity.IntensityH {
		return "", nil
	}

	restIdx := -1
	for i := tomorrowIdx + 1; i <= maxRescheduleLookaheadDays; i++ {
		if draft.Days[i].IsRestDay {
			restIdx = i
			break
		}
	}
	if restIdx == -1 {
		return "", nil // no rest day later this week to swap into
	}

	movedAssignment := tomorrow.Assignment
	draft.Days[restIdx].Assignment = movedAssignment
	draft.Days[restIdx].IsRestDay = false
	draft.Days[restIdx].Warning = ""

	draft.Days[tomorrowIdx].Assignment = cascade.SlotAssignment{}
	draft.Days[tomorrowIdx].IsRestDay = true
	draft.Days[tomorrowIdx].Warning = ""

	reselected, err := uc.Content.SelectContent(ctx, *draft)
	if err != nil {
		return "", fmt.Errorf("reselecting content after reschedule: %w", err)
	}
	draft.Days = reselected.Days

	if err := uc.Drafts.UpdateDaySlot(ctx, userID, draft.ID, tomorrowIdx, draft.Days[tomorrowIdx]); err != nil {
		return "", fmt.Errorf("saving rescheduled (rest) day: %w", err)
	}
	if err := uc.Drafts.UpdateDaySlot(ctx, userID, draft.ID, restIdx, draft.Days[restIdx]); err != nil {
		return "", fmt.Errorf("saving rescheduled (moved) day: %w", err)
	}

	return fmt.Sprintf("%s's session — moved to %s", titleCaseWords(tomorrow.Weekday), titleCaseWords(draft.Days[restIdx].Weekday)), nil
}
