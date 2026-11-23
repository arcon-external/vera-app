# Release runbook

`AGENTS.md` §14 is the always-loaded summary: when to release, how releases are
named, and the artifact contract with the updater. This file is what a release is
actually run from — the pre-tag checklist, the exact commands, and the rule for
the Dependabot pull requests that are open when the release starts.

Part of #555.

## Pre-tag checklist

Run these in order. A step that fails stops the release: fix it, then start that
step again rather than the one after it.

### 1. Read Dependabot

```bash
scripts/dependabot-status.sh          # the human read
scripts/dependabot-status.sh --json   # the same read as one JSON document
```

The script lists the open Dependabot alerts and the open Dependabot-authored
pull requests. Exit `0` means the read succeeded — **not** that the lists are
empty; read the counts. Exit `1` is a failed query and prints the API's own
message; a failed read is not zero open, so do not continue until it succeeds.

Paste the output into the release issue (or the literal `Dependabot: 0 open`
when nothing is open, as `AGENTS.md` §11 asks of a PR body). Then:

- **Alerts:** zero critical/high. Fix critical/high before the tag. A low/medium
  left open is named on the release issue with the reason it is not taken now.
- **Pull requests:** apply the batch/close rule below. The release does not
  start with a Dependabot PR still waiting for a decision.

### 2. Run every gate

```bash
gofmt -l .                        # must print nothing
go vet ./...
go test -race ./...
npm --prefix frontend run build   # strict TypeScript + production bundle
npm --prefix frontend test        # Vitest: the frontend's pure modules
```

A release also carries whatever `AGENTS.md` §13 requires for the change it
contains: the Postgres integration test for pricing, tax or persistence, and
`scripts/ui-snapshot.sh` (both widths, PNGs read) for a panel layout change. Run
them now, not after the tag.

### 3. Write the changelog

Write the `CHANGELOG.md` entry in staff language — Added / Changed / Fixed /
Security, user-visible changes only, internal chores omitted. Then copy it to the
embedded file, which is what the What's New modal ships:

```bash
cp CHANGELOG.md backend/api/changelog.md
go test ./backend/api -run TestChangelogCopyMatchesRoot
```

The copy must match byte-for-byte; the test enforces it.

Then prove the notes name every `fix:`/`feat:` commit in the range — end each
bullet with its issue or PR number (`(#NNN)`) so the check can match it:

```bash
scripts/release-check.sh --range "$(git describe --tags --abbrev=0)..HEAD"
```

It prints the matched/missing counts, names every missing `#NNN` (or subject),
and exits non-zero on a gap; `--json` is the same read as one document.

### 4. Bump the version

Set `wails.json` `info.productVersion` to the release version **without** the
`v` (Wails convention: tag `v0.16.0` → `"0.16.0"`) and commit it with the
changelog. This is the last commit before the tag.

### 5. Verify the stamp

```bash
VERSION=vX.Y.Z scripts/stamp-version.sh
```

It must print `version stamp OK`. It fails loudly when the Go ldflags and
`wails.json` disagree, which is cheaper than a mis-stamped installer.

### 6. Create the release (which creates the tag)

Push first, then create the release. Every artifact job uploads into the
release, so a missing release fails all of them, and `--target` pins the tag to
the pushed commit that carries the version bump:

```bash
git push origin main
gh release create vX.Y.Z --target "$(git rev-parse HEAD)" \
  --title "vX.Y.Z — Title" --notes "changelog"
```

`desktop.yml` now builds nine artifacts — the three installers and six CLI
binaries, each with a `.sha256` sidecar — all version-stamped. The `cli` job
fails the release if a binary does not carry the tag.

### 7. Watch the announce job

The `Announce to the updater` job runs last, gated on all three installer jobs.
It mirrors every artifact to the public bucket, proves each URL serves, writes
`latest.json`, sets the six `VERA_APP_*` variables on `vera-api` (carrying
`VERA_APP_MIN_SUPPORTED` forward), and fails if `/v1/app/latest` still reports
the old version. Nothing else tells installed copies about a release, so a red
or skipped announce job is a release nobody is offered.

