#!/bin/bash
# run_migrations.sh
# Usage: DATABASE_URL="postgresql://..." ./run_migrations.sh
#
# Safe to run repeatedly: applied migrations are tracked in a
# schema_migrations table, so each *.up.sql only ever runs once. The first
# run after upgrading to this version also "catches up" any migrations
# that were already applied by hand before this tracking existed — if a
# migration fails because its table/column/constraint already exists (or,
# for a DROP, no longer exists), that's treated as "already applied" and
# recorded rather than as a real failure. A genuine SQL error (bad syntax,
# wrong table name, etc.) still stops the script immediately.

set -e

if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL is not set"
  echo "Usage: DATABASE_URL=\"postgresql://...\" ./run_migrations.sh"
  exit 1
fi

MIGRATIONS_DIR="$(dirname "$0")/migrations"
ERR_LOG="$(mktemp)"
trap 'rm -f "$ERR_LOG"' EXIT

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -q -c "
  CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
"

echo "Running VIV migrations against Neon..."
echo ""

for f in "$MIGRATIONS_DIR"/*.up.sql; do
  version="$(basename "$f" .up.sql)"

  already="$(psql "$DATABASE_URL" -tAc "SELECT 1 FROM schema_migrations WHERE version = '$version'")"
  if [ "$already" = "1" ]; then
    echo "⏭  $version — already applied, skipping"
    continue
  fi

  echo "→ Applying $version..."
  if psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f" >/dev/null 2>"$ERR_LOG"; then
    psql "$DATABASE_URL" -c "INSERT INTO schema_migrations (version) VALUES ('$version')" >/dev/null
    echo "  ✓ applied"
  elif grep -qE "already exists|does not exist" "$ERR_LOG"; then
    psql "$DATABASE_URL" -c "INSERT INTO schema_migrations (version) VALUES ('$version') ON CONFLICT DO NOTHING" >/dev/null
    echo "  ⚠ already applied by hand before this tracking existed — recorded, continuing"
  else
    echo "  ✗ failed:"
    cat "$ERR_LOG"
    exit 1
  fi
done

echo ""
echo "All migrations applied."
echo ""
echo "Verifying tables:"
psql "$DATABASE_URL" -c "\dt"
