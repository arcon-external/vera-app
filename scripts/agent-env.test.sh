#!/usr/bin/env bash
# Behavioral test for scripts/agent-env.sh (#556).
#
# Drives the script against a throwaway VERA_AGENT_HOME and fake docker, curl
# and go binaries in a throwaway PATH. No real container, API, database or
# browser is started: the fake docker records calls and tracks container
# liveness in files, the fake curl answers only /healthz, and the fake go
# writes a stub server that touches a marker and sleeps.
#
#   bash scripts/agent-env.test.sh
#
# Needs bash, jq and python3; nothing else from the host is used.
#
# Exits 0 when every assertion passes, 1 otherwise.
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/agent-env.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/vera-agent-test.XXXXXX")" || exit 1

home="$tmp/home"
fake_bin="$tmp/fake-bin"
docker_state="$tmp/docker-state"
out="$tmp/stdout"
err="$tmp/stderr"
all_output="$tmp/all-output"
chrome_dir="$home/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64"

mkdir -p "$fake_bin" "$docker_state"
: > "$all_output"

# --- fakes -----------------------------------------------------------------

cat > "$fake_bin/docker" <<'FAKE'
#!/usr/bin/env bash
set -uo pipefail
printf '%s\n' "$*" >> "${FAKE_DOCKER_CALLS:?}"
cmd="${1:-}"
shift || true

last_arg() {
  local last="" arg
  for arg in "$@"; do last="$arg"; done
  printf '%s' "$last"
}

name_from_run() {
  local prev="" arg
  for arg in "$@"; do
    if [[ "$prev" == "--name" ]]; then printf '%s' "$arg"; return 0; fi
    prev="$arg"
  done
  return 1
}

case "$cmd" in
  info)
    exit 0
    ;;
  inspect|container)
    # docker [container] inspect --format ... NAME
    name="$(last_arg "$@")"
    if [[ -f "$FAKE_DOCKER_STATE/$name" ]]; then printf 'true\n'; exit 0; fi
    exit 1
    ;;
  run)
    if [[ "${FAKE_DOCKER_FAIL_RUN:-0}" == "1" ]]; then
      echo "fake docker: cannot bind the requested port" >&2
      exit 1
    fi
    name="$(name_from_run "$@")" || { echo "fake docker: run without --name" >&2; exit 1; }
    touch "$FAKE_DOCKER_STATE/$name"
    printf 'fake-container-id\n'
    ;;
  start)
    touch "$FAKE_DOCKER_STATE/$(last_arg "$@")"
    ;;
  stop|rm)
    rm -f "$FAKE_DOCKER_STATE/$(last_arg "$@")"
    ;;
  exec)
    name=""
    for arg in "$@"; do
      case "$arg" in
        -*) continue ;;
        *) name="$arg"; break ;;
      esac
    done
    if [[ -z "$name" || ! -f "$FAKE_DOCKER_STATE/$name" ]]; then
      echo "fake docker: no such container: $name" >&2
      exit 1
    fi
    case " $* " in
      *" psql "*) printf '1\n' ;;
    esac
    ;;
  *)
    echo "fake docker: unsupported subcommand: $cmd $*" >&2
    exit 1
    ;;
esac
FAKE

cat > "$fake_bin/curl" <<'FAKE'
#!/usr/bin/env bash
set -uo pipefail
printf '%s\n' "$*" >> "${FAKE_CURL_CALLS:?}"
url=""
for arg in "$@"; do
  case "$arg" in
    http://*|https://*) url="$arg" ;;
  esac
done
case "$url" in
  */healthz|*/health)
    if [[ -f "${FAKE_API_RUNNING:?}" ]]; then
      printf '{"status":"ok"}\n'
      exit 0
    fi
    exit 7
    ;;
esac
echo "fake curl: unsupported URL: $*" >&2
exit 7
FAKE

cat > "$fake_bin/go" <<'FAKE'
#!/usr/bin/env bash
set -uo pipefail
printf '%s\n' "$*" >> "${FAKE_GO_CALLS:?}"
out="" prev=""
for arg in "$@"; do
  if [[ "$prev" == "-o" ]]; then out="$arg"; fi
  prev="$arg"
done
if [[ -z "$out" ]]; then
  echo "fake go: unsupported invocation: $*" >&2
  exit 1
fi
cat > "$out" <<'STUB'
#!/usr/bin/env bash
touch "${FAKE_API_RUNNING:?}"
trap 'rm -f "$FAKE_API_RUNNING"; exit 0' TERM INT
while :; do sleep 1; done
STUB
chmod +x "$out"
FAKE

