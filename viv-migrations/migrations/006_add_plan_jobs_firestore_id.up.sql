-- plan_jobs was created without a firestore_id bridge column (unlike
-- checkins/plans), so the dual-write path had no way to look up a Neon row
-- by the Firestore-generated job ID. MarkRunning/MarkDone/MarkFailed were
-- passed that Firestore ID and tried to match it against the UUID `id`
-- column, which Postgres rejects outright (SQLSTATE 22P02).
ALTER TABLE plan_jobs ADD COLUMN firestore_id TEXT UNIQUE;

CREATE INDEX idx_plan_jobs_firestore_id ON plan_jobs(firestore_id);
