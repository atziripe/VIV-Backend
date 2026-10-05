package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/checkin"
	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.DailyCheckinRepository = (*NeonDailyCheckinRepository)(nil)

type NeonDailyCheckinRepository struct {
	pool *pgxpool.Pool
}

func NewNeonDailyCheckinRepository(pool *pgxpool.Pool) *NeonDailyCheckinRepository {
	return &NeonDailyCheckinRepository{pool: pool}
}

func (r *NeonDailyCheckinRepository) Upsert(ctx context.Context, c *domain.DailyCheckin) error {
	if c == nil {
		return nil
	}

	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, c.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO daily_checkins (
			user_id, date, sleep, body, demand, need,
			recovery_capacity, life_bandwidth, build_readiness, submitted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, date) DO UPDATE SET
			sleep              = EXCLUDED.sleep,
			body               = EXCLUDED.body,
			demand             = EXCLUDED.demand,
			need               = EXCLUDED.need,
			recovery_capacity  = EXCLUDED.recovery_capacity,
			life_bandwidth     = EXCLUDED.life_bandwidth,
			build_readiness    = EXCLUDED.build_readiness,
			submitted_at       = EXCLUDED.submitted_at
	`, userUUID, c.Date.Format("2006-01-02"),
		string(c.Answers.Sleep), string(c.Answers.Body), string(c.Answers.Demand), string(c.Answers.Need),
		string(c.Readiness.RecoveryCapacity), string(c.Readiness.LifeBandwidth), string(c.Readiness.BuildReadiness),
		c.SubmittedAt)
	return err
}

func (r *NeonDailyCheckinRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*domain.DailyCheckin, error) {
	var sleep, body, demand, need, recovery, bandwidth, build string
	var submittedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT dc.sleep, dc.body, dc.demand, dc.need,
		       dc.recovery_capacity, dc.life_bandwidth, dc.build_readiness, dc.submitted_at
		FROM daily_checkins dc
		JOIN users u ON u.id = dc.user_id
		WHERE u.firebase_uid = $1 AND dc.date = $2
	`, userID, date.Format("2006-01-02")).Scan(&sleep, &body, &demand, &need, &recovery, &bandwidth, &build, &submittedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &domain.DailyCheckin{
		UserID: userID,
		Date:   date,
		Answers: checkin.DailyCheckin{
			Sleep:  checkin.SleepAnswer(sleep),
			Body:   checkin.BodyAnswer(body),
			Demand: checkin.DemandAnswer(demand),
			Need:   checkin.NeedAnswer(need),
		},
		Readiness: checkin.ReadinessDimensions{
			RecoveryCapacity: checkin.RecoveryCapacity(recovery),
			LifeBandwidth:    checkin.LifeBandwidth(bandwidth),
			BuildReadiness:   checkin.BuildReadiness(build),
		},
		SubmittedAt: submittedAt,
	}, nil
}
