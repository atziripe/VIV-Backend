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

var _ usecase.LifestyleChangeRepository = (*NeonLifestyleChangeRepository)(nil)

type NeonLifestyleChangeRepository struct {
	pool *pgxpool.Pool
}

func NewNeonLifestyleChangeRepository(pool *pgxpool.Pool) *NeonLifestyleChangeRepository {
	return &NeonLifestyleChangeRepository{pool: pool}
}

func (r *NeonLifestyleChangeRepository) Create(ctx context.Context, e *domain.LifestyleChange) error {
	if e == nil {
		return nil
	}

	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, e.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO lifestyle_changes (
			user_id, firestore_id, type, space_train, possible_diet,
			energy, applies_to, plan_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (firestore_id) DO NOTHING
	`, userUUID, e.ID, e.Type, e.SpaceTrain, e.PossibleDiet,
		e.Energy, e.AppliesTo, e.PlanID, e.CreatedAt)
	return err
}

func (r *NeonLifestyleChangeRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.LifestyleChange, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT lc.firestore_id, lc.type, lc.space_train, lc.possible_diet,
		       lc.energy, lc.applies_to, lc.plan_id, lc.created_at
		FROM lifestyle_changes lc
		JOIN users u ON u.id = lc.user_id
		WHERE u.firebase_uid = $1
		ORDER BY lc.created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.LifestyleChange
	for rows.Next() {
		var id, changeType, spaceTrain, possibleDiet, energy string
		var appliesTo *time.Time
		var planID *string
		var createdAt time.Time
		if err := rows.Scan(&id, &changeType, &spaceTrain, &possibleDiet, &energy, &appliesTo, &planID, &createdAt); err != nil {
			return nil, err
		}
		results = append(results, &domain.LifestyleChange{
			ID:           id,
			UserID:       userID,
			CreatedAt:    createdAt,
			Type:         changeType,
			SpaceTrain:   spaceTrain,
			PossibleDiet: possibleDiet,
			Energy:       energy,
			AppliesTo:    appliesTo,
			PlanID:       planID,
		})
	}
	return results, rows.Err()
}

func (r *NeonLifestyleChangeRepository) GetByID(ctx context.Context, userID string, changeID string) (*domain.LifestyleChange, error) {
	var changeType, spaceTrain, possibleDiet, energy string
	var appliesTo *time.Time
	var planID *string
	var createdAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT lc.type, lc.space_train, lc.possible_diet, lc.energy,
		       lc.applies_to, lc.plan_id, lc.created_at
		FROM lifestyle_changes lc
		JOIN users u ON u.id = lc.user_id
		WHERE u.firebase_uid = $1 AND lc.firestore_id = $2
	`, userID, changeID).Scan(&changeType, &spaceTrain, &possibleDiet, &energy, &appliesTo, &planID, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &domain.LifestyleChange{
		ID:           changeID,
		UserID:       userID,
		CreatedAt:    createdAt,
		Type:         changeType,
		SpaceTrain:   spaceTrain,
		PossibleDiet: possibleDiet,
		Energy:       energy,
		AppliesTo:    appliesTo,
		PlanID:       planID,
	}, nil
}

func (r *NeonLifestyleChangeRepository) SetPlanID(ctx context.Context, userID string, changeID string, planID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE lifestyle_changes SET plan_id = $1
		FROM users
		WHERE lifestyle_changes.user_id = users.id
		  AND users.firebase_uid = $2
		  AND lifestyle_changes.firestore_id = $3
	`, planID, userID, changeID)
	return err
}
