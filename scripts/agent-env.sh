#!/usr/bin/env bash
# agent-env.sh — one command for the persistent agent workspace.
#
# The workspace lives outside /tmp so a temporary-directory cleanup cannot take
# the worktrees, the pinned Chrome, the agent Postgres volume or the API logs
# with it. VERA_AGENT_HOME defaults to ~/dev/vera-agent.
#
#   scripts/agent-env.sh up            start postgres, the API and the wrappers
#   scripts/agent-env.sh status        report every component (--json)
#   scripts/agent-env.sh check         verify the stack, no side effects (--json)
#   scripts/agent-env.sh down          stop only what this workspace started
#   scripts/agent-env.sh reset --yes   drop and recreate the agent schema
#
# The agent database is disposable and loopback-only. The API starts with
# --local-development, which refuses a non-local Postgres target before it
# opens the store. No command prints the signing key or the admin password.
# See docs/agents/local-dev.md.
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: scripts/agent-env.sh <command> [--json]

Commands:
  up              Start the agent Postgres, the dev API and the Chrome/PDF
                  wrappers, then write the state file.
  down            Stop only the API and container this workspace started.
  status          Print every component and its health. Exit 0.
  check           Verify the stack without side effects. Exit 1 when unhealthy.
  reset --yes     Drop and recreate the agent schema, then restart the API.

Options:
  --json          Machine-readable output on stdout.
  --yes           Required by reset; the reset is destructive.
  -h, --help      Print this help.

Environment:
  VERA_AGENT_HOME       Workspace directory (default ~/dev/vera-agent).
  VERA_AGENT_PG_PORT    Host port for the agent Postgres (default 55432).
  VERA_AGENT_API_PORT   Host port for the dev API (default 8081).
  VERA_AGENT_PG_IMAGE   Postgres image (default postgres:18-alpine).
  VERA_AGENT_CHROME_DIR Unpacked chrome-linux64 directory.

The agent DSN is the one in docs/local-seed.md:
  postgres://vera:vera@127.0.0.1:55432/veratest?sslmode=disable
USAGE
}

command_name=""
json=0
assume_yes=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --json) json=1 ;;
    --yes|-y) assume_yes=1 ;;
    -h|--help) usage; exit 0 ;;
    up|down|status|check|reset)
      if [[ -n "$command_name" ]]; then
        printf 'agent-env: unexpected argument %s\n' "$1" >&2
        usage >&2
        exit 2
      fi
      command_name="$1"
      ;;
    *)
      printf 'agent-env: unknown argument %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ -z "$command_name" ]]; then
  usage >&2
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  printf 'agent-env: python3 is required for the state file\n' >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

home="${VERA_AGENT_HOME:-$HOME/dev/vera-agent}"
container="${VERA_AGENT_CONTAINER:-vera-agent-postgres}"
volume="${VERA_AGENT_VOLUME:-vera-agent-pg}"
image="${VERA_AGENT_PG_IMAGE:-postgres:18-alpine}"
pg_host="127.0.0.1"
pg_port="${VERA_AGENT_PG_PORT:-55432}"
pg_database="veratest"
pg_user="vera"
# The throwaway local credential documented in docs/local-seed.md. It is not a
# secret: the database is loopback-only, disposable and never production.
pg_password="vera"
api_host="127.0.0.1"
api_port="${VERA_AGENT_API_PORT:-8081}"
schema="${VERA_AGENT_SCHEMA:-vera}"
chrome_dir="${VERA_AGENT_CHROME_DIR:-$home/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64}"

state_file="$home/state.json"
env_file="$home/env"
log_file="$home/logs/api.log"
signing_key_file="$home/secrets/signing-key"
admin_password_file="$home/secrets/admin-password"
admin_email="${VERA_AGENT_ADMIN_EMAIL:-admin@vera.local}"
admin_name="${VERA_AGENT_ADMIN_NAME:-Vera Agent Admin}"
chrome_wrapper="$home/chrome-wrapper/chrome"
pdf_wrapper="$home/bin/google-chrome"
server_bin="$home/bin/veraserver"

dsn="postgres://$pg_user:$pg_password@$pg_host:$pg_port/$pg_database?sslmode=disable"
api_url="http://$api_host:$api_port"

if ! [[ "$schema" =~ ^[a-z_][a-z0-9_]*$ ]]; then
  printf 'agent-env: invalid schema %q: use lowercase letters, digits, and underscores\n' "$schema" >&2
  exit 2
