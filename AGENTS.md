<!-- ensemble:start v7 -->
# Working inside Ensemble

You are one member of an Ensemble team, not a standalone assistant. Ensemble
injects your name and role. Reply as a colleague in the group chat: direct,
specific, readable Markdown.

## Your final text

Your final native text is the group message. Write it once; you do not need
`send_message` to chat.

- `$Name` routes work to that roster agent: they get a turn and answer in the
  group. `!Name` is accepted.
- A bare name is a reference only; it creates no work.
- Normal worker turns route `$Name` to a different teammate. Terminal turns
  treat `$Name` as a reference; the next section explains them.
- `@path` is a workspace file reference, never an agent or a skill.
- Code spans, URLs, and paths are literal: `$Name` there never routes.
- A native prompt belongs to its session agent; self-mentions are skipped.
  Direct conversations are one-to-one and never route.
- The server owns routing, budgets, and completion. Do not invent a
  recipient, a budget, or a completion hop.

## Terminal turns and the next ticket

One synthesis per command: the command root writes it. Intermediate hops
relay their child's result upward; they do not wake the leader or write a
second synthesis.

Synthesis, completion-review, and batch-review turns are terminal. In a
terminal turn `$Name` is only a reference: it starts no teammate work. The
final text still posts as the group message.

To start the next ticket, the leader calls `ensemble_dispatch` from a
terminal turn with a title, an instruction, and an assignment batch. It
creates a new command root with a fresh group and command budget, not a
continuation of the previous command. One terminal turn starts at most one
new command; a retry returns the existing command and assignments.

Normal worker, handoff, and peer turns are not terminal: `$Name` there
routes work to a different teammate as usual.

## Truthful coordination

Do not claim a teammate replied, agreed, or finished until their turn is
visible in the conversation. A delivered message is not an acknowledgement.

When you ask a teammate, their reply is the answer you deliver. Do not open a
second completion loop for a teammate you asked; your final text and the
server's completion decision already carry the result.

## Dispatch budgets

Four layers bound work: Task, Group, Command, and Depth. `ensemble_turn`
shows each used and remaining value; successful tool results show what is
left. You cannot raise a ceiling. On exhaustion, return your result to the
leader. A leader may continue an exhausted group with `ensemble_delegate`:
pass `continuationOf` (the exhausted group id) and a required `groupMaxHops`
tier (8, 16, or 24) that fits the command remaining. Only the leader may open
one; peers return their result instead. Naming an active continuation is
refused. When the command or continuation cap is reached, ask the user for a
new command for a fresh budget. Completion, batch review, synthesis, peer
turns, retries, restarts, recovery, and idempotent replays consume no hops;
cancellation does not refund them.

A terminal continuation with `ensemble_dispatch` starts a new command root
with a fresh group and budget and creates its assignment batch. The batch is
preflighted before any work exists; a refusal creates nothing and is visible.
A retry returns the existing command and assignments.

## Migration tools

`ensemble_delegate` (leader) and `ensemble_handoff` (peer) are migration and
rollback tools, not the normal chat path. Prefer `$Name` in your final text.
While the explicit surface is enabled they require `maxHops`; `maxHops: 0`
is a one-shot ask (answer and stop). Write a self-authored instruction —
what to do, with what context, and what good looks like — never paste the
user's message. Keep `task` short.

1. Write `.ensemble/handoffs/<task>-<from>-<to>-<timestamp>.md`.
2. Frontmatter: `ensemble`, `version`, `workspace`, `task`, `instruction`,
   `from`, `to`, `created`.
3. Sections: `# Handoff`, `## Task`, `## Instruction`, `## Constraints`,
   `## Expected`, `## Details`.
4. Call `ensemble_handoff` with the path, recipient, and `maxHops`. Never
   claim a path that does not exist.
5. Handoff already queues the hop: do not follow it with `ensemble_delegate`.
   Your turn waits for the relayed answer.

## Batch review

Several completions coalesce into one batch review turn (~1s quiet window or
when you are free). Decide every item with `ensemble_review` (`delegate`,
`start`, `end`, or `dismiss`); a `delegate` decision requires `maxHops`.
The batch closes only when every item is decided. Non-leader recipients hand
it onward (default leader); the leader may end it.

## Tools

Decision tools remain: `ensemble_task_status`, `ensemble_review`,
`ensemble_dispatch` (leader-only terminal continuation), `ensemble_ask_person`,
`ensemble_remember`, and `ensemble_share`.
Roster, conversation, and budget arrive in `ensemble_turn`; do not call a
read-only tool for them.

## Team roster

Every active workspace member. Title and role are stored values.

| Member | Title | Role |
|---|---|---|
| **Clark** (leader) | Chief of Staff | Chief of Staff that manages employees, The Engineering Manager for Vera who own delivery from approved goal to shipped change.<br><br>READ AGENTS MEMORIES FOR DETAILS. |
| Angkasa | Backend Engineer | Backend Engineer for Vera. Own the Go backend and PostgreSQL persistence.<br><br>READ AGENTS MEMORIES FOR DETAILS. |
| Frank | QA/Verification Engineer | QA and Verification Engineer for Vera. Assume the implementation is wrong until you prove otherwise.<br><br>READ AGENTS MEMORIES FOR DETAILS. |
| Jessy | UI/UX & Interaction Engineer | UX/UI and Interaction Designer for Vera. Own the experience staff actually use.<br><br>READ AGENTS MEMORIES FOR DETAILS. |
| Kakiko | Release & Reliability Engineer | Release and Reliability Engineer for Vera who own CI truth and release readiness. READ AGENTS MEMORIES FOR DETAILS. |
| Kaluna | Technical Architect/Code Reviewer | Technical Architect and Code Reviewer for Vera. Own the technical shape before parallel work begins.<br><br>READ AGENTS MEMORIES FOR DETAILS. |
| Loki | Frontend Engineer | Frontend Engineer for Vera. Own the React 19 and TypeScript frontend.<br><br>READ AGENTS MEMORIES FOR DETAILS. |
| Yara | Product Analyst/Domain Specialist | Product Analyst and Domain Specialist for Vera. Own what staff actually need and why.<br><br>READ AGENTS MEMORIES FOR DETAILS. |

## Boundaries

One workspace is one directory. Do not place credentials, conversation
transcripts, or runtime memories in `.ensemble/` or any repository file.
`.ensemble/config.json` is editable for workspace settings, but do not edit
it while the workspace is changing in the TUI. Do not remove or edit the
`ensemble:start` block. Respect native permission prompts for external
directories.
<!-- ensemble:end -->

# AGENTS.md

Operational guide for coding agents working in `vera-app`. Read this before making
changes.

**Work is orchestrated — see Section 12 before starting an issue.** It governs how
the rest of this guide is applied.

---

## 1. Project Snapshot

- **App:** Wails v2 desktop quotation generator for PT Arcon Penta Persada.
- **Backend:** Go in `backend/` — quotation pricing, HTML templating, PDF rendering
  via `chromedp` (headless Chrome `PrintToPDF`).
- **Persistence:** PostgreSQL via `backend/store` (pgx v5), used by `cmd/veraserver`
  and the CLI integration tests, configured entirely by environment variables. The
  desktop never opens a database; it talks to the API over HTTP.
- **Frontend:** React 19 + TypeScript + Vite in `frontend/`, styled with Tailwind CSS
  (v4).
- **Go module:** `vera-app` (Go 1.26).
- **Wails bindings:** `frontend/wailsjs/go/*` are generated. Treat them as build
  output; keep them in sync when Go signatures change.

## 2. Repository Map

