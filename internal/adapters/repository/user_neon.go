package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.UserRepository = (*NeonUserRepository)(nil)

type NeonUserRepository struct {
	pool *pgxpool.Pool
}

func NewNeonUserRepository(pool *pgxpool.Pool) *NeonUserRepository {
	return &NeonUserRepository{pool: pool}
}

func (r *NeonUserRepository) GetByID(ctx context.Context, firebaseUID string) (*domain.User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			u.firebase_uid, u.name, u.created_at, u.updated_at,
			p.dob, p.height_cm, p.weight_kg,
			p.daily_activity_level, p.training_often, p.training_duration,
			p.training_time, p.training_type, p.training_goals, p.priority,
			p.has_active_injury, p.training_paused, p.last_injury_reported_at,
			p.diet_restrictions, p.diet_protein_resources, p.digestion_conditions,
			p.eating_style, p.meals_per_day, p.meals_timing_stability,
			p.sleep_window, p.sleep_continuity, p.recovery_after_sleep,
			p.lingering_marker, p.stress_level, p.stress_reactivity,
			p.onboarding_completed,
			c.cycle_day, c.cycle_duration, c.cycle_phase, c.cycle_type,
			c.cycle_anchor_at, c.period_duration
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id = u.id
		LEFT JOIN user_cycles   c ON c.user_id = u.id
		WHERE u.firebase_uid = $1
	`, firebaseUID)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *NeonUserRepository) Save(ctx context.Context, user *domain.User) error {
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	user.UpdatedAt = time.Now().UTC()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Upsert identity — domain.User.ID is the Firebase UID
	var internalID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (firebase_uid, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (firebase_uid) DO UPDATE
		  SET name       = EXCLUDED.name,
		      updated_at = EXCLUDED.updated_at
		RETURNING id
	`, user.ID, user.Name, user.CreatedAt, user.UpdatedAt).Scan(&internalID)
	if err != nil {
		return err
	}

	// 2. Upsert profile
	_, err = tx.Exec(ctx, `
		INSERT INTO user_profiles (
			user_id,
			dob, height_cm, weight_kg,
			daily_activity_level, training_often, training_duration,
			training_time, training_type, training_goals, priority,
			has_active_injury, training_paused, last_injury_reported_at,
			diet_restrictions, diet_protein_resources, digestion_conditions,
			eating_style, meals_per_day, meals_timing_stability,
			sleep_window, sleep_continuity, recovery_after_sleep,
			lingering_marker, stress_level, stress_reactivity,
			onboarding_completed, updated_at
		) VALUES (
			$1,
			$2, $3, $4,
			$5, $6, $7,
			$8, $9, $10, $11,
			$12, $13, $14,
			$15, $16, $17,
			$18, $19, $20,
			$21, $22, $23,
			$24, $25, $26,
			$27, NOW()
		)
		ON CONFLICT (user_id) DO UPDATE SET
			dob                     = EXCLUDED.dob,
			height_cm               = EXCLUDED.height_cm,
			weight_kg               = EXCLUDED.weight_kg,
			daily_activity_level    = EXCLUDED.daily_activity_level,
			training_often          = EXCLUDED.training_often,
			training_duration       = EXCLUDED.training_duration,
			training_time           = EXCLUDED.training_time,
			training_type           = EXCLUDED.training_type,
			training_goals          = EXCLUDED.training_goals,
			priority                = EXCLUDED.priority,
			has_active_injury       = EXCLUDED.has_active_injury,
			training_paused         = EXCLUDED.training_paused,
			last_injury_reported_at = EXCLUDED.last_injury_reported_at,
			diet_restrictions       = EXCLUDED.diet_restrictions,
			diet_protein_resources  = EXCLUDED.diet_protein_resources,
			digestion_conditions    = EXCLUDED.digestion_conditions,
			eating_style            = EXCLUDED.eating_style,
			meals_per_day           = EXCLUDED.meals_per_day,
			meals_timing_stability  = EXCLUDED.meals_timing_stability,
			sleep_window            = EXCLUDED.sleep_window,
			sleep_continuity        = EXCLUDED.sleep_continuity,
			recovery_after_sleep    = EXCLUDED.recovery_after_sleep,
			lingering_marker        = EXCLUDED.lingering_marker,
			stress_level            = EXCLUDED.stress_level,
			stress_reactivity       = EXCLUDED.stress_reactivity,
			onboarding_completed    = EXCLUDED.onboarding_completed,
			updated_at              = NOW()
	`,
		internalID,
		nullableString(user.DOB), user.HeightCm, user.WeightKg,
		user.DailyActivityLevel, user.TrainingOften, user.TrainingDuration,
		user.TrainingTime, user.TrainingType, user.TrainingGoals, user.Priority,
		user.HasActiveInjury, user.TrainingPaused, user.LastInjuryReportedAt,
		user.DietRestrictions, user.DietProteinResources, user.DigestionConditions,
		user.EatingStyle, user.MealsPerDay, user.MealsTimingStability,
		user.SleepWindow, user.SleepContinuity, user.RecoveryAfterSleep,
		user.LingeringMarker, user.StressLevel, user.StressReactivity,
		user.OnboardingCompleted,
	)
	if err != nil {
		return err
	}

	// 3. Upsert cycle — CycleDuration is string in domain.User
	_, err = tx.Exec(ctx, `
		INSERT INTO user_cycles (
			user_id, cycle_day, cycle_duration, cycle_phase,
			cycle_type, cycle_anchor_at, period_duration, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			cycle_day       = EXCLUDED.cycle_day,
			cycle_duration  = EXCLUDED.cycle_duration,
			cycle_phase     = EXCLUDED.cycle_phase,
			cycle_type      = EXCLUDED.cycle_type,
			cycle_anchor_at = EXCLUDED.cycle_anchor_at,
			period_duration = EXCLUDED.period_duration,
			updated_at      = NOW()
	`,
		internalID,
		user.CycleDay, user.CycleDuration, user.CyclePhase,
		user.CycleType, user.CycleAnchorAt, user.PeriodDuration,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// -------------------- scanner --------------------

func scanUser(row pgx.Row) (*domain.User, error) {
	var (
		u domain.User

		dob        *string
		heightCm   *float64
		weightKg   *float64

		dailyActivityLevel *string
		trainingOften      *string
		trainingDuration   *string
		trainingTime       *string
		trainingType       *string
		trainingGoals      *string
		priority           *string
		hasActiveInjury    *bool
		trainingPaused     *bool
		lastInjury         *time.Time
		dietRestrictions   *string
		dietProtein        *string
		digestion          *string
		eatingStyle        *string
		mealsPerDay        *string
		mealsTiming        *string
		sleepWindow        *string
		sleepContinuity    *string
		recoveryAfterSleep *string
		lingeringMarker    *string
		stressLevel        *string
		stressReactivity   *string
		onboarding         *bool

		cycleDay       *int
		cycleDuration  *string // string in domain.User
		cyclePhase     *string
		cycleType      *string
		cycleAnchorAt  *time.Time
		periodDuration *string
	)

	err := row.Scan(
		&u.ID, &u.Name, &u.CreatedAt, &u.UpdatedAt,
		&dob, &heightCm, &weightKg,
		&dailyActivityLevel, &trainingOften, &trainingDuration,
		&trainingTime, &trainingType, &trainingGoals, &priority,
		&hasActiveInjury, &trainingPaused, &lastInjury,
		&dietRestrictions, &dietProtein, &digestion,
		&eatingStyle, &mealsPerDay, &mealsTiming,
		&sleepWindow, &sleepContinuity, &recoveryAfterSleep,
		&lingeringMarker, &stressLevel, &stressReactivity,
		&onboarding,
		&cycleDay, &cycleDuration, &cyclePhase, &cycleType,
		&cycleAnchorAt, &periodDuration,
	)
	if err != nil {
		return nil, err
	}

	derefStr := func(s *string) string {
		if s != nil {
			return *s
		}
		return ""
	}
	derefBool := func(b *bool) bool {
		if b != nil {
			return *b
		}
		return false
	}
	derefInt := func(i *int) int {
		if i != nil {
			return *i
		}
		return 0
	}
	derefF64 := func(f *float64) float64 {
		if f != nil {
			return *f
		}
		return 0
	}

	u.DOB = derefStr(dob)
	u.HeightCm = derefF64(heightCm)
	u.WeightKg = derefF64(weightKg)
	u.DailyActivityLevel = derefStr(dailyActivityLevel)
	u.TrainingOften = derefStr(trainingOften)
	u.TrainingDuration = derefStr(trainingDuration)
	u.TrainingTime = derefStr(trainingTime)
	u.TrainingType = derefStr(trainingType)
	u.TrainingGoals = derefStr(trainingGoals)
	u.Priority = derefStr(priority)
	u.HasActiveInjury = derefBool(hasActiveInjury)
	u.TrainingPaused = derefBool(trainingPaused)
	u.LastInjuryReportedAt = lastInjury
	u.DietRestrictions = derefStr(dietRestrictions)
	u.DietProteinResources = derefStr(dietProtein)
	u.DigestionConditions = derefStr(digestion)
	u.EatingStyle = derefStr(eatingStyle)
	u.MealsPerDay = derefStr(mealsPerDay)
	u.MealsTimingStability = derefStr(mealsTiming)
	u.SleepWindow = derefStr(sleepWindow)
	u.SleepContinuity = derefStr(sleepContinuity)
	u.RecoveryAfterSleep = derefStr(recoveryAfterSleep)
	u.LingeringMarker = derefStr(lingeringMarker)
	u.StressLevel = derefStr(stressLevel)
	u.StressReactivity = derefStr(stressReactivity)
	u.OnboardingCompleted = derefBool(onboarding)
	u.CycleDay = derefInt(cycleDay)
	u.CycleDuration = derefStr(cycleDuration)
	u.CyclePhase = derefStr(cyclePhase)
	u.CycleType = derefStr(cycleType)
	u.CycleAnchorAt = cycleAnchorAt
	u.PeriodDuration = derefStr(periodDuration)

	return &u, nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
