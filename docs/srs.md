# Vera — Deployment SRS (for DevOps handoff)

| Field | Value |
|---|---|
| Product | Vera, desktop quotation generator for PT Arcon Penta Persada |
| Version | See `git log`; server image tags are `sha-<full-commit>` (GHCR) or date tags (manual) |
| Audience | DevOps provisioning or taking over this system |
| Goal | Everything needed to provision, deploy, verify, operate, and roll back — from this document alone |
| Language | English (Indonesian version on request) |

## 1. System overview

Three tiers, each replaceable independently:

```
Staff laptops (macOS / Windows desktop app, Wails)
        │  HTTPS + bearer tokens, internet
        ▼
API host (stateless Go service `veraserver`, Docker)
        │  Postgres protocol + TLS, internet
        ▼
Managed Postgres (Neon, Singapore, PG18, schema `vera`)
```

- **Desktop** (`vera-app.app` / `vera-app.exe`): quotation form, PDF rendering (local),
  archive browser, admin screens. Holds **no database credentials** — only a
  remembered session token in the OS keychain; unremembered tokens stay in memory.
- **API** (`cmd/veraserver`, Echo/Go): the sole trust boundary. Owns auth
  (Argon2id, 8h standard / 30d remembered fixed-lifetime sessions, login rate limit 5 failures →
  cooldown), user admin, quotation numbering, archive, backup download/restore,
  audit log. Stateless: all state is in Postgres.
- **Database**: Neon project `vera-app`, region AWS Singapore, Postgres 18,
  database `neondb`, schema `vera`. Plain Postgres, **no extensions**. ~11 MB
  live data (Sep 2026). Migrations run automatically at API startup
  (forward-healing; an older dump self-migrates on restore).

Users and roles: `admin` (team + backup + restore) and `staff` (quotations).
One person per device. No self-registration: accounts are created by an admin,
activated via single-use expiring invite codes.

## 2. Target topology (where this is going)

| Layer | Target | Interim (today) |
|---|---|---|
| API host | $5–6/mo VPS, Singapore (e.g. Vultr Cloud Compute 1vCPU/1GB/25GB) | `3studio-server` (personal box, to be decommissioned from this role) |
| Public URL | `https://<domain>` (TLS via reverse proxy or tunnel) | `https://3studio-server.tailca1027.ts.net:10000` (Tailscale Funnel) + tailnet `:8081` |
| Database | Neon PG18 Singapore (no change) | Same |
| Backups | Neon PITR + nightly `pg_dump` to free object storage | Neon PITR + nightly dumps on API host disk |

Design rules that must survive any move: the database publishes **no host
port** (reachable only from the API host/network); the desktop talks to **one
HTTPS base URL** (override via `VERA_API_URL`); secrets live in **environment
only**, never in images, repos, or logs.

## 3. Infrastructure requirements (API host)

- **Machine:** 1 vCPU / 1 GB RAM / 25 GB SSD minimum. Observed usage: API
  ~20 MB RAM idle; dumps ~2 MB each. Region Singapore (next to the database).
- **OS:** Ubuntu 24.04 LTS (or any Docker-capable Linux). Root or sudo for
  Docker install; afterwards everything runs as an unprivileged user.
- **Runtime:** Docker Engine 25+ with the Compose plugin. No other
  dependencies (no system Postgres, no web server required for MVP).
- **Network (egress, must be open):**
  - `*.neon.tech:5432` TCP (database; TLS).
  - Container registries + package mirrors at build/deploy time
    (GHCR and/or Artifact Registry, `apt.postgresql.org` for the image build).
  - NTP (pool) for TLS/session-expiry correctness.
- **Network (ingress):**
  - Staff path: `443` (or current funnel port) → API container `:8080`.
    Nothing else needs inbound exposure. Do **not** publish `5432`.
  - Health endpoint for monitors: `GET /healthz` (no auth, no DB touch).
- **Disk budget:** images ~300 MB, dumps 2 MB × retention 14 (~30 MB),
  container logs capped by json-file rotation. 25 GB is ample.

## 4. Database requirements (Neon)

- **Project:** `vera-app`, AWS Singapore, **Postgres 18** (the API image pins
  `postgresql-client-18`; `pg_dump` refuses newer servers, so the majors must
  stay in lockstep — see AGENTS.md).
- **Connection:** the **direct** endpoint (not `-pooler`). The app pools
  itself via pgx; the pooled PgBouncer endpoint breaks prepared statements.
  DSN form:
  `postgresql://USER:PASSWORD@<host>.ap-southeast-1.aws.neon.tech/neondb?sslmode=require`
  (`channel_binding=require` also parses with the pinned pgx version).
- **Schema:** `vera` (configurable, default). Created/migrated by the API on
  boot; the DB role needs owner-level rights on its schema (Neon owner role).
