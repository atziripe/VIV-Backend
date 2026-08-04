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

	newDay := ((day - 1 + delta) % duration) + 1
	user.CycleDay = newDay
	user.CyclePhase = phaseForDay(newDay, duration, period)
	user.CycleUpdatedAt = &today
	return true
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

// ApplyCycleStartOverride: recalibra el ciclo cuando la usuaria reporta un CycleStart real.
func ApplyCycleStartOverride(user *domain.User, cycleStart time.Time, now time.Time) bool {
	duration := parseIntDefault(user.CycleDuration, 28)
	period := parseIntDefault(user.PeriodDuration, 5)

	today := localMidnightUTC(now)
	start := localMidnightUTC(cycleStart)

	// Si start está en el futuro, no hacemos nada (o lo clipeas a hoy)
	if start.After(today) {
		start = today
	}

	days := int(today.Sub(start).Hours() / 24)
	newDay := (days % duration) + 1

	user.CycleAnchorAt = &start
	user.CycleUpdatedAt = &today
	user.CycleDay = newDay
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
	case "early_follicular":
		// Ends at ovulation - 5
		return (ovulationDay - 5) - day + 1
	case "late_follicular":
		// Ends at ovulation - 1
		return (ovulationDay - 1) - day + 1
	case "ovulation":
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
