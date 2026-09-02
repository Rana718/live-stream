#!/bin/bash
# Dev helper: drop + recreate the schema_v2_test scratch database and apply
# every migrations/*.sql in order, failing on the first error. Not part of
# the deploy path — scripts/migrate.sh is.
set -euo pipefail
PG="${PG_CONTAINER:-live-platform-postgres}"
DB="${SCRATCH_DB:-schema_v2_test}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../migrations" && pwd)"

docker exec "$PG" psql -U postgres -tAc "DROP DATABASE IF EXISTS $DB" >/dev/null
docker exec "$PG" psql -U postgres -tAc "CREATE DATABASE $DB" >/dev/null

for f in $(find "$DIR" -maxdepth 1 -name '*.sql' | sort); do
    if docker exec -i "$PG" psql -U postgres -d "$DB" -v ON_ERROR_STOP=1 -q >/dev/null 2>/tmp/mig_err < "$f"; then
        echo "ok   $(basename "$f")"
    else
        echo "FAIL $(basename "$f")"
        cat /tmp/mig_err
        exit 1
    fi
done
echo "--- $(docker exec "$PG" psql -U postgres -d "$DB" -tAc "SELECT count(*) FROM pg_tables WHERE schemaname='public'") tables ---"
