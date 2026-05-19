-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Core identity table (minimal, never used in analytics queries)
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    firebase_uid    TEXT NOT NULL UNIQUE,
    email           TEXT UNIQUE,
    name            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_firebase_uid ON users(firebase_uid);

-- Onboarding profile (body metrics, training prefs, diet, sleep, stress)
-- Sensitive fields: dob, height_cm, weight_kg, digestion_conditions
-- For analytics/ML: use age_bucket view, never expose dob directly
CREATE TABLE user_profiles (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,

    -- Body (sensitive)
    dob                         DATE,
    height_cm                   REAL,
    weight_kg                   REAL,

    -- Training
    daily_activity_level        TEXT,
    training_often              TEXT,
    training_duration           TEXT,
    training_time               TEXT,
    training_type               TEXT,           -- comma-separated: "Strength,Pilates,HIIT,Cardio"
    training_goals              TEXT,
    priority                    TEXT,
    has_active_injury           BOOLEAN NOT NULL DEFAULT FALSE,
    training_paused             BOOLEAN NOT NULL DEFAULT FALSE,
    last_injury_reported_at     TIMESTAMPTZ,

    -- Diet
    diet_restrictions           TEXT,
    diet_protein_resources      TEXT,
    digestion_conditions        TEXT,           -- sensitive
    eating_style                TEXT,
    meals_per_day               TEXT,
    meals_timing_stability      TEXT,

    -- Sleep & recovery
    sleep_window                TEXT,
    sleep_continuity            TEXT,
    recovery_after_sleep        TEXT,
    lingering_marker            TEXT,

    -- Stress
    stress_level                TEXT,
    stress_reactivity           TEXT,

    -- Meta
    onboarding_completed        BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_profiles_user_id ON user_profiles(user_id);

-- Anonymized view for analytics/ML — NEVER exposes dob, exact weight, or height
CREATE VIEW user_profiles_anon AS
SELECT
    user_id,
    CASE
        WHEN dob IS NULL THEN NULL
        WHEN EXTRACT(YEAR FROM AGE(dob)) < 20 THEN '<20'
        WHEN EXTRACT(YEAR FROM AGE(dob)) BETWEEN 20 AND 24 THEN '20-24'
        WHEN EXTRACT(YEAR FROM AGE(dob)) BETWEEN 25 AND 29 THEN '25-29'
        WHEN EXTRACT(YEAR FROM AGE(dob)) BETWEEN 30 AND 34 THEN '30-34'
        WHEN EXTRACT(YEAR FROM AGE(dob)) BETWEEN 35 AND 39 THEN '35-39'
        WHEN EXTRACT(YEAR FROM AGE(dob)) BETWEEN 40 AND 44 THEN '40-44'
        ELSE '45+'
    END AS age_bucket,
    CASE
        WHEN height_cm IS NULL THEN NULL
        WHEN height_cm < 155 THEN '<155cm'
        WHEN height_cm BETWEEN 155 AND 164 THEN '155-164cm'
        WHEN height_cm BETWEEN 165 AND 174 THEN '165-174cm'
        ELSE '175cm+'
    END AS height_bucket,
    CASE
        WHEN weight_kg IS NULL THEN NULL
        WHEN weight_kg < 55  THEN '<55kg'
        WHEN weight_kg BETWEEN 55 AND 64  THEN '55-64kg'
        WHEN weight_kg BETWEEN 65 AND 74  THEN '65-74kg'
        WHEN weight_kg BETWEEN 75 AND 84  THEN '75-84kg'
        ELSE '85kg+'
    END AS weight_bucket,
    daily_activity_level,
    training_often,
    training_duration,
    training_time,
    training_type,
    training_goals,
    priority,
    diet_restrictions,
    diet_protein_resources,
    digestion_conditions,
    eating_style,
    meals_per_day,
    meals_timing_stability,
    sleep_window,
    sleep_continuity,
    recovery_after_sleep,
    stress_level,
    stress_reactivity,
    onboarding_completed
FROM user_profiles;
