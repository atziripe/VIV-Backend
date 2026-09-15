package repository

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"viv/internal/core/activity"
	"viv/internal/core/mesocycle"
	"viv/internal/core/usecase"
)

var _ usecase.ExercisePinRepository = (*FirestoreExercisePinRepository)(nil)

// FirestoreExercisePinRepository stores VIV-108 pinned exercise sets under:
// users/{userId}/exercise_pins/{activityType}_{muscleGroup}
//
// One doc per (activityType, muscleGroup) slot is exactly the granularity
// the task specifies ("per user, per muscle-group slot... for each
// mesocycle-eligible activity type"), so the doc ID is a deterministic
// composite key rather than a generated one — Get is a direct doc lookup,
// no query needed.
type FirestoreExercisePinRepository struct {
	client *firestore.Client
}

func NewFirestoreExercisePinRepository(client *firestore.Client) *FirestoreExercisePinRepository {
	return &FirestoreExercisePinRepository{client: client}
}

type exercisePinDoc struct {
	ActivityType string    `firestore:"activity_type"`
	MuscleGroup  string    `firestore:"muscle_group"`
	ExerciseIDs  []string  `firestore:"exercise_ids"`
	ActivatedOn  time.Time `firestore:"activated_on"`
	RotatesOn    time.Time `firestore:"rotates_on"`
}

func pinDocID(activityType activity.ID, muscleGroup activity.MuscleGroup) string {
	return string(activityType) + "_" + string(muscleGroup)
}

func (r *FirestoreExercisePinRepository) Get(
	ctx context.Context, userID string, activityType activity.ID, muscleGroup activity.MuscleGroup,
) (*mesocycle.PinnedExerciseSet, error) {
	doc, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("exercise_pins").
		Doc(pinDocID(activityType, muscleGroup)).
		Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var pd exercisePinDoc
	if err := doc.DataTo(&pd); err != nil {
		return nil, err
	}

	ids := make([]mesocycle.ExerciseID, len(pd.ExerciseIDs))
	for i, id := range pd.ExerciseIDs {
		ids[i] = mesocycle.ExerciseID(id)
	}

	return &mesocycle.PinnedExerciseSet{
		ActivityType: activity.ID(pd.ActivityType),
		MuscleGroup:  activity.MuscleGroup(pd.MuscleGroup),
		ExerciseIDs:  ids,
		ActivatedOn:  pd.ActivatedOn,
		RotatesOn:    pd.RotatesOn,
	}, nil
}

func (r *FirestoreExercisePinRepository) Save(ctx context.Context, userID string, pin mesocycle.PinnedExerciseSet) error {
	ids := make([]string, len(pin.ExerciseIDs))
	for i, id := range pin.ExerciseIDs {
		ids[i] = string(id)
	}

	doc := exercisePinDoc{
		ActivityType: string(pin.ActivityType),
		MuscleGroup:  string(pin.MuscleGroup),
		ExerciseIDs:  ids,
		ActivatedOn:  pin.ActivatedOn,
		RotatesOn:    pin.RotatesOn,
	}

	_, err := r.client.
		Collection("users").
		Doc(userID).
		Collection("exercise_pins").
		Doc(pinDocID(pin.ActivityType, pin.MuscleGroup)).
		Set(ctx, doc)
	return err
}
