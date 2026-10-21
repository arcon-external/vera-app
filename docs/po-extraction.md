# PO PDF extraction — text layer only (#477)

Status: **shipped as text-layer assistance, not validated accuracy.** This
ticket implements the text-first half of the pipeline proposed in
[`po-extraction-benchmark.md`](po-extraction-benchmark.md). Scanned and mixed
PDFs (OCR) are #478. The accuracy thresholds in that document (§2.5) have **not
been measured** — no authorised customer PDFs were available — so nothing here
claims that extraction is reliable. It claims that the plumbing, the evidence
contract, the stale-source rule and the explicit-apply rule are in place.

## What ships

- `cmd/vextract`: a restricted binary that reads a PDF on **stdin**, runs
  Poppler's `pdftotext -bbox-layout`, and writes the extraction contract to
  **stdout**.
- `docker/po-extract/Dockerfile`: the pinned image the server runs.
- `backend/extract`: the bbox parser, the conservative field/table proposals,
  the command engine, and the run service.
- `backend/store`: migration `038_purchase_order_extractions` and the run
  lifecycle.
- `backend/api` + `backend/po_extraction.go`: the authenticated API and desktop
  bindings.
- `frontend/src/poExtraction.ts` and the review panel: explicit, per-field and
  per-row apply into the draft.

Nothing here OCRs, calls an external AI/OCR service, writes a purchase-order
field automatically, or finalizes a PO.

## Pipeline

```
browser/desktop
  │  POST /v1/purchase-orders/:id/extractions
  ▼
API server ── store: queued run bound to (document id, version, sha256)
  │
  │  worker claims queued -> running (one claimant)
  ▼
GET /v1/purchase-orders/:id/document/content  (stored bytes, unchanged)
  │
  ▼
vextract (container: --network none, read-only, 512 MB, 1 CPU, 64 PIDs)
  │  stdin: PDF bytes
  │  subprocess: pdftotext -bbox-layout -enc UTF-8 -l <max+1> - -
  │  stdout: extraction contract JSON
  ▼
API server ── validate contract ── store: succeeded + JSONB result
  │                                or failed with a typed code
  ▼
GET /v1/purchase-orders/:id/extractions/latest
  │
  ▼
review panel ── staff tick fields/rows ── Apply selected ── editor reducer
```

The extractor has no database DSN, no signing key, no session token and no
network. The API server never executes a caller-supplied path or command; the
command comes from `VERA_EXTRACTOR_CMD` (operator configuration).

## The evidence contract

The same schema is defined once in `backend/apicontract/extraction.go` and used
by the extractor, the stored result, the API and the desktop binding.

```jsonc
{
  "schemaVersion": 1,
  "engine": "poppler",
  "engineVersion": "25.03.0",
  "pageCount": 1,
  "pagesWithText": 1,
  "pagesWithoutText": 0,
  "durationMs": 16,
  "pages": [{ "number": 1, "width": 595, "height": 842 }],
  "fields": [
    {
      "key": "poNumber",
      "value": "4500123456",
      "raw": "4500123456",
      "status": "extracted",       // extracted | needs_review | absent
      "page": 1,
      "bbox": { "x": 85.57, "y": 82.82, "width": 55.6, "height": 9.25 },
      "confidence": null           // Poppler does not supply one
    }
    // ... every header key is present, in display order
  ],
  "rows": [
    {
      "lineNumber": { "value": "1", "status": "extracted", "page": 1, "bbox": { ... } },
      "description": { "value": "Electrical panel", "status": "extracted", ... },
      "quantity": { "value": "2", "status": "extracted", ... },
      "uom": { "value": "Pc", "status": "extracted", ... },
      "unitPrice": { "value": "1250.00", "status": "extracted", ... },
      "amount": { "value": "2500.00", "status": "extracted", ... },
      "status": "extracted",       // extracted | needs_review
      "page": 1,
      "bbox": { ... }              // the whole row's region
    }
  ],
  "abstentions": { "fields": 2, "rows": 0 }
}
```

- **Value** is normalised (an ISO date, a plain decimal with `.`, an uppercase
  currency code). **Raw** is what was printed.
