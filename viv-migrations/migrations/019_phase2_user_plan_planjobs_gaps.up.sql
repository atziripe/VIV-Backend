-- Phase 2 of the Firestore→Neon cutover: close the Neon data gaps that
-- block cutting User, Plan (legacy), and PlanJobs over to Neon-only reads
-- (see Phase 1's commit for the entities already cut over).
--
-- users.pending_deletion_at/last_active_at/pending_deletion_reason already
-- exist (migrations 016/017) and are already written by
-- NeonUserRepository.Save — but GetByID never selected them back, nor did
-- user_profiles.user_catalog/goal_id/last_active_plan_id or
-- user_cycles.cycle_updated_at/cycle_estimation_disabled exist at all, so a
-- Neon-only read of a user was silently missing all of these.
ALTER TABLE users ADD COLUMN apple_refresh_token TEXT;

ALTER TABLE user_profiles
    ADD COLUMN user_catalog        TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN goal_id             TEXT,
    ADD COLUMN last_active_plan_id TEXT;

ALTER TABLE user_cycles
    ADD COLUMN cycle_updated_at          TIMESTAMPTZ,
    ADD COLUMN cycle_estimation_disabled BOOLEAN NOT NULL DEFAULT FALSE;

-- plans.generated_from/source_event_id were already being written by
-- NeonPlanRepository.Create and read by scanPlan — against columns that
-- never existed, so every legacy Plan.Create's secondary (Neon) write has
-- been failing silently (best-effort, logged only) since that code shipped.
ALTER TABLE plans
    ADD COLUMN start_date      TIMESTAMPTZ,
    ADD COLUMN end_date        TIMESTAMPTZ,
    ADD COLUMN generated_from  TEXT,
    ADD COLUMN source_event_id TEXT;

-- plan_jobs never had anywhere to put the plan ID a finished job produced
-- (domain.PlanJob.PlanID), so NeonPlanJobsRepository.MarkDone silently
-- discarded its planID argument. Stored as TEXT, not a UUID, because the
-- value is always the Firestore-style ID (plans.firestore_id), matching
-- how plan_jobs.firestore_id/plans.firestore_id already work.
ALTER TABLE plan_jobs ADD COLUMN plan_id TEXT;
CREATE INDEX idx_plan_jobs_plan_id ON plan_jobs(plan_id);
