#!/usr/bin/env bash
# browser-e2e.sh — print the repeatable browser E2E path, or check this host.
#
# Read-only by design. The script never starts the agent stack, a tunnel,
# `wails dev`, Chrome, a browse session or a browse pairing; `--check` only
# reads (`command -v`, `docker info`, `curl /healthz`, `ssh -G`,
# `browse status --json`), and every command in the printed plan is for a human
# or an agent to run later. See docs/agents/e2e-runbook.md.
#
#   scripts/browser-e2e.sh             same as --dry-run
#   scripts/browser-e2e.sh --dry-run   print every step; inspect nothing
#   scripts/browser-e2e.sh --check     print every step, then inspect this host;
#                                      exit 1 naming any missing prerequisite
#
# Environment:
#   VERA_AGENT_HOME       persistent agent workspace (default ~/dev/vera-agent)
#   VERA_E2E_API_URL      API the health check calls (default $VERA_API_URL or
#                         http://127.0.0.1:8081)
#   VERA_E2E_MAC_HOST     SSH alias for the Mac (default macbook)
#   VERA_E2E_APP_URL      Wails dev server URL (default http://localhost:34115)
#   VERA_E2E_VERSION      version stamped into the dev build
#                         (default v0.16.0-e2e)
#   OPENCODE_BROWSER_PORT browse relay port (default 19988)
#
set -euo pipefail

agent_home="${VERA_AGENT_HOME:-$HOME/dev/vera-agent}"
api_url="${VERA_E2E_API_URL:-${VERA_API_URL:-http://127.0.0.1:8081}}"
mac_host="${VERA_E2E_MAC_HOST:-macbook}"
app_url="${VERA_E2E_APP_URL:-http://localhost:34115}"
version="${VERA_E2E_VERSION:-v0.16.0-e2e}"
relay_port="${OPENCODE_BROWSER_PORT:-19988}"
chrome="${VERA_E2E_CHROME:-$agent_home/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64/chrome}"

usage() {
  cat <<'USAGE'
usage: scripts/browser-e2e.sh [--dry-run|--check]

Prints the repeatable browser E2E path. Nothing is started: step commands are
for a human or an agent to run later.

  (no flag)   same as --dry-run
  --dry-run   print every step; inspect nothing
  --check     print every step, inspect this host, and exit 1 naming any
              missing prerequisite it can see
  -h, --help  print this help

Environment:
  VERA_AGENT_HOME       persistent agent workspace (default ~/dev/vera-agent)
  VERA_E2E_API_URL      API the health check calls (default $VERA_API_URL or
                        http://127.0.0.1:8081)
  VERA_E2E_MAC_HOST     SSH alias for the Mac (default macbook)
  VERA_E2E_APP_URL      Wails dev server URL (default http://localhost:34115)
  VERA_E2E_VERSION      version stamped into the dev build
                        (default v0.16.0-e2e)
  VERA_E2E_CHROME       Chrome binary for the CDP fallback (default the pinned
                        build under $VERA_AGENT_HOME/chrome-dl)
  OPENCODE_BROWSER_PORT browse relay port (default 19988)
USAGE
}

mode="dry-run"
for arg in "$@"; do
  case "$arg" in
    --dry-run) mode="dry-run" ;;
    --check) mode="check" ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'browser-e2e: unknown argument %s\n' "$arg" >&2
      printf 'usage: scripts/browser-e2e.sh [--dry-run|--check]\n' >&2
      exit 2
      ;;
  esac
done

step() {
  printf '\n%s\n' "$1"
}

cmd() {
  printf '  %s\n' "$1"
}

note() {
  printf '       %s\n' "$1"
}

