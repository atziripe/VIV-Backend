package usecase

import (
	"strconv"
	"time"

	"viv/internal/core/domain"
)

func parseIntDefault(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func localMidnightUTC(t time.Time) time.Time {
	tt := t.UTC()
	return time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, time.UTC)
}

func daysBetweenUTC(from, to time.Time) int {
	f := localMidnightUTC(from)
	t := localMidnightUTC(to)
	return int(t.Sub(f).Hours() / 24)
}

func phaseForDay(day, duration, periodLen int) string {
	// Guardrails
	if day <= 0 || duration <= 0 {
		return ""
	}

	// Default fisiológico razonable
	if periodLen <= 0 {
		periodLen = 5
	}

	// Menstrual phase (based on real bleeding length)
	if day <= periodLen {
		return "menstrual"
	}

	ovulationDay := duration - 14

	// Ovulation window (±1 day)
	if day >= ovulationDay-1 && day <= ovulationDay+1 {
		return "ovulatory"
	}

	// Follicular phases
	if day < ovulationDay-1 {
		return "follicular"
	}

	// Luteal phases
	if day <= ovulationDay+5 {
		return "early_luteal"
	}

	return "late_luteal"
}

// Devuelve true si modificó algo en user
func SyncUserCycleDaily(user *domain.User, now time.Time) bool {
	duration := parseIntDefault(user.CycleDuration, 28)
	day := user.CycleDay
	period := parseIntDefault(user.PeriodDuration, 5)

	today := localMidnightUTC(now)

	// Si nunca se ha “sincronizado”, solo setea CycleUpdatedAt y phase para que empiece a correr desde hoy
	if user.CycleUpdatedAt == nil {
		user.CycleUpdatedAt = &today
		user.CyclePhase = phaseForDay(day, duration, period)
		// (opcional) normaliza CycleDay/Duration a strings válidos
		user.CycleDay = day
		user.CycleDuration = strconv.Itoa(duration)
		return true
	}

	delta := daysBetweenUTC(*user.CycleUpdatedAt, today)
	if delta <= 0 {
		return false
	}

	// Advance the day counter, but never let it wrap past the last day of
	// the assumed cycle on its own — only a real report (ApplyCycleStartOverride,
	// i.e. an actual logged period) may reset it back to day 1. Without this
	// clamp, a period that never arrives is invisible: the counter would
	// silently wrap into a "new" cycle exactly on schedule, and every phase
	// downstream (training/nutrition) would act as if the period had
	// happened. Clamping here instead freezes the user in late_luteal —
	// this IS the "holding" state the late-period flow needs, and it falls
	// out of this one change rather than a separate flag.
	rawDay := day - 1 + delta
	if rawDay >= duration {
		rawDay = duration - 1
	}
	newDay := rawDay + 1

	user.CycleDay = newDay
	user.CyclePhase = phaseForDay(newDay, duration, period)
	user.CycleUpdatedAt = &today
	return true
}

// ExpectedPeriodDate predicts when the next period should start, from the
// last reported anchor plus the currently-assumed cycle duration. Returns
// the zero time if there's nothing to predict from — no anchor has ever
// been recorded, or the user has explicitly opted out of prediction via
// CycleEstimationDisabled (the "it varies — stop estimating" nudge).
func ExpectedPeriodDate(user *domain.User) time.Time {
	if user.CycleAnchorAt == nil || user.CycleEstimationDisabled {
		return time.Time{}
	}
	duration := parseIntDefault(user.CycleDuration, 28)
	return localMidnightUTC(*user.CycleAnchorAt).AddDate(0, 0, duration)
}

// DaysLate returns how many days now is past the expected period date —
// positive means late, negative means early (reporting now would be
// ahead of schedule), zero means exactly on time. Returns 0 when there's
// no anchor to predict from (ExpectedPeriodDate is zero) — callers should
// treat that as "nothing to say," not "on time."
func DaysLate(user *domain.User, now time.Time) int {
	expected := ExpectedPeriodDate(user)
	if expected.IsZero() {
		return 0
	}
	return daysBetweenUTC(expected, now)
}

