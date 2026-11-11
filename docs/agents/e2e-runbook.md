# Browser E2E runbook

The browser pass drives the real Wails dev app against the agent API, with the
real Go bindings, so it exercises the store and the API rather than a mock. It
is hand-driven (`AGENTS.md` §7), and the record follows §13. This file is the
repeatable path; `scripts/browser-e2e.sh` prints it and checks this host.

Part of #555. The prior recorded pass (#501) is `docs/po-sprint-verification.md`
§11, which used the same topology through a Linux Chrome over CDP.

The commands assume `go`, `node`/`npm`, `wails` and `browse` are on `PATH`
(`~/.local/bin` and the nvm bin directory on this workstation); a non-login
shell often has none of them. `scripts/browser-e2e.sh --check` names each one
it cannot find.

## What runs where

```text
Linux workstation                          Mac
─────────────────────────────────          ──────────────────────────────
scripts/agent-env.sh up                    wails dev  (127.0.0.1:34115)
  vera-agent-postgres :55432*                VERA_API_URL=http://127.0.0.1:8081
  veraserver         127.0.0.1:8081  <─────  (SSH remote forward, step 1)
  browse relay       127.0.0.1:19988 <─────  Chrome + browse extension
  scripts/browser-e2e.sh                     (relay pairing, step 2)
```

`*` or `VERA_AGENT_PG_PORT` when 55432 is taken (step 0).

The API and the database live on the Linux host. The Mac runs the desktop
build and the browser; it never opens Postgres. The SSH forwards are the only
link between the two halves.

`scripts/browser-e2e.sh` checks the Linux half only. It never dials the Mac, so
step 4 is what proves the Mac half.

## 0. Bring the agent stack up (Linux)

```bash
scripts/agent-env.sh up
scripts/agent-env.sh status --json
curl -fsS http://127.0.0.1:8081/healthz
```

`up` starts the `vera-agent-postgres` container, builds `veraserver`, starts it
on `127.0.0.1:8081`, and writes the signing key and admin password under
`$VERA_AGENT_HOME/secrets`. A missing Chrome only warns: the primary browser
path uses the Mac's Chrome, and only the CDP fallback (step 8) needs the Linux
one.

If `status --json` says the API is `down` or `degraded`, stop here. A browser
failure against a down API is an environment failure, not a product failure.
`docs/agents/local-dev.md` has the DSN, ports, reset and admin bootstrap.

If `55432` is already taken — `frank518-pg` holds it on this workstation — start
the stack with a free port instead:
`VERA_AGENT_PG_PORT=55433 scripts/agent-env.sh up`. `status --json` reports the
configured/default port, so read `$VERA_AGENT_HOME/state.json` for the port the
running stack actually uses.

## 1. Tunnel the API to the Mac

Run on the Linux host, in its own terminal:

```bash
ssh -N -o ExitOnForwardFailure=yes -R 8081:127.0.0.1:8081 macbook
```

`-R 8081:127.0.0.1:8081` means: the **Mac** listens on its loopback `8081` and
forwards to **this host's** `127.0.0.1:8081`. `-N` opens no shell. The remote
listener binds loopback by default, so the API is not exposed on the Mac's LAN.

Check the alias first if the tunnel fails immediately:

```bash
ssh -G macbook | grep -E '^(hostname|user) '
```

The alias is `macbook` on this workstation; `VERA_E2E_MAC_HOST` overrides it for
the helper, and the raw `ssh` command takes the host directly.

If the Mac already uses `8081`, forward another port:

```bash
ssh -N -o ExitOnForwardFailure=yes -R 18081:127.0.0.1:8081 macbook
```

and set `VERA_API_URL=http://127.0.0.1:18081` on the Mac in step 3.

> The dev desktop refuses a non-loopback `VERA_API_URL` (`apibase_dev.go`). A
> LAN or Tailscale URL such as `http://100.x.y.z:8081` is rejected before the
> window opens; the reverse forward to the Mac's own loopback is what makes the
> URL valid.

## 2. Give the Mac's Chrome a browse relay

The supported path opens the relay to other computers and prints a relay
address plus a one-time pairing code (valid 10 minutes):

```bash
browse install --remote
```

In Chrome on the Mac, open browse → **Connect an agent on another computer**,
enter the relay address and the code. This is state-changing: it sets the relay
hostname to `0.0.0.0` and may add `~/.local/bin` to the shell profile. Restore
the loopback-only relay afterwards with:

```bash
browse set hostname 127.0.0.1
```

When the Mac cannot reach this host directly, forward the relay instead and
enter `127.0.0.1:19988` as the relay address on the Mac:

