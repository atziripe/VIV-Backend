package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.PlanRepository = (*NeonPlanRepository)(nil)

type NeonPlanRepository struct {
	pool *pgxpool.Pool
}

func NewNeonPlanRepository(pool *pgxpool.Pool) *NeonPlanRepository {
	return &NeonPlanRepository{pool: pool}
}

func (r *NeonPlanRepository) Create(ctx context.Context, p *domain.Plan) error {
	if p == nil {
		return nil
	}

	var userUUID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, p.UserID,
	).Scan(&userUUID)
	if err != nil {
		return err
	}

	var checkinUUID *string
	if strings.TrimSpace(p.CheckinID) != "" {
		row := r.pool.QueryRow(ctx,
			`SELECT id FROM checkins WHERE firestore_id = $1`, p.CheckinID,
		)
		var cid string
		if err := row.Scan(&cid); err == nil {
			checkinUUID = &cid
		}
	}

	var newID string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO plans (
			user_id, checkin_id, firestore_id,
			training_json, nutrition_json, recovery_json,
			cycle_day_range, plan_version, status,
			generated_from, source_event_id,
			created_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9,
			$10, $11,
			$12
		)
		ON CONFLICT (firestore_id) DO NOTHING
		RETURNING id
	`,
		userUUID, checkinUUID, nullableString(p.ID),
		jsonbOrNull(p.TrainingJSON), jsonbOrNull(p.NutritionJSON), jsonbOrNull(p.RecoveryJSON),
		p.CycleDayRange, p.PlanVersion, p.Status,
		p.GeneratedFrom, p.SourceEventID,
		p.CreatedAt,
	).Scan(&newID)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	return nil
}

func (r *NeonPlanRepository) GetByID(ctx context.Context, userID, planID string) (*domain.Plan, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			pl.firestore_id, u.firebase_uid,
			pl.training_json, pl.nutrition_json, pl.recovery_json,
			pl.cycle_day_range, pl.plan_version, pl.status,
			pl.generated_from, pl.source_event_id,
			pl.created_at,
			ch.firestore_id as checkin_firestore_id
		FROM plans pl
		JOIN users u ON u.id = pl.user_id
		LEFT JOIN checkins ch ON ch.id = pl.checkin_id
		WHERE u.firebase_uid = $1 AND pl.firestore_id = $2
	`, userID, planID)

	return scanPlan(row)
}

func (r *NeonPlanRepository) GetLatestByWeekStart(ctx context.Context, userID string, weekStart time.Time) (*domain.Plan, error) {
	start := time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, time.UTC)

	row := r.pool.QueryRow(ctx, `
		SELECT
			pl.firestore_id, u.firebase_uid,
			pl.training_json, pl.nutrition_json, pl.recovery_json,
			pl.cycle_day_range, pl.plan_version, pl.status,
			pl.generated_from, pl.source_event_id,
			pl.created_at,
			ch.firestore_id as checkin_firestore_id
		FROM plans pl
		JOIN users u ON u.id = pl.user_id
		LEFT JOIN checkins ch ON ch.id = pl.checkin_id
		WHERE u.firebase_uid = $1
		  AND DATE(pl.created_at AT TIME ZONE 'UTC') = $2::date
		ORDER BY pl.created_at DESC
		LIMIT 1
	`, userID, start)

	p, err := scanPlan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *NeonPlanRepository) GetLatest(ctx context.Context, userID string) (*domain.Plan, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			pl.firestore_id, u.firebase_uid,
			pl.training_json, pl.nutrition_json, pl.recovery_json,
			pl.cycle_day_range, pl.plan_version, pl.status,
			pl.generated_from, pl.source_event_id,
			pl.created_at,
			ch.firestore_id as checkin_firestore_id
		FROM plans pl
		JOIN users u ON u.id = pl.user_id
		LEFT JOIN checkins ch ON ch.id = pl.checkin_id
		WHERE u.firebase_uid = $1
		ORDER BY pl.created_at DESC
		LIMIT 1
	`, userID)

	p, err := scanPlan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

func (r *NeonPlanRepository) UpdateTrainingCompleted(ctx context.Context, userID, planID string, completed map[string]bool) error {
	data, err := json.Marshal(completed)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE plans SET training_json = training_json || jsonb_build_object('training_completed', $1::jsonb)
		FROM users
		WHERE plans.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plans.firestore_id = $3
	`, string(data), userID, planID)
	return err
}

func (r *NeonPlanRepository) UpdatedTrainingJSON(ctx context.Context, userID, planID string, trainingJSON []byte) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE plans SET training_json = $1
		FROM users
		WHERE plans.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plans.firestore_id = $3
	`, trainingJSON, userID, planID)
	return err
}

func (r *NeonPlanRepository) UpdateNutritionJSON(ctx context.Context, userID, planID string, nutritionJSON []byte) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE plans SET nutrition_json = $1
		FROM users
		WHERE plans.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plans.firestore_id = $3
	`, nutritionJSON, userID, planID)
	return err
}

func (r *NeonPlanRepository) UpdatePhaseFeedback(ctx context.Context, userID, planID string, feedback map[string]domain.PhaseFeedbackEntry) error {
	data, err := json.Marshal(feedback)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE plans SET training_json = training_json || jsonb_build_object('phase_feedback', $1::jsonb)
		FROM users
		WHERE plans.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plans.firestore_id = $3
	`, string(data), userID, planID)
	return err
}

func (r *NeonPlanRepository) UpdateMealSelections(ctx context.Context, userID, planID string, selections map[string]map[string]int) error {
	data, err := json.Marshal(selections)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE plans SET nutrition_json = nutrition_json || jsonb_build_object('meal_selections', $1::jsonb)
		FROM users
		WHERE plans.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plans.firestore_id = $3
	`, string(data), userID, planID)
	return err
}

func (r *NeonPlanRepository) IsTrainingDay(ctx context.Context, userID, weekday string) (bool, error) {
	plan, err := r.GetLatest(ctx, userID)
	if err != nil || plan == nil {
		return false, err
	}

	var trainingPlan struct {
		Days []struct {
			Weekday   string `json:"weekday"`
			IsRestDay bool   `json:"is_rest_day"`
		} `json:"days"`
	}

	if err := json.Unmarshal(plan.TrainingJSON, &trainingPlan); err != nil {
		return false, err
	}

	for _, day := range trainingPlan.Days {
		if day.Weekday == weekday && !day.IsRestDay {
			return true, nil
		}
	}
	return false, nil
}

// -------------------- helpers --------------------

func scanPlan(row pgx.Row) (*domain.Plan, error) {
	var (
		p                   domain.Plan
		firestoreID         *string
		checkinFirestoreID  *string
		trainingJSON        []byte
		nutritionJSON       []byte
		recoveryJSON        []byte
	)

	err := row.Scan(
		&firestoreID, &p.UserID,
		&trainingJSON, &nutritionJSON, &recoveryJSON,
		&p.CycleDayRange, &p.PlanVersion, &p.Status,
		&p.GeneratedFrom, &p.SourceEventID,
		&p.CreatedAt,
		&checkinFirestoreID,
	)
	if err != nil {
		return nil, err
	}

	if firestoreID != nil {
		p.ID = *firestoreID
	}
	if checkinFirestoreID != nil {
		p.CheckinID = *checkinFirestoreID
	}
	p.TrainingJSON = trainingJSON
	p.NutritionJSON = nutritionJSON
	p.RecoveryJSON = recoveryJSON

	return &p, nil
}

func jsonbOrNull(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}
