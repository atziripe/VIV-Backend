-- Phase 2 of the Firestore→Neon cutover. Split into one atomic ALTER TABLE
-- per migration file (019-023) rather than one file with several —
-- run_migrations.sh's self-healing "already applied by hand" detection
-- operates per *file*: if one statement in a multi-statement file hit a
-- column that already exists in production (we don't know its exact
-- current state), the whole file would be marked applied and every later
-- statement in it would be silently skipped.
ALTER TABLE users ADD COLUMN apple_refresh_token TEXT;