- **Backups:** Free tier gives 6-hour PITR — pair it with the nightly dumps
  (Section 8); the two layers cover each other's blind spots.
- **Plan triggers:** stay Free while compute < ~80/100 CU-hours and 6-hour
  history suffices; Launch buys 7-day PITR for single dollars; Scale is only
  for IP-allow/SOC2 demands. IP allowlisting does **not** exist below Scale —
  security is password + enforced TLS until then.

## 5. Application components

### 5.1 `veraserver` (API)

- **Source:** `cmd/veraserver`. **Image:** `Dockerfile.veraserver`
  (`linux/amd64` only — a darwin/arm64 image fails on servers with
  `exec format error`). Ships `postgresql-client-18` (PGDG) for the
  backup/restore endpoints; no other OS packages.
- **Runtime:** single container, `restart: unless-stopped`. Horizontally
  scalable (stateless); Cloud Run deploys with `--min-instances=1 --max-instances=2`.
- **Listens:** `$VERA_API_ADDR` (default `:8080`, all interfaces).
- **Boot sequence (fail-fast, in order):** read config → ping Postgres →
  run migrations → ensure bootstrap admin (only if email+password provided,
  else skip) → listen. Any failure exits non-zero with a credential-redacted
  message; Docker restart policy bridges Postgres boot windows.
- **Routes (all JSON):**

| Route | Auth | Purpose |
|---|---|---|
| `GET /healthz`, `GET /health` | none | Liveness (no DB touch — safe for monitors; `/health` where edges swallow `/healthz`, e.g. Cloud Run) |
| `POST /v1/auth/login` | no | `{email,password}` → `{token,user}`; generic 401, rate-limited |
| `POST /v1/auth/logout` | bearer | revoke token, `204` |
| `GET /v1/auth/me` | bearer | current user or `401` |
| `POST /v1/auth/redeem` | no | `{token,newPassword≥10}` → activate invite/reset code (single-use, 1h) |
| `PATCH /v1/auth/profile` | bearer | display name + avatar (≤512KB, ≤1024px) |
| `PATCH /v1/auth/password` | bearer | password change |
| `POST /v1/auth/logout-everywhere` | bearer | revoke all own sessions |
| `POST /v1/auth/deletion-request` | bearer | self-service deletion request |
| `POST /v1/quotations/prepare` | bearer | validate + price (staff OK) |
| `POST /v1/quotations/export` | bearer | allocate quote number + revision (staff OK) |
| `GET /v1/quotations`, `GET /v1/quotations/:id`, `GET /v1/quotations/:id/revisions` | bearer | archive reads |
| `DELETE /v1/quotations/:id` | admin | delete quotation |
| `PUT /v1/quotations/:id/revisions/:rev/document` | bearer | attach rendered PDF bytes |
| `GET/POST /v1/admin/users`, `PATCH /v1/admin/users/:id/role`, `PATCH .../active`, `POST .../reset`, `DELETE /v1/admin/users/:id` | admin | team management; DELETE is inactive-users-only, never self |
| `GET/POST /v1/admin/deletion-requests...` | admin | deletion queue |
| `GET /v1/admin/backup/latest` | admin | stream newest dump (404 when none) |
| `POST /v1/admin/backup/restore` | admin | validated restore w/ safety snapshot + audit |

### 5.2 Desktop (`vera-app.app` / `vera-app.exe`)

- **Build:** `wails build` on macOS (`.app`) / Windows (`.exe`, or NSIS via
  `wails build -platform windows -nsis` after `brew install nsis`).
  Unsigned: macOS needs right-click → Open on first launch; Windows needs
  *More info → Run anyway*; WebView2 `download` strategy assumed (staff are
  online by definition).
- **Config:** exactly one variable, `VERA_API_URL` (compiled default = public
  staff URL; env overrides for dev/personal; plain HTTP is loopback-only).
  No secrets, no DB access. An update check falls back to the public static
  `latest.json` manifest in `vera-downloads-508411` if the API is unreachable.
- **Distribution:** portable binaries, no installer needed.

## 6. Configuration reference (all environment variables)

| Variable | Where | Required | Default / notes |
|---|---|---|---|
| `VERA_API_URL` | desktop | no | Public staff URL; localhost fallback is dev-only |
| `VERA_API_ADDR` | API | no | `:8080` |
| `VERA_POSTGRES_DSN` | API | **yes** | Full DSN, `sslmode=require` for cloud; **never log, never commit** |
| `VERA_POSTGRES_SCHEMA` | API | no | `vera` |
| `VERA_BOOTSTRAP_ADMIN_EMAIL` / `_NAME` / `_PASSWORD` | API | no | One-time first-admin creation; leave empty afterwards |
| `VERA_BACKUP_DIR` | API | no | Dump staging dir (must be writable for restore snapshots) |
| `VERA_POSTGRES_TEST_DSN` | CI/tests | no | Disposable DB only; must never point at production |
| `VERA_PG_BIN_DIR`, `VERA_NO_KEYRING`, `VERA_RUN_CHROME_TESTS` | tests | no | Local test tooling |

