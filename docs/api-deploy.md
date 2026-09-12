# Vera API deployment (`veraserver`)

## What it is

`cmd/veraserver` is a small Echo HTTP service exposing quotation data over
the tailnet. The desktop app will talk to it instead of holding Postgres
credentials (cutover tracked in #80). PDF rendering stays local forever.

| Route | Auth | Purpose |
|---|---|---|
| `GET /healthz` | none | liveness probe |
| `POST /v1/auth/login` | no | `{email,password}` → `{token,user}`; generic 401 on failure |
| `POST /v1/auth/logout` | bearer | revokes the token, `204` |
| `GET /v1/auth/me` | bearer | current user or `401` |

Sessions reuse `backend/auth` exactly: 8h idle expiry, revocation-checked on
every call, login rate limiting (5 failures → cooldown).

## Server layout (`3studio-server`)

| What | Location |
|---|---|
| Compose file | `/home/tristan/vera-api/compose.yml` (mode `600`, holds the DSN) |
| Image | `vera-api:<date>` built from `Dockerfile.veraserver` (`--platform linux/amd64`) |
| Listen | `100.93.103.83:8081` → container `:8080` (Tailscale only) |
| Database | same `vera-postgres` Postgres, `vera` schema |

Transport encryption is provided by WireGuard (Tailscale). Moving behind NPM
with HTTPS requires a public hostname; until one exists, do not publish any
other port.

## Deploy a new build

```bash
# on the dev machine (uncommitted work must be committed first)
docker build --platform linux/amd64 -f Dockerfile.veraserver -t vera-api:YYYYmmdd .
docker save vera-api:YYYYmmdd | ssh 3studio-server-tail 'docker load'

# on the server
cd ~/vera-api
sed -i 's/^\(\s*image: vera-api:\).*/\1YYYYmmdd/' compose.yml
docker compose up -d --force-recreate
curl -s http://100.93.103.83:8081/healthz   # {"status":"ok"}
```

## Smoke test (no test users left behind)

```bash
BASE=http://100.93.103.83:8081
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

Last verified live: 2026-09-12 (login 200, me 200, logout 204, post-logout 401).

## Notes and limits

- Port `8080` on the server is taken by AdGuard; the API uses `8081`.
- Always build with `--platform linux/amd64` — a darwin/arm64 image fails on
  the server with `exec format error`.
- `GET /healthz` is the container healthcheck and the deploy check.