print_plan() {
  printf 'browser E2E — %s\n' "$mode"
  printf 'nothing is started by this script; run the commands below yourself.\n'

  step "0. Agent stack (this Linux host)"
  cmd "scripts/agent-env.sh up"
  cmd "scripts/agent-env.sh status --json"
  cmd "curl -fsS $api_url/healthz"
  note "up starts postgres + the API; a missing Chrome only warns here."
  note "If 55432 is taken: VERA_AGENT_PG_PORT=55433 scripts/agent-env.sh up"

  step "1. Tunnel the API to the Mac (run on this host)"
  cmd "ssh -N -o ExitOnForwardFailure=yes -R 8081:127.0.0.1:8081 $mac_host"
  note "Remote (Mac) 127.0.0.1:8081 -> local (this host) $api_url."
  note "If the Mac's 8081 is busy: -R 18081:127.0.0.1:8081 and set"
  note "VERA_API_URL=http://127.0.0.1:18081 on the Mac."

  step "2. Give the Mac's Chrome a browse relay"
  cmd "browse install --remote"
  note "Supported path: opens the relay to other computers and prints a relay"
  note "address plus a one-time pairing code (10 minutes). State-changing: it"
  note "sets the relay hostname and may edit the shell profile; restore with"
  note "browse set hostname 127.0.0.1."
  cmd "ssh -N -o ExitOnForwardFailure=yes -R $relay_port:127.0.0.1:$relay_port $mac_host"
  note "Loopback alternative when the Mac cannot reach this host: the Mac's"
  note "127.0.0.1:$relay_port then reaches this host's relay."

  step "3. Run the dev app on the Mac"
  cmd "cd <vera-app checkout on the Mac>"
  cmd "VERA_API_URL=http://127.0.0.1:8081 wails dev \\"
  cmd "  -ldflags \"-X vera-app/backend.AppVersion=$version\""
  note "Open $app_url (or add -browser). The dev build refuses a"
  note "non-loopback VERA_API_URL, so the tunnel must land on the Mac's loopback."

  step "4. Pair and identify the browser"
  cmd "browse status --json"
  cmd "browse pair --list"
  cmd "browse doctor"

  step "5. Fresh session and fresh tab"
  cmd "browse session new vera-e2e \\"
  cmd "  --allow-origin $app_url --allow-origin http://127.0.0.1:34115"
  cmd "browse session reset vera-e2e"
  cmd "browse execute --session vera-e2e \\"
  cmd "  'await page.goto(\"$app_url\"); return { url: page.url(), title: await page.title() }'"

  step "6. Verify the version stamp in the page"
  cmd "browse execute --session vera-e2e \\"
  cmd "  'return await page.evaluate(() => window.go.backend.UpdateAPI.Version())'"
  note "It must print $version, not dev."

  step "7. protected-ui recovery (only when browse status says protected-ui=true)"
  cmd "browse session reset vera-e2e"
  note "Ask the user to dismiss the password-manager or extension UI in the"
  note "browser, then run the fresh-tab command from step 5 again."
  cmd "browse session delete vera-e2e"
  note "Delete and recreate the session when a reset is not enough."

  step "8. CDP fallback when the extension cannot attach"
  cmd "ssh -N -o ExitOnForwardFailure=yes -L 34115:127.0.0.1:34115 $mac_host"
  cmd "$chrome --headless=new --no-sandbox \\"
  cmd "  --remote-debugging-port=9222 \\"
  cmd "  --user-data-dir=\"\$(mktemp -d /tmp/vera-e2e-cdp.XXXXXX)\" about:blank"
  cmd "curl -fsS http://127.0.0.1:9222/json/version"
  cmd "curl -fsS -X PUT 'http://127.0.0.1:9222/json/new?$app_url'"
  note "Then attach any CDP client; the page has the same window.go bindings."
}

check_name=()
check_state=()
check_detail=()
check_failures=0
check_warnings=0

add_check() {
  check_name+=("$1")
  check_state+=("$2")
  check_detail+=("$3")
  case "$2" in
    FAIL) check_failures=$((check_failures + 1)) ;;
    WARN) check_warnings=$((check_warnings + 1)) ;;
  esac
}

check_ok() {
  add_check "$1" OK "$2"
}

check_warn() {
  add_check "$1" WARN "$2"
}

check_fail() {
  add_check "$1" FAIL "$2"
}

has() {
  command -v "$1" >/dev/null 2>&1
}

check_command() {
  local name="$1" label="${2:-$1}"
  if has "$name"; then
    check_ok "$label" "$(command -v "$name")"
  else
    check_fail "$label" "$name is not on PATH"
  fi
}

check_mac_tool() {
  # node, npm and wails run on the Mac, not here. This host can only say
  # whether its own copy is on PATH, so absence is a warning, not a failure.
  local name="$1"
  if has "$name"; then
    check_ok "$name" "$(command -v "$name") (this host; the Mac has its own)"
  else
    check_warn "$name" "not on PATH here; required on the Mac for wails dev"
  fi
}

browse_field() {
  # Prints one field from `browse status --json`; empty when the JSON is absent.
  local field="$1" json="$2"
  printf '%s' "$json" | AE_FIELD="$field" python3 -c '
import json, os, sys
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(0)
field = os.environ["AE_FIELD"]
extension = data.get("extension") or {}
relay = data.get("relay") or {}
if field == "relay_running":
    print("true" if relay.get("running") else "false")
elif field == "pairing":
    print(extension.get("pairingState") or "unknown")
elif field == "connected":
    print("true" if extension.get("connected") else "false")
elif field == "browsers":
    print(",".join(browser.get("id", "") for browser in extension.get("browsers") or []))
' 2>/dev/null || true
}