- **Page/bbox** are in PDF points with the origin at the page's top-left, as
  `pdftotext` prints them. `pages` carries the page size so a UI can draw a
  relative region.
- **confidence** is always `null` for Poppler. The field exists so an engine
  that reports confidence can populate it without a schema change.
- **Absent** is a deliberate answer: the label or value was not found (or the
  page had no selectable text). It is never an empty string pretending to be a
  value.
- **needs_review** covers a value that cannot be normalised, an ambiguous date,
  a repeated label with a different value, a row missing a required cell, and a
  duplicate row detection. The note says why.
- **No inference:** an amount is never computed as quantity × price, a missing
  quantity is never filled, and a table without a readable header produces no
  rows.

### Staleness is part of the envelope

A run stores the document id, version and SHA-256 it read. The store checks
that generation at finish and on every latest read:

- PDF replaced or removed → status `stale`, `result` discarded, `errorCode =
  document_changed`.
- Server restarted → status `failed`, `errorCode = interrupted` at startup.
- The API response also includes the active document generation, so the client
  can detect a mismatch even if it missed the replacement event.

Manual editor values are never touched by either transition.

## Job lifecycle and storage

Migration `038` adds `purchase_order_extractions`. Rows are org-scoped and
cascade from the purchase order/document they describe.

| Status | Meaning | `result` |
| --- | --- | --- |
| `queued` | Accepted, worker not started | null |
| `running` | Worker claimed the run | null |
| `succeeded` | Contract stored | JSON |
| `failed` | Typed reason in `error_code`/`error_message` | null |
| `stale` | The active PDF generation no longer matches | null |

- A partial unique index permits **one queued/running run per PO**.
- Starting again while a run is active for the same document returns the same
  run (idempotent). Starting after a replacement marks the old run stale first.
- History rows are kept; the API exposes only the latest.
- The database checks make an inconsistent row unrepresentable: succeeded ⇔ a
  result exists, failed/stale requires an error code, terminal rows require a
  finish time and duration.

Startup recovery runs in `cmd/veraserver` before serving. A queued/running row
cannot survive a process restart, so it is failed with `interrupted`; a late
worker's finish is refused because the row is no longer active.

## API

Both routes require a session and resolve the caller's organization. The store
joins `purchase_orders` on the organization id, so an id from another tenant is
indistinguishable from a missing one.

| Method | Path | Answer |
| --- | --- | --- |
| `POST` | `/v1/purchase-orders/:id/extractions` | `202 { run, document }` |
| `GET` | `/v1/purchase-orders/:id/extractions/latest` | `200 { run, document }`; `run` is null before the first extraction |

Start failures:

| Condition | Status / code |
| --- | --- |
| No attached PDF | `409 po_extraction_document_required` |
| Unknown purchase order | `404` |
| No isolated extractor configured (or its command is unavailable) | `503 po_extraction_unavailable` |

The start endpoint **preflights** the configured command before it creates a
job. An unavailable extractor is a deployment answer, not a queued run: nothing
is stored, so history never claims extraction was attempted when it never ran.
A command that disappears *after* the preflight is still a typed run failure.

Run failure codes stored in `errorCode` (the manual path always remains):

`page_limit`, `timeout`, `invalid_output`, `output_too_large`,
`extractor_unavailable`, `document_too_large`, `document_missing`,
`document_changed`, `interrupted`, `cancelled`, `engine_failed`.

The desktop binding (`StartPurchaseOrderExtraction`,
`GetPurchaseOrderExtraction`) decodes the result into typed DTOs; the raw JSON
never reaches the React code.

## Bounds

| Bound | Value | Where enforced |
| --- | --- | --- |
| PDF input | 20 MiB | upload validation, store schema, engine |
| Pages | 40 | `vextract -max-pages` (or `VERA_EXTRACT_MAX_PAGES`), parent validation |
| Wall time | 45 s | service context; engine context; process group killed |
| Extractor stdout | 8 MiB | engine limited buffer |
| Poppler XML | 32 MiB | `vextract` capped buffer |
| Container memory | 512 MiB | wrapper/container flags |
| PIDs / CPU | 64 / 1 | wrapper/container flags |
| Network | none | `--network none` |
| Root filesystem | read-only, 64 MiB `/tmp` tmpfs | container flags |

