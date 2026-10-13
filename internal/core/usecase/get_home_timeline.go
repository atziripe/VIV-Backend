package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"viv/internal/core/checkin"
	"viv/internal/core/domain"
)

// ============================================================================
// GET HOME TIMELINE — the single read behind the Home screen redesign: a
// day's check-in, meals, session and recovery collapsed into one
// chronological timeline, plus a "now card" pointing at whichever moment
// is currently the one that matters. Pure aggregation over usecases/repos
// that already exist — no new writes, no new persisted concepts.
//
// Two real gaps in the data model, both resolved the same way (agreed
// with product before implementing):
//
//  1. Nothing here stores a literal clock time — TrainingTime is a coarse
//     bucket ("Morning"/"Afternoon"/"Evening"), MealSlot.TimingWindow is
//     either a fixed range ("7–9am") or session-relative text ("90min
//     before session"). Every *future* TimelineItem's Time is derived
//     from a small fixed lookup (see approxMealTime/sessionClockFor) and
//     flagged IsEstimated. Once something actually happens — check-in
//     submitted, a loggable session completed — its real timestamp is
//     used instead and IsEstimated is false.
//  2. There is no "meal marked done" concept (MealSelections only
//     records which option was picked, never whether it was eaten), and
//     no completion signal at all for a non-loggable (session-library)
//     training day. Rather than invent a new write path for v1, any item
//     without a real completion signal gets its Status derived purely
//     from whether its estimated time window has already passed.
// ============================================================================

// lightsOutHour/lightsOutMinute is a flat default — SleepWindow is free
// text from onboarding with no fixed format, so there's nothing reliable
// to parse it from. Same spirit as the TrainingTime bucket mapping below:
// a declared estimate, not a fact.
const (
	lightsOutHour   = 23
	lightsOutMinute = 0
)

type GetHomeTimelineInput struct {
	UserID string
	Date   time.Time // local calendar date, client-sent — same convention as every other date-scoped endpoint here
}

type TimelineItem struct {
	Time        string // "07:40" formatted local clock time, "" if it couldn't be estimated at all
	Kind        string // "checkin" | "meal" | "session" | "lights_out"
	Status      string // "done" | "pending" | "upcoming"
	Title       string
	Detail      string
	IsEstimated bool // true when Time is derived (see package doc), false when it's a real timestamp
}

// TimelineMoment is a TimelineItem promoted to the screen's single big
// "now" card, with the extra copy that card needs.
type TimelineMoment struct {
	TimelineItem
	Label     string // "NOW" | "IN 15 MIN" | "TODAY" | "TONIGHT"
	CTALabel  string
	CTAAction string // "METHOD /path" of the existing endpoint the button calls — "" when there's nothing to wire yet (see lights_out)
}

type DayDot struct {
	Weekday string
	Status  string // "done" | "pending" | "rest"
	IsToday bool
}

type WeekProgress struct {
	CompletedSessions int
	TotalSessions     int
	Days              []DayDot
}

type GetHomeTimelineOutput struct {
	// Found is false when no generated week covers Date at all.
	Found bool

	Date       string
	Weekday    string
	WeekNumber int

	CurrentPhase       string
	NextPhase          string
	DaysUntilNextPhase int

	NowCard      *TimelineMoment
	Timeline     []TimelineItem
	WeekProgress WeekProgress
}

// RecoveryCardProvider is GetRecoveryCardUseCase, declared narrowly here —
// same reasoning as DailyAdapter/WeeklyPlanGenerator elsewhere in this
// package: this usecase's own tests don't need GetRecoveryCardUseCase's
// full collaborator set (Content, in particular).
type RecoveryCardProvider interface {
	Execute(ctx context.Context, in GetRecoveryCardInput) (GetRecoveryCardOutput, error)
}

type GetHomeTimelineUseCase struct {
	Users        UserRepository
	Drafts       WeeklyPlanDraftRepository
	Checkins     DailyCheckinRepository
	SessionLogs  SessionLogRepository
	Nutrition    NutritionPlanRepository
	RecoveryCard RecoveryCardProvider
}

func NewGetHomeTimelineUseCase(
	users UserRepository,
	drafts WeeklyPlanDraftRepository,
	checkins DailyCheckinRepository,
	sessionLogs SessionLogRepository,
	nutrition NutritionPlanRepository,
	recoveryCard RecoveryCardProvider,
) *GetHomeTimelineUseCase {
	return &GetHomeTimelineUseCase{
		Users: users, Drafts: drafts, Checkins: checkins,
		SessionLogs: sessionLogs, Nutrition: nutrition, RecoveryCard: recoveryCard,
	}
}