func normalizeCycleDuration(input string) string {
	switch input {
	case "21-25 days":
		return "23"
	case "26-30 days":
		return "28"
	case "31-35 days":
		return "33"
	case "Irregular":
		return "35"
	default:
		return input
	}
}

// minLearnedCycleDuration/maxLearnedCycleDuration bound what
// ApplyCycleStartOverride will accept as a recalibrated cycle length from
// an observed interval between two reported periods. A single mis-tap (or
// a genuinely erratic outlier) shouldn't be allowed to skew every future
// phase prediction — outside this range, the prior duration is kept
// as-is instead.
const (
	minLearnedCycleDuration = 15
	maxLearnedCycleDuration = 60
)

// ApplyCycleStartOverride: recalibra el ciclo cuando la usuaria reporta un CycleStart real.
//
// Also recalibrates the LEARNED CycleDuration itself: when there's a prior
// anchor to measure against, the real interval since that anchor replaces
// the stored duration (within the sane range above) — so a period that
// consistently runs long/short updates VIV's assumption going forward,
// instead of the duration staying frozen at whatever bucket was picked at
// onboarding forever. The very first report (no prior anchor) has nothing
// to measure an interval against, so it leaves the duration untouched.
func ApplyCycleStartOverride(user *domain.User, cycleStart time.Time, now time.Time) bool {
	duration := parseIntDefault(user.CycleDuration, 28)
	period := parseIntDefault(user.PeriodDuration, 5)

	today := localMidnightUTC(now)
	start := localMidnightUTC(cycleStart)

	// Si start está en el futuro, no hacemos nada (o lo clipeas a hoy)
	if start.After(today) {
		start = today
	}

	if user.CycleAnchorAt != nil {
		observed := daysBetweenUTC(*user.CycleAnchorAt, start)
		if observed >= minLearnedCycleDuration && observed <= maxLearnedCycleDuration {
			duration = observed
		}
	}

	days := int(today.Sub(start).Hours() / 24)
	newDay := (days % duration) + 1

	user.CycleAnchorAt = &start
	user.CycleUpdatedAt = &today
	user.CycleDay = newDay
	user.CycleDuration = strconv.Itoa(duration)
	user.CyclePhase = phaseForDay(newDay, duration, period)
	return true
}

// DaysUntilNextPhase calculates how many days until the user transitions
// to the next cycle phase, based on current cycle day and duration.
func DaysUntilNextPhase(user *domain.User) int {
	day := user.CycleDay
	duration := parseIntDefault(user.CycleDuration, 28)
	period := parseIntDefault(user.PeriodDuration, 5)

	ovulationDay := duration - 14
	currentPhase := phaseForDay(day, duration, period)

	switch currentPhase {
	case "menstrual":
		// Ends when period ends
		return period - day + 1
	case "follicular":
		// Ends the day before the ovulation window starts (ovulationDay-1)
		return (ovulationDay - 2) - day + 1
	case "ovulatory":
		// 3 day window: ovulation-1 to ovulation+1
		return (ovulationDay + 1) - day + 1
	case "early_luteal":
		// Ends at ovulation + 5
		return (ovulationDay + 5) - day + 1
	case "late_luteal":
		// Ends at cycle end (day = duration)
		return duration - day + 1
	default:
		return 1
	}
}

// NextPhaseName returns what phase comes after the current one.
func NextPhaseName(currentPhase domain.CyclePhase) string {
	switch currentPhase {
	case domain.PhaseMenstrual:
		return "follicular"
	case domain.PhaseFollicular:
		return "ovulatory"
	case domain.PhaseOvulatory:
		return "early_luteal"
	case domain.PhaseEarlyLuteal:
		return "late_luteal"
	case domain.PhaseLateLuteal:
		return "menstrual"
	default:
		return "follicular"
	}
}