```bash
ssh -N -o ExitOnForwardFailure=yes -R 19988:127.0.0.1:19988 macbook
```

Use one of the two. The helper only reads `browse status`; it never pairs,
installs or changes the relay.

## 3. Run the dev app on the Mac

In the `vera-app` checkout on the Mac:

```bash
VERA_API_URL=http://127.0.0.1:8081 \
  wails dev -ldflags "-X vera-app/backend.AppVersion=v0.16.0-e2e"
```

Run it from the repository root: the dev build reads `VERA_API_URL` from the
environment first, then the gitignored `.env` in the working directory. The
exported value wins, so a stale `.env` cannot redirect this run.

Wails serves the app at `http://localhost:34115` by default; `-devserver`
changes the address and `-browser` opens it in the system browser. Leave the
terminal open — `wails dev` rebuilds on change.

Install the CLI on the Mac if it is missing, matching `go.mod` (currently
v2.17.1):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
```

A CLI older than `go.mod` still builds but prints a version warning; the
warning is harmless for this pass, and a mismatched pair is worth fixing before
a release build.

## 4. Pair and identify the browser

Run on the Linux host:

```bash
browse status --json
browse pair --list
browse doctor
```

`status` and `doctor` are observational: they read the relay and the browser
registry, and start neither. The first operational command opens the relay when
it is needed, so do not run `browse serve` first.

Read these fields:

| Field | Where | Meaning |
| --- | --- | --- |
| `extension.connected` | `status --json` | `true` only when the Mac's extension is talking to the relay |
| `extension.pairingState` | `status --json` | `connected`, `paired-offline`, `unpaired`, `refused`, `incompatible`, `not-seen` |
| `extension.browsers[].id` | `status --json` | the browser id a session can name, e.g. `chrome-macbook` |
| `profile` | `pair --list` | the Chrome profile UUID the browser was paired from |
| `last seen` | `pair --list` | when that profile last connected |

`paired-offline` means the relay knows the browser but the profile is closed or
the extension is disabled. Open the **paired profile** — not a new one — and
the extension reconnects on its own. `browse doctor` prints the same next step.

A new profile (or a new machine) is a new pairing: add browse from the Chrome
Web Store, run `browse install --remote` again on the relay host, and enter the
fresh code in that profile. A new profile gets a new browser id; the old id
stays paired until `browse pair --revoke <id>`.

The CLI cannot choose a browser: `browse session new` takes only `--read-only`
and `--allow-origin`, so CLI sessions use the default browser. When two
browsers are paired, either revoke the stale one or create the session through
the MCP tool with `browser: "<id>"`. The CLI path assumes one connected
browser.

## 5. Fresh session and fresh tab

```bash
browse session new vera-e2e \
  --allow-origin http://localhost:34115 \
  --allow-origin http://127.0.0.1:34115

browse session reset vera-e2e

browse execute --session vera-e2e \
  'await page.goto("http://localhost:34115"); return { url: page.url(), title: await page.title() }'
```

The first `execute` on a session with no page opens a fresh relay-owned tab;
`page.goto` navigates it to the dev server. `session reset` drops the session's
default page and state, which is the recovery for a tab that was closed,
detached or left on a stale page. `browse execute --rm '<code>'` is the
one-shot version: it creates a session, runs the code and closes its tabs.

Inspect before acting, then verify the result:

```js
await snapshot()
await ref("e12").click()
await page.getByRole("heading", { name: "..." }).waitFor()
return { url: page.url() }
```

A click that returned is not proof; assert the URL or a stable element
afterwards, as the browse skill says.

When the user already has the app open in their own tab, do not open a second
one: ask them to lend it with the browse toolbar button, then adopt it by tab
id (the `tab=` value in `status`) or URL substring:

```bash
browse session adopt --session vera-e2e --target-url localhost:34115
```

Delete the session when the pass is done:

```bash
browse session delete vera-e2e
```

## 6. The version stamp

The dev build carries whatever `-ldflags` stamped into
`backend.AppVersion`; an unstamped build reports `dev`. Read it from the page
itself, through the real binding:

```bash
browse execute --session vera-e2e \
  'return await page.evaluate(() => window.go.backend.UpdateAPI.Version())'