| Path                                        | Purpose                                                                    |
| ------------------------------------------- | -------------------------------------------------------------------------- |
| `main.go`                                   | Wails startup, API endpoint resolution, binding registration               |
| `cmd/vera/`                                 | The `vera` CLI: every feature as a command, plus local validate/render     |
| `cmd/veraserver/`                           | The API server binary                                                      |
| `docs/cli.md`                               | The CLI guide: every command, the input format, exit codes                 |
| `backend/quotation.go`                      | `Prepare`/`Export` flow, input + data models                               |
| `backend/builder.go`                        | Per-item pricing, totals, markup + per-row discount math                   |
| `backend/tax.go`                            | Tax zone/category → basis-point rates, tax + nett                          |
| `backend/preview.go`                        | Customer/internal preview DTOs, template HTML preview                      |
| `backend/archive.go`                        | Archive list/detail/revision Wails methods                                 |
| `backend/format.go`                         | Rupiah + description formatting                                            |
| `backend/template.go`                       | Embedded template/asset loading                                            |
| `backend/theme/`                            | Brand theme: four bases → derived role palette for both renderers + portal |
| `backend/store/`                            | Postgres config, migrations, repositories                                  |
| `backend/template/html/template-fixed.html` | Customer-facing PDF/preview template                                       |
| `backend/template/css/template-fixed.css`   | PDF/preview styles                                                         |
| `frontend/src/App.tsx`                      | Application shell/state, page routing                                      |
| `frontend/src/components/`                  | UI components (shell, ledger, review, archive, detail)                     |
| `frontend/src/tax.ts`                       | Tax options, rates, paste normalizers                                      |
| `frontend/src/format.ts`                    | IDR formatting + client-side margin and discount math                      |
| `frontend/wailsjs/go/*`                     | Generated Wails bindings                                                   |
| `.env.example`                              | Documented Postgres variable names (no secrets)                            |
| `test-data/`                                | Local Excel fixtures (gitignored)                                          |

## 3. Setup Commands

Run from the repository root unless noted.

```bash
go mod download
npm --prefix frontend install
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Postgres must be configured for the API server (`cmd/veraserver`) to start; the
desktop does not open a database. Create a local, gitignored `.env`:

```bash
VERA_POSTGRES_DSN=postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=require
VERA_POSTGRES_SCHEMA=vera
```

Never commit credentials or a real DSN. `.env.example` documents the variable names.
The server runs versioned migrations in the configured schema at startup.

## 4. Development Commands

```bash
# Terminal 1: local API on 127.0.0.1:8081 (sources .env, forces the loopback bind
# and --local-development, which refuses non-local database targets before open)
scripts/dev-api.sh

# Terminal 2: desktop dev loop (reads only VERA_API_URL from the environment or .env)
wails dev

# Frontend-only loop
npm --prefix frontend run dev
```

`scripts/agent-env.sh` brings the local Postgres, the dev API and the Chrome/PDF
wrappers up, down and checked in one persistent workspace. The commands and the
test DSN are in `docs/agents/local-dev.md`.

## 5. Build Commands

```bash
# The CLI. Links no cgo -- rendering is pure Go -- so it cross-compiles anywhere.
go build -o vera ./cmd/vera
go build -tags chromedp -o vera ./cmd/vera   # the browser engine instead
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o vera ./cmd/vera

# Frontend type-check + production bundle (also the TypeScript gate)
npm --prefix frontend run build

# Compile all Go packages (compiles Wails and its dependencies too)
go build ./...

# Wails desktop build
wails build
wails build --clean
```

Platform helper scripts live in `scripts/`.

## 6. Lint / Format / Static Analysis

There is no frontend lint script and no Makefile task.

```bash
gofmt -w .            # format (writes)
gofmt -l .            # format check (must print nothing)
go vet ./...          # static analysis
npm --prefix frontend run build   # strict TypeScript validation
scripts/check-agent-docs.sh       # every docs/agents pointer in AGENTS.md resolves
```

## 7. Test Commands

Go tests cover the backend. The frontend's **pure** modules — the draft payload
parser, the release-notes parser, `tax.ts` normalizers, `format.ts` and
`computePricing` — are covered by **Vitest**: `npm --prefix frontend test`.
Vitest shares Vite's transform and resolver, so a test is checked the same way
`vite build` checks the code it imports. There are no component or DOM tests;
anything needing a browser is driven by hand.

```bash
go test ./...                                # all packages
go test -tags chromedp ./...                 # the browser engine (needs Chrome to render)
go test ./backend                            # quotation/pricing/tax/archive units
go test ./cmd/vera                           # CLI dispatch, input model, verify parser
go test -race ./...                          # race detector
go test ./backend -run '^TestBuildPricing' -v
```

`cmd/vera`'s end-to-end test (`TestEndToEndThroughTheCLI`) drives the whole CLI
in-process against a real `veraserver` and a throwaway schema: login → clients →
validate → export (a real PDF and a signed revision) → verify → archive → drafts
→ logout. It skips without `VERA_POSTGRES_TEST_DSN`, and it drops the schema it
uses — point it only at a throwaway database. `backend/store` and `backend/api`
skip the same way (the store's `testDSN` and the API's `testDatabase` carry the
empty-DSN guard), so a bare `go test -race ./...` passes with no database — and
the DSN is what makes the integration paths actually run.

The store and API suites do not replay the migrations per test. Both provision
their databases through one test-only package, `backend/testdb`, which once per
test process creates and migrates a template database, then gives each test a
`CREATE DATABASE … TEMPLATE` clone and drops it in `t.Cleanup`. The template is
named after the test binary, so `store.test` and `api.test` never share (or drop)
each other's template when `go test ./...` runs them in parallel against one
database. The template is migrated with the app schema `vera_test` and every
clone keeps it: Postgres does not dump the built-in `public` schema, so the
restore drill could neither restore a dump of its own clone after `ResetSchema`
drops the schema, nor prove the dump names the app schema. The schema name is
baked into the trigger functions the migration chain creates, so it is fixed
rather than renamed per clone. The role in `VERA_POSTGRES_TEST_DSN` therefore
needs `CREATEDB` (the helper names the missing privilege rather than falling
back).

`openTestSchema` (store) and `testServer` / `adminTestSetup` / `draftTestServer`
/ `openTestStore` (API) keep their names and signatures and wrap the shared
helper plus `store.Open`. Repeated helper calls in one top-level test with one
identity reach the same clone — subtests included — which is how a server and a
direct store share data. The API restore drill dumps its own clone, so the
safety snapshot and the upload are scoped to that test's database, and it
asserts the dump's table of contents names only the app schema. The production
restore handler still restores the whole database.

Tests that assert migration behaviour are the exception: they keep a genuinely
fresh scratch schema on the base database (`resetTestSchema` + `loadMigrations`),
because a clone is already migrated and cannot replay the chain.

The Postgres repository test is skipped unless a test DSN is supplied. Use a
throwaway database/schema, never the production one:

```bash
VERA_POSTGRES_TEST_DSN='postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=disable' \
  go test ./backend/store -run TestQuotationRevisionPersistence -v
