#!/bin/bash
# Restore a backup produced by scripts/backup-db.sh.
#
# Usage:
#   ./scripts/restore-db.sh latest                    # restore newest MinIO backup
#   ./scripts/restore-db.sh live_platform_2026....dump # restore a specific backup (local or MinIO)
#   ./scripts/restore-db.sh latest --target=live_platform --yes   # overwrite the LIVE db
#
# By default this restores into a scratch database
# ("<PG_DB>_restore_test"), never the live one — so running this to verify
# a backup is safe by default. Overwriting the real database requires
# BOTH --target=<the live db name> AND --yes.
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-live-platform-postgres}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-live_platform}"
MINIO_CONTAINER="${MINIO_CONTAINER:-live-platform-minio}"
MINIO_NETWORK="${MINIO_NETWORK:-live-stream_live-platform}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
MINIO_BUCKET="${MINIO_BUCKET:-db-backups}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="$SCRIPT_DIR/../backups"
mkdir -p "$BACKUP_DIR"

if [ $# -lt 1 ]; then
  echo "usage: $0 <backup-filename|latest> [--target=<db_name>] [--yes]" >&2
  exit 64
fi

backup_arg="$1"; shift
target_db="${PG_DB}_restore_test"
confirmed=false
for arg in "$@"; do
  case "$arg" in
    --target=*) target_db="${arg#--target=}" ;;
    --yes) confirmed=true ;;
    *) echo "unknown flag: $arg" >&2; exit 64 ;;
  esac
done

if [ "$target_db" = "$PG_DB" ] && [ "$confirmed" != true ]; then
  echo "[restore-db] REFUSING: --target=$PG_DB is the live database." >&2
  echo "[restore-db] This will DROP and recreate every table in it. Re-run with --yes to confirm." >&2
  exit 1
fi

mc_run() {
  docker run --rm \
    --network "$MINIO_NETWORK" \
    --entrypoint sh \
    -e "MC_HOST_backupsrc=http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@${MINIO_CONTAINER}:9000" \
    -v "$BACKUP_DIR:/backups" \
    minio/mc:latest \
    -c "$1"
}

if [ "$backup_arg" = "latest" ]; then
  echo "[restore-db] resolving latest backup from minio://$MINIO_BUCKET"
  # busybox sh (this image's shell) has no awk/sed/rev — use the shell's
  # own word-splitting to grab the last whitespace-separated field
  # (the filename) regardless of how `mc ls` pads the date/size columns.
  latest_name="$(mc_run 'line="$(mc ls backupsrc/'"$MINIO_BUCKET"' | sort | tail -1)"; set -- $line; eval "echo \${$#}"')"
  latest_name="$(echo "$latest_name" | tr -d '\r\n')"
  if [ -z "$latest_name" ]; then
    echo "[restore-db] FATAL: no backups found in minio://$MINIO_BUCKET" >&2
    exit 1
  fi
  backup_arg="$latest_name"
fi

local_path="$BACKUP_DIR/$backup_arg"
if [ ! -f "$local_path" ]; then
  echo "[restore-db] $backup_arg not found locally, pulling from minio://$MINIO_BUCKET"
  mc_run "mc cp backupsrc/$MINIO_BUCKET/$backup_arg /backups/$backup_arg"
fi
if [ ! -f "$local_path" ]; then
  echo "[restore-db] FATAL: could not obtain $backup_arg" >&2
  exit 1
fi

echo "[restore-db] restoring $backup_arg -> database '$target_db' on $PG_CONTAINER"

docker exec "$PG_CONTAINER" psql -U "$PG_USER" -d postgres -tc \
  "SELECT 1 FROM pg_database WHERE datname = '$target_db'" | grep -q 1 || \
  docker exec "$PG_CONTAINER" createdb -U "$PG_USER" "$target_db"

docker exec -i "$PG_CONTAINER" pg_restore \
  -U "$PG_USER" -d "$target_db" \
  --clean --if-exists --no-owner --no-privileges \
  < "$local_path"

echo "[restore-db] restore complete: $target_db"
echo "[restore-db] verify row counts / spot-check data before treating this as a validated restore."
