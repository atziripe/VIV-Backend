DROP TRIGGER IF EXISTS trg_plan_jobs_updated_at    ON plan_jobs;
DROP TRIGGER IF EXISTS trg_device_tokens_updated_at ON device_tokens;
DROP TRIGGER IF EXISTS trg_user_cycles_updated_at   ON user_cycles;
DROP TRIGGER IF EXISTS trg_user_profiles_updated_at ON user_profiles;
DROP TRIGGER IF EXISTS trg_users_updated_at         ON users;
DROP FUNCTION IF EXISTS set_updated_at();