chmod +x "$fake_bin/docker" "$fake_bin/curl" "$fake_bin/go"

export VERA_AGENT_HOME="$home"
original_path="$PATH"
export PATH="$fake_bin:$PATH"
export FAKE_DOCKER_STATE="$docker_state"
export FAKE_DOCKER_CALLS="$tmp/docker-calls"
export FAKE_CURL_CALLS="$tmp/curl-calls"
export FAKE_GO_CALLS="$tmp/go-calls"
export FAKE_API_RUNNING="$tmp/api-running"
: > "$FAKE_DOCKER_CALLS"
: > "$FAKE_CURL_CALLS"
: > "$FAKE_GO_CALLS"

# --- helpers ---------------------------------------------------------------

checks=0
failures=0

pass() { checks=$((checks + 1)); printf 'ok   %s\n' "$1"; }
fail() { checks=$((checks + 1)); failures=$((failures + 1)); printf 'FAIL %s\n' "$1"; }

assert_eq() {
  local name="$1" want="$2" got="$3"
  if [[ "$want" == "$got" ]]; then pass "$name"; else fail "$name: want [$want], got [$got]"; fi
}

assert_contains() {
  local name="$1" file="$2" pattern="$3"
  if grep -qF -- "$pattern" "$file"; then pass "$name"; else fail "$name: [$pattern] not found in $file"; fi
}

assert_absent() {
  local name="$1" file="$2" pattern="$3"
  if grep -qF -- "$pattern" "$file"; then fail "$name: [$pattern] leaked into $file"; else pass "$name"; fi
}

assert_executable() {
  local name="$1" path="$2"
  if [[ -x "$path" ]]; then pass "$name"; else fail "$name: $path is not executable"; fi
}

assert_absent_path() {
  local name="$1" path="$2"
  if [[ -e "$path" ]]; then fail "$name: $path exists"; else pass "$name"; fi
}

mode_of() {
  if stat -c %a "$1" >/dev/null 2>&1; then stat -c %a "$1"; else stat -f %Lp "$1"; fi
}

json() { jq -r "$1" "$out"; }

last_status=0
run_script() {
  : > "$out"
  : > "$err"
  "$script" "$@" > "$out" 2> "$err"
  last_status=$?
  cat "$out" "$err" >> "$all_output"
}

