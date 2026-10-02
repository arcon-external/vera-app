# AGENTS.md

Operational guide for coding agents working in `vera-app`. Read this before making
changes.

---

## 1. Project Snapshot

- **App:** Wails v2 desktop quotation generator for PT Arcon Penta Persada.
- **Backend:** Go in `backend/` — quotation pricing, HTML templating, PDF rendering
  via `chromedp` (headless Chrome `PrintToPDF`).
- **Persistence:** PostgreSQL via `backend/store` (pgx v5), configured entirely by
  environment variables.
- **Frontend:** React 18 + TypeScript + Vite in `frontend/`, styled with Tailwind CSS
  (v3.1).
- **Go module:** `vera-app` (Go 1.26).
- **Wails bindings:** `frontend/wailsjs/go/*` are generated. Treat them as build
  output; keep them in sync when Go signatures change.

## 2. Repository Map

| Path | Purpose |
| --- | --- |
| `main.go` | Wails startup, Postgres open, binding registration |
| `backend/quotation.go` | `Prepare`/`Export` flow, input + data models |
| `backend/builder.go` | Per-item pricing, totals, markup math |
| `backend/tax.go` | Tax zone/category → basis-point rates, tax + nett |
| `backend/preview.go` | Customer/internal preview DTOs, template HTML preview |
| `backend/archive.go` | Archive list/detail/revision Wails methods |
| `backend/format.go` | Rupiah + description formatting |
| `backend/template.go` | Embedded template/asset loading |
| `backend/store/` | Postgres config, migrations, repositories |
| `backend/template/html/template-fixed.html` | Customer-facing PDF/preview template |
| `backend/template/css/template-fixed.css` | PDF/preview styles |
| `frontend/src/App.tsx` | Application shell/state, page routing |
| `frontend/src/components/` | UI components (shell, ledger, review, archive, detail) |
| `frontend/src/tax.ts` | Tax options, rates, paste normalizers |
| `frontend/src/format.ts` | IDR formatting + client-side margin math |
| `frontend/wailsjs/go/*` | Generated Wails bindings |
| `.env.example` | Documented Postgres variable names (no secrets) |
| `test-data/` | Local Excel fixtures (gitignored) |

## 3. Setup Commands

Run from the repository root unless noted.

```bash
go mod download
npm --prefix frontend install
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Postgres must be configured for the app to start. Create a local, gitignored `.env`:

```bash
VERA_POSTGRES_DSN=postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=require
VERA_POSTGRES_SCHEMA=vera
```

Never commit credentials or a real DSN. `.env.example` documents the variable names.
Startup runs versioned migrations in the configured schema.

## 4. Development Commands

```bash
# Load Postgres configuration into the shell, then run the desktop loop
set -a; source .env; set +a
wails dev

