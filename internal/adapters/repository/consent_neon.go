package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/domain"
	"viv/internal/core/usecase"
)

var (
	_ usecase.ConsentRepository = (*NeonConsentRepository)(nil)
	_ usecase.ConsentChecker    = (*NeonConsentRepository)(nil)
)

type NeonConsentRepository struct {
	pool *pgxpool.Pool
}

func NewNeonConsentRepository(pool *pgxpool.Pool) *NeonConsentRepository {
	return &NeonConsentRepository{pool: pool}
}

func (r *NeonConsentRepository) Grant(ctx context.Context, userID string, purpose domain.ConsentPurpose, granted bool, policyVersion string) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&userUUID); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO consents (user_id, purpose, granted, policy_version)
		VALUES ($1, $2, $3, $4)
	`, userUUID, string(purpose), granted, policyVersion)
	return err
}

// CreateHistorical inserts a consent row with an explicit created_at,
// for the backfill path only — Grant above always stamps "now" (correct
// for a live grant/revoke), but copying a user's actual consent history
// from Firestore needs to preserve each event's original timestamp so
// GetAll's "most recent per purpose" resolution and the GDPR Art. 7.1
// paper trail stay accurate.
func (r *NeonConsentRepository) CreateHistorical(ctx context.Context, userID string, purpose domain.ConsentPurpose, granted bool, policyVersion string, createdAt time.Time) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&userUUID); err != nil {
		return err
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO consents (user_id, purpose, granted, policy_version, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, userUUID, string(purpose), granted, policyVersion, createdAt)
	return err
}

// GetAll returns each purpose's most recent row via DISTINCT ON, Postgres's
// native way to express "latest per group" — no in-memory reduction needed,
// unlike the Firestore counterpart.
func (r *NeonConsentRepository) GetAll(ctx context.Context, userID string) (map[domain.ConsentPurpose]domain.Consent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (c.purpose) c.purpose, c.granted, c.policy_version, c.created_at
		FROM consents c
		JOIN users u ON u.id = c.user_id
		WHERE u.firebase_uid = $1
		ORDER BY c.purpose, c.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[domain.ConsentPurpose]domain.Consent{}
	for rows.Next() {
		var c domain.Consent
		var purpose string
		if err := rows.Scan(&purpose, &c.Granted, &c.PolicyVersion, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.UserID = userID
		c.Purpose = domain.ConsentPurpose(purpose)
		result[c.Purpose] = c
	}
	return result, rows.Err()
}

func (r *NeonConsentRepository) IsGranted(ctx context.Context, userID string, purpose domain.ConsentPurpose) (bool, error) {
	all, err := r.GetAll(ctx, userID)
	if err != nil {
		return false, err
	}
	c, ok := all[purpose]
	return ok && c.Granted, nil
}
