ALTER TABLE user_cycles
    DROP COLUMN IF EXISTS cycle_updated_at,
    DROP COLUMN IF EXISTS cycle_estimation_disabled;
