-- Mirrors domain.User.LastActiveAt/PendingDeletionReason. Firestore stays
-- the read path RunInactivitySweepUseCase and RecordUserActivityUseCase
-- actually query/update; these columns exist so Neon's copy isn't silently
-- missing the fields once they're written, same reasoning as migration 016.
ALTER TABLE users ADD COLUMN last_active_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN pending_deletion_reason TEXT;