Over a bound is a **failed run** with a typed reason, never a partial result.

## Security and operations

- **Production isolation is the dedicated container, not a child process.** A
  same-UID child in the server container is not isolated: it can read
  `/proc/<server-pid>/environ` and recover the database DSN or the signing key,
  so clearing its environment proves nothing. Extraction therefore runs only
  through the dedicated restricted image (`docker/po-extract`) — or an
  equivalent isolated worker/service that holds no database or signing
  credentials. `Dockerfile.veraserver` ships neither `vextract` nor Poppler, and
  without `VERA_EXTRACTOR_CMD` extraction is disabled (`503`), not silently
  degraded.
- **Restricted worker.** PDF bytes in, result JSON out. No DSN, no signing key,
  no session token, no caller-supplied path. The child gets a minimal
  environment (`PATH`, `HOME=/tmp`, `TMPDIR=/tmp`, `LANG`, the page cap, plus
  explicit operator/test extras), never `os.Environ()`. The container adds
  `--network none`, a read-only root, `--cap-drop ALL`,
  `--security-opt no-new-privileges`, and memory/PID/CPU limits.
- **Build context.** `.dockerignore` excludes `.env`, `test-data/*.pdf`,
  `frontend/node_modules`, `frontend/dist` and repository history, so local
  credentials and customer PDFs cannot enter an image layer.
- **Original preserved.** The stored PDF is never rewritten; extraction reads
  the stored bytes.
- **No external service.** No document bytes or derived text leave the host.
- **Hostile input.** The store validator rejects malformed/encrypted PDFs on
  upload; Poppler failures and timeouts become clean typed failures.
- **Google hosting remains a subprocessor.** Server-side processing runs on
  Google infrastructure; this is not on-device processing.

### Image pinning, licensing and the reviewed configuration

`docker/po-extract/Dockerfile` is the **reviewed configuration**. It pins both
bases by digest and the engine package by exact version:

- builder `golang:1.26-bookworm@sha256:d9c68c2c…` (matches the Go toolchain);
- runtime `debian:trixie-slim@sha256:62bd423a…`;
- `poppler-utils=25.03.0-5+deb13u4` from that image's Debian suite (Poppler
  reports `pdftotext version 25.03.0`).

An exact package pin is intentional: if the Debian suite rotates the package
away, the image build fails loudly instead of silently shipping a different
Poppler. Moving the pin is a reviewed change, not a rebuild side effect.

The Dockerfile records the installed version in `/etc/vextract-engine-version`;
`vextract -version` prints it, and every result carries the same
`engineVersion`, so a repeat run is comparable.

**License review basis (user-approved, 2026-10-21).** Poppler is GPL. Vera uses
it as a separate server-side subprocess inside this dedicated image and does not
link it into the desktop application or the server binary, so the desktop
distribution is not conveying Poppler. The corresponding source for the pinned
Debian package is obtainable from the Debian archive
(`apt-get source poppler=25.03.0-5+deb13u4`) and from the matching Poppler
25.03.0 release; any distribution of a rebuilt image carries the usual
source-offer duty for that GPL program, which the release notes must satisfy.
The server image (`Dockerfile.veraserver`) ships no Poppler and no extractor.

Build and check:

```bash
scripts/build-po-extract-image.sh          # builds vera-po-extract:dev, prints version
```

In development the server reaches the image through
`scripts/vextract-docker.sh` on a Docker-capable host. That wrapper is a
convenience around the same isolation flags, not a production default. The
result's `engineVersion` makes the engine that produced it attributable.

### Environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `VERA_EXTRACTOR_CMD` | *(none)* | The isolated extractor command. The server splits on whitespace; no shell is invoked. Empty disables extraction and the start endpoint answers `503`; there is no in-process fallback. |
| `VERA_EXTRACT_MAX_PAGES` | `40` | Page cap read by `vextract`; the server also validates the result against its own cap. |