fi

for port in "$pg_port" "$api_port"; do
  if ! [[ "$port" =~ ^[0-9]+$ ]] || [[ "$port" -lt 1 || "$port" -gt 65535 ]]; then
    printf 'agent-env: invalid port %q: use a number between 1 and 65535\n' "$port" >&2
    exit 2
  fi
done

err() { printf 'agent-env: %s\n' "$*" >&2; }
have() { command -v "$1" >/dev/null 2>&1; }
timestamp() { date -u +%Y-%m-%dT%H:%M:%SZ; }

pid_alive() {
  local pid="${1:-}"
  [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null
}

pid_is_ours() {
  local pid="${1:-}" cmd
  pid_alive "$pid" || return 1
  cmd="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  [[ "$cmd" == *"$server_bin"* ]]
}

container_exists() {
  docker inspect --format '{{.State.Running}}' "$container" >/dev/null 2>&1
}

container_running() {
  local value
  value="$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null)" || return 1
  [[ "$value" == "true" ]]
}

api_health() {
  curl -fsS --max-time 2 "$api_url/healthz" >/dev/null 2>&1
}

state_value() {
  [[ -f "$state_file" ]] || return 1
  python3 -c '
import json, sys
try:
    with open(sys.argv[1], encoding="utf-8") as handle:
        data = json.load(handle)
except Exception:
    sys.exit(1)
value = data.get(sys.argv[2], "")
if value is None:
    value = ""
print(value)
' "$state_file" "$1"
}

write_state() {
  local api_pid="${1:-}" started_at="${2:-}"
  AE_HOME="$home" AE_CONTAINER="$container" AE_VOLUME="$volume" AE_IMAGE="$image" \
    AE_DSN="$dsn" AE_SCHEMA="$schema" AE_API_PID="$api_pid" AE_API_URL="$api_url" \
    AE_API_LOG="$log_file" AE_STARTED_AT="$started_at" python3 <<'PY'
import json, os

env = os.environ
state = {
    "version": 1,
    "home": env["AE_HOME"],
    "container": env["AE_CONTAINER"],
    "volume": env["AE_VOLUME"],
    "image": env["AE_IMAGE"],
    "dsn": env["AE_DSN"],
    "schema": env["AE_SCHEMA"],
    "api_pid": int(env["AE_API_PID"]) if env["AE_API_PID"] else None,
    "api_url": env["AE_API_URL"],
    "api_log": env["AE_API_LOG"],
    "started_at": env["AE_STARTED_AT"],
}
path = os.path.join(env["AE_HOME"], "state.json")
with open(path, "w", encoding="utf-8") as handle:
    json.dump(state, handle, indent=2, sort_keys=True)
    handle.write("\n")
PY
  chmod 600 "$state_file"
}

write_env_file() {
  cat > "$env_file" <<EOF
# Agent workspace environment, written by scripts/agent-env.sh up.
# Load it with:
#   set -a; . "$env_file"; set +a
VERA_API_URL=$api_url
VERA_POSTGRES_DSN=$dsn
VERA_POSTGRES_SCHEMA=$schema
EOF
  chmod 644 "$env_file"
}

ensure_secret_file() {
  local file="$1" bytes="$2" value=""
  mkdir -p "$(dirname "$file")"
  if [[ ! -s "$file" ]]; then
    if have openssl; then
      value="$(openssl rand -base64 "$bytes")"
    else
      value="$(head -c "$bytes" /dev/urandom | base64 | tr -d '\n')"
    fi
    printf '%s\n' "$value" >"$file"
  fi
  chmod 600 "$file"
}

write_wrappers() {
  mkdir -p "$(dirname "$chrome_wrapper")" "$(dirname "$pdf_wrapper")"

  cat >"$chrome_wrapper" <<'WRAPPER'
#!/bin/sh
# Container-side Chrome wrapper used by scripts/ui-snapshot.sh: the wrapper is
# mounted at /wrapper and the unpacked Chrome at /chrome.
# Written by scripts/agent-env.sh up.
exec /chrome/chrome --no-sandbox "$@"
WRAPPER
  chmod +x "$chrome_wrapper"

  cat >"$pdf_wrapper" <<WRAPPER
#!/bin/sh
# Host-side Chrome shim for chromedp: its default allocator looks for
# google-chrome on PATH, so \`PATH="$home/bin:\$PATH"\` makes
# \`go run -tags chromedp ./cmd/vera\` render with the pinned agent Chrome.
# Written by scripts/agent-env.sh up.
chrome="$chrome_dir/chrome"
if [ ! -x "\$chrome" ]; then
  echo "agent-env: Chrome not found at \$chrome" >&2
  exit 1
fi
exec "\$chrome" --no-sandbox "\$@"
WRAPPER
  chmod +x "$pdf_wrapper"
}

collect_status() {
  status_pg="down"
  if ! have docker; then
    status_pg="unknown"
  elif ! docker info --format '{{.ServerVersion}}' >/dev/null 2>&1; then
    status_pg="unknown"
  elif container_running; then
    status_pg="up"
  fi

  status_api_pid="$(state_value api_pid 2>/dev/null || true)"
  status_api_pid_alive="false"
  if pid_alive "$status_api_pid"; then
    status_api_pid_alive="true"
  fi

  status_api="down"
  if [[ "$status_api_pid_alive" == "true" ]] && api_health; then
    status_api="up"
  elif [[ "$status_api_pid_alive" == "true" ]]; then
    status_api="degraded"
  elif api_health; then
    status_api="foreign"
  fi

  status_chrome="missing"
  if [[ -x "$chrome_dir/chrome" ]]; then
    status_chrome="ready"
  fi

  status_wrappers="missing"
  if [[ -x "$chrome_wrapper" && -x "$pdf_wrapper" ]]; then
    status_wrappers="ready"
  fi

  # Wrappers are static assets, not a running component: a workspace with the
  # wrappers written but nothing running is down, not degraded.
  status_overall="down"
  if [[ "$status_pg" == "up" && "$status_api" == "up" && "$status_wrappers" == "ready" ]]; then
    status_overall="up"
  elif [[ "$status_pg" == "up" || "$status_api" == "up" || "$status_api" == "degraded" ||
    "$status_api" == "foreign" ]]; then
    status_overall="degraded"
  fi
}

print_status_json() {
  AE_HOME="$home" AE_OVERALL="$status_overall" AE_PG="$status_pg" \
    AE_CONTAINER="$container" AE_VOLUME="$volume" AE_IMAGE="$image" \
    AE_PG_HOST="$pg_host" AE_PG_PORT="$pg_port" AE_PG_DATABASE="$pg_database" \
    AE_PG_USER="$pg_user" AE_API="$status_api" AE_API_URL="$api_url" \
    AE_API_PID="$status_api_pid" AE_API_PID_ALIVE="$status_api_pid_alive" \
    AE_API_LOG="$log_file" AE_CHROME="$status_chrome" AE_CHROME_DIR="$chrome_dir" \
    AE_WRAPPERS="$status_wrappers" AE_CHROME_WRAPPER="$chrome_wrapper" \
    AE_PDF_WRAPPER="$pdf_wrapper" python3 <<'PY'
import json, os

env = os.environ
print(json.dumps({
    "home": env["AE_HOME"],
    "state": env["AE_OVERALL"],
    "postgres": {
        "state": env["AE_PG"],
        "container": env["AE_CONTAINER"],
        "volume": env["AE_VOLUME"],
        "image": env["AE_IMAGE"],
        "host": env["AE_PG_HOST"],
        "port": int(env["AE_PG_PORT"]),
        "database": env["AE_PG_DATABASE"],
        "user": env["AE_PG_USER"],
    },
    "api": {
        "state": env["AE_API"],
        "url": env["AE_API_URL"],
        "pid": int(env["AE_API_PID"]) if env["AE_API_PID"] else None,
        "pid_alive": env["AE_API_PID_ALIVE"] == "true",
        "log": env["AE_API_LOG"],
    },
    "chrome": {
        "state": env["AE_CHROME"],
        "path": env["AE_CHROME_DIR"],
    },
    "wrappers": {
        "state": env["AE_WRAPPERS"],
        "chrome": env["AE_CHROME_WRAPPER"],
        "pdf": env["AE_PDF_WRAPPER"],
    },
}, indent=2, sort_keys=True))
PY
}

print_status_human() {
  local pid_text=""
  if [[ -n "$status_api_pid" ]]; then
    pid_text=", pid $status_api_pid"
  fi
  printf 'agent home  %s\n' "$home"
  printf 'postgres   %-9s %s (%s:%s/%s, user %s)\n' \
    "$status_pg" "$container" "$pg_host" "$pg_port" "$pg_database" "$pg_user"
  printf 'api        %-9s %s%s\n' "$status_api" "$api_url" "$pid_text"
  printf 'chrome     %-9s %s\n' "$status_chrome" "$chrome_dir"
  printf 'wrappers   %-9s %s, %s\n' "$status_wrappers" "$chrome_wrapper" "$pdf_wrapper"
  printf 'overall    %s\n' "$status_overall"
}

build_server() {
  if ! have go; then
    err "go is required to build veraserver"
    return 1
  fi
  mkdir -p "$(dirname "$server_bin")"
  if ! (cd "$repo_root" && go build -o "$server_bin" ./cmd/veraserver); then
    err "go build ./cmd/veraserver failed"
    return 1
  fi
}

start_postgres() {
  if ! have docker; then
    err "docker is required"
    return 1
  fi
  if ! docker info --format '{{.ServerVersion}}' >/dev/null 2>&1; then
    err "the Docker daemon is not reachable"
    return 1
  fi

  if container_exists; then
    if ! container_running; then
      if ! docker start "$container" >/dev/null; then
        err "docker start failed for $container"
        return 1
      fi
    fi
  else
    if ! docker run -d \
      --name "$container" \
      --label vera-agent=postgres \
      -p "$pg_host:$pg_port:5432" \
      -e POSTGRES_USER="$pg_user" \
      -e POSTGRES_PASSWORD="$pg_password" \
      -e POSTGRES_DB="$pg_database" \
      --mount "type=volume,source=$volume,target=/var/lib/postgresql" \
      "$image" >/dev/null; then
      err "docker run failed for $container (is $pg_host:$pg_port already in use?)"
      return 1
    fi
  fi

  # Record the container before waiting: a slow or failed start still leaves
  # down a record of what up created. Keep the API pid an earlier up recorded,
  # or the reuse check would mistake our own server for a foreign one.
  local recorded_pid=""
  recorded_pid="$(state_value api_pid 2>/dev/null || true)"
  write_state "$recorded_pid" "$(timestamp)"

  local attempt=0
  while [[ "$attempt" -lt 60 ]]; do
    if docker exec "$container" pg_isready -U "$pg_user" -d "$pg_database" >/dev/null 2>&1; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  err "postgres did not become ready within 60s (container $container)"
  return 1
}

stop_api_pid() {
  local pid="${1:-}"
  pid_is_ours "$pid" || return 0
  kill "$pid" 2>/dev/null || true
  local attempt=0
  while [[ "$attempt" -lt 100 ]]; do
    pid_alive "$pid" || return 0
    attempt=$((attempt + 1))
    sleep 0.1
  done
  kill -9 "$pid" 2>/dev/null || true
  return 0
}

start_api() {
  local existing_pid=""
  existing_pid="$(state_value api_pid 2>/dev/null || true)"
  if pid_is_ours "$existing_pid" && api_health; then
    printf '%s\n' "$existing_pid"
    return 0
  fi
  if api_health; then
    # A lost state file must not make this workspace's own server look
    # foreign: adopt the process if its command names our binary.
    local found_pid=""
    if have pgrep; then
      found_pid="$(pgrep -f "$server_bin" 2>/dev/null | head -n 1 || true)"
    fi
    if pid_is_ours "$found_pid"; then
      printf '%s\n' "$found_pid"
      return 0
    fi
    err "$api_url already answers /healthz but is not this workspace's API; stop it or set VERA_AGENT_API_PORT"
    return 1
  fi
  if pid_alive "$existing_pid"; then
    stop_api_pid "$existing_pid"
  fi

  build_server || return 1
  mkdir -p "$(dirname "$log_file")"
  printf '\n==> agent-env up %s\n' "$(timestamp)" >>"$log_file"

  (
    export VERA_API_ADDR="$api_host:$api_port"
    export VERA_POSTGRES_DSN="$dsn"
    export VERA_POSTGRES_SCHEMA="$schema"
    VERA_SIGNING_KEY="$(cat "$signing_key_file")"
    export VERA_SIGNING_KEY
    export VERA_BOOTSTRAP_ADMIN_EMAIL="$admin_email"
    export VERA_BOOTSTRAP_ADMIN_NAME="$admin_name"
    VERA_BOOTSTRAP_ADMIN_PASSWORD="$(cat "$admin_password_file")"
    export VERA_BOOTSTRAP_ADMIN_PASSWORD
    export VERA_PUBLIC_BASE_URL="$api_url"
    exec "$server_bin" --local-development
  ) >>"$log_file" 2>&1 &
  local pid=$!

  local attempt=0
  while [[ "$attempt" -lt 120 ]]; do
    if api_health; then
      if pid_alive "$pid"; then
        printf '%s\n' "$pid"
        return 0
      fi
    fi
    if ! pid_alive "$pid"; then
      err "the API exited during startup; last log lines from $log_file:"
      tail -n 20 "$log_file" >&2 || true
      return 1
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  err "the API did not answer $api_url/healthz within 120s; see $log_file"
  return 1
}

cmd_up() {
  mkdir -p "$home" "$home/logs" "$home/secrets" "$home/bin" "$home/chrome-wrapper"
  ensure_secret_file "$signing_key_file" 32
  ensure_secret_file "$admin_password_file" 24
  write_env_file
  write_wrappers

  start_postgres || return 1

  local pid=""
  if ! pid="$(start_api)"; then
    err "the API did not start; the container is recorded in $state_file (run down to clean up)"
    return 1
  fi
  write_state "$pid" "$(timestamp)"

  collect_status
  if [[ "$json" == "1" ]]; then
    print_status_json
  else
    print_status_human
  fi
  if [[ "$status_chrome" != "ready" ]]; then
    err "warning: Chrome is not installed at $chrome_dir/chrome; snapshots and chromedp PDFs will not run until it is"
  fi
  if [[ "$status_overall" != "up" ]]; then
    err "the stack is not fully up"
    return 1
  fi
  return 0
}

cmd_down() {
  local stopped=0
  if [[ -f "$state_file" ]]; then
    local pid=""
    pid="$(state_value api_pid 2>/dev/null || true)"
    if pid_is_ours "$pid"; then
      stop_api_pid "$pid"
      stopped=1
    fi
    if have docker; then
      if container_exists; then
        docker rm -f "$container" >/dev/null
        stopped=1
      fi
      rm -f "$state_file"
    else
      err "docker is not available; keeping $state_file so a later down can remove $container"
      return 1
    fi
  fi

  collect_status
  if [[ "$json" == "1" ]]; then
    print_status_json
  else
    print_status_human
  fi
  if [[ "$stopped" == "0" ]]; then
    # stderr, so --json keeps stdout a single JSON document.
    err "nothing to stop"
  fi
  return 0
}

cmd_status() {
  collect_status
  if [[ "$json" == "1" ]]; then
    print_status_json
  else
    print_status_human
  fi
  return 0
}

check_names=()
check_oks=()
check_details=()

add_check() {
  check_names+=("$1")
  check_oks+=("$2")
  check_details+=("$3")
}

cmd_check() {
  collect_status
  check_names=()
  check_oks=()
  check_details=()
  local ok="true"

  if have docker && docker info --format '{{.ServerVersion}}' >/dev/null 2>&1; then
    add_check docker true "daemon reachable"
  else
    add_check docker false "docker is missing or the daemon is not reachable"
    ok="false"
  fi

  if [[ "$status_pg" == "up" ]]; then
    add_check postgres true "$container is running"
  else
    add_check postgres false "$container is not running"
    ok="false"
  fi

  if [[ "$status_pg" == "up" ]] &&
    docker exec "$container" pg_isready -U "$pg_user" -d "$pg_database" >/dev/null 2>&1; then
    add_check postgres-ready true "pg_isready accepted $pg_database"
  else
    add_check postgres-ready false "pg_isready did not accept $pg_database"
    ok="false"
  fi

  local schema_present=""
  if [[ "$status_pg" == "up" ]]; then
    schema_present="$(docker exec "$container" psql -U "$pg_user" -d "$pg_database" \
      -tAc "SELECT 1 FROM information_schema.schemata WHERE schema_name = '$schema'" 2>/dev/null |
      tr -d '[:space:]' || true)"
  fi
  if [[ "$schema_present" == "1" ]]; then
    add_check schema true "$schema exists in $pg_database"
  else
    add_check schema false "$schema is missing from $pg_database"
    ok="false"
  fi

  if [[ "$status_api" == "up" ]]; then
    add_check api true "$api_url/healthz answers"
  else
    add_check api false "$api_url/healthz does not answer from this workspace's API"
    ok="false"
  fi

  local state_pid=""
  state_pid="$(state_value api_pid 2>/dev/null || true)"
  if [[ -f "$state_file" ]] && pid_is_ours "$state_pid"; then
    add_check state-file true "$state_file records pid $state_pid"
  else
    add_check state-file false "$state_file is missing or does not record a live API pid"
    ok="false"
  fi

  if [[ -s "$signing_key_file" ]]; then
    add_check signing-key true "$signing_key_file"
  else
    add_check signing-key false "$signing_key_file is missing"
    ok="false"
  fi

  if [[ -s "$admin_password_file" ]]; then
    add_check admin-password true "$admin_password_file"
  else
    add_check admin-password false "$admin_password_file is missing"
    ok="false"
  fi

  if [[ -x "$chrome_dir/chrome" ]]; then
    add_check chrome true "$chrome_dir/chrome"
  else
    add_check chrome false "Chrome is missing at $chrome_dir/chrome"
    ok="false"
  fi

  if [[ -x "$chrome_wrapper" && -x "$pdf_wrapper" ]]; then
    add_check wrappers true "$chrome_wrapper, $pdf_wrapper"
  else
    add_check wrappers false "the Chrome/PDF wrappers are missing"
    ok="false"
  fi

  if [[ "$json" == "1" ]]; then
    {
      local i
      for i in "${!check_names[@]}"; do
        printf '%s\t%s\t%s\n' "${check_oks[$i]}" "${check_names[$i]}" "${check_details[$i]}"
      done
    } | AE_HOME="$home" python3 -c '
import json, os, sys
checks = []
for line in sys.stdin:
    ok, name, detail = line.rstrip("\n").split("\t", 2)
    checks.append({"name": name, "ok": ok == "true", "detail": detail})
print(json.dumps({
    "home": os.environ["AE_HOME"],
    "ok": all(check["ok"] for check in checks),
    "checks": checks,
}, indent=2))
'
  else
    local i
    for i in "${!check_names[@]}"; do
      if [[ "${check_oks[$i]}" == "true" ]]; then
        printf 'ok   %-16s %s\n' "${check_names[$i]}" "${check_details[$i]}"
      else
        printf 'FAIL %-16s %s\n' "${check_names[$i]}" "${check_details[$i]}"
      fi
    done
    printf '\n'
    if [[ "$ok" == "true" ]]; then
      printf 'stack ok: %s\n' "$home"
    else
      printf 'stack not ok: %s\n' "$home"
    fi
  fi

  [[ "$ok" == "true" ]]
}