func (uc *GetHomeTimelineUseCase) Execute(ctx context.Context, in GetHomeTimelineInput) (GetHomeTimelineOutput, error) {
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: userID is required")
	}
	date := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()

	user, err := uc.Users.GetByID(ctx, userID)
	if err != nil {
		return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: loading user: %w", err)
	}
	if user == nil {
		return GetHomeTimelineOutput{}, ErrUserNotFound(userID)
	}

	draft, err := uc.Drafts.GetByDate(ctx, userID, date)
	if err != nil {
		return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: loading week: %w", err)
	}
	if draft == nil {
		return GetHomeTimelineOutput{Found: false}, nil
	}
	idx, ok := dayIndexForDate(*draft, date)
	if !ok {
		return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: %s is not part of the loaded week", date.Format("2006-01-02"))
	}
	today := draft.Days[idx]

	dailyCheckin, err := uc.Checkins.GetByDate(ctx, userID, date)
	if err != nil {
		return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: loading checkin: %w", err)
	}

	var sessionStart, sessionEnd time.Time
	var sessionConsideredDone bool
	// loggableToday mirrors WeeklyPlanDayDetail.Loggable: only a
	// mesocycle-pinned (exercise-list) day has a real SessionLog to check.
	// A session-library day has no completion signal at all today, so —
	// same rule as meals/lights_out (gap 2) — it's considered done once
	// its estimated window has passed rather than staying "pending"
	// forever.
	loggableToday := !today.IsRestDay && today.Content != nil && today.Content.Exercises != nil
	if !today.IsRestDay {
		h, m := sessionClockFor(user.TrainingTime)
		sessionStart = time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, time.UTC)
		sessionEnd = sessionStart.Add(time.Duration(sessionDurationMinutes(today)) * time.Minute)

		if loggableToday {
			sessionLog, err := uc.SessionLogs.GetByDate(ctx, userID, date)
			if err != nil {
				return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: loading session log: %w", err)
			}
			sessionConsideredDone = sessionLog != nil && sessionLog.Status == domain.SessionLogDone
		} else {
			sessionConsideredDone = !now.Before(sessionEnd)
		}
	}

	var nutritionDay *domain.NutritionDayPlan
	if plan, err := uc.Nutrition.GetByUserID(ctx, userID); err != nil {
		return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: loading nutrition plan: %w", err)
	} else if plan != nil {
		for i := range plan.Plan.Days {
			if strings.EqualFold(plan.Plan.Days[i].Weekday, today.Weekday) {
				nutritionDay = &plan.Plan.Days[i]
				break
			}
		}
	}

	var recoveryCard domain.RecoveryCard
	if today.IsRestDay {
		out, err := uc.RecoveryCard.Execute(ctx, GetRecoveryCardInput{UserID: userID, Date: date})
		if err != nil {
			return GetHomeTimelineOutput{}, fmt.Errorf("get home timeline: loading recovery card: %w", err)
		}
		if out.Found {
			recoveryCard = out.Card
		}
	}

	lightsOut := time.Date(date.Year(), date.Month(), date.Day(), lightsOutHour, lightsOutMinute, 0, 0, time.UTC)

	// -------------------- timeline rows --------------------
	type row struct {
		sortKey time.Time
		item    TimelineItem
	}
	var rows []row

	if dailyCheckin != nil {
		rows = append(rows, row{
			sortKey: dailyCheckin.SubmittedAt,
			item: TimelineItem{
				Time:   dailyCheckin.SubmittedAt.Format("15:04"),
				Kind:   "checkin",
				Status: "done",
				Title:  "Checked in",
				Detail: checkinSummary(dailyCheckin.Answers),
			},
		})
	}

	if nutritionDay != nil {
		for _, meal := range nutritionDay.Meals {
			t, estimated := approxMealTime(meal.TimingWindow, sessionStart, sessionEnd)
			item := TimelineItem{
				Kind:        "meal",
				Title:       meal.MealName,
				Detail:      fmt.Sprintf("%.0f g protein", meal.MacroTargets.ProteinG),
				IsEstimated: true,
			}
			sortKey := sessionStart // fallback sort position for an unrecognized window — keeps the row near the session rather than at midnight
			if estimated {
				item.Time = t.Format("15:04")
				item.Status = timeOnlyStatus(now, t)
				sortKey = t
			} else {
				item.Status = "pending"
			}
			rows = append(rows, row{sortKey: sortKey, item: item})
		}
	}

	if !today.IsRestDay {
		sessionStatus := "upcoming"
		switch {
		case sessionConsideredDone:
			sessionStatus = "done"
		case loggableToday && !now.Before(sessionStart):
			sessionStatus = "pending" // overdue and actionable — not just time having passed
		case !loggableToday && !now.Before(sessionStart):
			sessionStatus = "done" // no completion signal to wait for — see loggableToday's doc comment
		}
		rows = append(rows, row{
			sortKey: sessionStart,
			item: TimelineItem{
				Time:        sessionStart.Format("15:04"),
				Kind:        "session",
				Status:      sessionStatus,
				Title:       titleFor(today.Assignment.MuscleGroup, today.Assignment.ActivityType),
				Detail:      fmt.Sprintf("%d min", sessionDurationMinutes(today)),
				IsEstimated: true,
			},
		})
	}

	rows = append(rows, row{
		sortKey: lightsOut,
		item: TimelineItem{
			Time:        lightsOut.Format("15:04"),
			Kind:        "lights_out",
			Status:      timeOnlyStatus(now, lightsOut),
			Title:       "Lights out",
			IsEstimated: true,
		},
	})

	sortRows(rows, func(a, b row) bool { return a.sortKey.Before(b.sortKey) })

	timeline := make([]TimelineItem, 0, len(rows))
	for _, r := range rows {
		timeline = append(timeline, r.item)
	}

	// -------------------- now card --------------------
	var nowCard *TimelineMoment
	switch {
	case dailyCheckin == nil:
		nowCard = &TimelineMoment{
			TimelineItem: TimelineItem{Kind: "checkin", Status: "pending", Title: "Check in"},
			Label:        "NOW",
			CTALabel:     "Check in",
			CTAAction:    "POST /checkin",
		}
	case !today.IsRestDay && !sessionConsideredDone:
		nowCard = &TimelineMoment{
			TimelineItem: TimelineItem{
				Time: sessionStart.Format("15:04"), Kind: "session", Status: "pending",
				Title:  titleFor(today.Assignment.MuscleGroup, today.Assignment.ActivityType),
				Detail: fmt.Sprintf("%d min", sessionDurationMinutes(today)), IsEstimated: true,
			},
			Label:     proximityLabel(now, sessionStart, "TODAY"),
			CTALabel:  "Start session",
			CTAAction: "POST /training/weekly-plan/day/start",
		}
	case today.IsRestDay && recoveryCard.LoggedAction == nil && recoveryCard.Primary != nil:
		ctaLabel := "Done"
		if len(recoveryCard.AvailableActions) > 0 {
			ctaLabel = recoveryCard.AvailableActions[0].Label
		}
		nowCard = &TimelineMoment{
			TimelineItem: TimelineItem{Kind: "recovery_action", Status: "pending", Title: recoveryCard.Primary.Title, Detail: recoveryCard.Primary.Detail},
			Label:        "TODAY",
			CTALabel:     ctaLabel,
			CTAAction:    "POST /recovery/card/action",
		}
	case now.Before(lightsOut):
		nowCard = &TimelineMoment{
			TimelineItem: TimelineItem{Time: lightsOut.Format("15:04"), Kind: "lights_out", Status: "pending", Title: "Lights out", IsEstimated: true},
			Label:        "TONIGHT",
			CTALabel:     "Remind me",
			// Scheduling an actual reminder isn't wired to anything yet —
			// no endpoint exists to schedule a one-off local/push
			// reminder. Left blank on purpose rather than pointing at a
			// route that doesn't do that.
			CTAAction: "",
		}
	default:
		nowCard = nil // everything today is already behind us — not covered by the mockups, left as "no card" until product says otherwise
	}

	phase := mapCyclePhase(user.CyclePhase)

	return GetHomeTimelineOutput{
		Found:      true,
		Date:       date.Format("2006-01-02"),
		Weekday:    today.Weekday,
		WeekNumber: weekNumber(draft.StartDate),

		CurrentPhase:       string(phase),
		NextPhase:          NextPhaseName(phase),
		DaysUntilNextPhase: DaysUntilNextPhase(user),

		NowCard:      nowCard,
		Timeline:     timeline,
		WeekProgress: weekProgress(ctx, uc.SessionLogs, userID, *draft, date),
	}, nil
}