```

It must print `v0.16.0-e2e` (the value from step 3), not `dev`. This is the
same variable `scripts/stamp-version.sh` sets for a release build:

```bash
VERSION=v0.16.0 scripts/stamp-version.sh
```

`stamp-version.sh` checks the Go ldflags against `wails.json` and fails when
they disagree. For a no-side-effect read of the build options Wails would use
(the `LDFlags` line is the one to check):

```bash
wails build -ldflags "-X vera-app/backend.AppVersion=v0.16.0-e2e" -dryrun
```

The CLI carries the same variable, so a stamp can also be proved without a
window:

```bash
go build -ldflags "-X vera-app/backend.AppVersion=v0.16.0-e2e" -o /tmp/vera-stamp ./cmd/vera
/tmp/vera-stamp version
```

## 7. `protected-ui` recovery

`browse status` prints `protected-ui=true` on the target when Chromium refuses
to attach automation to protected browser UI (the JSON field is
`targets[].protectedUi`). The execution error is the matching one:

```text
Chromium blocked protected extension UI, possibly a password manager.
Ask the user to finish or dismiss it in the browser, then retry.
```

Recovery, in order:

1. Do not retry-loop, and never try to automate `chrome://`,
   `chrome-extension://`, `devtools://`, `view-source:`, the Chrome Web Store or
   the new-tab page. browse refuses the internal schemes and marks any target
   Chromium protects as `protected-ui`.
2. If a password manager or extension popup is covering the page, ask the user
   to finish or dismiss it in the browser. That UI belongs to the extension and
   no automation setting may weaken it.
3. Reset the session and rerun the step-5 execute:
   ```bash
   browse session reset vera-e2e
   ```
4. If the same tab is still marked, delete the session and create it again; a
   new session opens a new relay-owned tab:
   ```bash
   browse session delete vera-e2e
   ```
5. If the user's own adopted tab is the protected one, ask them to lend a
   normal app tab instead. Never adopt a protected tab and never clear browser
   state to get around the block.

A tab that the extension cannot attach to is not a product defect; it is the
browser protecting its own UI.

## 8. CDP fallback when the extension cannot attach

When browse is paired-offline or the extension is unavailable, drive the same
tunnel with Chrome's DevTools protocol. The page is served by `wails dev`, so
it still carries the real `window.go` bindings.

Forward the Mac's dev server to this host:

```bash
ssh -N -o ExitOnForwardFailure=yes -L 34115:127.0.0.1:34115 macbook
```

`-L` is the opposite direction: this host's `127.0.0.1:34115` now reaches the
**Mac's** dev server.

Start the pinned Linux Chrome headless with a debugging port:

```bash
"$VERA_AGENT_HOME/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64/chrome" \
  --headless=new --no-sandbox \
  --remote-debugging-port=9222 \
  --user-data-dir="$(mktemp -d /tmp/vera-e2e-cdp.XXXXXX)" about:blank
```

Prove the endpoint and open the app:

```bash
curl -fsS http://127.0.0.1:9222/json/version
curl -fsS -X PUT 'http://127.0.0.1:9222/json/new?http://localhost:34115'
curl -fsS http://127.0.0.1:9222/json/list
```

A render check without a driver, useful as the first evidence:

```bash
"$VERA_AGENT_HOME/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64/chrome" \
  --headless=new --no-sandbox \
  --screenshot=/tmp/vera-e2e.png --window-size=1280,900 \
  http://localhost:34115
```

From here the driver is your choice over `http://127.0.0.1:9222`: the #501 pass
used a small Node script over the DevTools WebSocket, kept outside the
repository as local evidence. There is no committed driver bundle, so the
runbook does not pretend one exists. Record what was driven, the viewport and
the keys or pointer actions, as `AGENTS.md` §13 requires.

## Shutdown

```bash
browse session delete vera-e2e      # if it still exists
# Ctrl-C the wails dev terminal, then both ssh tunnels
scripts/agent-env.sh down           # removes the container, keeps the volume
```

`down` acts only on the API pid and container recorded in the state file. The
database volume `vera-agent-pg` survives, so the next `up` reuses it.

## Live pass (2026-11-11, Linux workstation)

What actually ran on this host, against the agent stack and the Linux browse
relay. The Mac and its browser were not available, so the legs that need them
are listed as not run rather than claimed.

### Ran

