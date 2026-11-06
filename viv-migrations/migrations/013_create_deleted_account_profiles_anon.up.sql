-- Anonymized snapshot kept after an account is deleted — required both
-- by the App Store account-deletion guideline (5.1.1v) and by the
-- product's own retention policy: identifying data must go away, but
-- aggregate/bucketed profile data stays for analytics.
--
-- No column here can ever point back to a real person — no user_id, no
-- firebase_uid, nothing that references `users` or any other
-- identifying table. Same bucket shape user_profiles_anon (migration
-- 001) has always exposed live; captured once at deletion time because
-- that view's source row won't exist anymore afterward.
CREATE TABLE deleted_account_profiles_anon (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    age_bucket              TEXT,
    height_bucket           TEXT,
    weight_bucket           TEXT,

    daily_activity_level    TEXT,
    training_often          TEXT,
    training_duration       TEXT,
    training_time           TEXT,
    training_type           TEXT,
    training_goals          TEXT,
    priority                TEXT,

    diet_restrictions       TEXT,
    diet_protein_resources  TEXT,
    digestion_conditions    TEXT,
    eating_style            TEXT,
    meals_per_day           TEXT,
    meals_timing_stability  TEXT,

    sleep_window            TEXT,
    sleep_continuity        TEXT,
    recovery_after_sleep    TEXT,

    stress_level            TEXT,
    stress_reactivity       TEXT,

    onboarding_completed    BOOLEAN,

    deleted_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Deliberately no index on anything identifying — this table is for
-- aggregate analysis, never a per-person lookup.
CREATE INDEX idx_deleted_account_profiles_anon_deleted_at ON deleted_account_profiles_anon(deleted_at);