// -------------------- helpers --------------------

// sessionClockFor maps the coarse TrainingTime onboarding bucket to a
// representative clock hour — see package doc, gap 1.
func sessionClockFor(trainingTime string) (hour, minute int) {
	switch trainingTime {
	case "Morning":
		return 8, 0
	case "Afternoon":
		return 14, 0
	default: // "Evening", "", or anything unrecognized
		return 18, 0
	}
}

// sessionDurationMinutes mirrors GetWeeklyPlanDayUseCase's duration
// derivation for the two possible Content shapes.
func sessionDurationMinutes(day DayPlan) int {
	if day.Content == nil {
		return 0
	}
	if day.Content.Session != nil {
		return day.Content.Session.DurationMinutes
	}
	if day.Content.Exercises != nil {
		total := 0
		for _, pe := range day.Content.Exercises {
			total += pe.Prescription.Sets * (pe.Prescription.RestSeconds + assumedSecondsPerSet)
		}
		return (total + 30) / 60
	}
	return 0
}

// approxMealTime maps the small, fixed vocabulary of TimingWindow strings
// this codebase actually produces (see internal/core/training/meal_structure.go)
// to a representative clock time. Deliberately a lookup over the known
// literal set rather than a parser over free text — if content authors add
// a new window string later, this degrades to "no estimate" rather than
// guessing. sessionStart/sessionEnd are the zero time.Time when the day has
// no session (rest day); the session-relative branches are simply skipped then.
func approxMealTime(window string, sessionStart, sessionEnd time.Time) (time.Time, bool) {
	hasSession := !sessionStart.IsZero()
	base := sessionStart
	switch window {
	case "6–7am":
		return atClock(base, 6, 0), true
	case "7–9am":
		return atClock(base, 7, 0), true
	case "12–2pm":
		return atClock(base, 12, 0), true
	case "4–4:30pm":
		return atClock(base, 16, 0), true
	case "6–8pm":
		return atClock(base, 18, 0), true
	case "90min before session":
		if !hasSession {
			return time.Time{}, false
		}
		return sessionStart.Add(-90 * time.Minute), true
	case "within 1hr after session":
		if !hasSession {
			return time.Time{}, false
		}
		return sessionEnd.Add(15 * time.Minute), true
	case "post-session":
		if !hasSession {
			return time.Time{}, false
		}
		return sessionEnd.Add(30 * time.Minute), true
	default:
		return time.Time{}, false
	}
}

