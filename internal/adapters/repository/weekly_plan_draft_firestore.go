package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"

	"viv/internal/core/usecase"
)

var _ usecase.WeeklyPlanDraftRepository = (*FirestoreWeeklyPlanDraftRepository)(nil)

// FirestoreWeeklyPlanDraftRepository stores VIV-106 weekly plan drafts under:
// users/{userId}/weekly_plan_drafts/{draftId}
//
// This is a separate collection from users/{userId}/plans (domain.Plan) —
// a WeekDraft is a different shape (built from the new-system's
// activity/goal/checkin/weeklytarget/cascade packages, not the old rule
// engine's domain types), so it gets its own home rather than being forced
// into the old Plan document shape.
type FirestoreWeeklyPlanDraftRepository struct {
	client *firestore.Client
}

func NewFirestoreWeeklyPlanDraftRepository(client *firestore.Client) *FirestoreWeeklyPlanDraftRepository {
	return &FirestoreWeeklyPlanDraftRepository{client: client}
}

// weeklyPlanDraftDoc is the Firestore document shape. Following the same
// "raw payload as source of truth for the UI" pattern already used by
// planDoc's TrainingJSON/NutritionJSON/RecoveryJSON: the full draft is kept
// as one JSON blob (DraftJSON) rather than mapped field-by-field, with only
// what's needed for querying (user/status/dates) broken out.
type weeklyPlanDraftDoc struct {
	UserID         string    `firestore:"user_id"`
	GenerationDate time.Time `firestore:"generation_date"`
	StartDate      time.Time `firestore:"start_date"`
	EndDate        time.Time `firestore:"end_date"`
	Status         string    `firestore:"status"`
	GoalID         string    `firestore:"goal_id"`
	DraftJSON      string    `firestore:"draft_json"`
	CreatedAt      time.Time `firestore:"created_at"`
}

func (r *FirestoreWeeklyPlanDraftRepository) col(userID string) *firestore.CollectionRef {
	return r.client.Collection("users").Doc(userID).Collection("weekly_plan_drafts")
}

// SaveDraft persists the draft and sets its generated ID on the passed-in
// pointer, mirroring PlanRepository.Create's convention.
func (r *FirestoreWeeklyPlanDraftRepository) SaveDraft(ctx context.Context, draft *usecase.WeekDraft) error {
	if draft == nil {
		return nil
	}

	payload, err := json.Marshal(draft)
	if err != nil {
		return fmt.Errorf("marshaling weekly plan draft: %w", err)
	}

	doc := weeklyPlanDraftDoc{
		UserID:         draft.UserID,
		GenerationDate: draft.GenerationDate,
		StartDate:      draft.StartDate,
		EndDate:        draft.EndDate,
		Status:         draft.Status,
		GoalID:         string(draft.GoalID),
		DraftJSON:      string(payload),
		CreatedAt:      draft.CreatedAt,
	}

	docRef := r.col(draft.UserID).NewDoc()
	draft.ID = docRef.ID

	_, err = docRef.Set(ctx, doc)
	return err
}

// GetByDate finds the draft whose [StartDate, EndDate] span contains date.
// Firestore can't do a single-query inequality range across two different
// fields, so this queries the most recent drafts starting on or before
// date and filters the EndDate side in memory — simple and sufficient
// given a user has at most a handful of drafts at once.
func (r *FirestoreWeeklyPlanDraftRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*usecase.WeekDraft, error) {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	q := r.col(userID).
		Where("start_date", "<=", d).
		OrderBy("start_date", firestore.Desc).
		Limit(10)

	docs, err := q.Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		var dd weeklyPlanDraftDoc
		if err := doc.DataTo(&dd); err != nil {
			return nil, err
		}
		if d.After(dd.EndDate) {
			continue
		}

		var draft usecase.WeekDraft
		if err := json.Unmarshal([]byte(dd.DraftJSON), &draft); err != nil {
			return nil, fmt.Errorf("unmarshaling weekly plan draft: %w", err)
		}
		draft.ID = doc.Ref.ID
		return &draft, nil
	}

	return nil, nil
}

// UpdateDaySlot rewrites exactly one day (0-6) of an already-saved draft.
// This is a read-modify-write over the same JSON-blob field SaveDraft
// writes — matching this repository's "raw payload as source of truth"
// pattern rather than mapping Days to per-field Firestore updates — so
// concurrent adaptations of two different days of the same week could
// race; no other repository in this codebase guards against that either
// (e.g. PlanRepository.UpdateTrainingCompleted), so this matches the
// existing bar rather than introducing transactions unilaterally.
func (r *FirestoreWeeklyPlanDraftRepository) UpdateDaySlot(ctx context.Context, userID, draftID string, dayIndex int, day usecase.DayPlan) error {
	if dayIndex < 0 || dayIndex > 6 {
		return fmt.Errorf("weekly plan draft: dayIndex %d out of range", dayIndex)
	}

	docRef := r.col(userID).Doc(draftID)
	doc, err := docRef.Get(ctx)
	if err != nil {
		return err
	}

	var dd weeklyPlanDraftDoc
	if err := doc.DataTo(&dd); err != nil {
		return err
	}

	var draft usecase.WeekDraft
	if err := json.Unmarshal([]byte(dd.DraftJSON), &draft); err != nil {
		return fmt.Errorf("unmarshaling weekly plan draft: %w", err)
	}

	draft.Days[dayIndex] = day

	payload, err := json.Marshal(draft)
	if err != nil {
		return fmt.Errorf("marshaling weekly plan draft: %w", err)
	}

	_, err = docRef.Update(ctx, []firestore.Update{
		{Path: "draft_json", Value: string(payload)},
	})
	return err
}
