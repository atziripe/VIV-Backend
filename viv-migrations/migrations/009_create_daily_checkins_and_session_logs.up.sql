-- VIV-103 daily check-in — distinct from the legacy weekly `checkins`
-- table (different vocabulary, different cadence). One row per
-- (user, date); a second submission for the same date overwrites via
-- ON CONFLICT, mirroring the Firestore doc-ID-by-date Upsert convention.
CREATE TABLE daily_checkins (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date                DATE NOT NULL,

    sleep               TEXT,
    body                TEXT,
    demand              TEXT,
    need                TEXT,
    recovery_capacity   TEXT,
    life_bandwidth      TEXT,
    build_readiness     TEXT,

    submitted_at        TIMESTAMPTZ NOT NULL,

    UNIQUE (user_id, date)
);

CREATE INDEX idx_daily_checkins_user_date ON daily_checkins(user_id, date);

-- Real-time set-by-set logging for a Loggable (Strength) training day —
-- one row per (user, date). `sets` kept as jsonb: a variable-length list
-- of {exercise_id, exercise_name, set_number, weight_kg, reps, logged_at},
-- same "relational for stable fields, jsonb for nested blobs" split as
-- `plans.training_json`.
CREATE TABLE session_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date            DATE NOT NULL,

    activity_type   TEXT,
    status          TEXT NOT NULL,
    started_at      TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    sets            JSONB NOT NULL DEFAULT '[]',
    feedback        TEXT,
    updated_at      TIMESTAMPTZ NOT NULL,

    UNIQUE (user_id, date)
);

CREATE INDEX idx_session_logs_user_date ON session_logs(user_id, date);
