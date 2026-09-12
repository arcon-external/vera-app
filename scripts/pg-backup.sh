#!/usr/bin/env bash
# Nightly Postgres backup for the Vera quotation database.
#
# Usage (on 3studio-server, via cron):
#   PGPASSWORD='<redacted>' /home/tristan/vera-postgres/backup.sh
#
# Configuration via environment (defaults shown):
#   BACKUP_DIR=/home/tristan/backups/vera   destination directory
#   CONTAINER=vera-postgres                 Docker container name
#   PGUSER=vera_app                         database user (read/write is fine)
#   PGDATABASE=vera                         database name
#   PGPASSWORD=(required)                   never commit this value
#   KEEP=14                                 number of dumps to retain
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/home/tristan/backups/vera}"
CONTAINER="${CONTAINER:-vera-postgres}"
PGUSER="${PGUSER:-vera_app}"
PGDATABASE="${PGDATABASE:-vera}"
KEEP="${KEEP:-14}"

if [ -z "${PGPASSWORD:-}" ]; then
  echo "pg-backup: PGPASSWORD is not set; refusing to run" >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
DEST="$BACKUP_DIR/vera-$STAMP.dump"
LOG="$BACKUP_DIR/backup.log"

{
  echo "[$STAMP] starting pg_dump of $PGDATABASE"
  if docker exec -e PGPASSWORD="$PGPASSWORD" "$CONTAINER" \
      pg_dump -U "$PGUSER" -d "$PGDATABASE" -Fc -f "/tmp/vera-$STAMP.dump" \
    && docker cp "$CONTAINER:/tmp/vera-$STAMP.dump" "$DEST" \
    && docker exec "$CONTAINER" rm -f "/tmp/vera-$STAMP.dump"; then
    echo "[$STAMP] wrote $DEST ($(du -h "$DEST" | cut -f1))"
  else
    echo "[$STAMP] BACKUP FAILED" >&2
    exit 1
  fi
  # Retention: keep the newest $KEEP dumps.
  # shellcheck disable=SC2012
  ls -1t "$BACKUP_DIR"/vera-*.dump 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -f
  echo "[$STAMP] retention pass done (keeping $KEEP)"
} >>"$LOG" 2>&1
