-- Consent grant/revoke events — append-only. Every Grant call (see
-- NeonConsentRepository) inserts a new row rather than updating one, so the
-- full history for a user is always queryable, not just the current state —
-- this is what lets the product demonstrate consent was given (GDPR Art. 7.1)
-- rather than just assert it.
CREATE TABLE consents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose         TEXT NOT NULL,
    granted         BOOLEAN NOT NULL,
    policy_version  TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_consents_user_purpose_created ON consents(user_id, purpose, created_at DESC);
