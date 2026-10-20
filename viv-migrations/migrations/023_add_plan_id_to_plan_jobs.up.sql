-- Phase 2 of the Firestore→Neon cutover — see migration 019's comment for
-- why each table gets its own file.
--
-- plan_jobs never had anywhere to put the plan ID a finished job produced
-- (domain.PlanJob.PlanID), so NeonPlanJobsRepository.MarkDone silently
-- discarded its planID argument. Stored as TEXT, not a UUID, because the
-- value is always the Firestore-style ID (plans.firestore_id), matching
-- how plan_jobs.firestore_id/plans.firestore_id already work.
ALTER TABLE plan_jobs ADD COLUMN plan_id TEXT;
