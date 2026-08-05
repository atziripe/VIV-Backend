DROP INDEX IF EXISTS idx_plan_jobs_firestore_id;
ALTER TABLE plan_jobs DROP COLUMN IF EXISTS firestore_id;
