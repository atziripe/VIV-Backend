ALTER TABLE user_profiles
    DROP COLUMN IF EXISTS user_catalog,
    DROP COLUMN IF EXISTS goal_id,
    DROP COLUMN IF EXISTS last_active_plan_id;
