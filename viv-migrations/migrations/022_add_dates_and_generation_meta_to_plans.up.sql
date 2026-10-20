-- Phase 2 of the Firestore→Neon cutover — see migration 019's comment for
-- why each table gets its own file.
--
-- generated_from/source_event_id: NeonPlanRepository.Create was already
-- inserting into these columns before this migration existed, so every
-- legacy Plan's secondary (Neon) write has been failing silently
-- (best-effort, logged only) since that code shipped.
-- IF NOT EXISTS per column: a single ALTER TABLE with several bare ADD
-- COLUMN clauses is one atomic statement in Postgres — if any one column
-- already existed (e.g. generated_from, from the bug above), the whole
-- statement fails and NONE of the columns get added, silently masked by
-- run_migrations.sh's "already applied by hand" recovery matching the
-- error text. IF NOT EXISTS makes each clause independently safe.
ALTER TABLE plans
    ADD COLUMN IF NOT EXISTS start_date      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS end_date        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS generated_from  TEXT,
    ADD COLUMN IF NOT EXISTS source_event_id TEXT;