| Command | Exit | Result |
| --- | --- | --- |
| `bash -n scripts/browser-e2e.sh` | 0 | syntax clean |
| `scripts/browser-e2e.sh --dry-run` (stack down) | 0 | printed all 9 steps; `docker ps` and the `veraserver` pid list were byte-identical before and after |
| `scripts/browser-e2e.sh --check` (stack down, full PATH) | 1 | named `agent API` as the missing prerequisite; 3 warnings (Chrome, extension, no connected browser) |
| `PATH=/usr/bin:/bin scripts/browser-e2e.sh --check` | 1 | named `go`, `browse` and `agent API`; toolchain warnings for `node`, `npm`, `wails` and Chrome; proves a missing prerequisite exits non-zero |
| `VERA_AGENT_PG_PORT=55433 scripts/agent-env.sh up` | 0 | postgres up on `127.0.0.1:55433`, API up on `127.0.0.1:8081` (pid 3415955), wrappers ready; Chrome missing (warning) |
| `curl -fsS http://127.0.0.1:8081/healthz` | 0 | `{"status":"ok"}`, HTTP 200 |
| `scripts/browser-e2e.sh --check` (stack up, full PATH) | 0 | this host ready; 3 warnings: Chrome missing, extension `paired-offline`, no connected browser |
| `scripts/browser-e2e.sh --dry-run` + `--check` (stack up) | 0 / 0 | `docker ps` and the API pid were unchanged; `/healthz` still answered |
| `browse pair --list` | 0 | two paired Mac profiles (browser ids ending `-pro` and `-pro-2`), last seen 2026-11-09 and 2026-11-11 |
| `browse status --json` | 0 | relay running on `127.0.0.1:19988`; `connected: false`, `pairingState: paired-offline`, `browsers: []` |
| `browse status --json` in an isolated `HOME`/XDG with a free port | 4 | `relay_running: false`, `extension: null`, and no listener on that port afterwards: `status` is observational and starts no relay |
| `browse doctor` | 1 | `Status: fail`, extension disconnected, `paired-offline`; the printed next step is to open the paired profile |
| `browse execute --rm 'return 1'` | 4 | refused cleanly: “Open the browser profile already paired with browse and enable the extension; it reconnects automatically.” |
| `go build -ldflags "-X vera-app/backend.AppVersion=v0.16.0-e2e" -o /tmp/opencode/vera-stamp ./cmd/vera` | 0 | binary built |
| `/tmp/opencode/vera-stamp version` | 0 | `v0.16.0-e2e (gpdf)` |
| `/tmp/opencode/vera-stamp version --json` | 0 | `{"renderer":"gpdf","version":"v0.16.0-e2e"}` |
| `wails build -ldflags "-X vera-app/backend.AppVersion=v0.16.0-e2e" -dryrun` | 0 | `LDFlags` line carries the stamp; warns the local CLI is v2.16.0 while `go.mod` is v2.17.1 |
| `VERSION=v0.16.0 scripts/stamp-version.sh` | 0 | `version stamp OK: Go ldflags and wails.json info agree on v0.16.0` |
| `VERA_AGENT_PG_PORT=55433 scripts/agent-env.sh down` | 0 | API and container stopped; `docker ps` and the pid list matched the pre-run baseline exactly |

The stack used: agent Postgres 18 in `vera-agent-postgres` on
`127.0.0.1:55433` (55432 was held by `frank518-pg`), `veraserver` on
`127.0.0.1:8081` (pid 3415955), browse 0.0.16 with its relay on
`127.0.0.1:19988`.

### Not run (and why)

- **The SSH tunnel to the Mac.** No Mac is available to this environment; the
  alias resolves in the SSH config, but the helper deliberately only reads
  `ssh -G` and never dials it.
- **`wails dev` on the Mac.** No Mac and no display here; the Linux host has no
  WKWebView/GTK session. The Linux half it would call (`/healthz`, the stamp)
  was proved instead.
- **Pairing a Mac browser; the fresh-tab flow; `protected-ui` recovery.** The
  paired Mac profiles are offline (`paired-offline`), so no tab could be
  attached. The commands are the browse CLI's own (`session new`, `reset`,
  `execute`, `doctor`); the recovery order is the tool's own error text.
- **The CDP fallback.** No Chrome binary exists at the documented path on this
  host (`~/dev/vera-agent/chrome-dl/...` is absent), and there is no display.
  The commands are Chrome's documented flags; nothing in that leg was executed.
- **`window.go.backend.UpdateAPI.Version()` in the page.** It needs the Wails
  dev page. The same `backend.AppVersion` was proved through the CLI build.
- **A real macOS key/paste/layout pass.** Unchanged from the #501 residual list:
  no WKWebView, screen reader, native file picker, or OS display zoom.

### Surprises

- `browse install --remote` is not read-only: it sets the relay hostname to
  `0.0.0.0` and appends a PATH line to the shell profile. It is a pairing step,
  never a check; the helper does not run it. The relay was restored with
  `browse set hostname 127.0.0.1` after this pass.
- `scripts/agent-env.sh status --json` reported `55432` after an `up` that used
  `VERA_AGENT_PG_PORT=55433` (the running stack used 55433). `status` prints the
  configured/default port, so the runbook points at `state.json` for the port
  the stack actually uses.
