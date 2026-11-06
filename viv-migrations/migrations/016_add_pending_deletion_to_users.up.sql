-- Mirrors domain.User.PendingDeletionAt — set by ScheduleAccountDeletionUseCase
-- (DELETE /me), cleared by CancelAccountDeletionUseCase. Firestore stays the
-- read path RunPendingAccountDeletionsUseCase's sweep actually queries; this
-- column exists so Neon's copy of the user isn't silently missing the field
-- once it's written, and so a future Neon-backed sweep has it available.
ALTER TABLE users ADD COLUMN pending_deletion_at TIMESTAMPTZ;