Secrets handling (non-negotiable): mode-`600` files, env only, redacted in
logs (`RedactedDSN`), never pasted into issues/chat. Rotating the DB password
= set it at the provider, update the API env, recreate, verify login shape.

## 7. Deployment procedures

### 7.1 Initial provisioning (new host)

1. Provision the VPS (Singapore, §3), install Docker + Compose plugin.
2. Create the app dir: `compose.yml` (API image, env file mode `600`,
   backup volume, published staff port, `restart: unless-stopped`),
   `.env` holding `VERA_POSTGRES_DSN` (+ bootstrap vars only for first boot).
3. `docker compose up -d`; confirm migrations in logs; `GET /healthz` → ok.
4. Point staff URL (proxy/tunnel/DNS) at the host; verify `401` on
   `/v1/admin/users` without token and generic `401` on bogus login (proves
   the full chain to the database).
5. Bootstrap or verify the admin; remove bootstrap vars from `.env`.

### 7.2 Release (new image)

1. CI builds + pushes (`sha-<commit>` tags only); or build `linux/amd64`
   locally and transfer the image.
2. Update the compose image pin; `docker compose up -d --force-recreate`.
3. Verify: `/healthz` ok, one authenticated-route `401`, bogus-login generic
   `401`. Any failure → set the pin back and recreate (rollback is one line).

### 7.3 Rollback

Image rollback (previous tag + recreate) or, for data, restore a dump via
the dashboard flow or the `docs/backup-restore.md` shell procedure. Keep the
previous image tag until the new one is verified.

## 8. Backup, restore, and disaster recovery

- **Layer 1 (provider):** Neon PITR, 6-hour window on Free. First call for
  same-day mistakes.
- **Layer 2 (self-managed):** nightly `pg_dump -Fc` (cron), 14-dump
  retention, plus dashboard download (`GET /v1/admin/backup/latest`) and
  guarded restore (`POST /v1/admin/backup/restore`: PGDMP validation before
  touching the DB, automatic safety snapshot, row-count verification,
  `backup.restore` audit entry). Restores sign everyone out by design.
- **RPO/RTO at current scale:** worst case ~24h of data (past PITR window),
  recovery ~30–60 min with an operator.
- **Drill requirement:** every restore path must be proven on a scratch
  database before production use; production restores need an explicit
  go-ahead and a quiet window. Details: `docs/backup-restore.md`.

## 9. Monitoring and alerting

- Poll `GET /healthz` (1–5 min interval; UptimeRobot free tier suffices).
  It never touches the DB, so monitoring costs zero compute budget.
- Never poll authenticated endpoints on a timer (keeps Neon/serverless free
  tiers asleep; wakes cost money).
- Review: container restarts (`docker ps`), disk use, backup-log freshness
  (a missing nightly line is an alert by itself), `auth_audit` anomalies.
- For read-only Cloud Run describe/revision/log reconnaissance, dispatch
  `.github/workflows/gcp-recon.yml` (WIF; no local gcloud required).
- Page on: healthz down >5 min, no backup in 26h, disk >80%.

## 10. Security requirements

- Internet exposure is limited to the staff HTTPS URL; the database accepts
  no inbound host ports anywhere.
- Passwords: Argon2id, ≥10 chars on setup; rate-limited login; sessions
  8h standard / 30d remembered, fixed at issuance and revocation-checked per
  request. Logout revokes server-side; remembered tokens live in the OS keychain.
- Abuse posture today is adequate for a team app behind no public signup;
  revisit before any public/exposed-contract use (WAF, lockout review).
- Desktop distribution stays unsigned until the $99 Apple decision; document
  the first-launch bypass in onboarding. No secrets in installers/binaries.
- Audit (`auth_audit`) is append-only evidence: never delete rows; user
  deletion nulls attributions by schema design (`SET NULL`) while entries stay.

## 11. Acceptance criteria (DevOps done-definition)

- [ ] Fresh host provisioned per §3; `docker compose up -d` from stored files only.
- [ ] `/healthz` ok; admin routes `401` without token; bogus login generic `401`.
- [ ] Admin sign-in works; archive shows expected quotations.
- [ ] Nightly dump appears and retention holds; dashboard download byte-matches.
- [ ] Restore proven on a scratch database (never straight to production first).
- [ ] Monitoring polls `/healthz` with an alert path to a human.
- [ ] No secrets in repo, images, logs, or chat; compose env is mode `600`.
- [ ] This document updated with any deviation found during provisioning.