run_checks() {
  printf '\nprerequisites — %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  if has bash; then
    check_ok bash "$(command -v bash)"
  else
    check_fail bash "bash is not on PATH"
  fi

  if has python3; then
    check_ok python3 "$(command -v python3)"
  else
    check_fail python3 "python3 is required by scripts/agent-env.sh and the JSON reads"
  fi

  if has curl; then
    check_ok curl "$(command -v curl)"
  else
    check_fail curl "curl is not on PATH"
  fi

  if ! has docker; then
    check_fail docker "docker is not on PATH (the agent Postgres runs in a container)"
  elif docker info --format '{{.ServerVersion}}' >/dev/null 2>&1; then
    check_ok docker "daemon reachable"
  else
    check_fail docker "the Docker daemon is not reachable"
  fi

  check_command go
  check_mac_tool node
  check_mac_tool npm
  check_mac_tool wails
  if has browse; then
    check_ok browse "$(command -v browse)"
  else
    check_fail browse "browse is not on PATH (usually ~/.local/bin; open a new shell after install)"
  fi

  if has ssh; then
    local ssh_config=""
    ssh_config="$(ssh -G "$mac_host" 2>/dev/null || true)"
    if [[ -n "$ssh_config" ]]; then
      local resolved user
      resolved="$(printf '%s\n' "$ssh_config" | awk '$1 == "hostname" { print $2; exit }')"
      user="$(printf '%s\n' "$ssh_config" | awk '$1 == "user" { print $2; exit }')"
      check_ok "ssh $mac_host" "resolves to ${user:-?}@${resolved:-?} (config only; not dialled)"
    else
      check_fail "ssh $mac_host" "ssh -G $mac_host could not resolve the host"
    fi
  else
    check_fail ssh "ssh is not on PATH (the Mac tunnel needs it)"
  fi

  if has curl && curl -fsS --max-time 2 "$api_url/healthz" >/dev/null 2>&1; then
    check_ok "agent API" "$api_url/healthz answers"
  else
    check_fail "agent API" "$api_url/healthz does not answer; run scripts/agent-env.sh up"
  fi

  if [[ -x "$chrome" ]]; then
    check_ok chrome "$chrome"
  else
    check_warn chrome "not found at $chrome (CDP fallback only; the primary path uses the Mac's Chrome)"
  fi

  if has browse; then
    local browse_json relay_running pairing connected browsers
    if browse_json="$(browse status --json 2>/dev/null)"; then
      relay_running="$(browse_field relay_running "$browse_json")"
      pairing="$(browse_field pairing "$browse_json")"
      connected="$(browse_field connected "$browse_json")"
      browsers="$(browse_field browsers "$browse_json")"

      if [[ "$relay_running" == "true" ]]; then
        check_ok "browse relay" "running on port $relay_port"
      else
        check_warn "browse relay" "not running on port $relay_port (the first browse command starts it)"
      fi
      if [[ "$connected" == "true" ]]; then
        check_ok "browse extension" "connected ($pairing)"
      else
        check_warn "browse extension" "$pairing; open the paired browser profile (browse doctor)"
      fi
      if [[ -n "$browsers" ]]; then
        check_ok "connected browsers" "$browsers"
      else
        check_warn "connected browsers" "none connected; browse pair --list shows the paired profiles"
      fi
    else
      check_warn browse "browse status could not read the relay (it is observational; nothing was started)"
    fi
  fi

  local i
  for i in "${!check_name[@]}"; do
    printf '%-4s %-18s %s\n' "${check_state[$i]}" "${check_name[$i]}" "${check_detail[$i]}"
  done

  printf '\n'
  if [[ "$check_failures" -gt 0 ]]; then
    printf '%d prerequisite(s) missing' "$check_failures"
    if [[ "$check_warnings" -gt 0 ]]; then
      printf ' (%d warning(s))' "$check_warnings"
    fi
    printf '. Fix them, then run this check again.\n'
    printf 'The plan above is printed for reference; nothing was started.\n'
    return 1
  fi
  printf 'this host is ready'
  if [[ "$check_warnings" -gt 0 ]]; then
    printf ' with %d warning(s)' "$check_warnings"
  fi
  printf '. The Mac and its browser are not inspected over SSH; step 4 proves them.\n'
  return 0
}

print_plan

if [[ "$mode" == "check" ]]; then
  run_checks
fi
