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

// ListByUser returns every recovery action the user ever logged, newest
// first — see usecase.RecoveryActionRepository.ListByUser's doc comment.
// Not read by DualRecoveryActionRepository today (reads go to Firestore),
// but kept in parity so a future Neon-backed read path has it available.
func (r *NeonRecoveryActionRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.RecoveryAction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ra.date, ra.kind, ra.logged_at
		FROM recovery_actions ra
		JOIN users u ON u.id = ra.user_id
		WHERE u.firebase_uid = $1
		ORDER BY ra.date DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.RecoveryAction
	for rows.Next() {
		var date time.Time
		var kind string
		var loggedAt time.Time
		if err := rows.Scan(&date, &kind, &loggedAt); err != nil {
			return nil, err
		}
		results = append(results, &domain.RecoveryAction{
			UserID:   userID,
			Date:     date,
			Kind:     domain.RecoveryActionKind(kind),
			LoggedAt: loggedAt,
		})
	}
	return results, rows.Err()
}
