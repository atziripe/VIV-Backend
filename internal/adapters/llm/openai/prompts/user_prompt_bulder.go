package prompts

import (
	"fmt"
	"strings"
	"time"

	"viv/internal/core/domain"
)

func BuildUserPromptVIVV1(user *domain.User, checkin *domain.Checkin) string {

	np := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return "not provided"
		}
		return s
	}

	// Optional fields as strings
	dietRestr := np(user.DietRestrictions)

	// ---- Weekly Check-in block (supports nil) ----
	weeklyCheckinBlock := buildWeeklyCheckinBlock(checkin, np)

	return fmt.Sprintf(`
		USER DATA

		User:
		- age: %d
		- height_cm: %.0f
		- weight_kg: %.0f
		- onboarding_completed: %t

		Cycle:
		- cycle_type: %s
		- cycle_day: %s
		- cycle_duration: %s
		- cycle_phase: %s
		- menstrual_period_duration: %s days

		Training:
		- training_often_per_week: %s
		- training_duration_preference: %s
		- training_type_preference: %s
		- training_time_preference: %s
		- has_active_injury: %t
		- training_paused: %t
		- training_goals: %s


		Nutrition:
		- diet_restrictions: %s
		- diet_protein_resources: %s
		- meals_per_day: %s
		- meals_timing_stability: %s
		- digestion_conditions: %s


		Lifestyle:
		- sleep_window: %s
		- recovery_after_sleep: %s
		- sleep_interruption: %s
		- lingerin_fatigue: %s
		- stress_reactivity: %s
		- baseline_stress_level: %s
		- priority: %s


		Remember: return ONLY JSON matching the schema. Include all fields for exactly 7 days.
		`,
		CalculateAgeFromDOB(user.DOB),
		user.HeightCm,
		user.WeightKg,
		user.OnboardingCompleted,

		np(user.CycleType),
		np(fmt.Sprintf("%v", user.CycleDay)),      // por si CycleDay es int o string
		np(fmt.Sprintf("%v", user.CycleDuration)), // idem
		np(user.CyclePhase),
		np(user.PeriodDuration),

		np(fmt.Sprintf("%v", user.TrainingOften)),
		np(user.TrainingDuration),
		np(user.TrainingType),
		np(user.TrainingTime),
		user.HasActiveInjury,
		user.TrainingPaused,
		np(user.TrainingGoals),

		dietRestr,
		np(user.DietProteinResources),
		np(fmt.Sprintf("%v", user.MealsPerDay)),
		np(fmt.Sprintf("%v", user.MealsTimingStability)),
		np(user.DigestionConditions),

		np(user.SleepWindow),
		np(user.RecoveryAfterSleep),
		np(user.SleepContinuity),
		np(user.LingeringMarker),
		np(user.StressReactivity),
		np(user.StressLevel),
		np(user.Priority),

		weeklyCheckinBlock,
	)
}

func buildWeeklyCheckinBlock(checkin *domain.Checkin, np func(string) string) string {
	// Starter plan: no weekly check-in yet
	if checkin == nil {
		// Para que el modelo no “invente”, dejamos claro que NO hay check-in.
		// También le damos defaults conservadores.
		return strings.TrimSpace(`
			Weekly Check-in:
			- available: false
			- note: First plan after onboarding; no weekly check-in data yet.
			- instruction: Generate a conservative starter plan using onboarding data only.
			- assumptions: sleep_quality=average, stress_level=moderate, appetite=stable, mental_energy=moderate.
			`)
	}

	// With check-in
	cycleStart := "not provided"
	if checkin.CycleStart != nil {
		cycleStart = checkin.CycleStart.Format("2006-01-02")
	}
	weekStart := checkin.WeekStart.Format("2006-01-02")

	return fmt.Sprintf(strings.TrimSpace(`
		Weekly Check-in:
		- available: true
		- week_start: %s
		- sleep_quality: %s
		- body_status: %s
		- appetite: %s
		- stress_level: %s
		- workload_prediction: %s
		- mental_energy (1-5): %d
		- last_week_feeling: %s
		- cycle_start (if known): %s
		`),
		weekStart,
		np(checkin.SleepQuality),
		np(checkin.BodyStatus),
		np(checkin.StressLevel),
		np(checkin.WorkloadPrediction),
		checkin.MentalEnergy,
		np(checkin.LastWeekFeeling),
		cycleStart,
	)
}

// Helper útil si un día quieres construir week_start afuera (no obligatorio)
func ParseISODate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func CalculateAgeFromDOB(dob string) int {
	birthDate, err := time.Parse("2006-01-02", dob)
	if err != nil {
		return 0
	}

	now := time.Now().UTC()

	age := now.Year() - birthDate.Year()

	// Ajustar si aún no ha pasado el cumpleaños este año
	if now.Month() < birthDate.Month() ||
		(now.Month() == birthDate.Month() && now.Day() < birthDate.Day()) {
		age--
	}

	return age
}
