-- Phase 2 of the Firestore→Neon cutover — see migration 019's comment for
-- why each table gets its own file.
ALTER TABLE user_profiles
    ADD COLUMN user_catalog        TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN goal_id             TEXT,
    ADD COLUMN last_active_plan_id TEXT;
