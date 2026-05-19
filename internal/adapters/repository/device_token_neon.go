package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.DeviceTokenRepository = (*NeonDeviceTokenRepository)(nil)

type NeonDeviceTokenRepository struct {
	pool *pgxpool.Pool
}

func NewNeonDeviceTokenRepository(pool *pgxpool.Pool) *NeonDeviceTokenRepository {
	return &NeonDeviceTokenRepository{pool: pool}
}

func (r *NeonDeviceTokenRepository) Upsert(ctx context.Context, token *domain.DeviceToken) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, token.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO device_tokens (user_id, fcm_token, platform, timezone, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (fcm_token) DO UPDATE SET
			user_id    = EXCLUDED.user_id,
			platform   = EXCLUDED.platform,
			timezone   = EXCLUDED.timezone,
			active     = EXCLUDED.active,
			updated_at = NOW()
	`, userUUID, token.Token, token.Platform, token.Timezone, token.Active, now, now)
	return err
}

func (r *NeonDeviceTokenRepository) GetAllActive(ctx context.Context) ([]*domain.DeviceToken, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.firebase_uid, dt.fcm_token, dt.platform, dt.timezone, dt.active, dt.created_at, dt.updated_at
		FROM device_tokens dt
		JOIN users u ON u.id = dt.user_id
		WHERE dt.active = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*domain.DeviceToken
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(
			&t.UserID, &t.Token, &t.Platform, &t.Timezone,
			&t.Active, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			continue
		}
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}

func (r *NeonDeviceTokenRepository) Deactivate(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE device_tokens SET active = false, updated_at = NOW()
		FROM users
		WHERE device_tokens.user_id = users.id
		  AND users.firebase_uid = $1
	`, userID)
	return err
}

func (r *NeonDeviceTokenRepository) GetAllActiveByTimezone(ctx context.Context) (map[string][]*domain.DeviceToken, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.firebase_uid, dt.fcm_token, dt.platform, dt.timezone, dt.active, dt.created_at, dt.updated_at
		FROM device_tokens dt
		JOIN users u ON u.id = dt.user_id
		WHERE dt.active = true
		ORDER BY dt.timezone
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]*domain.DeviceToken)
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(
			&t.UserID, &t.Token, &t.Platform, &t.Timezone,
			&t.Active, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			continue
		}
		result[t.Timezone] = append(result[t.Timezone], &t)
	}
	return result, rows.Err()
}
