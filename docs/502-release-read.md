# #502 release read — CI, migration 041, deploy/rollback impact

**Worktree:** `/var/lib/openbot/OpenBot/Shared/vera-502`, detached HEAD `dca8890` (head of `feat/464-po-sprint`).
**Date:** 2026-10-28. **Author:** Kakiko (release & reliability).
**Read-only assessment:** no code edited, no commit/push/merge/deploy, no workflow changed.
**Sources read:** `.scratch/vera-502/spec.md`, tickets `01` + `04` in `.scratch/vera-502/issues/`, `.github/workflows/` (`pr.yml`, `deploy.yml`, `desktop.yml`, `backup.yml`, `gcp-recon.yml`), `AGENTS.md` §§10–13 (+§§14/16/17 for release context), `docs/backup-restore.md`, `docs/502-verification-plan.md` (Frank), `docs/po-reconciliation-contract.md` + `docs/502-ux-brief.md` (context). `handover.md` §§5/11 **could not be verified** — file absent at repo root and in `docs/` in this worktree (spec says it is uncommitted context in the main worktree).

## 1. Which gates run for a push on `feat/464-po-sprint`, and expected duration

`pr.yml` triggers **only** on `pull_request` → `main` and `push` → `main` (verified in file, lines 11–15). A bare `git push` on `feat/464-po-sprint` runs **no workflow**. Gates run as checks on **PR #484** (each push to the branch = `synchronize` event), plus the post-merge `main` run that seeds the Go cache.

Per-PR gate set (`Gates` workflow):

| Job | What runs (verified in `pr.yml`) | Timeout / expected duration |
|---|---|---|
| Go gates | scope step → full suite unless diff is entirely `frontend/src/**` or `frontend/public/**`; frontend build (go:embed needs `dist`); `gofmt -l`; `go vet ./...`; `#495` dev/production build-matrix vet+test; `go build -tags chromedp ./...` + vet; pg-client-18 install; `go test -race -timeout 30m ./...` (Postgres 18 service, `VERA_POSTGRES_TEST_DSN` on `localhost:55432`); re-run `TestGeneratePDF` and assert `--- PASS` | timeout 45 min; last known full Go run at this head **34m10s** (run `40291930218`, per channel history — not re-verified this turn); expect ~30–40 min cold, faster on warm `main` cache |
| Frontend build | `npm ci`; `npm run build` (strict type-check incl. tests); `npm test` (Vitest pure modules + component suites); Dependabot alert report step (fails only on successful read with critical/high; API failure fails **open**) | timeout 10 min; expect ~3–6 min |

What does **not** run on the branch: `deploy.yml` (push→`main` only, docs-path-filtered), `desktop.yml` (tags `v*` only), `backup.yml` (schedule/manual), `gcp-recon.yml` (manual dispatch, read-only). Merging to `main` deploys the API image; only a `v*` tag builds installers + announce (AGENTS.md §11 note + §14).

Scope-step caveat (verified): `#502` touches `backend/**` (migration 041, store, API) so `needs_go` stays true and the full suite runs. The frontend-only fast path never applies here. The scope step fails open (crashed filter → full run). Concurrency: PR pushes cancel superseded runs; `main` runs never cancel (cache seeding).

## 2. Migration 041 — deploy and rollback impact

**Status:** 041 does **not exist yet** in `backend/store/migrations/` (verified: ledger ends at `040_drop_purchase_order_extractions.sql`). Everything below is the ticket-04/spec contract, not applied SQL.

Contract (ticket 04 + spec §Persistence and API): one additive migration adding reconciliation **session + item tables**, org-scoped, one active session per PO + source revision (uniqueness rule), immutability for sessions attached to saved records. `038` stays as applied history, `040` stays as the extraction drop; 041 is additive.

Deploy mechanics (verified: `AGENTS.md` §3 + `backend/store/db.go` calls `store.Migrate(ctx)` at startup in the configured schema):

