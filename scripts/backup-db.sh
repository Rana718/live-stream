#!/bin/bash
# Nightly Postgres backup: pg_dump (custom format) -> local backups/ ->
# MinIO (db-backups bucket) -> prune local files past retention.
#
# Usage:
#   ./scripts/backup-db.sh
#
# Cron example (2:30am daily, logs to a file cron/logrotate can pick up):
#   30 2 * * * cd /path/to/live-stream && ./scripts/backup-db.sh >> /var/log/live-platform/backup.log 2>&1
#
# Exits non-zero on any failure so cron mail / a monitoring wrapper (e.g.
# healthchecks.io curl, Alertmanager webhook) can page on a missed backup.
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-live-platform-postgres}"
PG_USER="${PG_USER:-postgres}"
PG_DB="${PG_DB:-live_platform}"
MINIO_CONTAINER="${MINIO_CONTAINER:-live-platform-minio}"
MINIO_NETWORK="${MINIO_NETWORK:-live-stream_live-platform}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
MINIO_BUCKET="${MINIO_BUCKET:-db-backups}"
LOCAL_RETENTION_DAYS="${LOCAL_RETENTION_DAYS:-14}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="$SCRIPT_DIR/../backups"
mkdir -p "$BACKUP_DIR"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
filename="${PG_DB}_${timestamp}.dump"
local_path="$BACKUP_DIR/$filename"

echo "[backup-db] dumping $PG_DB from $PG_CONTAINER -> $local_path"
if ! docker exec "$PG_CONTAINER" pg_isready -U "$PG_USER" >/dev/null 2>&1; then
  echo "[backup-db] FATAL: $PG_CONTAINER is not accepting connections" >&2
  exit 1
fi

# -Fc: custom format — compressed, supports selective/parallel restore via
# pg_restore. Written inside the container then streamed out, so a partial
# dump on our side (disk full, network drop) never leaves a truncated file
# in the container itself.
docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -Fc "$PG_DB" > "$local_path.tmp"
mv "$local_path.tmp" "$local_path"

size="$(du -h "$local_path" | cut -f1)"
echo "[backup-db] dump complete: $filename ($size)"

echo "[backup-db] uploading to minio://$MINIO_BUCKET/$filename"
docker run --rm \
  --network "$MINIO_NETWORK" \
  --entrypoint sh \
  -e "MC_HOST_backupsrc=http://${MINIO_ACCESS_KEY}:${MINIO_SECRET_KEY}@${MINIO_CONTAINER}:9000" \
  -v "$BACKUP_DIR:/backups:ro" \
  minio/mc:latest \
  -c "mc mb --ignore-existing backupsrc/$MINIO_BUCKET && mc cp /backups/$filename backupsrc/$MINIO_BUCKET/$filename"

echo "[backup-db] uploaded OK"

echo "[backup-db] pruning local backups older than ${LOCAL_RETENTION_DAYS}d"
find "$BACKUP_DIR" -name "${PG_DB}_*.dump" -mtime "+${LOCAL_RETENTION_DAYS}" -print -delete

echo "[backup-db] done"
