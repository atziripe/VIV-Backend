-- Phase 2 of the Firestore→Neon cutover — see migration 019's comment for
-- why each table gets its own file.
ALTER TABLE user_cycles
    ADD COLUMN cycle_updated_at          TIMESTAMPTZ,
    ADD COLUMN cycle_estimation_disabled BOOLEAN NOT NULL DEFAULT FALSE;
