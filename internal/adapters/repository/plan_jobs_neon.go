package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.PlanJobsRepository = (*NeonPlanJobsRepository)(nil)

type NeonPlanJobsRepository struct {
	pool *pgxpool.Pool
}

func NewNeonPlanJobsRepository(pool *pgxpool.Pool) *NeonPlanJobsRepository {
	return &NeonPlanJobsRepository{pool: pool}
}

func (r *NeonPlanJobsRepository) CreateQueued(ctx context.Context, userID, checkinID string) (string, error) {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&userUUID); err != nil {
		return "", err
	}

	var checkinUUID *string
	if strings.TrimSpace(checkinID) != "" {
		var cid string
		err := r.pool.QueryRow(ctx,
			`SELECT id FROM checkins WHERE firestore_id = $1`, checkinID,
		).Scan(&cid)
		if err == nil {
			checkinUUID = &cid
		}
	}

	now := time.Now().UTC()
	var jobID string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO plan_jobs (user_id, checkin_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userUUID, checkinUUID, string(domain.PlanJobQueued), now, now).Scan(&jobID)
	if err != nil {
		return "", err
	}

	return jobID, nil
}

// CreateQueuedLinked inserts a Neon job row tagged with the Firestore job ID
// that the caller (DualPlanJobsRepository) already generated via the primary
// repo, so later Mark*/GetByID calls — which only ever carry that Firestore
// ID — can find this row by firestore_id instead of Neon's own UUID.
func (r *NeonPlanJobsRepository) CreateQueuedLinked(ctx context.Context, userID, checkinID, firestoreJobID string) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&userUUID); err != nil {
		return err
	}

	var checkinUUID *string
	if strings.TrimSpace(checkinID) != "" {
		var cid string
		if err := r.pool.QueryRow(ctx,
			`SELECT id FROM checkins WHERE firestore_id = $1`, checkinID,
		).Scan(&cid); err == nil {
			checkinUUID = &cid
		}
	}

	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO plan_jobs (user_id, checkin_id, firestore_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (firestore_id) DO NOTHING
	`, userUUID, checkinUUID, firestoreJobID, string(domain.PlanJobQueued), now, now)
	return err
}

func (r *NeonPlanJobsRepository) MarkRunning(ctx context.Context, userID, jobID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE plan_jobs SET status = $1, updated_at = NOW()
		FROM users
		WHERE plan_jobs.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plan_jobs.firestore_id = $3
	`, string(domain.PlanJobRunning), userID, jobID)
	return err
}

func (r *NeonPlanJobsRepository) MarkDone(ctx context.Context, userID, jobID, planID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE plan_jobs SET status = $1, error = NULL, updated_at = NOW()
		FROM users
		WHERE plan_jobs.user_id = users.id
		  AND users.firebase_uid = $2
		  AND plan_jobs.firestore_id = $3
	`, string(domain.PlanJobDone), userID, jobID)
	return err
}

func (r *NeonPlanJobsRepository) MarkFailed(ctx context.Context, userID, jobID, errorMsg string) error {
	e := strings.TrimSpace(errorMsg)
	if e == "" {
		e = "unknown error"
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE plan_jobs SET status = $1, error = $2, updated_at = NOW()
		FROM users
		WHERE plan_jobs.user_id = users.id
		  AND users.firebase_uid = $3
		  AND plan_jobs.firestore_id = $4
	`, string(domain.PlanJobFailed), e, userID, jobID)
	return err
}

func (r *NeonPlanJobsRepository) GetByID(ctx context.Context, userID, jobID string) (*domain.PlanJob, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			pj.firestore_id, u.firebase_uid,
			pj.status, pj.error,
			ch.firestore_id as checkin_firestore_id,
			pj.created_at, pj.updated_at
		FROM plan_jobs pj
		JOIN users u ON u.id = pj.user_id
		LEFT JOIN checkins ch ON ch.id = pj.checkin_id
		WHERE u.firebase_uid = $1 AND pj.firestore_id = $2
	`, userID, jobID)

	var (
		job        domain.PlanJob
		errMsg     *string
		checkinFID *string
	)

	err := row.Scan(
		&job.ID, &job.UserID,
		&job.Status, &errMsg,
		&checkinFID,
		&job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if errMsg != nil {
		job.Error = *errMsg
	}
	if checkinFID != nil {
		job.CheckinID = *checkinFID
	}

	return &job, nil
}
