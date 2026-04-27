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
		return "Menstrual"
	}

	ovulationDay := duration - 14

	// Ovulation window (±1 day)
	if day >= ovulationDay-1 && day <= ovulationDay+1 {
		return "Ovulatory"
	}

	// Follicular phases
	if day < ovulationDay-1 {
		return "Follicular"
	}

	// Luteal phases
	if day <= ovulationDay+5 {
		return "Early Luteal"
	}

	return "Late Luteal"
}

// Devuelve true si modificó algo en user
func SyncUserCycleDaily(user *domain.User, now time.Time) bool {
	duration := parseIntDefault(user.CycleDuration, 28)
	day := parseIntDefault(user.CycleDay, 1)
	period := parseIntDefault(user.PeriodDuration, 5)

	today := localMidnightUTC(now)

	// Si nunca se ha “sincronizado”, solo setea CycleUpdatedAt y phase para que empiece a correr desde hoy
	if user.CycleUpdatedAt == nil {
		user.CycleUpdatedAt = &today
		user.CyclePhase = phaseForDay(day, duration, period)
		// (opcional) normaliza CycleDay/Duration a strings válidos
		user.CycleDay = strconv.Itoa(day)
		user.CycleDuration = strconv.Itoa(duration)
		return true
	}

	delta := daysBetweenUTC(*user.CycleUpdatedAt, today)
	if delta <= 0 {
		return false
	}

	newDay := ((day - 1 + delta) % duration) + 1
	user.CycleDay = strconv.Itoa(newDay)
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
	period := parseIntDefault(user.CycleDuration, 5)

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
	user.CycleDay = strconv.Itoa(newDay)
	user.CyclePhase = phaseForDay(newDay, duration, period)
	return true
}
