package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
	"viv/internal/core/usecase"
)

var _ usecase.ExercisePinRepository = (*NeonExercisePinRepository)(nil)

type NeonExercisePinRepository struct {
	pool *pgxpool.Pool
}

func NewNeonExercisePinRepository(pool *pgxpool.Pool) *NeonExercisePinRepository {
	return &NeonExercisePinRepository{pool: pool}
}

func (r *NeonExercisePinRepository) Get(
	ctx context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup,
) (*mesocycle.PinnedExerciseSet, error) {
	var exerciseIDs []string
	var activatedOn, rotatesOn *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT ep.exercise_ids, ep.activated_on, ep.rotates_on
		FROM exercise_pins ep
		JOIN users u ON u.id = ep.user_id
		WHERE u.firebase_uid = $1 AND ep.activity_type = $2 AND ep.muscle_group = $3
	`, userID, string(activityType), string(muscleGroup)).Scan(&exerciseIDs, &activatedOn, &rotatesOn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	ids := make([]mesocycle.ExerciseID, len(exerciseIDs))
	for i, id := range exerciseIDs {
		ids[i] = mesocycle.ExerciseID(id)
	}

	pin := &mesocycle.PinnedExerciseSet{
		ActivityType: activityType,
		MuscleGroup:  muscleGroup,
		ExerciseIDs:  ids,
	}
	if activatedOn != nil {
		pin.ActivatedOn = *activatedOn
	}
	if rotatesOn != nil {
		pin.RotatesOn = *rotatesOn
	}
	return pin, nil
}

func (r *NeonExercisePinRepository) Save(ctx context.Context, userID string, pin mesocycle.PinnedExerciseSet) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, userID,
	).Scan(&userUUID); err != nil {
		return err
	}

	ids := make([]string, len(pin.ExerciseIDs))
	for i, id := range pin.ExerciseIDs {
		ids[i] = string(id)
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO exercise_pins (user_id, activity_type, muscle_group, exercise_ids, activated_on, rotates_on)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, activity_type, muscle_group) DO UPDATE SET
			exercise_ids  = EXCLUDED.exercise_ids,
			activated_on  = EXCLUDED.activated_on,
			rotates_on    = EXCLUDED.rotates_on
	`, userUUID, string(pin.ActivityType), string(pin.MuscleGroup), ids, pin.ActivatedOn, pin.RotatesOn)
	return err
}
