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

var _ usecase.RecoveryActionRepository = (*NeonRecoveryActionRepository)(nil)

type NeonRecoveryActionRepository struct {
	pool *pgxpool.Pool
}

func NewNeonRecoveryActionRepository(pool *pgxpool.Pool) *NeonRecoveryActionRepository {
	return &NeonRecoveryActionRepository{pool: pool}
}

func (r *NeonRecoveryActionRepository) Upsert(ctx context.Context, a *domain.RecoveryAction) error {
	if a == nil {
		return nil
	}

	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, a.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO recovery_actions (user_id, date, kind, logged_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, date) DO UPDATE SET
			kind      = EXCLUDED.kind,
			logged_at = EXCLUDED.logged_at
	`, userUUID, a.Date.Format("2006-01-02"), string(a.Kind), a.LoggedAt)
	return err
}

func (r *NeonRecoveryActionRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*domain.RecoveryAction, error) {
	var kind string
	var loggedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT ra.kind, ra.logged_at
		FROM recovery_actions ra
		JOIN users u ON u.id = ra.user_id
		WHERE u.firebase_uid = $1 AND ra.date = $2
	`, userID, date.Format("2006-01-02")).Scan(&kind, &loggedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &domain.RecoveryAction{
		UserID:   userID,
		Date:     date,
		Kind:     domain.RecoveryActionKind(kind),
		LoggedAt: loggedAt,
	}, nil
}
