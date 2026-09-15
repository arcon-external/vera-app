# Backup & Restore — Vera Postgres (Neon)

Two independent layers. Either one alone recovers the system; together
they cover each other's blind spots.

## Layer 1 — Neon PITR (minutes)

Provider point-in-time recovery, 6-hour window on the Free tier. First call
for same-day mistakes. Console → Branches → restore, or create a branch from
a past timestamp to inspect before rewinding.

## Layer 2 — Nightly dumps (days)

| What | Location |
|---|---|
| Dump files | `gs://vera-backups-508411/vera-YYYYmmdd-HHMMSS.dump` (custom format, `pg_dump -Fc`) |
| Job | `.github/workflows/backup.yml` (scheduled 02:00 WIB + manual dispatch) |
| DB password | Secret Manager `vera-neon-password` (read at runtime via WIF; masked in logs) |

Retention: newest **14 dumps** kept; older pruned automatically after each run.

## Schedule

GitHub Actions cron (`0 19 * * *` UTC = 02:00 WIB), plus
`gh workflow run backup.yml --ref main` anytime. The job authenticates with
Workload Identity Federation — no stored cloud keys; the DB password is
fetched from Secret Manager at runtime and never logged.

## How to check the last backup succeeded

```bash
gcloud storage ls -l gs://vera-backups-508411/ | tail -3
# or: Actions tab → "Nightly database dump" → latest run green
```

A healthy run uploads `vera-<stamp>.dump` (~2–3 MB) and prunes past 14.
If a run fails, the run log names the step (auth, dump, upload, prune).

To verify a dump's integrity without touching production:

```bash
gcloud storage cp gs://vera-backups-508411/vera-<stamp>.dump /tmp/check.dump
head -c 5 /tmp/check.dump   # want PGDMP
pg_restore --list /tmp/check.dump | grep -c "TABLE DATA"
```

## Restore from the dashboard (guarded API flow)

Admins restore from **Settings → Restore database** (no shell needed):

1. Type `RESTORE` to arm the button (first confirmation).
2. **Select backup and restore**, pick a `vera-*.dump` file (the file dialog
   is the second confirmation).
3. The server validates the `PGDMP` header **before touching the database**,
   takes a safety snapshot, then drops and restores the schema, verifies row
   counts, and records a `backup.restore` audit entry.
4. **Everyone is signed out immediately** — the sessions table is wiped with
   the schema by design. Sign in again and confirm the archive list.

Non-dump uploads are rejected with 422 and leave the database untouched.
Staff receive 403. If the restore fails halfway, the error names the safety
snapshot — roll back to it with the same flow.

> Known limit: safety snapshots currently land on the API instance's
> ephemeral disk, not in the bucket — they survive the restore at hand but
> not a later instance recycle. Bucket snapshots are tracked work, not yet
> built. Until then, download a fresh dump yourself before any production
> restore and keep it local.

## Restore drill (do this before any real restore)

Prove any dump against scratch first. Cheapest sandbox is a Neon branch
(free, seconds, disposable); alternatively a local Postgres 18:

```bash
# Local scratch drill with the repo's pinned toolchain:
createdb -h localhost -U vera vera_restore_drill   # local PG18
pg_restore -U vera -d vera_restore_drill --no-owner /tmp/check.dump
psql -U vera -d vera_restore_drill -Atc \
  "SELECT 'quotations='||COUNT(*) FROM vera.quotations;
   SELECT 'revisions='||COUNT(*) FROM vera.quotation_revisions;"
dropdb -h localhost -U vera vera_restore_drill
```

Last successful drill: 2026-09-14 — production dump restored into scratch,
`quotations=2`, `revisions=3`, scratch dropped afterwards.

## Real restore (production)

Prefer the dashboard flow above. Shell fallback (needs the DB password and
a PG18 client):

```bash
export PGPASSWORD='<from Secret Manager, never chat>'
export PGHOST=ep-frosty-field-b3szqzqb.c-4.ap-southeast-1.aws.neon.tech
# 1. Safety dump of the current state first:
pg_dump -h $PGHOST -U neondb_owner -d neondb -Fc -f /tmp/vera-pre-restore.dump
# 2. Restore the chosen good dump over neondb:
psql -h $PGHOST -U neondb_owner -d neondb -c "DROP SCHEMA vera CASCADE;"
pg_restore -h $PGHOST -U neondb_owner -d neondb --no-owner /tmp/restore.dump
# 3. Verify counts, sign in, confirm the archive list.
```

The API runs migrations at startup, so a slightly old dump self-heals
forward to the current schema on next launch.

## What is NOT covered

- PITR window is 6 hours on Free — anything older needs the nightly dumps.
  That is precisely why both layers exist.
- No cross-provider copy: Neon holds the live data and PITR; the bucket
  holds dumps. A simultaneous loss of both is accepted as out of scope at
  current scale (revisit if contracts demand it).