If it failed, fix the cause and re-run just the failed jobs:

```bash
gh run rerun --repo arcon-external/vera-app --failed
```

Outside CI the same sequence is `scripts/announce-release.sh --version vX.Y.Z`
(takes `--dry-run`, which runs every check and changes nothing).

### 8. Verify the app can see the release

```bash
curl -s https://api.arconpentapersada.co.id/v1/app/latest \
  | grep -o '"latest": "[^"]*"'
```

It must print the tag just created. Anything older means the announce job did
not converge — read its log before doing anything else.

### 9. Add the Download section

List the installers **and the six CLI binaries**, copying every asset name out of
`gh release view vX.Y.Z --json assets` rather than typing it, with one install
line each. Staff links point at the `vera-downloads-508411` bucket mirror, never
at GitHub release assets. Include the Windows SmartScreen and macOS
right-click-to-open notes from `AGENTS.md` §14.

## Allocation rollout (v0.24.0)

The quotation-to-purchase-order allocation model (#648) is deployed in two
stages, and the order is not a preference. Strict mode refuses the payloads a
pre-v0.24.0 desktop still sends, so the compatible server must be live and the
v0.24.0 desktop must be in use **before** the flag is flipped.

The switch is `VERA_PO_ALLOCATIONS_STRICT`, read once by `store.LoadConfig` on
the API server. Unset, `false` or `0` is compatibility mode; `true` or `1` is
strict; any other value stops the server instead of silently staying compatible.
`scripts/dev-api.sh --local-development` inherits it like every other API
variable.

### Stage A — deploy the compatible server

1. Merge #654–#657. A merge to `main` deploys `veraserver` (`deploy.yml`), and
   `store.Open` applies migrations 054–056 at startup. Every column is additive
   or defaulted and every new JSON field is optional, so an older server or
   desktop keeps working through this stage. Leave `VERA_PO_ALLOCATIONS_STRICT`
   unset.
2. Confirm the deployment and the release state:

   ```bash
   curl -s https://api.arconpentapersada.co.id/v1/app/latest \
     | grep -o '"latest": "[^"]*"'
   ```

3. Smoke-test a pre-v0.24.0 desktop: a blank draft, a free line and a row
   discount still save. That is the compatibility window, and it is expected.

### Stage B — ship v0.24.0, verify uptake, then flip strict

1. Release v0.24.0 with the normal steps above. The desktop is built only by
   the release tag (`desktop.yml`); a merge is not a desktop release.
2. Wait for the announce job, then confirm the app is offered the release:

   ```bash
   curl -s https://api.arconpentapersada.co.id/v1/app/latest \
     | grep -o '"latest": "[^"]*"'
   ```

   It must print `v0.24.0`. Anything older means the announce job failed.
3. Verify uptake. The server has no per-desktop version telemetry, so this is a
   human check: each workstation opens Settings → App updates and confirms
   Current version is v0.24.0. Do **not** flip strict while any workstation is
   older; a missed workstation would start receiving `po_free_line_retired` and
   `po_discount_forbidden` on its next save.
4. Raise the updater floor. `scripts/announce-release.sh` reads the live
   `VERA_APP_MIN_SUPPORTED` and carries it forward; it never raises it. The only
   way the minimum moves is this manual command:

   ```bash
   gcloud run services update vera-api --project=vera-app-508411 \
     --region=asia-southeast1 \
     --update-env-vars VERA_APP_MIN_SUPPORTED=v0.24.0
   ```

   Then confirm the endpoint reports it:

   ```bash
   curl -s https://api.arconpentapersada.co.id/v1/app/latest
   ```

   The floor is an advisory banner, not a block; the strict gates below are the
   actual enforcement.
5. Flip strict. The same `--update-env-vars` merges this one variable and leaves
   the `VERA_APP_*` set alone:

   ```bash
   gcloud run services update vera-api --project=vera-app-508411 \
     --region=asia-southeast1 \
     --update-env-vars VERA_PO_ALLOCATIONS_STRICT=true
   ```

6. Verify the cutover: a blank draft returns `409 po_blank_draft_retired`; a new
   free line returns `422 po_free_line_retired`; a row discount or a header
   discount/adjustment returns `422 po_discount_forbidden`. A v0.24.0 desktop
   creates only quotation-backed drafts and explicit extra lines, so it saves
   normally. The v0.24.0 purchase-order editor still shows its row-discount
   control; strict mode is what retires the value, so a staff member who sets
   one receives `po_discount_forbidden` and must clear it. That refusal is the
   cutover working, not a defect.

### Rollback

1. **Turn strict off first.** It is configuration, not a deploy:

   ```bash
   gcloud run services update vera-api --project=vera-app-508411 \
     --region=asia-southeast1 \
     --update-env-vars VERA_PO_ALLOCATIONS_STRICT=false
   ```

2. **A server rollback is safe.** Migrations are forward-only
   (`backend/store/migrations.go` applies each file once and has no down path)
   and the newer schema is additive, so an older `veraserver` image ignores the
   new columns and keeps serving. Roll the Cloud Run revision back the normal
   way; do not roll the schema back with it.
3. **Correct a purchase order, do not rewrite it.** The supported per-record
   reversal is another audited correction through Vera, which snapshots the
   replaced values. Stored revisions are immutable and remain verifiable.
4. **A destructive schema rollback is a last resort** and needs a new,
   hand-written forward migration. It cannot restore migration 053's
   one-purchase-order-per-quotation index while any quotation carries more than
   one live purchase order (drafts count). Run the precheck and treat every
   returned row as a refusal:

   ```bash
   psql "$VERA_POSTGRES_DSN" -v schema=vera \
     -f docs/agents/allocation-rollback-precheck.sql
   ```

   Zero rows is the only answer that permits the destructive step. The rehearsal
   in `backend/store/rollout_rehearsal_test.go` proves the same migrations,
   corrections and report figures on a disposable schema; it never writes
   production data.

## Open Dependabot PRs: batch or close

A release that starts with open Dependabot PRs is a release with an unmade
decision, and making that decision at the tag is how six PRs from one sprint
were rediscovered mid-release. So:

- **Never merge Dependabot PRs one at a time onto `main`.** Each is its own
  merge, its own full CI run and its own line in the release notes, and the
  alerts they fix are one decision, not six.
- **Batch them into one issue, one branch, one PR** (`AGENTS.md` §11). Take the
  smallest bump that clears every open alert, and move the direct dependency
  when an alert is nested under it: `vitest@2` carried its own `vite@5`,
  `esbuild@0.21` and `tinypool@1`, and only a vitest major removed them.
- **Close what the batch does not take, with a comment saying why.** Superseded
  by a newer major, the package is no longer in the tree, or the batch
  deliberately does not adopt that major — each is a decision. A closed PR that
  says why is a record; a PR left open until the next release is the same
  rediscovery again.
- **A release starts only when every open Dependabot PR is merged in the batch
  or closed.** Step 1 must report `Dependabot pull requests: 0 open` before step
  6. If it does not, the release is not ready.

Dependency batches are always a **PATCH** release (`AGENTS.md` §11), even when
the only visible change is a lockfile.

## See also

- `AGENTS.md` §11 — the Dependabot read before and after every PR.
- `AGENTS.md` §13 — the testing gates in full.
- `AGENTS.md` §14 — release naming, the artifact table and the announce job.
- `docs/agents/issue-tracker.md` — the issue/branch/PR lifecycle.
- Spec #555 — the agent-environment hardening this runbook is part of.
