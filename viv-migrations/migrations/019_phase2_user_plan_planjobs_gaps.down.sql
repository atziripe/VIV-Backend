DROP INDEX IF EXISTS idx_plan_jobs_plan_id;
ALTER TABLE plan_jobs DROP COLUMN IF EXISTS plan_id;

ALTER TABLE plans
    DROP COLUMN IF EXISTS start_date,
    DROP COLUMN IF EXISTS end_date,
    DROP COLUMN IF EXISTS generated_from,
    DROP COLUMN IF EXISTS source_event_id;

ALTER TABLE user_cycles
    DROP COLUMN IF EXISTS cycle_updated_at,
    DROP COLUMN IF EXISTS cycle_estimation_disabled;

ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS user_catalog,
    DROP COLUMN IF EXISTS goal_id,
    DROP COLUMN IF EXISTS last_active_plan_id;

ALTER TABLE users DROP COLUMN IF EXISTS apple_refresh_token;
