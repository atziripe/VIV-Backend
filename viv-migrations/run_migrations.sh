#!/bin/bash
# run_migrations.sh
# Usage: DATABASE_URL="postgresql://..." ./run_migrations.sh

set -e

if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL is not set"
  echo "Usage: DATABASE_URL=\"postgresql://...\" ./run_migrations.sh"
  exit 1
fi

MIGRATIONS_DIR="$(dirname "$0")/migrations"

echo "Running VIV migrations against Neon..."
echo ""

for f in "$MIGRATIONS_DIR"/*.up.sql; do
  echo "→ Applying $(basename $f)..."
  psql "$DATABASE_URL" -f "$f"
  echo "  ✓ done"
done

echo ""
echo "All migrations applied successfully."
echo ""
echo "Verifying tables:"
psql "$DATABASE_URL" -c "\dt"