cleanup() {
  local pid=""
  if [[ -f "$home/state.json" ]]; then
    pid="$(jq -r '.api_pid // empty' "$home/state.json" 2>/dev/null || true)"
  fi
  if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi
  pkill -f "$home/bin/veraserver" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT

# --- 1. status on an untouched workspace -----------------------------------

printf '\n== status on an untouched workspace ==\n'
run_script status --json
assert_eq "status exits 0" 0 "$last_status"
assert_eq "status emits valid JSON" "true" "$(jq -e . "$out" >/dev/null 2>&1 && echo true || echo false)"
assert_eq "status reports the home" "$home" "$(json '.home')"
assert_eq "overall state is down" "down" "$(json '.state')"
assert_eq "postgres is down" "down" "$(json '.postgres.state')"
assert_eq "api is down" "down" "$(json '.api.state')"
assert_eq "chrome is missing" "missing" "$(json '.chrome.state')"
assert_eq "wrappers are missing" "missing" "$(json '.wrappers.state')"
assert_absent_path "status created no workspace" "$home"

# --- 2. check on an untouched workspace ------------------------------------

printf '\n== check on an untouched workspace ==\n'
run_script check --json
assert_eq "check exits 1 when the stack is down" 1 "$last_status"
assert_eq "check reports not ok" "false" "$(json '.ok')"
assert_eq "check names failed components" "true" \
  "$(jq -e '[.checks[] | select(.ok == false)] | length > 0' "$out" >/dev/null 2>&1 && echo true || echo false)"
assert_absent_path "check created no workspace" "$home"

# --- 2b. up fails fast when docker run fails -------------------------------

printf '\n== up fails fast when docker run fails ==\n'
export FAKE_DOCKER_FAIL_RUN=1
start_seconds=$SECONDS
run_script up --json
elapsed=$((SECONDS - start_seconds))
unset FAKE_DOCKER_FAIL_RUN
assert_eq "up fails when docker run fails" 1 "$last_status"
if [[ "$elapsed" -lt 10 ]]; then
  pass "up fails fast (${elapsed}s)"
else
  fail "up waited ${elapsed}s for a container that never started"
fi
assert_eq "failed up writes no state file" "no" "$([[ -f "$home/state.json" ]] && echo yes || echo no)"

# --- 3. up -----------------------------------------------------------------

printf '\n== up ==\n'
mkdir -p "$chrome_dir"
printf '#!/bin/sh\nexit 0\n' > "$chrome_dir/chrome"
chmod +x "$chrome_dir/chrome"

run_script up --json
if [[ "$last_status" -ne 0 ]]; then
  printf '%s\n' "--- up failed; stderr follows ---"
  cat "$err"
fi
assert_eq "up exits 0" 0 "$last_status"
assert_eq "up reports the stack up" "up" "$(json '.state')"
assert_eq "up reports postgres up" "up" "$(json '.postgres.state')"
assert_eq "up reports the api up" "up" "$(json '.api.state')"
assert_eq "up reports chrome ready" "ready" "$(json '.chrome.state')"
assert_eq "up reports wrappers ready" "ready" "$(json '.wrappers.state')"
assert_eq "up wrote the state file" "yes" "$([[ -f "$home/state.json" ]] && echo yes || echo no)"
assert_eq "up wrote the env file" "yes" "$([[ -f "$home/env" ]] && echo yes || echo no)"
assert_executable "up wrote the container chrome wrapper" "$home/chrome-wrapper/chrome"
assert_executable "up wrote the pdf chrome shim" "$home/bin/google-chrome"
assert_eq "signing key is private" "600" "$(mode_of "$home/secrets/signing-key")"
assert_eq "admin password is private" "600" "$(mode_of "$home/secrets/admin-password")"
assert_eq "state file is private" "600" "$(mode_of "$home/state.json")"

assert_contains "postgres runs the pinned image" "$FAKE_DOCKER_CALLS" "postgres:18-alpine"
assert_contains "postgres binds the agent port" "$FAKE_DOCKER_CALLS" "127.0.0.1:55432:5432"
assert_contains "postgres uses the agent database" "$FAKE_DOCKER_CALLS" "POSTGRES_DB=veratest"
assert_contains "postgres uses the agent user" "$FAKE_DOCKER_CALLS" "POSTGRES_USER=vera"
assert_contains "postgres keeps a named volume" "$FAKE_DOCKER_CALLS" "source=vera-agent-pg"
assert_contains "postgres has the standard agent name" "$FAKE_DOCKER_CALLS" "--name vera-agent-postgres"
assert_contains "up builds the server" "$FAKE_GO_CALLS" "./cmd/veraserver"

signing_key="$(cat "$home/secrets/signing-key")"
admin_password="$(cat "$home/secrets/admin-password")"
assert_absent "signing key never reaches output" "$all_output" "$signing_key"
assert_absent "admin password never reaches output" "$all_output" "$admin_password"
assert_absent "signing key is not in the state file" "$home/state.json" "$signing_key"
assert_absent "admin password is not in the state file" "$home/state.json" "$admin_password"
assert_absent "signing key is not in the env file" "$home/env" "$signing_key"
assert_absent "admin password is not in the env file" "$home/env" "$admin_password"

# --- 4. check after up -----------------------------------------------------

printf '\n== check after up ==\n'
run_script check --json
assert_eq "check exits 0 when the stack is up" 0 "$last_status"
assert_eq "check reports ok" "true" "$(json '.ok')"

# --- 5. up is idempotent ---------------------------------------------------

printf '\n== up is idempotent ==\n'
runs_before="$(grep -c '^run ' "$FAKE_DOCKER_CALLS" || true)"
run_script up --json
assert_eq "second up exits 0" 0 "$last_status"
runs_after="$(grep -c '^run ' "$FAKE_DOCKER_CALLS" || true)"
assert_eq "second up creates no second container" "$runs_before" "$runs_after"

# --- 5b. up recovers a lost state file -------------------------------------

printf '\n== up recovers a lost state file ==\n'
recovered_pid="$(json '.api.pid')"
rm -f "$home/state.json"
run_script up --json
assert_eq "up after a lost state file exits 0" 0 "$last_status"
assert_eq "up adopts the running API" "$recovered_pid" "$(json '.api.pid')"
assert_eq "up rewrites the state file" "yes" "$([[ -f "$home/state.json" ]] && echo yes || echo no)"

# --- 6. down ---------------------------------------------------------------

printf '\n== down ==\n'
api_pid="$(json '.api.pid')"
touch "$docker_state/vera-agent-decoy"
run_script down --json
assert_eq "down exits 0" 0 "$last_status"
assert_eq "down reports the stack down" "down" "$(json '.state')"
assert_eq "down removed the state file" "no" "$([[ -f "$home/state.json" ]] && echo yes || echo no)"
if kill -0 "$api_pid" 2>/dev/null; then fail "down left the API running (pid $api_pid)"; else pass "down stopped the API"; fi
assert_contains "down removed the agent container" "$FAKE_DOCKER_CALLS" "rm -f vera-agent-postgres"
if [[ -f "$docker_state/vera-agent-decoy" ]]; then pass "down left the decoy container"; else fail "down removed the decoy container"; fi
if grep -qF 'vera-agent-decoy' "$FAKE_DOCKER_CALLS"; then fail "down touched the decoy container"; else pass "down never named the decoy"; fi

# --- 7. down is idempotent -------------------------------------------------

printf '\n== down is idempotent ==\n'
run_script down --json
assert_eq "second down exits 0" 0 "$last_status"
assert_eq "second down still reports down" "down" "$(json '.state')"

# --- 8. reset refuses without --yes ---------------------------------------

printf '\n== reset refuses without --yes ==\n'
run_script up --json
assert_eq "up before reset exits 0" 0 "$last_status"
old_pid="$(json '.api.pid')"
calls_before="$(wc -l < "$FAKE_DOCKER_CALLS")"
run_script reset
assert_eq "reset without --yes exits 2" 2 "$last_status"
calls_after="$(wc -l < "$FAKE_DOCKER_CALLS")"
assert_eq "reset without --yes touches nothing" "$calls_before" "$calls_after"
assert_contains "reset refusal names --yes" "$err" "--yes"

# --- 8b. reset refuses before dropping when go is missing ------------------

printf '\n== reset refuses before dropping when go is missing ==\n'
: > "$FAKE_DOCKER_CALLS"
mv "$fake_bin/go" "$fake_bin/go.hidden"
export PATH="$fake_bin:/usr/bin:/bin"
run_script reset --yes
export PATH="$fake_bin:$original_path"
mv "$fake_bin/go.hidden" "$fake_bin/go"
assert_eq "reset without go exits 1" 1 "$last_status"
if grep -qF 'DROP SCHEMA' "$FAKE_DOCKER_CALLS"; then
  fail "reset without go dropped the schema"
else
  pass "reset without go dropped nothing"
fi
if grep -qF 'rm -f' "$FAKE_DOCKER_CALLS"; then
  fail "reset without go removed a container"
else
  pass "reset without go removed no container"
fi
assert_contains "reset without go names go" "$err" "go is required"

# --- 9. reset drops and recreates the agent schema -------------------------

printf '\n== reset ==\n'
run_script reset --yes --json
assert_eq "reset exits 0" 0 "$last_status"
assert_eq "reset leaves the stack up" "up" "$(json '.state')"
assert_contains "reset drops the agent schema" "$FAKE_DOCKER_CALLS" "DROP SCHEMA IF EXISTS vera CASCADE"
assert_contains "reset recreates the agent schema" "$FAKE_DOCKER_CALLS" "CREATE SCHEMA vera"
new_pid="$(json '.api.pid')"
if [[ -n "$new_pid" ]]; then pass "reset reports the restarted API"; else fail "reset reports no API pid"; fi
if [[ "$old_pid" != "$new_pid" ]] && kill -0 "$old_pid" 2>/dev/null; then
  fail "reset left the old API running (pid $old_pid)"
else
  pass "reset stopped the old API"
fi

# --- 10. usage -------------------------------------------------------------

printf '\n== usage ==\n'
run_script
assert_eq "no command exits 2" 2 "$last_status"
run_script bogus
assert_eq "unknown command exits 2" 2 "$last_status"
export VERA_AGENT_PG_PORT=70000
run_script status --json
assert_eq "an out-of-range port exits 2" 2 "$last_status"
unset VERA_AGENT_PG_PORT
run_script --help
assert_eq "--help exits 0" 0 "$last_status"
for word in up down status check reset VERA_AGENT_HOME; do
  assert_contains "--help mentions $word" "$out" "$word"
done

# --- 11. human status ------------------------------------------------------

printf '\n== human status ==\n'
run_script status
assert_eq "human status exits 0" 0 "$last_status"
for word in postgres api chrome wrappers; do
  assert_contains "human status names $word" "$out" "$word"
done

# --- summary ---------------------------------------------------------------

printf '\n%d checks, %d failures\n' "$checks" "$failures"
if [[ "$failures" -ne 0 ]]; then
  exit 1
fi
exit 0
