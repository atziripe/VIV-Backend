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

var _ usecase.SessionLogRepository = (*NeonSessionLogRepository)(nil)

type NeonSessionLogRepository struct {
	pool *pgxpool.Pool
}

func NewNeonSessionLogRepository(pool *pgxpool.Pool) *NeonSessionLogRepository {
	return &NeonSessionLogRepository{pool: pool}
}

func (r *NeonSessionLogRepository) Upsert(ctx context.Context, log *domain.SessionLog) error {
	if log == nil {
		return nil
	}

	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, log.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	setsJSON, err := json.Marshal(log.Sets)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO session_logs (
			user_id, date, activity_type, status, started_at, ended_at, sets, feedback, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, date) DO UPDATE SET
			activity_type = EXCLUDED.activity_type,
			status        = EXCLUDED.status,
			started_at    = EXCLUDED.started_at,
			ended_at      = EXCLUDED.ended_at,
			sets          = EXCLUDED.sets,
			feedback      = EXCLUDED.feedback,
			updated_at    = EXCLUDED.updated_at
	`, userUUID, log.Date.Format("2006-01-02"), log.ActivityType, string(log.Status),
		log.StartedAt, log.EndedAt, setsJSON, string(log.Feedback), log.UpdatedAt)
	return err
}

func (r *NeonSessionLogRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*domain.SessionLog, error) {
	var activityType, status, feedback string
	var startedAt time.Time
	var endedAt *time.Time
	var setsJSON []byte
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT sl.activity_type, sl.status, sl.started_at, sl.ended_at, sl.sets, sl.feedback, sl.updated_at
		FROM session_logs sl
		JOIN users u ON u.id = sl.user_id
		WHERE u.firebase_uid = $1 AND sl.date = $2
	`, userID, date.Format("2006-01-02")).Scan(&activityType, &status, &startedAt, &endedAt, &setsJSON, &feedback, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var sets []domain.SetLog
	if err := json.Unmarshal(setsJSON, &sets); err != nil {
		return nil, err
	}

	return &domain.SessionLog{
		UserID:       userID,
		Date:         date,
		ActivityType: activityType,
		Status:       domain.SessionLogStatus(status),
		StartedAt:    startedAt,
		EndedAt:      endedAt,
		Sets:         sets,
		Feedback:     domain.SessionFeedback(feedback),
		UpdatedAt:    updatedAt,
	}, nil
}

// ListByUser returns every session log the user ever has, newest first —
// see usecase.SessionLogRepository.ListByUser's doc comment. Not read by
// DualSessionLogRepository today (reads go to Firestore), but kept in
// parity so a future Neon-backed read path has it available.
func (r *NeonSessionLogRepository) ListByUser(ctx context.Context, userID string, limit int) ([]*domain.SessionLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sl.date, sl.activity_type, sl.status, sl.started_at, sl.ended_at, sl.sets, sl.feedback, sl.updated_at
		FROM session_logs sl
		JOIN users u ON u.id = sl.user_id
		WHERE u.firebase_uid = $1
		ORDER BY sl.date DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.SessionLog
	for rows.Next() {
		var date time.Time
		var activityType, status, feedback string
		var startedAt time.Time
		var endedAt *time.Time
		var setsJSON []byte
		var updatedAt time.Time
		if err := rows.Scan(&date, &activityType, &status, &startedAt, &endedAt, &setsJSON, &feedback, &updatedAt); err != nil {
			return nil, err
		}

		var sets []domain.SetLog
		if err := json.Unmarshal(setsJSON, &sets); err != nil {
			return nil, err
		}

		results = append(results, &domain.SessionLog{
			UserID:       userID,
			Date:         date,
			ActivityType: activityType,
			Status:       domain.SessionLogStatus(status),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			Sets:         sets,
			Feedback:     domain.SessionFeedback(feedback),
			UpdatedAt:    updatedAt,
		})
	}
	return results, rows.Err()
}