- New image on an old DB: migration applies at boot inside the normal forward-migration path (same pattern as 039's `ADD COLUMN IF NOT EXISTS` / 040's `DROP TABLE IF EXISTS` guards). No manual step.
- Old image on a new (041-applied) DB: forward-compatible. Old code never references the new tables; extra tables are ignored. This is the same direction 040 already relies on (ledger must keep matching applied files).

Rollback: **image swap, not a data restore.** Redeploy the previous Cloud Run revision (`gcloud run revisions list` / `gcp-recon.yml` read path; rollback approver + explicit revision auth required per channel memory). Reasons no restore is needed:

- 041 is specified additive: no column drop, no constraint tightening on existing PO rows, no backfill rewriting saved records (contrast 039's trigger-disabled backfill — 041 has no equivalent in the contract).
- Rows written into the new session/item tables by the new image are simply unread by the old image; saved POs, revisions, and corrections keep their existing shapes.
- A data restore (Neon PITR ≤6h window, or nightly `vera-*.dump` via Settings → Restore, which signs everyone out) is only warranted on **data corruption or a destructive DDL**, neither of which 041 is contracted to do. If the implementation deviates (e.g. adds a `NOT NULL` without default, tightens the PO CHECK, or drops anything), this read is void and must be redone.

Residual risk even with additive DDL: a half-applied 041 (unique index present, trigger missing) if the boot migration is interrupted — the deploy health poll (`/health` + auth-gate probe) is the tripwire; Cloud Run keeps serving the prior revision until the new one passes.

## 3. PR #484 draft state and the no-merge / no-production-deploy rule

- Spec status line (verified): **branch `feat/464-po-sprint`, head `dca8890`, PR #484 stays draft**. Out-of-scope list explicitly names: merging PR #484 and any production deploy (spec §§Out of Scope, Further Notes).
- Live PR state (draft vs open, check runs on head SHA) **could not be verified from this worktree** — no `gh` auth/token in this environment and no network GitHub read was attempted. Treat the spec's "stays draft" as the standing order, not as a live PR read.
- Rules that enforce it: `deploy.yml` fires only on `main` (a merge deploys `veraserver` to Cloud Run automatically); `desktop.yml` fires only on a `v*` tag and its announce job is what makes installed desktops offer the version. So **merge = API deploy, tag = desktop release**. Neither happens from the feature branch. Any merge/deploy/rollback needs explicit user authorization naming the revision (standing remit).
- Worktree hygiene (verified `git status`): this tree already carries uncommitted ticket-01/02/UX-brief work on top of `dca8890` (`GLOSSARY.md`, `App.tsx`, `AppShell.tsx`, editors, `pageHistory.*`, contract fixtures/tests, `.scratch/`, `docs/`). That drift is why the verification plan declares any result on this tree a pre-check, not a verdict (§1).

## 4. What must be green before a Judge read

Per `AGENTS.md` §13 + §12.5 and Frank's `docs/502-verification-plan.md` (§§1/6/8 — plan only, not a verdict):

Local gates (each with direct exit code, no piped `$?`):

1. `gofmt -l .` → empty; `git diff --check` → clean.
2. `go vet ./...` (plus the `#495` dev/production matrix and `go vet -tags chromedp ./...` — CI-only, but a broken dev tag is invisible locally otherwise).
3. `go test -race -timeout 30m ./...` **with** `VERA_POSTGRES_TEST_DSN` — and the skip-count asserted at **0**. Without the DSN the store/API/CLI suites skip and exit 0, proving nothing.
4. `npm --prefix frontend test` and `npm --prefix frontend run build` (build type-checks tests too).
5. `bindings_test.go` (`TestGeneratedBindingsCoverEveryBoundMethod` / `...NoOrphans`) after any Wails surface change — bindings are regenerated text, never hand-edited.
6. `scripts/ui-snapshot.sh` PNGs read at 360 + 1280 (layout only, never committed) **plus** the hand-driven record: 1024 boundary, keyboard/focus/paste, sticky rail/sheet, blocked-Final-Save (no API call, first blocker focused), stale-version conflict, reduced motion, screen-reader announcements.
7. Per-ticket red proofs: each new guard seen failing (exit 1) on a deliberate mutation, then restored green — notably position-keyed line identity (`L-1`) and the live-pane→finalize payload mapping (`F-9`).

**Postgres integration gate + DSN blocker (the load-bearing one):** CI provides Postgres 18 + `VERA_POSTGRES_TEST_DSN=postgres://vera:secret@localhost:55432/quotes?sslmode=disable`. Locally the DSN must point at a **throwaway Postgres 18** (`postgres:18-alpine`, client major 18 — newer client vs older server fails on `transaction_timeout`). Tests drop schemas; never production. **Without a valid DSN there is no persistence verdict** — §6 of the verification plan is blocked (no local Postgres, Docker socket inaccessible), and any `go test` green without it is stale for tickets 04–06. The pre-Judge run must log the DSN identity (host only, never the secret), the full 40-char SHA, empty `git status`, and per-command exit codes; a PASS on any other SHA/tree is stale.

## 5. Any CI change needed?

**None.** Verified reasoning:

- `pr.yml` already runs the exact set #502 needs: full race suite with real Postgres 18, chromedp-tag build, dev/production tag matrix, frontend build+tests, Dependabot tripwire. Backend-touching diffs always take the full path.
- `deploy.yml` path-filters exclude `docs/**` but **not** `CHANGELOG.md` (embedded copy) — correct as-is; no #502 doc change can silently skip a needed deploy, and #502 itself must not deploy anyway.
- Migration 041 needs no workflow edit: applied by the server at boot, covered by the existing `backend/store` Postgres suites.
- Adding a workflow, trigger, or secret for this feature would itself be a reviewed decision (public-repo fork-PR threat model, `AGENTS.md` §16). Not warranted.

If CI proves too slow (Go ~35 min), that is a scheduling observation, not a change request in this ticket.

## 6. Release risks and the one thing most likely to break

Risks (ordered by blast radius):

1. **Line-identity mapping across draft save/reopen** — today draft saves delete + reinsert lines and the current comparison keys by line position (`po_review.go` / `poFinalize.ts`), while the contract demands stable id/client-key identity with no re-keying on reorder and `line_removed` history on delete (Kaluna §8, Frank `L-1`/`L-2`, `PG-08`). A miss here silently re-keys decisions onto the wrong line — wrong POs finalize.
2. **Live-pane decisions not reaching the Final Save payload** (`F-9`, Jessy §flag-1): server recomputes from submitted values and refuses; UI shows resolved while the API refuses (or vice versa). Most likely user-visible break: stuck Final Save with a clean-looking pane.
3. **Migration 041 guards too strict or too loose**: unique-active-session rejects a legitimate repair retry; immutability trigger blocks draft writes; or (worse) the gate trusts the session alone and finalizes undecided diffs.
4. **Tax-basis triple** (zone/type/service-category) link-time freeze vs later correction path still product-undefined (Kaluna §8, HR-03) — an inline "use Quotation" or a post-link write that the store refuses = dead-end UI.
5. **Unknown source reference**: pane offers acknowledgement, saved-reference validator still refuses (Kaluna §8, `L-4`) — unfinalizable PO with no corrective action.
6. **1024 boundary + 200% zoom untested by harness** (360/1280 only) — rail/sheet/action-bar overlap ships unseen.
7. **Long-suite flake**: 30-min Postgres suite near the 45-min job cap; a timeout reads as a hang, and any rerun must be same-SHA (a green on another SHA is stale per Checker rules).

**The one thing most likely to break: the stable line-identity + save mapping (#1), surfacing as #2.** Everything the user sees (notices → items → keep/use → count → Final Save gate → archive history) hangs off item keys surviving a draft save round-trip. The current implementation's position keys + delete/reinsert saves are exactly what the contract says must change, and tickets 04–06 all assume the new mapping. If it slips, Final Save either blocks forever on phantom items or — worse — finalizes with decisions attached to the wrong lines. The `L-1` reorder red proof is the single highest-value guard in the plan.
