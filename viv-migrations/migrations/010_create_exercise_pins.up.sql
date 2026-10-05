-- VIV-108 pinned exercise sets — one row per (user, activity_type,
-- muscle_group) slot, the exact granularity the task specifies. A
-- deterministic composite key, same as the Firestore doc ID
-- ({activityType}_{muscleGroup}), so Get is a direct row lookup.
CREATE TABLE exercise_pins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_type   TEXT NOT NULL,
    muscle_group    TEXT NOT NULL,
    exercise_ids    TEXT[] NOT NULL DEFAULT '{}',
    activated_on    TIMESTAMPTZ,
    rotates_on      TIMESTAMPTZ,

    UNIQUE (user_id, activity_type, muscle_group)
);

CREATE INDEX idx_exercise_pins_user_id ON exercise_pins(user_id);
