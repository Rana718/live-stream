#!/usr/bin/env bash
# seed_apply.sh — applies the Vidya Warrior seed in idempotent order.
#
# Run this once after a fresh `goose up`, or after a partial migration
# state where 033 failed because the user_role enum lacked super_admin.
# Idempotent — safe to re-run.
#
# Usage:
#   cd live-stream
#   ./migrations/seed_apply.sh             # uses docker exec on local compose
#   ./migrations/seed_apply.sh prod        # reads $DATABASE_URL instead

set -euo pipefail

run_sql() {
  if [ "${1:-}" = "prod" ]; then
    psql "$DATABASE_URL" -v ON_ERROR_STOP=1
  else
    docker exec -i live-platform-postgres psql -U postgres -d live_platform -v ON_ERROR_STOP=1
  fi
}

# 1. Patch the enum so super_admin / parent values are accepted before
#    migration 033 tries to use them. ADD VALUE IF NOT EXISTS is a no-op
#    when the value is already present (Postgres 14+).
echo "==> patching user_role enum"
run_sql "$@" <<'SQL'
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'super_admin';
ALTER TYPE user_role ADD VALUE IF NOT EXISTS 'parent';
SQL

# 2. Re-apply migration 033 if it failed previously. It's idempotent
#    (constraints use DROP IF EXISTS / ADD CONSTRAINT, indexes use
#    IF NOT EXISTS).
echo "==> applying 033_super_admin_role_seed.sql"
run_sql "$@" < "$(dirname "$0")/033_super_admin_role_seed.sql"

# 3. Apply the Vidya Warrior tenant seed.
echo "==> applying 038_seed_vidya_warrior.sql"
run_sql "$@" < "$(dirname "$0")/038_seed_vidya_warrior.sql"

# 4. Course bundles schema. Idempotent (CREATE TABLE IF NOT EXISTS, etc.).
echo "==> applying 039_course_bundles.sql"
run_sql "$@" < "$(dirname "$0")/039_course_bundles.sql"

# 5. Sample courses + two bundles on the Vidya Warrior tenant. Idempotent
#    (ON CONFLICT on slug for courses, on id for bundles).
echo "==> applying 040_seed_vidya_warrior_courses_bundles.sql"
run_sql "$@" < "$(dirname "$0")/040_seed_vidya_warrior_courses_bundles.sql"


# 6. Marketing CMS schema + seed posts/FAQs/pages.
echo "==> applying 041_cms_content.sql"
run_sql "$@" < "$(dirname "$0")/041_cms_content.sql"

echo "==> done. Login with org code RANJAN24 → phone +919999900001 (admin)"
echo "    Or +919999900002 for the demo student."
echo "    The student app's /store will show 5 courses + 2 bundles."
echo "    Marketing site (vidyawarrior.com) has 3 sample posts + 8 FAQs + Privacy/Terms."