For local development, point the server at the container wrapper:

```bash
VERA_EXTRACTOR_CMD="$PWD/scripts/vextract-docker.sh" \
  go run ./cmd/veraserver
```

The wrapper adds `--network none`, `--read-only`, the `/tmp` tmpfs, memory/PID/
CPU limits and `--cap-drop ALL`. It forwards `VERA_EXTRACT_MAX_PAGES`.

## UI behaviour

- **Extract and compare** is enabled whenever the draft/correction editor has
  an attached PDF. A saved view's panel explains that a correction is needed,
  because a saved record's values are protected.
- Progress is honest: “Queued for extraction…” then “Reading the PDF text…”;
  no percentage is invented.
- The review shows every header field and row with its status, printed raw
  value, source page and a small relative region map.
- Default selection is conservative: only an `extracted` header proposal into
  an **empty** editor field starts ticked. A field staff already filled is not
  ticked, whether equal or different; `needs_review` and `absent` wait for a
  tick. Every row starts unticked.
- **Apply selected** is the only path into the editor. Currency is evidence
  only — the PO's currency comes from the Quotation and is protected.
- Choosing any row **replaces the current line set** with the ticked extracted
  rows. The panel states this before the action; mixing rows into the
  quotation-seeded lines would silently duplicate quantities.
- A stale result is discarded with a visible explanation; failed runs show the
  typed reason and state that manual entry is unchanged.
- Replacing or removing the PDF clears the current result and the tick
  selection immediately, before the document refresh returns. The panel and
  **Apply selected** compare the run's document id, version and SHA-256 against
  the active document, so a refresh still in flight cannot leave proposals for
  the previous PDF actionable.

This ticket does not implement paternal conflict resolution (#479), retry
policy, or OCR (#478).

## Tests

```bash
# Parser, proposals, abstention, the command engine boundary and the
# production-isolation guards (no Docker)
go test ./backend/extract ./cmd/vextract

# Store lifecycle, stale binding, interrupted recovery (disposable Postgres)
VERA_POSTGRES_TEST_DSN='postgres://…' go test ./backend/store

# API: start/read, 409 without a PDF, 503 when no isolated extractor is
# configured or its command is unavailable (nothing queued), stale
# replacement, typed failures, timeout, idempotent start, interrupted
# recovery, no PO writes
VERA_POSTGRES_TEST_DSN='postgres://…' go test ./backend/api -run Extraction -v

# Real Poppler, opt-in (the image must be built)
VERA_EXTRACTOR_DOCKER=1 go test ./backend/extract -run Docker -v
VERA_POSTGRES_TEST_DSN='postgres://…' VERA_EXTRACTOR_DOCKER=1 \
  go test ./backend/api -run Docker -v   # full server + store + container

# Frontend pure decisions + DOM review/apply
npm --prefix frontend test
```

The default API/UI suite uses a **stub engine**, so it needs neither Poppler nor
Docker. Synthetic PDFs are generated in code (`backend/extract/pdfgen`); no
customer PDF is committed.

## Manual tuning (never committed)

`test-data/*.pdf` is gitignored local material. Run the wrapper directly:

```bash
scripts/vextract-docker.sh < test-data/some-po.pdf | python3 -m json.tool
```

Read the result as evidence, not as a score: the extractor may abstain on
dates, right-aligned money, a table without a repeated header, or a multi-column
layout. Record what it did; do not turn one customer document into a fixture or
an accuracy claim.

## Known limitations (stated, not hidden)

- No OCR: an image-only page is `absent`, and a mixed document only reads its
  text pages.
- A continued table that does not repeat its column header is not parsed.
- A single separator followed by three digits (`1.250`) is returned as
  `needs_review` rather than guessed as 1.25 or 1250.
- Slash dates where both components are ≤ 12 are `needs_review`.
- Right-aligned numeric columns and multi-column header blocks are heuristics;
  they are not a layout model.
- No PO totals are proposed in this slice.
- **Accuracy is unmeasured.** The synthetic test proves plumbing and contract
  shape only; #476 §6 still blocks any real-PO accuracy claim.
