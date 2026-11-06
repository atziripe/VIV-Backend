-- VIV-106..113 weekly plan drafts — the core of the current training
-- pipeline. draft_json holds the full WeekDraft blob (all 7 days,
-- hydrated content, warnings, and the VIV-113 weekly-note cache), same
-- "raw payload as source of truth for the UI" pattern as
-- `plans.training_json`. firestore_id bridges to the Firestore-generated
-- draft ID (SaveDraft sets it via col.NewDoc()), same convention as
-- `plan_jobs.firestore_id`.
--
-- start_date/end_date are real columns (not buried in the jsonb) so
-- GetByDate can do a direct range query — Postgres has no equivalent of
-- Firestore's single-inequality-field limitation, so this replaces that
-- repository's "query start_date DESC then filter end_date in memory"
-- workaround with one real WHERE clause.
CREATE TABLE weekly_plan_drafts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    firestore_id        TEXT UNIQUE,

    generation_date     TIMESTAMPTZ,
    start_date          TIMESTAMPTZ NOT NULL,
    end_date            TIMESTAMPTZ NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft',
    goal_id             TEXT,
    draft_json          JSONB NOT NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_weekly_plan_drafts_user_id    ON weekly_plan_drafts(user_id);
CREATE INDEX idx_weekly_plan_drafts_user_range ON weekly_plan_drafts(user_id, start_date DESC, end_date);
CREATE INDEX idx_weekly_plan_drafts_gin        ON weekly_plan_drafts USING GIN(draft_json);
