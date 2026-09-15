# Vera API deployment (`veraserver`)

## What it is

`cmd/veraserver` is a small Echo HTTP service exposing quotation data over
HTTPS. The desktop app talks to it instead of holding Postgres credentials.
PDF rendering stays local forever.

| Route | Auth | Purpose |
|---|---|---|
| `GET /health`, `GET /healthz` | none | liveness probes (no DB touch; note `/healthz` is swallowed by some edges — probe `/health`) |
| `POST /v1/auth/login` | no | `{email,password}` → `{token,user}`; generic 401 on failure |
| `POST /v1/auth/logout` | bearer | revokes the token, `204` |
| `GET /v1/auth/me` | bearer | current user or `401` |
| `POST /v1/auth/redeem` | no | invite/reset code → password set (single-use, 1h) |

Sessions reuse `backend/auth` exactly: 8h idle expiry, revocation-checked on
every call, login rate limiting (5 failures → cooldown).

## Where it runs (Cloud Run, primary)

| What | Value |
|---|---|
| Service | `vera-api`, region `asia-southeast1`, project `vera-app-508411` |
| URL | `https://vera-api-171860943410.asia-southeast1.run.app` |
| Image | `asia-southeast1-docker.pkg.dev/vera-app-508411/vera/veraserver:sha-<commit>` |
| Shape | 1 vCPU / 512 MB, min 0 (cold starts OK), max 2 |
| Env | `VERA_API_ADDR=:8080`, `VERA_POSTGRES_SCHEMA=vera`, `VERA_BACKUP_BUCKET=vera-backups-508411`, `VERA_POSTGRES_DSN` (secret, TLS required) |
| Database | Neon PG18 Singapore, `neondb`, `vera` schema, direct endpoint (never `-pooler`) |

Transport encryption is Cloud Run managed TLS. The desktop holds no
`VERA_POSTGRES_DSN` and never opens a database connection. PDF rendering
stays local. Offline mode is gone by design.

History: 3studio-server (Docker) and Tailscale Funnel hosted this before
Sep 2026; both retired. Nothing project-related remains on that box.

## Deploy (automatic)

Merging to `main` triggers `.github/workflows/deploy.yml`: build
`linux/amd64`, push the `sha-<commit>` tag, deploy image-only (env,
including secrets, persists untouched), then probe `/health` + one
authenticated-route `401`. Any failure fails the run loudly; traffic never
moves to a bad revision. Auth is Workload Identity Federation — no stored
GCP keys. Rollback is the previous Cloud Run revision, one click.

Manual fallback (same checks by hand):

```bash
SHA=$(git rev-parse HEAD)
docker build --platform linux/amd64 -f Dockerfile.veraserver \
  -t asia-southeast1-docker.pkg.dev/vera-app-508411/vera/veraserver:sha-$SHA .
docker push asia-southeast1-docker.pkg.dev/vera-app-508411/vera/veraserver:sha-$SHA
gcloud run deploy vera-api --image ...:sha-$SHA \
  --region asia-southeast1 --platform managed --project=vera-app-508411
BASE=https://vera-api-171860943410.asia-southeast1.run.app
curl -s $BASE/health                                   # {"status":"ok"}
curl -s -o /dev/null -w "%{http_code}\n" $BASE/v1/admin/users   # 401
```

## Smoke test (no test users left behind)

```bash
BASE=https://vera-api-171860943410.asia-southeast1.run.app
# create a temporary user with a known Argon2id hash, then:
curl -s -X POST $BASE/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"<drill>","password":"<drill>"}'      # 200 + token
curl -s $BASE/v1/auth/me -H "Authorization: Bearer <token>"  # 200
curl -s -o /dev/null -w "%{http_code}\n" -X POST $BASE/v1/auth/logout \
  -H "Authorization: Bearer <token>"                 # 204
curl -s -o /dev/null -w "%{http_code}\n" $BASE/v1/auth/me \
  -H "Authorization: Bearer <token>"                 # 401
# delete the drill user afterwards
```

Last verified live: 2026-09-15 (health ok, gates 401, bogus-login generic 401, auto-deploy green end to end).

## Credential rotation (standing procedure)

Rotations happen only as a dance — reset, hand over, confirm live — because
pooled connections mask a dead password for up to an hour while health
checks stay green:

1. Reset the password in Neon console (Roles → `neondb_owner`).
2. Update **both** consumers in one pass: Cloud Run env
   (`gcloud run services update vera-api ... --update-env-vars
   'VERA_POSTGRES_DSN=...'`) and Secret Manager `vera-neon-password`
   (new version; the dumps workflow reads it).
3. Verify: `/health` ok + bogus-login generic 401 (proves fresh connections)
   + one dispatched backup run (proves the secret path).
4. Never rotate solo: an un-updated consumer fails silently behind its pool.

## Notes and limits

- Always build with `--platform linux/amd64` — a darwin/arm64 image fails on
  Cloud Run with `exec format error`.
- Cold starts are normal (min-instances 0); startup retries the DB briefly
  for Neon wake-ups, then fails fast.
- Startup needs a reachable database; a bad DSN fails the revision (traffic
  stays put) — check Cloud Run logs, not the service URL.
