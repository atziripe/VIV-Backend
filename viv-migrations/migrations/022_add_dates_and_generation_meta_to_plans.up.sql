-- Phase 2 of the Firestore→Neon cutover — see migration 019's comment for
-- why each table gets its own file.
--
-- generated_from/source_event_id: NeonPlanRepository.Create was already
-- inserting into these columns before this migration existed, so every
-- legacy Plan's secondary (Neon) write has been failing silently
-- (best-effort, logged only) since that code shipped.
ALTER TABLE plans
    ADD COLUMN start_date      TIMESTAMPTZ,
    ADD COLUMN end_date        TIMESTAMPTZ,
    ADD COLUMN generated_from  TEXT,
    ADD COLUMN source_event_id TEXT;
