package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var _ usecase.CheckinRepository = (*NeonCheckinRepository)(nil)

type NeonCheckinRepository struct {
	pool *pgxpool.Pool
}

func NewNeonCheckinRepository(pool *pgxpool.Pool) *NeonCheckinRepository {
	return &NeonCheckinRepository{pool: pool}
}

func (r *NeonCheckinRepository) Create(ctx context.Context, c *domain.Checkin) error {
	if c == nil {
		return nil
	}

	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, c.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	var pmsSymptoms *string
	if c.PMSSymptoms != nil {
		s := string(*c.PMSSymptoms)
		pmsSymptoms = &s
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO checkins (
			user_id, firestore_id,
			readiness, demand, sleep_quality, last_week_feel,
			body_status, appetite, predictability, modality_skip,
			pms_symptoms, additional_notes, cycle_start,
			week_start, created_at
		) VALUES (
			$1, $2,
			$3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15
		)
		ON CONFLICT (firestore_id) DO NOTHING
	`,
		userUUID, c.ID,
		string(c.Readiness), string(c.Demand), string(c.Sleep), string(c.LastWeekFeel),
		string(c.Body), string(c.AppetiteV2), string(c.Predictability), c.ModalitySkip,
		pmsSymptoms, c.AdditionalNotes, c.CycleStart,
		c.WeekStart, c.CreatedAt,
	)
	return err
}

func (r *NeonCheckinRepository) GetByID(ctx context.Context, userID, id string) (*domain.Checkin, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			ch.firestore_id, u.firebase_uid,
			ch.readiness, ch.demand, ch.sleep_quality, ch.last_week_feel,
			ch.body_status, ch.appetite, ch.predictability, ch.modality_skip,
			ch.pms_symptoms, ch.additional_notes, ch.cycle_start,
			ch.week_start, ch.created_at
		FROM checkins ch
		JOIN users u ON u.id = ch.user_id
		WHERE u.firebase_uid = $1 AND ch.firestore_id = $2
	`, userID, id)

	c, err := scanCheckin(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *NeonCheckinRepository) GetLatestByUser(ctx context.Context, userID string) (*domain.Checkin, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			ch.firestore_id, u.firebase_uid,
			ch.readiness, ch.demand, ch.sleep_quality, ch.last_week_feel,
			ch.body_status, ch.appetite, ch.predictability, ch.modality_skip,
			ch.pms_symptoms, ch.additional_notes, ch.cycle_start,
			ch.week_start, ch.created_at
		FROM checkins ch
		JOIN users u ON u.id = ch.user_id
		WHERE u.firebase_uid = $1
		ORDER BY ch.created_at DESC
		LIMIT 1
	`, userID)

	c, err := scanCheckin(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

// -------------------- scanner --------------------

func scanCheckin(row pgx.Row) (*domain.Checkin, error) {
	var (
		c           domain.Checkin
		firestoreID *string
		readiness   string
		demand      string
		sleep       string
		lastWeek    string
		body        string
		appetite    string
		predict     string
		pmsSymptoms *string
	)

	err := row.Scan(
		&firestoreID, &c.UserID,
		&readiness, &demand, &sleep, &lastWeek,
		&body, &appetite, &predict, &c.ModalitySkip,
		&pmsSymptoms, &c.AdditionalNotes, &c.CycleStart,
		&c.WeekStart, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if firestoreID != nil {
		c.ID = *firestoreID
	}

	c.Readiness = domain.CheckinReadiness(readiness)
	c.Demand = domain.CheckinDemand(demand)
	c.Sleep = domain.CheckinSleep(sleep)
	c.LastWeekFeel = domain.CheckinLastWeek(lastWeek)
	c.Body = domain.CheckinBody(body)
	c.AppetiteV2 = domain.CheckinAppetite(appetite)
	c.Predictability = domain.CheckinPredictability(predict)

	if pmsSymptoms != nil {
		ps := domain.CheckinPMSSymptoms(*pmsSymptoms)
		c.PMSSymptoms = &ps
	}

	return &c, nil
}
