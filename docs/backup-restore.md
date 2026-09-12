# Backup & Restore — Vera Postgres

## Where backups live

| What | Location |
|---|---|
| Dump files | `/home/tristan/backups/vera/vera-YYYYmmdd-HHMMSS.dump` (custom format, `pg_dump -Fc`) |
| Run log | `/home/tristan/backups/vera/backup.log` |
| Backup script (server copy) | `/home/tristan/vera-postgres/backup.sh` |
| Backup script (source of truth) | `scripts/pg-backup.sh` in this repo |
| Database password | `/home/tristan/vera-postgres/.env` (`POSTGRES_PASSWORD`) — never committed |

## Schedule

cron on `3studio-server` (timezone WIB), nightly at 02:00:

```cron
0 2 * * * set -a; . $HOME/vera-postgres/.env; set +a; PGPASSWORD=$POSTGRES_PASSWORD PGUSER=$POSTGRES_USER PGDATABASE=$POSTGRES_DB $HOME/vera-postgres/backup.sh
```

Retention: newest **14 dumps** kept; older pruned automatically after each run.

## How to check the last backup succeeded

```bash
ssh 3studio-server-tail 'tail -5 ~/backups/vera/backup.log; ls -lht ~/backups/vera/*.dump | head -3'
```

A healthy run logs `wrote .../vera-<stamp>.dump (<size>)` plus `retention pass done`.
If you see `BACKUP FAILED`, check that the `vera-postgres` container is up and
that `.env` still has `POSTGRES_PASSWORD`.

## Restore from the dashboard (guarded API flow)

Admins can restore without shell access from **Settings → Restore database**:

1. Type `RESTORE` to arm the button (this is the first confirmation).
2. Click **Select backup and restore** and pick a `vera-*.dump` file
   (second confirmation is the file dialog).
3. The server validates the `PGDMP` header **before touching the database**,
   takes a safety snapshot
   (`vera-pre-restore-<stamp>.dump` in the backup dir), then drops and
   restores the schema, verifies row counts, and records a `backup.restore`
   audit entry.
4. **Everyone is signed out immediately** — the sessions table is wiped with
   the schema by design. Sign in again and confirm the archive list.

Non-dump uploads are rejected with 422 and leave the database untouched.
Staff receive 403. If the restore fails halfway, the error names the safety
snapshot — roll back to it with the same flow.

The API container mounts the backup dir **read-write** (snapshots must be
writable) and ships `postgresql-client-16` to match the server version —
`pg_dump` refuses to talk to a newer server, so the client major must stay
in lockstep with `vera-postgres`.

## Restore drill (do this before any real restore)

Always prove the dump against a throwaway database first:

```bash
ssh 3studio-server-tail
export PGPASSWORD="$(grep ^POSTGRES_PASSWORD= ~/vera-postgres/.env | cut -d= -f2-)"

# 1. Create a scratch database
docker exec vera-postgres psql -U vera_app -d vera -c "CREATE DATABASE vera_restore_drill;"

# 2. Restore the newest dump into it
LATEST=$(ls -1t ~/backups/vera/vera-*.dump | head -1)
docker cp "$LATEST" vera-postgres:/tmp/restore-drill.dump
docker exec vera-postgres pg_restore -U vera_app -d vera_restore_drill --no-owner /tmp/restore-drill.dump
docker exec vera-postgres rm -f /tmp/restore-drill.dump

# 3. Verify schema + row counts
docker exec vera-postgres psql -U vera_app -d vera_restore_drill -c "\dt vera.*"
docker exec vera-postgres psql -U vera_app -d vera_restore_drill -Atc \
  "SELECT 'quotations='||COUNT(*) FROM vera.quotations;
   SELECT 'revisions='||COUNT(*) FROM vera.quotation_revisions;"

# 4. Clean up the scratch database
docker exec vera-postgres psql -U vera_app -d vera -c "DROP DATABASE vera_restore_drill;"
```

Last successful drill: 2026-09-12 — restored 2.0 MB dump with the in-image
`pg_restore` 16.15 (vera-api container) into a scratch DB, all 10 `vera.*`
tables present, `quotations=2`, `revisions=3`, scratch DB dropped afterwards.

## Real restore (production `vera` database)

> Stop the app first so nothing writes mid-restore. There is currently no
> auto-deploy; ensure no `wails dev` / desktop instance is running against it.

```bash
ssh 3studio-server-tail
export PGPASSWORD="$(grep ^POSTGRES_PASSWORD= ~/vera-postgres/.env | cut -d= -f2-)"

# 1. Safety dump of the current (broken?) state, just in case
docker exec -e PGPASSWORD="$PGPASSWORD" vera-postgres \
  pg_dump -U vera_app -d vera -Fc -f /tmp/vera-pre-restore.dump
docker cp vera-postgres:/tmp/vera-pre-restore.dump ~/backups/vera/vera-pre-restore.dump

# 2. Restore the chosen good dump over vera
docker cp ~/backups/vera/vera-<stamp>.dump vera-postgres:/tmp/restore.dump
docker exec vera-postgres psql -U vera_app -d vera -c "DROP SCHEMA vera CASCADE;"
docker exec vera-postgres pg_restore -U vera_app -d vera --no-owner /tmp/restore.dump
docker exec vera-postgres rm -f /tmp/restore.dump

# 3. Verify, then restart the app and confirm login + archive list work
docker exec vera-postgres psql -U vera_app -d vera -Atc \
  "SELECT 'quotations='||COUNT(*) FROM vera.quotations;"
```

The app runs migrations at startup, so a slightly old dump self-heals forward
to the current schema on next launch.

## What is NOT covered

- No point-in-time recovery (no WAL archiving) — recovery point is the last
  nightly dump (worst case ~24h of data).
- No off-machine copy yet — dumps live on the same SSD as the database.
  Recommended follow-up: weekly copy to a second disk or object storage.
