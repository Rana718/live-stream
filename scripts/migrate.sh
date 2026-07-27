#!/bin/bash
# Applies every migrations/*.sql file, in lexical filename order, straight
# through psql — the same way sqlc.yaml already treats this folder for
# codegen ("sqlc applies migration files in lexical order").
#
# This project's migrations/ was never actually compatible with
# golang-migrate (which needs paired NNN_name.up.sql/.down.sql files —
# these are single NNN_name.sql files) or with sql-migrate as a runtime
# tool (no `github.com/rubenv/sql-migrate` in go.mod; the `-- +migrate Up`
# markers in the earlier files are vestigial and inert as far as psql is
# concerned — plain `--` SQL comments). `make migrate-up` calling the
# `migrate` CLI never worked against this repo's actual file layout.
#
# Idempotent: tracks applied filenames in schema_migrations_applied so
# re-running only picks up new files, safe to call on every deploy.
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-live-platform-postgres}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-live_platform}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="$SCRIPT_DIR/../migrations"

echo "Waiting for PostgreSQL to be ready..."
until docker exec "$PG_CONTAINER" pg_isready -U "$PG_USER" >/dev/null 2>&1; do
  sleep 2
done

docker exec "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 -q -c "
  CREATE TABLE IF NOT EXISTS schema_migrations_applied (
    filename    TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
  );
"

applied_count=0
for path in $(find "$MIGRATIONS_DIR" -maxdepth 1 -name "*.sql" | sort); do
  filename="$(basename "$path")"
  already="$(docker exec "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -tAc \
    "SELECT 1 FROM schema_migrations_applied WHERE filename = '$filename'")"
  if [ "$already" = "1" ]; then
    continue
  fi

  echo "Applying $filename ..."
  docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 -q < "$path"
  docker exec "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 -q -c \
    "INSERT INTO schema_migrations_applied (filename) VALUES ('$filename')"
  applied_count=$((applied_count + 1))
done

if [ "$applied_count" -eq 0 ]; then
  echo "Already up to date — nothing to apply."
else
  echo "Applied $applied_count migration(s) successfully."
fi
