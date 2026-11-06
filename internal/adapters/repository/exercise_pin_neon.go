package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
	"viv/internal/core/strengthlibrary"
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
	var activeExercisesJSON []byte
	var blockNumber int
	var activatedOn, rotatesOn *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT ep.active_exercises, ep.block_number, ep.activated_on, ep.rotates_on
		FROM exercise_pins ep
		JOIN users u ON u.id = ep.user_id
		WHERE u.firebase_uid = $1 AND ep.activity_type = $2 AND ep.muscle_group = $3
	`, userID, string(activityType), string(muscleGroup)).Scan(&activeExercisesJSON, &blockNumber, &activatedOn, &rotatesOn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var raw map[string]string
	if err := json.Unmarshal(activeExercisesJSON, &raw); err != nil {
		return nil, err
	}
	active := make(map[strengthlibrary.MovementGroup]mesocycle.ExerciseID, len(raw))
	for group, id := range raw {
		active[strengthlibrary.MovementGroup(group)] = mesocycle.ExerciseID(id)
	}

	pin := &mesocycle.PinnedExerciseSet{
		ActivityType:    activityType,
		MuscleGroup:     muscleGroup,
		ActiveExercises: active,
		BlockNumber:     blockNumber,
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

	raw := make(map[string]string, len(pin.ActiveExercises))
	for group, id := range pin.ActiveExercises {
		raw[string(group)] = string(id)
	}
	activeExercisesJSON, err := json.Marshal(raw)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO exercise_pins (user_id, activity_type, muscle_group, active_exercises, block_number, activated_on, rotates_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, activity_type, muscle_group) DO UPDATE SET
			active_exercises = EXCLUDED.active_exercises,
			block_number     = EXCLUDED.block_number,
			activated_on     = EXCLUDED.activated_on,
			rotates_on       = EXCLUDED.rotates_on
	`, userUUID, string(pin.ActivityType), string(pin.MuscleGroup), activeExercisesJSON, pin.BlockNumber, pin.ActivatedOn, pin.RotatesOn)
	return err
}
