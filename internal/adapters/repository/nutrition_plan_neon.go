package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.NutritionPlanRepository = (*NeonNutritionPlanRepository)(nil)

type NeonNutritionPlanRepository struct {
	pool *pgxpool.Pool
}

func NewNeonNutritionPlanRepository(pool *pgxpool.Pool) *NeonNutritionPlanRepository {
	return &NeonNutritionPlanRepository{pool: pool}
}

func (r *NeonNutritionPlanRepository) GetByUserID(ctx context.Context, userID string) (*domain.NutritionPlan, error) {
	var planJSON []byte
	var createdAt, updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT np.plan_json, np.created_at, np.updated_at
		FROM nutrition_plans np
		JOIN users u ON u.id = np.user_id
		WHERE u.firebase_uid = $1
	`, userID).Scan(&planJSON, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var plan domain.NutritionWeekPlan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return nil, err
	}

	return &domain.NutritionPlan{
		UserID:    userID,
		Plan:      plan,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func (r *NeonNutritionPlanRepository) Save(ctx context.Context, userID string, plan domain.NutritionWeekPlan) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&userUUID); err != nil {
		return err
	}

	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	_, err = r.pool.Exec(ctx, `
		INSERT INTO nutrition_plans (user_id, plan_json, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			plan_json  = EXCLUDED.plan_json,
			updated_at = $3
	`, userUUID, planJSON, now)
	return err
}
