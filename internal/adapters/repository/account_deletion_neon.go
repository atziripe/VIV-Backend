package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/usecase"
)

var _ usecase.AccountAnonymizer = (*NeonAccountAnonymizer)(nil)

// NeonAccountAnonymizer implements the "keep an anonymized snapshot, then
// wipe everything else" half of account deletion. See
// usecase.DeleteAccountUseCase's doc comment for why this is best-effort
// rather than fatal.
type NeonAccountAnonymizer struct {
	pool *pgxpool.Pool
}

func NewNeonAccountAnonymizer(pool *pgxpool.Pool) *NeonAccountAnonymizer {
	return &NeonAccountAnonymizer{pool: pool}
}

// AnonymizeAndDeleteUser snapshots userID's bucketed profile (the same
// shape user_profiles_anon has always exposed live) into
// deleted_account_profiles_anon — which has no user_id/firebase_uid column,
// so the snapshot can never be traced back to them — then deletes the
// users row. Every other Neon table referencing it cascades via its own
// ON DELETE CASCADE foreign key, so nothing else needs to be touched here.
// Both steps run in one transaction: either the snapshot is captured and
// the row is gone, or neither happens.
func (r *NeonAccountAnonymizer) AnonymizeAndDeleteUser(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var internalID string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&internalID); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO deleted_account_profiles_anon (
			age_bucket, height_bucket, weight_bucket,
			daily_activity_level, training_often, training_duration,
			training_time, training_type, training_goals, priority,
			diet_restrictions, diet_protein_resources, digestion_conditions,
			eating_style, meals_per_day, meals_timing_stability,
			sleep_window, sleep_continuity, recovery_after_sleep,
			stress_level, stress_reactivity,
			onboarding_completed
		)
		SELECT
			age_bucket, height_bucket, weight_bucket,
			daily_activity_level, training_often, training_duration,
			training_time, training_type, training_goals, priority,
			diet_restrictions, diet_protein_resources, digestion_conditions,
			eating_style, meals_per_day, meals_timing_stability,
			sleep_window, sleep_continuity, recovery_after_sleep,
			stress_level, stress_reactivity,
			onboarding_completed
		FROM user_profiles_anon
		WHERE user_id = $1
	`, internalID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, internalID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