cmd_reset() {
  if [[ "$assume_yes" != "1" ]]; then
    err "reset drops the $schema schema in $pg_database; re-run with --yes"
    return 2
  fi
  if ! have docker; then
    err "docker is required"
    return 1
  fi
  if ! container_running; then
    err "postgres is not running; run up first"
    return 1
  fi

  local pid="" was_running=0
  pid="$(state_value api_pid 2>/dev/null || true)"
  if pid_is_ours "$pid"; then
    # Prove the API can be rebuilt before the destructive drop: a missing
    # toolchain must not leave the workspace with an empty schema and no API.
    if ! have go; then
      err "go is required to restart the API after the reset"
      return 1
    fi
    build_server || return 1
    stop_api_pid "$pid"
    was_running=1
  fi

  docker exec -i "$container" psql -q -U "$pg_user" -d "$pg_database" -v ON_ERROR_STOP=1 \
    -c "SET client_min_messages = warning" \
    -c "DROP SCHEMA IF EXISTS $schema CASCADE" >/dev/null
  docker exec -i "$container" psql -q -U "$pg_user" -d "$pg_database" -v ON_ERROR_STOP=1 \
    -c "SET client_min_messages = warning" \
    -c "CREATE SCHEMA $schema" >/dev/null

  if [[ "$was_running" == "1" ]]; then
    write_state "" "$(timestamp)"
    local new_pid=""
    if ! new_pid="$(start_api)"; then
      err "the API did not restart after the reset"
      return 1
    fi
    write_state "$new_pid" "$(timestamp)"
  fi

  collect_status
  if [[ "$json" == "1" ]]; then
    print_status_json
  else
    print_status_human
  fi
  return 0
}

case "$command_name" in
  up) cmd_up ;;
  down) cmd_down ;;
  status) cmd_status ;;
  check) cmd_check ;;
  reset) cmd_reset ;;
esac
