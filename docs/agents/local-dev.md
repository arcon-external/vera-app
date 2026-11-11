# Local development workspace

The agent workspace is a persistent directory — default
`~/dev/vera-agent` — that holds everything a local run needs, so a `/tmp`
cleanup cannot take the agent Postgres, the API logs, the Chrome wrappers or
the state file with it. `scripts/agent-env.sh` is the only entry point.

```bash
scripts/agent-env.sh up          # postgres + API + wrappers, write the state file
scripts/agent-env.sh status      # every component and its health
scripts/agent-env.sh status --json
scripts/agent-env.sh check       # verify the stack; no side effects
scripts/agent-env.sh check --json
scripts/agent-env.sh down        # stop only what this workspace started
scripts/agent-env.sh reset --yes # drop and recreate the agent schema
```

`status` always exits 0. `check` exits 0 only when every component is healthy.
`up` exits 1 when the stack is not fully up. Usage errors exit 2. `reset`
refuses to run without `--yes`, and it rebuilds the server before dropping the
schema: a missing `go` refuses before anything is destroyed.

## What `up` starts

| Component | Value |
| --- | --- |
| Container | `vera-agent-postgres` (`postgres:18-alpine`, loopback only) |
| Volume | `vera-agent-pg`, mounted at `/var/lib/postgresql` |
| Postgres | `127.0.0.1:55432`, user `vera`, password `vera`, database `veratest` |
| API | `127.0.0.1:8081`, `veraserver --local-development`, schema `vera` |
| Signing key | `$VERA_AGENT_HOME/secrets/signing-key` (0600, generated once) |
| Admin bootstrap | `admin@vera.local`; password in `secrets/admin-password` (0600) |
| Chrome wrapper | `$VERA_AGENT_HOME/chrome-wrapper/chrome` for `ui-snapshot.sh` |
| PDF wrapper | `$VERA_AGENT_HOME/bin/google-chrome` for chromedp |

The password `vera` is the documented throwaway local credential from
`docs/local-seed.md`; the database is disposable, loopback-only and never
production. The signing key and the admin password are generated on first
`up`, stored `0600`, and never printed by any command. The state file contains
no secrets.

`--local-development` makes the server refuse a remote, multi-host or
unparseable Postgres target before it opens the store, so a stray production
DSN in the environment cannot migrate or seed anything here.

If another local Postgres already holds `55432` — the old ad-hoc `frank518-pg`
does — set `VERA_AGENT_PG_PORT` to a free port before `up`. The DSN, the
container mapping and the state file all follow it. The same applies to
`VERA_AGENT_API_PORT` when something already answers on `8081`.

## Test DSN

```bash
export VERA_POSTGRES_TEST_DSN='postgres://vera:vera@127.0.0.1:55432/veratest?sslmode=disable'

go test ./backend/store -run TestQuotationRevisionPersistence -v
go test ./backend/api ./cmd/vera -v
```

The database is throwaway. `reset --yes` drops and recreates the `vera`
schema and restarts the API so migrations and the admin bootstrap run again.
A full wipe is `docker volume rm vera-agent-pg` after `down`.

## Admin sign-in

The API creates the first admin only when the database has no users:

```bash
cat "$VERA_AGENT_HOME/secrets/admin-password"
```

Use `admin@vera.local` with that password. Once any user exists the bootstrap
variables are ignored; `reset --yes` empties the schema and recreates the admin
on the next API start.

## Environment file

`up` writes `$VERA_AGENT_HOME/env` with the non-secret variables:

```bash
set -a; . "$VERA_AGENT_HOME/env"; set +a
```

It carries `VERA_API_URL`, `VERA_POSTGRES_DSN` and `VERA_POSTGRES_SCHEMA`. The
signing key and the admin password stay in `secrets/` and are passed to the
server by `up`.

## Chrome and PDFs

`up` writes both wrappers; it does not download Chrome. Put the pinned build at
the snapshot script's default:

```text
$VERA_AGENT_HOME/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64/chrome
```

`scripts/ui-snapshot.sh` defaults its output, Chrome and wrapper paths to
`$VERA_AGENT_HOME` (`ui-snap/`, `chrome-dl/…`, `chrome-wrapper/`). The PDF shim
lets chromedp find that build:

```bash
PATH="$VERA_AGENT_HOME/bin:$PATH" go run -tags chromedp ./cmd/vera --help
```

A missing Chrome binary is reported by `status` as `chrome: missing` and fails
`check`; the rest of the stack still starts.

## Status shape

`status --json` is one object with `home`, `state` (`up`, `degraded`, `down`)
and the `postgres`, `api`, `chrome` and `wrappers` components. Each component
carries a `state`; `api` also carries `pid`, `pid_alive`, `url` and `log`, and
`postgres` carries the container, volume, image, host, port, database and user.

`check --json` is `{"home", "ok", "checks": [{"name", "ok", "detail"}]}`. It
checks docker, postgres, `pg_isready`, the schema, the API health endpoint, the
state file, both secrets, Chrome and both wrappers.

## Layout

| Path | Contents |
| --- | --- |
| `state.json` | container, volume, API pid, DSN, schema; no secrets (0600) |
| `env` | non-secret exports for shells and tests |
| `secrets/signing-key` | disposable Ed25519 seed (0600) |
| `secrets/admin-password` | first-admin password (0600) |
| `logs/api.log` | veraserver stdout/stderr |
| `bin/veraserver` | the built dev server |
| `bin/google-chrome` | chromedp PDF shim |
| `chrome-wrapper/chrome` | container-side snapshot wrapper |
| `chrome-dl/` | the pinned unpacked Chrome |
| `ui-snap/` | `ui-snapshot.sh` output |

## Safety

- `down` acts only on the API pid and container recorded in `state.json`; it
  never names another container or process.
- `check` and `status` write nothing and start nothing.
- `reset` drops only the `vera` schema in `veratest` and refuses without
  `--yes`.
- No command prints the signing key or the admin password.

## Browser E2E

The browser pass — agent API, the Mac's `wails dev`, and browse over the SSH
tunnel — is in `docs/agents/e2e-runbook.md`. `scripts/browser-e2e.sh --check`
prints every step and names a missing prerequisite without starting anything.

## Tests

```bash
bash scripts/agent-env.test.sh
```

The test runs the script against a temporary `VERA_AGENT_HOME` and fake
`docker`, `curl` and `go` binaries in a temporary `PATH`. It starts no real
container, database, API or browser. It covers the `status --json` shape,
`check` being side-effect-free, `up`/`down` idempotence, `down` touching only
its own container, `reset --yes`, and the absence of the generated secrets in
every output and in the state/env files.
