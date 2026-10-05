package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"viv/internal/core/usecase"
)

var _ usecase.WeeklyPlanDraftRepository = (*NeonWeeklyPlanDraftRepository)(nil)

// NeonWeeklyPlanDraftRepository is the dual-write secondary for the
// current pipeline's core entity. Reads still go through the Firestore
// primary (see DualWeeklyPlanDraftRepository) — this exists to keep Neon's
// copy current so a later read cutover has real data to cut over to.
type NeonWeeklyPlanDraftRepository struct {
	pool *pgxpool.Pool
}

func NewNeonWeeklyPlanDraftRepository(pool *pgxpool.Pool) *NeonWeeklyPlanDraftRepository {
	return &NeonWeeklyPlanDraftRepository{pool: pool}
}

// SaveDraft satisfies usecase.WeeklyPlanDraftRepository directly. It
// cannot be the dual-write secondary's real entry point, though: the
// primary (Firestore) generates draft.ID as a side effect of its own
// SaveDraft call, and this method has no way to know that ID in advance
// to keep the two stores linked. DualWeeklyPlanDraftRepository detects
// SaveDraftLinked below (same pattern as planJobLinker /
// weeklyPlanDraftLinker) and calls that instead whenever it's available.
func (r *NeonWeeklyPlanDraftRepository) SaveDraft(ctx context.Context, draft *usecase.WeekDraft) error {
	if draft.ID == "" {
		return errors.New("neon weekly plan draft: SaveDraft called without an ID — use SaveDraftLinked from a dual-write caller")
	}
	return r.SaveDraftLinked(ctx, draft, draft.ID)
}

// SaveDraftLinked persists draft under the given Firestore-generated ID,
// without mutating draft.ID itself (the primary already owns that).
func (r *NeonWeeklyPlanDraftRepository) SaveDraftLinked(ctx context.Context, draft *usecase.WeekDraft, firestoreDraftID string) error {
	var userUUID string
	if err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE firebase_uid = $1`, draft.UserID,
	).Scan(&userUUID); err != nil {
		return err
	}

	draftJSON, err := json.Marshal(draft)
	if err != nil {
		return err
	}

	createdAt := draft.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO weekly_plan_drafts (
			user_id, firestore_id, generation_date, start_date, end_date,
			status, goal_id, draft_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (firestore_id) DO UPDATE SET
			generation_date = EXCLUDED.generation_date,
			start_date      = EXCLUDED.start_date,
			end_date        = EXCLUDED.end_date,
			status          = EXCLUDED.status,
			goal_id         = EXCLUDED.goal_id,
			draft_json      = EXCLUDED.draft_json
	`, userUUID, firestoreDraftID, draft.GenerationDate, draft.StartDate, draft.EndDate,
		draft.Status, string(draft.GoalID), draftJSON, createdAt)
	return err
}

// GetByDate is not used by the dual-write path today (reads still go
// through the Firestore primary — see DualWeeklyPlanDraftRepository) but
// is implemented for real, not stubbed, so this repository is already
// correct on the day reads cut over to Neon. Unlike the Firestore
// implementation, this needs no in-memory EndDate filter: Postgres can
// express the real two-sided range query FirestoreWeeklyPlanDraftRepository
// works around.
func (r *NeonWeeklyPlanDraftRepository) GetByDate(ctx context.Context, userID string, date time.Time) (*usecase.WeekDraft, error) {
	d := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	var firestoreID string
	var draftJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT wpd.firestore_id, wpd.draft_json
		FROM weekly_plan_drafts wpd
		JOIN users u ON u.id = wpd.user_id
		WHERE u.firebase_uid = $1 AND wpd.start_date <= $2 AND wpd.end_date >= $2
		ORDER BY wpd.start_date DESC
		LIMIT 1
	`, userID, d).Scan(&firestoreID, &draftJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var draft usecase.WeekDraft
	if err := json.Unmarshal(draftJSON, &draft); err != nil {
		return nil, err
	}
	draft.ID = firestoreID
	return &draft, nil
}

// UpdateDaySlot mirrors FirestoreWeeklyPlanDraftRepository.UpdateDaySlot's
// read-modify-write over the whole draft_json blob, including clearing
// the Notes cache — see that method's doc comment for why.
func (r *NeonWeeklyPlanDraftRepository) UpdateDaySlot(ctx context.Context, userID, draftID string, dayIndex int, day usecase.DayPlan) error {
	if dayIndex < 0 || dayIndex > 6 {
		return errors.New("neon weekly plan draft: dayIndex out of range")
	}

	var draftJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT wpd.draft_json
		FROM weekly_plan_drafts wpd
		JOIN users u ON u.id = wpd.user_id
		WHERE u.firebase_uid = $1 AND wpd.firestore_id = $2
	`, userID, draftID).Scan(&draftJSON)
	if err != nil {
		return err
	}

	var draft usecase.WeekDraft
	if err := json.Unmarshal(draftJSON, &draft); err != nil {
		return err
	}
	draft.Days[dayIndex] = day
	draft.Notes = nil

	updated, err := json.Marshal(draft)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE weekly_plan_drafts SET draft_json = $1
		FROM users
		WHERE weekly_plan_drafts.user_id = users.id
		  AND users.firebase_uid = $2
		  AND weekly_plan_drafts.firestore_id = $3
	`, updated, userID, draftID)
	return err
}

// SetNote mirrors FirestoreWeeklyPlanDraftRepository.SetNote's
// read-modify-write, caching a generated weekly note for one date.
func (r *NeonWeeklyPlanDraftRepository) SetNote(ctx context.Context, userID, draftID, dateKey, note string) error {
	var draftJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT wpd.draft_json
		FROM weekly_plan_drafts wpd
		JOIN users u ON u.id = wpd.user_id
		WHERE u.firebase_uid = $1 AND wpd.firestore_id = $2
	`, userID, draftID).Scan(&draftJSON)
	if err != nil {
		return err
	}

	var draft usecase.WeekDraft
	if err := json.Unmarshal(draftJSON, &draft); err != nil {
		return err
	}
	if draft.Notes == nil {
		draft.Notes = map[string]string{}
	}
	draft.Notes[dateKey] = note

	updated, err := json.Marshal(draft)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE weekly_plan_drafts SET draft_json = $1
		FROM users
		WHERE weekly_plan_drafts.user_id = users.id
		  AND users.firebase_uid = $2
		  AND weekly_plan_drafts.firestore_id = $3
	`, updated, userID, draftID)
	return err
}