# Frontend-only loop
npm --prefix frontend run dev
```

## 5. Build Commands

```bash
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
```

## 7. Test Commands

Current tests are Go tests (backend packages). There is no frontend test runner.

```bash
go test ./...                                # all packages
go test ./backend                            # quotation/pricing/tax/archive units
go test -race ./...                          # race detector
go test ./backend -run '^TestBuildPricing' -v
```

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
- Use `uint64`/`int64` IDR amounts; never float for money. Multiply before divide.
  Signed results (profit/loss) use `int64`.

### React / TypeScript

- Use ES modules; keep imports minimal and consistently ordered.
- Use configured alias imports (`@backend/*`) for Wails-generated bindings.
- Keep TypeScript strict-mode compatible (`frontend/tsconfig.json`).
- Explicit types for component props and public utility APIs.
- Components/files `PascalCase`; functions/variables `camelCase`.
- All IDR amounts render with `tabular-nums` and `whitespace-nowrap`.

### Styling

- Tailwind is the primary styling path; use the tokens in
  `frontend/tailwind.config.cjs` (colors, type scale, shadows, easing).
- Keep `frontend/src/index.css` minimal and global-only.
- **Tailwind is v3.1: there is no numeric `min-h-*` / `line-clamp-*` utility.**
  Use the `minHeight` scale defined in `tailwind.config.cjs`, `py` for height, or
  arbitrary values (`min-h-[42px]`). Verify any new utility actually appears in
  `frontend/dist/assets/*.css`.
- Do not change the light theme or introduce competing styling systems.

## 9. Wails Integration Rules

- Frontend-callable methods must be exported on bound structs.
- If method signatures change, regenerate and verify `frontend/wailsjs/go/*`.
- Keep `Startup` initialization safe for both runtime and tests.
- `main.go` opens Postgres before `wails.Run`; startup must fail clearly when the
  database is misconfigured.

## 10. Domain Invariants (do not break)

- **Per-item margin:** each line has an independent `MarginEnabled` + `MarginPercent`.
  `MarginalizedUnit = UnitCost + UnitCost * margin% / 100`, then multiply by quantity.
- **Tax is internal only:** calculated once on the marginalized grand total
  (`Tax = Grand * bps / 10000`), never allocated per line.
- **Nett profit** = marginalized grand after tax − original grand total (signed).
- **The customer PDF/template shows only marginalized prices before tax.** Margin
  percentage, tax, and nett profit must never appear in the customer output. There is
  a test asserting this (`TestRenderedQuotationHidesInternalValues`) — keep it green.
- **Revisions are immutable:** first export is revision `00`; each edit creates the
  next revision. Never mutate a stored revision.
- **Discount** applies to the sell side; cost-side discount is shown for internal
  transparency only.
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
- **Verification:** every revision is signed at export (Ed25519, key from
  `VERA_SIGNING_KEY`, never in source or the database) over a canonical field
  set pinned by `backend/signing.go`'s format test. The public portal at
  `/v/<shortId>` (and `POST /verify`) renders valid / superseded / mismatch /
  not-found. `short_id` identifies one *revision*. Never change the canonical
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

> **No auto-deploy.** `vera-app` is a desktop application: merging to `main` does not
> deploy anything. Build a release locally when needed (Section 13).

> **Cross-repo transfer is not available.** GitHub only allows issue transfer within
> the same owner; `arcon-external/vera-app` and a personal-owner repo cannot exchange
> issues. File in the right repo the first time.

### Closing issues

Close the issue only after:

1. The PR is squash-merged to `main` (the `Resolves #NNN` body auto-closes it).
2. All gates pass (Section 12).
3. The issue's success criteria are met.

---

## 12. Testing Gates

Every commit must pass all gates before push:

```bash
gofmt -l .                        # must print nothing
go vet ./...                      # must pass
go test -race ./...               # must pass
npm --prefix frontend run build   # TypeScript + production bundle
```

If a gate fails, fix it before committing. Do not commit partially working code.
For anything touching pricing, tax, or persistence, also run the Postgres integration
test from Section 7.

---

## 13. Release Process

### When to release

- After a batch of related features.
- After a significant bug fix.
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
5. Create the GitHub Release **before** pushing the tag. The tag triggers
   `.github/workflows/desktop.yml`, and that workflow uploads its artefacts with
   `gh release upload` — which fails with `release not found` if the release
   does not exist yet. Create it first, then the tag, then the build succeeds
   on its first run instead of needing a re-run:
   ```bash
   gh release create v0.2.0 --title "v0.2.0 — Title" --notes "changelog"
   ```
6. Create and push an annotated tag:
   ```bash
   git tag -a v0.2.0 -m "v0.2.0: description"
   git push origin v0.2.0
   ```
7. Pushing the tag triggers `.github/workflows/desktop.yml`: Windows
   `-nsis` installers (amd64 + arm64) and macOS DMG, all version-stamped
   with sha256 sidecars, attached as
   `Vera-<version>-windows-amd64-installer.exe`,
   `Vera-<version>-windows-arm64-installer.exe`, and
   `Vera-<version>-macos-universal.dmg`. Those names are a contract
   with the updater — never rename by hand. Staff download links must
   point at the public `vera-downloads-508411` bucket mirror, never at
   GitHub release assets (the private repo 404s anyone without access).
8. **The `Announce to the updater` job does the mirroring and the updater
   variables for you.** It runs last, gated on all three installer jobs, and is
   the only thing that makes an installed desktop offer the new version. Watch
   for it to go green — it is not cosmetic:

   - mirrors the three installers to `gs://vera-downloads-508411/`, after
     verifying each release asset against its `.sha256` sidecar. Staff cannot
     reach GitHub release assets (the repo is private), so a release that is not
     mirrored is a release nobody can download;
   - proves each public URL serves before anything is announced, so the app is
     never told to offer a 404;
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

   To do it outside CI, the script is the whole sequence and takes `--dry-run`,
   which runs every check and changes nothing:
   ```bash
   scripts/announce-release.sh --version v0.4.0 --dry-run
   scripts/announce-release.sh --version v0.4.0
   ```

9. **Confirm the app can see the release** — the check that was missing when
   v0.4.0 shipped invisible:
   ```bash
   curl -s https://api.arconpentapersada.co.id/v1/app/latest | grep -o '"latest": "[^"]*"'
   ```
   It must print the tag you just pushed. If it prints anything older, the
   announce job failed; read its log before doing anything else.
10. Add a Download section to the release notes with all three filenames plus one
    install line each, and these prerequisites:
   - Windows: SmartScreen will warn on unsigned builds — expected until signing.
     A Chrome installation is no longer required for exporting quotations; the
     native renderer replaced it in v0.4.0.
   - macOS: drag Vera to Applications; on first launch of an unsigned build,
     right-click → Open instead of double-clicking.

---

## 14. Architecture Principles

- **Single pricing pipeline:** `Prepare` validates, computes per-item prices, then
  derives original/marginalized/tax/nett totals. Both preview and PDF read from the
  same computed `QuotationData`.
- **Split presentation:** `GetCustomerPreview` (safe to show) is separated from
  `GetInternalBreakdown` (never exported). Keep the boundary explicit.
- **Normalized persistence:** customers, quotations, immutable revisions, revision
  items, revision totals, and rendered documents. Referential integrity via foreign
  keys; money as `BIGINT`; rates as basis points.
- **Config through environment only:** no secrets in source, logs, or errors; DSNs
  are redacted before logging.
- **Fail clearly:** startup errors name the missing configuration/operation, never
  print credentials.

## 15. Environment & Secrets

| Variable | Purpose |
| --- | --- |
| `VERA_POSTGRES_DSN` | Primary Postgres connection string |
| `VERA_POSTGRES_SCHEMA` | Schema name (default `vera`) |
| `PGHOST` / `PGPORT` / `PGDATABASE` / `PGUSER` / `PGPASSWORD` / `PGSSLMODE` | Fallback |
| `VERA_POSTGRES_TEST_DSN` | Test-only DSN for `backend/store` integration tests |
| `VERA_APP_LATEST` / `VERA_APP_MIN_SUPPORTED` / `VERA_APP_NOTES_URL` / `VERA_APP_DOWNLOAD_WINDOWS` / `VERA_APP_DOWNLOAD_WINDOWS_ARM64` / `VERA_APP_DOWNLOAD_MACOS` | Desktop updater release state (empty = no update info, never prompts) |

`.env` is gitignored. Never commit a real password, paste one into an issue, or print
one in logs.

## 16. Known Gotchas

- **Tailwind 3.1** lacks numeric `min-h-*` and `line-clamp-*`; see Section 8.
- `test-data/` is gitignored: fixture changes are on-disk only and never land via PR.
- `frontend/wailsjs/*` is generated; regenerate rather than hand-editing behavior.
- `backend/store` tests skip without `VERA_POSTGRES_TEST_DSN`; a green `go test ./...`
  does not mean the Postgres path ran.
- `golang.org/x/crypto`, `pgx/v5`, and `echo/v4` are the dependency areas most often
  flagged by Dependabot; `echo` is transitive via Wails and may need a direct override.
- The repo is private, so `github.com/.../releases/download/...` links 404 for
  anyone without GitHub access (i.e. all staff). `VERA_APP_DOWNLOAD_*` must
  point at the public `vera-downloads-508411` bucket, never at release assets
  directly. Mirror new installers there at release time; the backups bucket
  stays private.

## 17. Cursor / Copilot Rules Status

Checked paths:

- `.cursorrules`
- `.cursor/rules/`
- `.github/copilot-instructions.md`

Status: none present. If added later, read and follow them before making changes.