func atClock(base time.Time, hour, minute int) time.Time {
	if base.IsZero() {
		base = time.Now().UTC()
	}
	return time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, time.UTC)
}

// timeOnlyStatus is gap 2's rule for anything with no real completion
// signal (meals, lights_out, a non-loggable session-library day): "done"
// once its estimated time has passed, "upcoming" while it's still ahead.
// There's no "pending" here — nothing to wait on, only the clock.
func timeOnlyStatus(now, estimated time.Time) string {
	if now.Before(estimated) {
		return "upcoming"
	}
	return "done"
}

// proximityLabel is the now-card's countdown copy.
func proximityLabel(now, target time.Time, farLabel string) string {
	diff := target.Sub(now)
	switch {
	case diff <= 0:
		return "NOW"
	case diff <= 30*time.Minute:
		return fmt.Sprintf("IN %d MIN", int(diff.Minutes()))
	default:
		return farLabel
	}
}

// checkinSummary humanizes the four raw snake_case answers into the short
// "Normal · Strong & responsive · ..." line the Home screen shows — a
// generic transform (not a per-value lookup) so it keeps working if the
// checkin package's answer vocabulary grows.
func checkinSummary(a checkin.DailyCheckin) string {
	parts := []string{
		humanizeAnswer(string(a.Sleep)),
		humanizeAnswer(string(a.Body)),
		humanizeAnswer(string(a.Demand)),
		humanizeAnswer(string(a.Need)),
	}
	return strings.Join(parts, " · ")
}

func humanizeAnswer(s string) string {
	s = strings.ReplaceAll(s, "_and_", " & ")
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// weekNumber is a simple ISO week number of the draft's start date — good
// enough for the Home screen's "Week 6" label.
func weekNumber(start time.Time) int {
	_, wk := start.ISOWeek()
	return wk
}

// weekProgress tallies this week's training days against how many have a
// completed session log. SessionLogRepository has no range query today,
// so this is up to 7 GetByDate calls — fine for a once-per-screen-open
// read; worth a dedicated range method if this becomes a hot path.
//
// A non-loggable (session-library) day never gets a SessionLog (see
// loggableToday's doc comment in Execute) — applying gap 2's rule here
// too keeps a past session-library day showing "done" instead of stuck
// on "pending" forever, consistent with how the Timeline treats it.
func weekProgress(ctx context.Context, logs SessionLogRepository, userID string, draft WeekDraft, now time.Time) WeekProgress {
	wp := WeekProgress{Days: make([]DayDot, 0, 7)}
	for _, d := range draft.Days {
		dot := DayDot{Weekday: d.Weekday, IsToday: sameDate(d.Date, now)}
		if d.IsRestDay {
			dot.Status = "rest"
			wp.Days = append(wp.Days, dot)
			continue
		}
		wp.TotalSessions++

		done := false
		if d.Content != nil && d.Content.Exercises != nil {
			log, err := logs.GetByDate(ctx, userID, d.Date)
			done = err == nil && log != nil && log.Status == domain.SessionLogDone
		} else {
			done = d.Date.Before(now) && !sameDate(d.Date, now)
		}

		if done {
			dot.Status = "done"
			wp.CompletedSessions++
		} else {
			dot.Status = "pending"
		}
		wp.Days = append(wp.Days, dot)
	}
	return wp
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func sortRows[T any](items []T, less func(a, b T) bool) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && less(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