```

`TestGeneratePDF` exercises `chromedp` and needs a working Chrome runtime.

The disposable test database major must match the toolchain pin (currently
Postgres 18: run `postgres:18-alpine` locally, same as `.github/workflows/pr.yml`,
client from `postgresql@18`). A newer dump/restore client against an older
server fails (`unrecognized configuration parameter "transaction_timeout"`).

## 8. Code Style Guidelines

### General

- Keep changes small, targeted, and aligned with nearby code (KISS/YAGNI).
- Preserve boundaries: pricing/domain logic in Go, presentation in React.
- Prefer explicit control flow and clear state transitions.

### Go

- Run `gofmt` on every edited Go file.
- Import groups: stdlib, then external/module imports.
- Remove unused imports and dead code before finishing.
- Exported identifiers `PascalCase`; unexported `camelCase`; keep acronyms
  consistent (`PDF`, `HTML`, `CSS`, `IDR`).
- Check every returned error; wrap with context: `fmt.Errorf("context: %w", err)`.
- Validate inputs before expensive operations or file writes.
- `os.WriteFile` uses explicit mode (`0o644` unless required otherwise).
- Use `uint64`/`int64` minor-unit amounts in the company currency; never float for money. Multiply before divide.
  Signed results (profit/loss) use `int64`.

### React / TypeScript

- Use ES modules; keep imports minimal and consistently ordered.
- Use configured alias imports (`@backend/*`) for Wails-generated bindings.
- Keep TypeScript strict-mode compatible (`frontend/tsconfig.json`).
- Explicit types for component props and public utility APIs.
- Components/files `PascalCase`; functions/variables `camelCase`.
- All money amounts render with `tabular-nums` and `whitespace-nowrap`.

### Styling

- Tailwind is the primary styling path; use the tokens in
  `frontend/tailwind.config.cjs` (colors, type scale, shadows, easing).
- Keep `frontend/src/index.css` minimal and global-only.
- Tests live in `src/` beside the module they cover, named `<module>.test.ts`,
  and run with `npm --prefix frontend test`. A test is not shipped without the
  runner to execute it.
- **Tailwind is v4** (`tailwindcss ^4.3.3`, loaded via `@import "tailwindcss"`
  plus `@config` in `frontend/src/index.css`). Numeric `min-h-*` compiles two
  ways: the custom `minHeight` scale in `tailwind.config.cjs` (6–14) and v4's
  dynamic spacing scale (any other number, e.g. `min-h-13`). `line-clamp-*` is
  a v4 core utility — no workaround needed. A class that compiles to nothing
  still looks correct in source, so verify any unfamiliar utility actually
  appears in `frontend/dist/assets/*.css` after a frontend build.
- Do not change the light theme or introduce competing styling systems.

## 9. Wails Integration Rules

- Frontend-callable methods must be exported on bound structs.
- If method signatures change, regenerate `frontend/wailsjs/go/*` with
  `wails generate module` and commit it. This is enforced rather than asked for:
  `bindings_test.go` in the root package fails when a bound method is absent
  from the generated file, or when the generated file names a method Go no
  longer has. It went stale once (#294 added `ExportTo` and `RenderPDF` without
  regenerating, and nothing noticed) and that is what the test exists to stop.
  It needs no Wails CLI — the generated files are text and the Go side is
  reflection.
- Keep `Startup` initialization safe for both runtime and tests.
- `main.go` resolves the API endpoint through the build-selected `apiBaseURL`
  before `wails.Run`: packaged builds use the production API, `wails dev` builds
  read only a loopback `VERA_API_URL` and fail clearly when it is absent.
  `cmd/veraserver` opens Postgres; the desktop never does.

## 10. Domain Invariants (do not break)

- **Per-item margin:** each line has an independent `MarginEnabled` + `MarginPercent`.
  `MarginalizedUnit = UnitCost + UnitCost * margin% / 100`, then multiply by quantity.
- **Two taxes, never the same treatment** (issue #270):
  - **PPN** (11%, non-FTZ goods only) is charged **on top of the price**. The customer
    pays `SellGrand + PPN`; it is collected for the state and is **never profit**.
    It prints on the customer quotation and on the verification page.
  - **PPH23** (2% non-construction, 1.75% construction, services only, any zone) is
    **internal**: calculated once on the sell grand after discount
    (`Tax = Grand * bps / 10000`), never allocated per line, never printed.
- **Nett profit** = sell grand after discount − PPH23 − cost (signed). It must never be
  computed from the customer total, or the PPN we collected counts as our profit.
- **The customer PDF/template shows marginalized prices, plus PPN when charged.**
  Margin percentage, PPH23, and nett profit must never appear in the customer output.
  There is a test asserting this (`TestRenderedQuotationHidesInternalValues`) — keep it
  green.
- **Revisions are immutable:** first export is revision `00`; each edit creates the
  next revision. Never mutate a stored revision.
- **A discount cuts the price only.** What we pay suppliers does not move when the
  customer is given a lower price, so the cost side carries no discount at all:
  `OriginalGrand == OriginalSubtotal`, and profit loses the full discount. Both
  `buildPricing` (Go) and `computePricing` (SummaryRail) implement this and must agree.
- **Per-row discount** (issue #274): each line and sub-item carries its own
  `DiscountEnabled` + `DiscountPercent`, mirroring margin. Order is **margin first,
  then discount** — the marginalised figure is the list price, and the discount
  comes off it, so the printed list price always equals what staff see in the
  editor. The cost side still does not move, so a row discounted below cost shows a
  **negative** margin: `MarginAmount` is signed, and unsigned arithmetic would wrap
  a loss round as a huge gain. The whole-quotation discount still applies on top,
  to the summed subtotal. The customer document's `Disc.` column appears **only when
  some row carries one** (`applyRowDiscountColumn`), so a column is never added to
  every quotation to be empty on most of them.
- **Adding a signed field to the canonical payload:** emit it only when non-zero,
  exactly as `Subs` does. A revision with no per-row discount then produces
  byte-identical canonical bytes to every revision signed before the field
  existed, so every printed verification code keeps verifying. **Never bump
  `CanonicalPrefix` for this** — that stops every code already printed from
  verifying. `TestCanonicalOmitsAnAbsentRowDiscount` pins it.
- **Numbering:** quote numbers are plain-digit `15CCYYNNNN` (no grouped display),
  allocated per (customer, issue-year) cycle starting at 1 every January; past 9999
  the sequence widens to 5 digits instead of wrapping, with a warn flag at 8000.
  Numbers are issued in the creation year (no backdating). UUID on `quotations`
  is the stable external key; the bigint id stays the relational key.
- **Company profile** is a singleton row edited only via admin endpoints and shown
  only in the admin Company settings section. Only the name and address print
  on quotations (phone/email/NPWP appear nowhere in the template); the NPWP
  field is hidden from the form while its column stays dormant for invoices.
  Artwork uploads to `logo_bytes` (512KB cap, allowlisted types) and prints
  profile-first with baked-asset fallback. Header edits affect future
  quotations only — archived snapshots keep theirs.
- **Brand theme:** eight `#RRGGBB` columns (4 quotation + 4 portal, independently
  settable, strict hex validation, seeded with today's palette). Empty means
  the stylesheet default. Portal UI copy stays hardcoded (it is language, not
  company data); issuer falls back to the live profile name.
- **Clients are database rows, never code:** the editor dropdown, Prepare
  resolution, and peek validation all read the client directory (server via
  store, desktop via fetched cache). Codes are immutable once created.
  Codes 02-06 were seeded by migration 011 from the retired hard-coded maps.
- **Email is a sign-in identifier, not a delivery channel:** nothing is sent to it;
  if any feature delivers credentials or links by email, verification must gate it first.
- **Verification:** every revision is signed at export (Ed25519, key from
  `VERA_SIGNING_KEY`, never in source or the database) over a canonical field
  set pinned by `backend/signing.go`'s format test. The public portal at
  `/v/<shortId>` (and `POST /verify`) renders valid / superseded / mismatch /
  not-found. `short_id` identifies one _revision_. Never change the canonical
  format without a migration story: existing printed codes must keep
  verifying. TLS is Google-managed — never self-signed.

---

## 11. GitHub Issues as Task Management

All work is tracked via GitHub Issues at **`arcon-external/vera-app`**.

> Always pass `--repo arcon-external/vera-app` explicitly. The same GitHub account
> owns unrelated projects; filing here by default is not guaranteed. Never file
> vera-app work in another repository.

### Issue lifecycle

```
Open → Label & Assign self → Plan (comment) → Branch → Implement → Test
     → PR → Merge → Delete branch → Close → Next issue
```

- Label: assign a proper label (`development`, `documentation`, `bug`, `enhancement`,
  `qa-audit`).
- Assignee: assign yourself.
- One issue = one branch = one PR. Never work on `main` directly.

### Issue body (problem statement)

Every issue must contain:

- **Title:** imperative mood with a conventional-commit prefix (`feat:`, `fix:`,
  `chore:`, `tune:`, `docs:`, `refactor:`, `test:`).
- **Problem:** what is broken or missing, with exact `file:line` references.
- **Goal:** what "done" looks like.
- **Scope:** what is in and out.
- **Success Criteria:** checklist of verifiable conditions.

### Issue comment (implementation plan)

The first comment on every issue must contain:

- Step-by-step implementation plan.
- File-by-file changes.
- Test plan.
- Verification steps.

### Issue → Commit linking

Every commit that resolves an issue must reference it (same repository number):

```
fix: description of change

Resolves #37
```

For multiple commits per issue, each commit references the issue number.

### Branch & Pull Request workflow

1. **Branch naming:** `<type>/NNN-short-slug`, e.g. `feat/89-archive-filter`,
   `fix/76-pdf-margin`.
2. **Branch from up-to-date `main`:**
   ```bash
   git checkout main && git pull --ff-only && git checkout -b feat/NNN-slug
   ```
3. **Work only on that branch** — one issue per branch, one PR per branch.
4. **PR:** title = the conventional commit message; body = short summary +
   `Resolves #NNN`; base = `main`.
5. **Merge:** squash-merge so `main` stays linear, keeping `Resolves #NNN` in the
   squash body so GitHub auto-closes the issue:
   ```bash
   gh pr merge --squash --delete-branch
   ```
6. **Delete the branch** after merge (the flag above does it).
7. Pick the next issue and repeat.

> **Merging to `main` deploys the API, not the desktop.** `deploy.yml` builds and
> deploys `veraserver` to Cloud Run on every push to `main`. Desktop installers and
> the updater are built only by a release tag (`desktop.yml`); a merge is not a
> release (Section 14).

> **Sprint integration branch.** A parent issue with an approved set of child
> tickets may use one integration branch and one PR. Every child is still planned,
> implemented and judged on that branch; the PR body resolves the parent and every
> child; each child's plan comment records its commit and verdict. This is the only
> exception to the one-issue-per-branch rule.

> **Cross-repo transfer is not available.** GitHub only allows issue transfer within
> the same owner; `arcon-external/vera-app` and a personal-owner repo cannot exchange
> issues. File in the right repo the first time.

### Dependabot — before and after every PR

Dependabot is read **twice for every pull request, and both reads are mandatory**.
The command is the same each time:

```bash
gh api "repos/arcon-external/vera-app/dependabot/alerts?state=open" \
  --jq '.[] | "\(.security_advisory.severity)\t\(.dependency.package.name)\t\(.security_vulnerability.first_patched_version.identifier)"'
```

**Before opening the PR.** Run the command and put **what it printed** in the PR
body — the alert lines themselves, or the literal `Dependabot: 0 open` when it
printed nothing. Then decide explicitly what this PR does about the result:

- **Nothing open:** that line is the whole record.
- **Something open:** either fix it in this PR, or say in the PR body why it is out of
  scope and file it as its own issue. An alert that is seen and silently left is the
  failure this rule exists to prevent.

**After the merge.** Run it again and record the result as a **comment on the issue
you just closed**. A merge is not finished until this is done — the comment is what
makes the read visible afterwards, which a merge otherwise has no place for.

> **What enforces this now.** A `pr.yml` step ("Report Dependabot alerts", in the
> Frontend build job) queries the open-alert list on every PR and writes it to the
> job summary, so the "before" read is visible without anyone typing anything. It
> fails the job only when a _successful_ read finds a `critical`/`high` alert:
> `low`/`medium` are reported without failing, and any API failure — bad
> permission, rate limit, network, an unexpected response — warns and passes, so a
> blind check never blocks a PR. That fail-open is load-bearing, not decorative:
> `GITHUB_TOKEN` has returned HTTP 403 on this endpoint even with broad grants
> (upstream discussion `github.com/orgs/community/discussions/60612`), while the
> docs promise `security-events` only for code-scanning alerts — so until a real
> run shows a successful read, treat a green tick as "no blocking alerts _that this
> check could see_" and keep doing the manual reads above. By §12.5's own standard
> this step becomes a guard the day a run proves it can go red; today it is a
> visible reminder with a tripwire attached.

**When something is open**, fix it immediately and release a patch (Section 14):

1. File one issue for the batch and branch as usual.
2. Upgrade the flagged packages to at least `first_patched_version`, preferring
   the smallest bump that clears every alert. An alert nested under a direct
   dependency is usually cleared by moving that dependency, not the transient
   one: `vitest@2` carried its own `vite@5`, `esbuild@0.21` and `tinypool@1`,
   and nothing short of a vitest major removed them.
3. Refresh `frontend/package.json.md5` (`md5 -q frontend/package.json >
frontend/package.json.md5`). It is tracked and must match
   `frontend/package.json`, but it is a **Wails artifact rather than a CI
   fingerprint**: nothing in `.github/` or `scripts/` reads it, so a stale copy
   fails nothing and the step is hygiene, not a gate. `wails build` rewrites it,
   which is how it drifts.
4. Run every gate (Section 13) and release a PATCH: a dependency bump is not a
   feature.

Most alerts here are dev-only (`vite`, `vitest`, `esbuild`, `tinypool`), which
is why the patch may carry no user-visible change — that is still a release.
The point is that the alert list is read rather than assumed, because a
transitive dependency silently pinning an old copy is exactly the thing nobody
notices until it is exploitable in a shipped artifact.

### Closing issues

Close the issue only after:

1. The PR is squash-merged to `main` (the `Resolves #NNN` body auto-closes it).
2. All gates pass (Section 13).
3. The issue's success criteria are met.

---

## 12. The Orchestrated Workflow

Work is run by an **orchestrator** — the assistant in this session — acting as
**chief of staff**. The orchestrator does not write the implementation. It
decomposes the problem, briefs the people who do the work, validates what comes
back, runs the gates, commits and reports. Two of those people are **persistent
Antigravity sessions** driven through Herdr: the **Planner** (Gemini 4 Neon High)
and the **Judge** (Gemini 4 Neon High). **Workers** are OpenCode subagents on
`opencode-go/deepseek-v4.1-flash#max`. No implementation starts without a plan the
user has approved, and nothing merges without a Judge verdict.

### 12.1 The roles and the three persistent sessions

| Role             | Who                                                        | Job                                                                                       |
| ---------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| **Orchestrator** | this OpenCode session (chief of staff)                     | decompose, brief, dispatch, validate, run gates, commit, merge, report. No IC coding.      |
| **Planner**      | persistent agy TUI, Gemini 4 Neon High                        | turn a problem/feature/revision into a mature, performance-aware, engineered plan          |
| **Worker**       | `subagent` with `opencode-go/deepseek-v4.1-flash#max`      | implement one closed slice, with its test, and stop                                        |
| **Judge**        | persistent agy TUI, Gemini 4 Neon High                     | independently verify a finished ticket/PR and return PASS/NOT PASS                        |

The Planner and Judge are **not** OpenCode subagents: they are the two agy panes
below. Both run Gemini 4 Neon High (`agy --model gemini-4-neon-high --effort high`
at the time of writing). Workers never use a fallback
model; the requested `#max` variant is part of the dispatch.

#### 12.1.1 The session contract (Herdr)

Three long-lived roles in one Herdr tab:

| Role         | Herdr pane / agent                  | Runtime identity                                              | Address it with                                      |
| ------------ | ----------------------------------- | ------------------------------------------------------------- | ---------------------------------------------------- |
| Orchestrator | `w9:p14` (agent `vera-640`)         | OpenCode `ses_f13c0a26dffel94rbZI4jwKnL8` (`$OPENCODE_SESSION_ID`) | `opencode api POST …/prompt`                         |
| Planner      | `w9:p15` (top-right, agent `planner`) | agy conversation `a3a93336-3d4b-445e-bb5d-e4b194cb1797`     | `herdr agent prompt planner '<text>'`                |
| Judge        | `w9:p16` (bottom-right, agent `judge`) | agy conversation `d080d68f-f93c-440e-8f8f-243a2ecaff7e`   | `herdr agent prompt judge '<text>'`                  |

The layout is a full-height left pane (orchestrator) and a right column split into
Planner above Judge. Re-read the live mapping with:

```bash
herdr agent list
herdr pane layout --pane "$HERDR_PANE_ID"
echo "$OPENCODE_SESSION_ID"
```

If the tab is recreated, update this table in the same commit as the new IDs and
restart the watcher. Resume an agy conversation with `agy --conversation <id>`
(plus its model flags) when the pane is gone; never start a bare `agy` or
`opencode` for messaging, because that creates a new session.

**Workspace and documents.** Code worktrees and the clean read tree live under
`/home/tristan/dev`, never `/tmp`. The current clean tree is
`/home/tristan/dev/vera-app-main` at `origin/main`; the user's original
`/home/tristan/dev/vera-app` checkout is dirty and is never reset or overwritten.
Agent documents — dispatch briefs, plans, reviews, prototypes and reports — live
under `/home/tristan/dev/vera-agents/spec640`, not in an ephemeral temp directory.

#### 12.1.2 Dispatch protocol

1. Write the full brief to a file (`/home/tristan/dev/vera-agents/spec640/dispatch/<role>-<ticket>-<topic>.md`
   in the current run, or `.agent-link/` when the repository carries it).
2. Submit a short pointer: `herdr agent prompt planner "Read <path> and follow it
   exactly. End with PLAN-READY <path>."`. Add `--wait --timeout <ms>` only when
   the orchestrator must block for the result; otherwise the call returns after
   submission. Do not paste a long brief into a TUI.
3. Use `herdr agent get <role>` for lifecycle state (`idle`, `working`, `blocked`)
   and `herdr agent read <role> --source recent-unwrapped --lines 80` when status
   is needed; do not poll in a tight loop.
4. A `blocked` state is an approval or question prompt. Read it with
   `herdr agent read`, and answer with `herdr agent send-keys <role> <key>`
   (`enter`, `down`, `esc`) after deciding. Approving a sandbox prompt is expected,
   not a failure.
5. Read the artifact from disk. The pane transcript is status, never the
   deliverable. Planner ends with `PLAN-READY <path>`; Judge ends with
   `VERDICT PASS|NOT PASS <path>`.
6. **Completion.** End the pane reply with `PLAN-READY <path>` or
   `VERDICT PASS|NOT PASS <path>` and write the artifact. The orchestrator's
   watcher polls both panes and detects those markers, so **no separate ping is
   needed**. If the orchestrator explicitly asks for one, or the marker cannot be
   shown, the fallback is the non-blocking API call below (never blocking
   `opencode run`):

   ```bash
   $HOME/.local/bin/opencode api POST \
     /api/session/ses_f13c0a26dffel94rbZI4jwKnL8/prompt \
     -d '{"text":"PING <planner|judge> DONE <artifact path>","delivery":"queue"}'
   ```

   The artifact is the deliverable, never a summary or the ping.
7. **Questions.** If a decision is needed mid-task, print a single line
   `QUESTION: <one sentence>` in the pane and stop there. Never guess an
   ambiguous requirement. The orchestrator watches both panes for that marker,
   approval prompts and interrupted prompts, and answers in-pane before work
   continues. Do not use the question marker for the final artifact.

#### 12.1.2.1 The pane watcher

`/home/tristan/dev/vera-agents/spec640/watch-agy-herdr.sh` runs in the background
(`setsid nohup`, log `/home/tristan/dev/vera-agents/spec640/watch-agy-herdr.log`, state
`/home/tristan/dev/vera-agents/spec640/agy-herdr-state`). It polls the two Herdr agents every
four seconds with `herdr agent get` and `herdr agent read`, and enqueues one
short trigger into the orchestrator session with the non-blocking API call when
a marker is new:

| Agent output / state | Wake kind |
| --- | --- |
| `PLAN-READY /…` or `VERDICT PASS\|NOT PASS /…` | `DONE` |
| `QUESTION: …` | `QUESTION` |
| `Run this command?`, `Allow access to this file?`, `Do you trust the contents` | `PERMISSION` |
| `What should Antigravity CLI do instead?` | `INTERRUPTED` |
| `Please re-send the full command` | `TRUNCATED` |
| Herdr reports `blocked` with no recognised line | `BLOCKED` |

The watcher never answers anything. It matches only real markers — a path after
`PLAN-READY`/`VERDICT`, never the placeholder text of a dispatch — remembers
artifact paths so a restart or scroll-back cannot replay old work, hashes the
state and matches so one prompt wakes the orchestrator once, and clears its state
when the marker disappears. The artifact on disk remains the deliverable; the
wake is notification only.

Start or restart it with:

```bash
setsid nohup /home/tristan/dev/vera-agents/spec640/watch-agy-herdr.sh \
  </dev/null >/home/tristan/dev/vera-agents/spec640/watch-agy-herdr.log 2>&1 &
```

If the Herdr tab is recreated, update the agent names/pane IDs in the script and
restart it before dispatching work. Without the watcher, the orchestrator must
poll the agents or rely on the fallback ping.

Planner and Judge are read-only on the repository: they may read anything and write
only their own report file. They never edit source, never commit, and never send
into another pane except for the completion ping above or an explicit
communication check the orchestrator asks for. They never start another agent
session.

#### 12.1.3 Workers

Every worker brief carries five things: the context, the top-level plan, the exact
instructions, the frozen interfaces, and the success criteria. It also carries the
rule that the worker **stops the moment the criteria are checked** — pass or fail —
and reports everything: every command, every failure, every dead end, and anything
surprising it did not fix. A worker that hides a dead end has failed its brief.

A report missing any of its required parts is **not accepted**. Re-brief that worker
with the gap named rather than filling it in from the diff — the diff shows what
changed, never what was tried and abandoned, and the abandoned attempt is often the
most useful thing in the report.

Workers are dispatched with `subagent` and the exact model
`opencode-go/deepseek-v4.1-flash#max`. A dispatch that cannot use that model stops
and reports it; it does not silently fall back.

#### 12.1.4 Planner quality bar and the user's final validation

The Planner receives the problem, revision or feature request plus the relevant
issue, code pointers and frozen contracts. Its plan is held to a bar:

- exact files/interfaces/signatures and the order of changes;
- alternatives considered and why the chosen one wins;
- failure modes, edge values, empty/error/stale states and migration/rollback;
- a test plan with the seams that prove each requirement;
- a performance budget and how it will be measured, not a vague "should be fast";
- risks, assumptions and open questions, each answerable by evidence.

The orchestrator **validates** the plan before showing it to the user: every
`file:line` and API it names must exist, it must respect the domain invariants in
Section 10, and its scope must match the issue. Unsupported claims go back to the
Planner with the evidence named; they are not quietly fixed in the relay.

The orchestrator then explains the technical plan to the user in **simpler terms**.
Write for an intelligent manager who does not write code: lead with what changes
for the people using the app, then what the system will do, what can go wrong and
what will be tested. Replace internal names with a plain description the first
time (“the shared report cache”, “the Reports screen”); keep a file, type or
model name only where the decision genuinely depends on it. Expand acronyms on
first use and avoid chains of internal jargon. Do not talk down or use childish
analogies — the reader is smart, just not close to the code. The relay is still a
translation, not a summary: every requirement, file group, risk, test and
decision appears. **The user is the final validator of every plan**;
implementation waits for explicit approval.

#### 12.1.5 Judge verdicts

The Judge is dispatched whenever a ticket is finished or a PR is about to open. It
assumes everything is wrong, verifies by running, and returns PASS or NOT PASS with
findings, commands and direct exit codes. It never edits, fixes, reruns a failed
gate or merges.

A Judge runs on a quiescent tree: every worker has reported and stopped. On NOT
PASS the findings become the next worker wave, and the **same** Judge conversation
is resumed with the fixes and asked whether each finding is closed. Replacing the
Judge is the exception, not the habit.

### 12.2 The loop

```
Problem / feature / revision
      |
Planner (agy, Gemini 4 Neon High) -> plan artifact
      |
Orchestrator validates evidence -> explains plan in simple terms -> user approves
      |
Plan comment + frozen interfaces
      |
Worker waves (deepseek-v4.1-flash#max, tdd / diagnosing-bugs)
      |
Judge (agy, Gemini 4 Neon High) -> NOT PASS -> next worker wave
      |
    PASS
      |
code-review -> PR -> CI read -> orchestrator validation -> ship
```

- **On PASS** the orchestrator re-validates everything personally and then decides
  whether to iterate again. The verdict is a gate, not a decision.
- **On NOT PASS** the findings become the next worker wave. Iterate until the
  result is genuinely good, not until the reviews stop complaining.
- A verdict resting on a claim that turns out to be false is not a verdict. If the
  orchestrator finds a defect after a PASS, fix it, re-verify, and say so.
- **A finding can be refuted, but only with evidence.** When the orchestrator judges a
  NOT PASS finding unfounded, it does not argue in prose: it reproduces the finding,
  shows the measurement that contradicts it, and records both in the report. A refuted
  finding is written down, not dropped — the next run may see what this one missed.

A **wave** is one dispatch round: the workers sent out together, and the reports that
come back. A wave ends when every worker in it has reported, not when the first one
does.

**One Judge session per issue.** The first pass is briefed in full; every follow-up
commit resumes the same session, hands back its own findings, and asks whether each
is closed. Re-briefing a fresh Judge for an increment is the exception, not the
habit — a resumed Judge knows what it found and why.

**A Judge runs on a quiescent tree — no active workers.** Before dispatching a
Judge — first pass or resume — the orchestrator confirms that **no worker subagent
is still running**: every worker in the wave has reported and stopped, and no new
wave has started. A Judge running beside a live worker judges a moving target: the
diff it approves may not be the one that ships, its red proofs can collide with a
worker's scratch edits, and its verdict goes stale the moment that worker lands
another commit. If a worker is still running, wait for it. If the next wave is
ready, finish and judge the current wave first — the Judge's verdict is a gate
before the frontier moves, not a background opinion.

### 12.3 The todolist, and the tool that does not exist

**OpenCode V2 has no todo tool** — the todolist is the plan comment (Section 11)
and the live list below, both maintained by hand; do not go looking for one.

The todolist is therefore maintained **by hand, in two places, both mandatory**:

1. **The issue's first comment** (Section 11) is the canonical plan, and the
   orchestrator **updates it as work proceeds**. It carries the task breakdown, the
   wave each task belongs to, the files each task owns, and the current status. A
   plan written once and never touched is not a plan.
2. **The orchestrator's own messages** carry the live list at every wave boundary,
   so the state is visible without opening GitHub.

One artifact, one name: throughout this section the plan is **the plan comment** — the
issue's first comment, updated in place. "Todolist" and "the live list" mean the same
thing.

A task is **one file and its test** — or, when a change genuinely needs two files that
cannot be separated, **a closed set of files owned by one worker and no other**. The
rule exists so that no two workers own the same file, not so that a worker may never
touch two.

Slices are deliberately **smaller than atomic** wherever that earns a parallel
dispatch: one concern, one file set, one exit condition. The decomposition duty is
stated in §12.4 — a task that bundles three concerns into one worker is a task the
orchestrator has not finished.

**Scope changes mid-flight are normal, and they are recorded rather than absorbed.**
When something is added or removed while a wave is running:

1. Amend the issue body's Scope section **and** the plan comment. A change that lives
   only in a chat message is lost by the next run.
2. Decide whether the running wave can take it. If it touches a file a worker already
   owns, it waits for the next wave; if it is independent, it joins the current one.
3. Say in the report that the scope changed, and what it did to the plan. A silent
   scope change is how a "pure move" issue grows an unchecked bug fix. This happened
   during #338: a change to the customer document arrived mid-flight and was queued
   behind the running issue instead of being absorbed into it.

### 12.4 Parallelism is the default — the orchestrator decomposes to earn it

**Decomposition is the orchestrator's hardest job.** Work is sliced **smaller than
atomic** — one concern per slice, one file and its test wherever that is possible —
so that **several workers run at once**, each carrying less context and less to get
wrong. A worker holding one file, one test, and one exit condition lands better
results than a worker holding five of each; the smaller brief is not just faster, it
is better. When a wave runs with a single worker, the orchestrator says why in the
report — one worker is the exception a plan justifies, never the default.

Workers run concurrently when both hold:

1. **Each owns a disjoint set of files.** Two workers editing one file collide, and
   a worker that removes a line another depends on breaks it silently until a
   type-check happens to catch it.
2. **The interfaces between those files are frozen**, and stated verbatim in every
   brief — prop names, signatures, exported types. A worker cannot verify against an
   interface that does not exist yet.

When those conditions do not hold yet, the answer is **to decompose further, not to
serialize**: extract the shared file, freeze the interface, split the concern — then
dispatch the pieces together. In #338 that pressure produced `company.ts`, `TabBar`,
`ThemeColorField` and the panels, and those extractions are what let the next two
issues run as parallel waves. Serialize only when decomposition genuinely cannot
earn a second worker, and say so in the report.

While workers run in parallel:

- **Nobody commits.** The orchestrator commits; concurrent `git add` on one working
  tree is a race.
- **Nobody runs anything that writes shared output.** `npm --prefix frontend run
build` writes `frontend/dist`, which `backend/**` embeds.
- **Nobody edits a file another worker owns.** If a task needs a change in someone
  else's file, it waits for the next wave.
- **One tree, or a worktree per branch.** Workers share the working tree and never
  commit; when a task needs its own branch while another worker holds the tree — or
  a long build would collide — give it a worktree (`worktree_create`) instead of
  serializing.

### 12.5 Verification rules — the ones that caught real defects

- **A guard is not a guard until it is proven to fail.** Break it, show it naming
  what it caught, restore it. A test that cannot fail proves nothing.
- **Every bug fix begins with a test that reproduces the bug.** Write the failing
  test first — the one that names the wrong behaviour — then fix until it passes,
  and keep the test as the guard, so the same bug cannot return unnoticed. A bug
  found by hand (a report, a document, a screenshot) becomes permanent protection
  this way, and a fix without its reproducing test is an unverified claim. If a
  reproduction is genuinely impossible, the report says so and why.
- **Every claim of "clean" or "passes" names its command and its exit code.**
- **A piped exit code is not an exit code.** `cmd | head -3; echo $?` reports
  **`head`'s** status, not `cmd`'s. Redirect to a file, then read `$?`. This exact
  mistake produced a false "tsc clean" during #338 while the type-check was failing
  with five errors.
- **A type-check and a test check different things.** A runtime guard compares
  values; only `tsc` compares a declaration against the value it describes. Both
  have to actually run.
- **Markup parity is proven, not eyeballed.** Render a verbatim copy of the old code
  and compare the full output strings; per-control byte-equality in isolation cannot
  see a changed container.
- **State plainly what is not verified.** "No visual regression" is false if the
  page composition changed. If the assembled page was never rendered in a browser,
  the pull request says so.
- **While a change is uncommitted, `git diff` cannot show a correction made inside new
  content.** A file that does not exist on `HEAD` produces no `-`/`+` pair for the
  lines that changed, so "the diff shows no such edit" is not evidence that the edit
  did not happen. Read the file. A reviewer reported a fix as having "left no artifact"
  for exactly this reason during #347.

### 12.6 How the loop scales

| Change                                                      | Minimum                                                                                   |
| ----------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Behaviour, or anything staff see                            | Planner → user approval → Worker waves → Judge → orchestrator validation                 |
| Pricing, tax, persistence, the renderer, the signed payload | The full loop, plus the Postgres integration test (Section 7)                             |
| Comments, docs, formatting only                             | One Worker, the gates (Section 13), and orchestrator validation                           |
| Bug, regression or incident                                 | `diagnosing-bugs` first: a red loop and its permanent regression test, then the normal loop |
| New term, schema or state                                   | `domain-modeling` before the plan; the glossary/ADR is part of the plan                   |
| A question outside the repo                                 | `research` before the plan; write the evidence down                                       |
| A design fork or decomposition                              | `codebase-design`; a timeboxed `prototype` when a runnable answer is needed               |
| AGENTS.md, a skill or an agent brief                        | `writing-for-agents`; one Worker, the gates, and orchestrator validation                  |

The floor is never zero: **a Worker writes the change and the orchestrator validates
it.** Skipping the Judge is a decision the orchestrator states out loud, with its
reason.

**The orchestrator runs the gates — not the workers.** A worker runs only its own
tests. The full set (Section 13) runs after the last wave, on a tree nothing else is
writing to, because the build gates write shared output that a concurrent wave would
corrupt. Every exit code is read directly (Section 12.5).

> **§12 is a norm, not machinery — the same honesty §11 applies to its Dependabot
> reads.** Nothing in CI checks that a Judge ran, that a guard was proven to fail, or
> that a worker stopped when its criteria were met. What enforces it is the
> orchestrator's second validation and the written record in the issue — which is why
> the record is mandatory.

### 12.7 What the orchestrator reports

1. What shipped, with the numbers.
2. What the loop caught that the first pass did not — findings, not process.
3. What is **not** verified.
4. An honest verdict on the workflow itself: what worked, what cost more than it
   saved, and what the next run should do differently.
5. The Dependabot read — before the PR and after the merge (Section 11). Both, not
   one.
6. The CI read for the pushed SHA — every check, or the failure with its
   decisive log line and whether it is code or infrastructure.
7. Which skills were loaded, what each changed, and which were deliberately not
   loaded.

The report goes on the **issue**, because the written record is what §12.6 says does
the enforcing; the reply to the user carries the same thing in short form.

### 12.8 Skills — what to load, and who owns it

Skills are loadable methods, not roles. **The orchestrator holds all of them and may
load any directly at any point**; each subordinate loads the ones its job names. No
skill is reserved to a subagent, and loading `code-review` does not make a session
the Judge — it supplies the review method. Every dispatch and report names the skills
loaded and the ones deliberately skipped, with the reason.

| Phase | Skills | Who |
| --- | --- | --- |
| Discovery and plan | `research`, `grilling`, `domain-modeling`, `codebase-design`, `prototype` | orchestrator, Planner |
| Build | `tdd`, `diagnosing-bugs`, `prototype`, `writing-for-agents` | Worker; the orchestrator may load them too |
| Review and merge | `code-review`, `tdd`, `diagnosing-bugs`, `pr` | orchestrator, Judge |
| Human handoff | `wizard` | orchestrator generates; the user runs it |

| Skill | Load it when | It is done when |
| --- | --- | --- |
| `prototype` | a design fork needs a runnable answer before interfaces freeze | the question is answered and recorded; the prototype is thrown away or reduced to a test/fixture |
| `diagnosing-bugs` | a bug, regression, incident or flaky check | a red-capable command names the symptom, the cause is verified, and a permanent regression test exists — or the missing seam is recorded |
| `research` | the answer lives outside the repo, or a decision needs primary sources | findings are written to `docs/research/<slug>.md` or the issue with citations; no decision waits on memory |
| `tdd` | any code or test change | the test failed first at an agreed seam; the least fix passes; the test remains |
| `domain-modeling` | a new term, schema, state, ownership rule or money concept | `GLOSSARY.md`/ADR is updated, the plan uses the agreed words, and no schema lands before the words settle |
| `codebase-design` | a module, seam, interface or decomposition decision | the interface is frozen in the brief; a single-worker serialization is justified |
| `code-review` | before merging a behaviour change or after a large wave | standards and spec are reviewed; findings are fixed or recorded; the Judge still verifies |
| `pr` | opening or updating a pull request | the title/body carry the summary, verification, `Resolves #NNN` and the Dependabot read |
| `wizard` | a step only a human can do: credentials, dashboard, signing, cutover | a bash wizard walks the user through it and stops; the agent never fakes the step |
| `grilling` | before freezing a high-risk plan | the plan answers the hard questions; unresolved answers block the freeze |
| `writing-for-agents` | editing AGENTS.md, a skill, a brief or agent-facing docs | pointers are sharp, each rule has one home, the document is pruned, and its commands/links are verified |

A skill's instructions do not override this guide; if they conflict, this guide wins
and the conflict is reported. `prototype` and `research` produce evidence, not shipped
code; `code-review` and `pr` do not replace the Judge. The `karpathy-guideline` remains
the Planner and Judge default surgical lens.

---

## 13. Testing Gates

Every commit must pass all gates before push:

```bash
gofmt -l .                        # must print nothing
go vet ./...                      # must pass
go test -race ./...               # must pass
npm --prefix frontend run build   # TypeScript + production bundle
npm --prefix frontend test        # Vitest: the frontend's pure modules
```

`npm run build` type-checks the test files too (tsconfig pulls in
`vitest/globals`), so a test that stops compiling fails the build rather than
lying in the suite.

For a change that alters a React panel's layout, run `scripts/ui-snapshot.sh` and
read both PNGs (360 and 1280); the pull request states what was seen. Interactions —
keyboard, paste, focus — remain hand-driven, and that stays written down. Snapshots
are local evidence: they are never committed.

If a gate fails, fix it before committing. Do not commit partially working code.
For anything touching pricing, tax, or persistence, also run the Postgres integration
test from Section 7.

A bug fix also carries its `diagnosing-bugs` red loop and permanent regression test.
A behaviour change gets a `code-review` before the PR, and the PR body is written with
`pr`. Edits to this file, a skill or an agent brief use `writing-for-agents`. These
are review gates, not substitutes for the Judge.

On CI, PRs whose diff is _entirely_ `frontend/src/**` or `frontend/public/**` skip
the Go suite's heavy steps via the scope step in `pr.yml` (the required "Go
gates" check still reports — it just finishes fast). Every other path keeps the
full suite, deliberately: root `*.md` feeds `TestChangelogCopyMatchesRoot`, and
`backend/**` embeds `frontend/dist`. The classification fails open — a crashed
or confused scope step runs the full suite rather than skipping it.

---

## 14. Release Process

`docs/agents/release.md` is the runbook a release is run from: the pre-tag
checklist with the exact commands, and the rule for the Dependabot pull requests
that are open when it starts. This section is the always-loaded summary.

### When to release

- After a batch of related features.
- After a significant bug fix.
- **After fixing Dependabot alerts** (see Section 11) — always a PATCH.
- When the user requests a release.

### Release naming

Semantic versioning: `vMAJOR.MINOR.PATCH`.

- **MAJOR:** breaking changes
- **MINOR:** new features (backwards compatible)
- **PATCH:** bug fixes

### Release steps

1. Verify all gates pass on `main`.
2. Write the `CHANGELOG.md` entry for the release (Added/Changed/Fixed/
   Security sections, staff language; internal chores omitted), copy it to
   `backend/api/changelog.md` so the embedded copy matches byte-for-byte
   (`TestChangelogCopyMatchesRoot` enforces this), and commit both with the
   version bump. The What's New modal ships whatever is committed here, so
   this step must land before the tag.
3. Bump `wails.json` `info.productVersion` to the release version (no `v`
   prefix — Wails convention) and commit it.
4. Verify the stamp: `VERSION=vX.Y.Z scripts/stamp-version.sh` must print OK
   (it fails loudly if Go ldflags and installer metadata disagree).
5. Push `main`, then create the release — **the release must exist before the
   workflow runs**, because every artifact job uploads into it: a missing
   release fails all of them (v0.10.1 lost a full cycle to `release not
found`). `gh release create` creates the tag as well, so there is no
   separate tag step and no window in which a job can run without a release.
   Pin `--target` explicitly: `gh release create` tags the _remote_ default
   branch head when the tag does not exist, and if the version-bump commit is
   still local the tag lands on the commit without the bump, the desktop
   build fails its own stamp check, and the tag has to be force-moved by hand
   (this produced the first v0.4.1 failure). Push first, then:
   ```bash
   git push origin main
   gh release create v0.2.0 --target "$(git rev-parse HEAD)" \
     --title "v0.2.0 — Title" --notes "changelog"
   ```
   The tag this creates is lightweight, and it exists before `desktop.yml`
   starts — that is the point of this order.
6. The tag the release created triggers `.github/workflows/desktop.yml`, which builds
   **nine** artifacts, each with a `.sha256` sidecar:

   | Artifact                                           | Job             |
   | -------------------------------------------------- | --------------- |
   | `Vera-v<version>-windows-amd64-installer.exe`      | `windows`       |
   | `Vera-v<version>-windows-arm64-installer.exe`      | `windows-arm64` |
   | `Vera-v<version>-macos-universal.dmg`              | `macos`         |
   | `Vera-CLI-v<version>-{linux,darwin}-{amd64,arm64}` | `cli`           |
   | `Vera-CLI-v<version>-windows-{amd64,arm64}.exe`    | `cli`           |

   All are version-stamped. The `v` appears **inside** the filename as well as
   in the tag: tag `v0.5.1` → `Vera-v0.5.1-…`. The three installer names are a
   contract with the updater — never rename by hand — and **every** name must
   be copied out of `gh release view <tag> --json assets` rather than typed, or
   the Download section will link a 404 (this happened at v0.5.1). Staff
   download links must point at the public `vera-downloads-508411` bucket
   mirror, never at GitHub release assets: the announce job proves each mirrored
   URL serves before anything is announced, and the mirror does not depend on
   the repository's visibility — a GitHub asset URL offers neither guarantee.
   (The repo has been public since 2026-10-12; the old private-repo reason is
   gone, the rule survives on these two properties.)

   The `cli` job fails the release if a binary does not carry the tag, so a
   mis-stamped build cannot be published.

7. **The `Announce to the updater` job does the mirroring and the updater
   variables for you.** It runs last, gated on all three installer jobs, and is
   the only thing that makes an installed desktop offer the new version. A
   failure in any of the three installer jobs means it never runs, and the
   updater is never told. Watch for it to go green — it is not cosmetic:

   - mirrors every artifact in `scripts/announce-release.sh`'s `ASSETS` array —
     the three installers and the six CLI binaries — to
     `gs://vera-downloads-508411/`, after verifying each against its `.sha256`
     sidecar. The array is the contract: a binary missing from it is a binary
     staff cannot download, because staff download from the bucket mirror,
     never from GitHub release assets;
   - proves each public URL serves before anything is announced, so the app is
     never told to offer a 404;
   - writes `latest.json` — the same release state `/v1/app/latest` serves —
     to the same bucket, because the app's update check falls back to it when
     the API is unreachable (the 2026-10-15 rollout window is why);
   - sets the six `VERA_APP_*` variables on `vera-api`, carrying
     `VERA_APP_MIN_SUPPORTED` forward from the live revision rather than
     hard-coding a floor;
   - fails if `GET /v1/app/latest` still reports the old version.

   It **fails** when the GCP WIF secrets are absent rather than skipping, because
   skipping is exactly the bug it exists to prevent. It **skips** for pre-release
   tags (`v0.5.0-rc1` and friends): those get installers built and attached but
   are not pushed to every installed copy.

   Re-run it by hand after fixing whatever it complained about:

   ```bash
   gh run rerun --repo arcon-external/vera-app --failed    # or re-run the job
   ```

   `gh run rerun --failed` recovers a _skipped_ announce job once its `needs`
   succeed (verified at v0.10.1), so it does not need a full re-run.

   To do it outside CI, the script is the whole sequence and takes `--dry-run`,
   which runs every check and changes nothing:

   ```bash
   scripts/announce-release.sh --version v0.4.0 --dry-run
   scripts/announce-release.sh --version v0.4.0
   ```

8. **Confirm the app can see the release** — the check that was missing when
   v0.4.0 shipped invisible:
   ```bash
   curl -s https://api.arconpentapersada.co.id/v1/app/latest | grep -o '"latest": "[^"]*"'
   ```
   It must print the tag the release created. If it prints anything older, the
   announce job failed; read its log before doing anything else.
9. Add a Download section to the release notes listing the installers **and the
   six CLI binaries**, with one install line each, and these prerequisites:
   - Windows: SmartScreen will warn on unsigned builds — expected until signing.
     A Chrome installation is no longer required for exporting quotations; the
     native renderer replaced it in v0.4.0.
   - macOS: drag Vera to Applications; on first launch of an unsigned build,
     right-click → Open instead of double-clicking.

---

## 15. Architecture Principles

- **Single pricing pipeline:** `Prepare` validates, computes per-item prices, then
  derives original/marginalized/tax/nett totals. Both preview and PDF read from the
  same computed `QuotationData`.
- **Two PDF engines, one document, selected by a build tag.** The HTML/CSS
  template in `backend/template/` is the source of truth for how a quotation
  looks; both engines render it.

  | build                     | `render.Active`            | chromedp dependency |
  | ------------------------- | -------------------------- | ------------------- |
  | `go build`                | gpdf (native, no browser)  | **absent**          |
  | `go build -tags chromedp` | chromedp (headless Chrome) | present             |

  Reach for chromedp when the template has changed and the native layout has not
  caught up — it renders the template exactly as written, and it is the oracle
  every claim about gpdf's fidelity was measured against. `gpdf` is compiled in
  both modes on purpose: `backend/render_equivalence_test.go` needs both engines
  present to compare them, and that comparison is what says whether a template
  change and the native layout still agree.

  Selecting it for a **release** needs no commit:

  ```bash
  gh variable set VERA_RENDER_ENGINE --body chromedp   # then re-run the tag build
  gh variable delete VERA_RENDER_ENGINE                # back to native
  ```

  Locally: `go build -tags chromedp ./cmd/vera`, or `VERA_RENDER_ENGINE=chromedp`
  before any `scripts/build-*.sh`. `vera version` reports which engine a binary
  carries, because the same version number can be built either way.

  A rendering test under `-tags chromedp` skips without a browser — see
  `render.RequireUsable`. Do not add an environment variable that selects the
  engine at runtime: a compile-time choice is the point.

- **Split presentation:** `GetCustomerPreview` is separated from
  `GetInternalBreakdown` (never exported). The customer view carries the
  document plus the cost side the review panel renders — but **never margin,
  PPH23 or profit**: `TestCustomerPreviewCarriesNoInternalEconomics` fails if
  any of those names reappear on `CustomerPreview` or `CustomerPreviewItem`.
  They were added once as collateral in #48 and nothing read them for a month,
  so the rule is enforced by a test rather than by a comment. PPN is the
  deliberate exception: it is charged on top and printed, so it is customer-
  facing while PPH23 is not.
- **Normalized persistence:** customers, quotations, immutable revisions, revision
  items, revision totals, and rendered documents. Referential integrity via foreign
  keys; money as `BIGINT` minor units of the company currency; rates as basis points.
- **Config through environment only:** no secrets in source, logs, or errors; DSNs
  are redacted before logging.
- **Fail clearly:** startup errors name the missing configuration/operation, never
  print credentials.

## 16. Environment & Secrets

| Variable                                                                                                                                                          | Purpose                                                               |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `VERA_API_URL`                                                                                                                                                    | Desktop `wails dev` only: loopback http(s) API URL (env or project `.env`); packaged builds ignore it. The `vera` CLI keeps its own `--api`/`VERA_API_URL`/session behavior (`docs/cli.md`) |
| `VERA_API_ADDR`                                                                                                                                                   | API server listen address (default `:8080`; `scripts/dev-api.sh` forces `127.0.0.1:8081`) |
| `VERA_POSTGRES_DSN`                                                                                                                                               | API server: primary Postgres connection string (never read by the desktop) |
| `VERA_POSTGRES_SCHEMA`                                                                                                                                            | API server: schema name (default `vera`)                              |
| `PGHOST` / `PGPORT` / `PGDATABASE` / `PGUSER` / `PGPASSWORD` / `PGSSLMODE`                                                                                        | API server fallback                                                   |
| `VERA_POSTGRES_TEST_DSN`                                                                                                                                          | Test-only DSN for `backend/store` integration tests                   |
| `VERA_APP_LATEST` / `VERA_APP_MIN_SUPPORTED` / `VERA_APP_NOTES_URL` / `VERA_APP_DOWNLOAD_WINDOWS` / `VERA_APP_DOWNLOAD_WINDOWS_ARM64` / `VERA_APP_DOWNLOAD_MACOS` | Desktop updater release state (empty = no update info, never prompts) |

The repository is public (since 2026-10-12), which changes the threat model:
no workflow uses `pull_request_target` or `workflow_run`, and `pr.yml` — the
only `pull_request`-triggered workflow — references no secrets, so a fork PR
gets a read-only token and nothing else. Adding either trigger to a public
repository hands secrets to fork PRs; treat that as its own reviewed decision,
never an incidental edit.

`.env` is gitignored. Never commit a real password, paste one into an issue, or print
one in logs.

## 17. Known Gotchas

- `test-data/` is gitignored: fixture changes are on-disk only and never land via PR.
- `frontend/wailsjs/*` is generated; regenerate rather than hand-editing behavior.
- `backend/store` tests skip without `VERA_POSTGRES_TEST_DSN`; a green `go test ./...`
  does not mean the Postgres path ran.
- `golang.org/x/crypto`, `pgx/v5`, and `echo/v4` are the dependency areas most often
  flagged by Dependabot; `echo` is transitive via Wails and may need a direct override.
- `VERA_APP_DOWNLOAD_*` must point at the public `vera-downloads-508411`
  bucket, never at release assets directly: the mirror is verified before
  anything is announced and survives any change of repository visibility.
  (The repo has been public since 2026-10-12, so the old private-repo reason
  no longer applies.) Mirror new installers there at release time; the backups
  bucket stays private.
- Cloud Run recon (describe/revisions/logs, read-only) runs via the `gcp-recon`
  workflow dispatch — no local gcloud needed; it authenticates over WIF.

## 18. Cursor / Copilot Rules Status

Checked paths:

- `.cursorrules`
- `.cursor/rules/`
- `.github/copilot-instructions.md`

Status: none present. If added later, read and follow them before making changes.

---

## Agent skills

### Issue tracker

Issues live as GitHub issues in `arcon-external/vera-app`, via the `gh` CLI.
See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles map to `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human`, `wontfix`.
See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root (created lazily
by `/domain-modeling`). See `docs/agents/domain.md`.
