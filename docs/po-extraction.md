# PO PDF extraction — text and selective OCR (#477, #478)

Status: **shipped as assistance, not validated accuracy.** This document covers
both halves of the pipeline proposed in
[`po-extraction-benchmark.md`](po-extraction-benchmark.md): the text layer
(#477) and scanned/mixed pages (#478). The accuracy thresholds in that
document (§2.5) have **not been measured** — no authorised customer PDFs, no
human-corrected ground truth and no frozen holdout were available — so nothing
here claims that extraction is reliable. It claims that the plumbing, the
evidence contract, the per-page decision, the uncertainty policy, the
stale-source rule and the explicit-apply rule are in place.

**The scan/mixed position for this release is therefore the stated limitations
below, not a measured acceptance result.** #476 §2.5 remains unvalidated.

## What ships

- `cmd/vextract`: a restricted binary that reads a PDF on **stdin** and writes
  the extraction contract to **stdout**. It drives Poppler (`pdftotext`,
  `pdftoppm`) and Tesseract (`tesseract`).
- `docker/po-extract/Dockerfile`: the pinned image the server runs.
- `backend/extract`: the bbox parser, the Tesseract TSV parser, the per-page
  decision, the OCR pipeline, the conservative field/table proposals, the
  command engine and the run service.
- `backend/store`: migration `038_purchase_order_extractions` and the run
  lifecycle.
- `backend/api` + `backend/po_extraction.go`: the authenticated API and desktop
  bindings.
- `frontend/src/poExtraction.ts` and the review panel: explicit, per-field and
  per-row apply into the draft, plus per-page OCR provenance and low-confidence
  labels.

Nothing here writes a purchase-order field automatically, calls an external
AI/OCR service, finalizes a PO, or stores a PDF password.

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
vextract (container: --network none, read-only root, 512 MB, 1 CPU, 64 PIDs)
  │  stdin: PDF bytes
  │  per page:
  │    text layer: pdftotext -bbox-layout
  │    decision:    usable text -> keep it (never OCR)
  │                 otherwise  -> pdftoppm at a bounded DPI
  │                              -> tesseract ... tsv (word + bbox + confidence)
  │  one shared line/row proposer over text words and OCR words
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

### Per-page processing decision

The decision is made for **every page**, never once for the document. That is
what stops a readable text header from concealing a scanned table page.

| Page's own text layer | Mode | Processing |
| --- | --- | --- |
| ≥ 8 words **and** ≥ 40 letters/digits | `text` | The text words are used as they are. The page is **never** OCRed. |
| Anything less (image-only, blank, a scan with a stray page number or stamp) | `ocr` | The page is rendered at a bounded DPI and read by Tesseract. |

Both counts ignore punctuation-only tokens (`-`, `|`, `—`), because a scan
often yields a few of those from its text layer. The threshold is a
**usability floor**, not a quality score. Every page's mode, both counts and
the reason are stored in the result envelope, so the decision is auditable
after the fact.

Known gap: the decision is page-level, not region-level. A page that mixes a
rich text header with an *image* table on the same page is classified `text`
and the image table is not read. That is a documented limitation, not a silent
success: the absent rows are visible in the review.

### Bounded OCR

Only pages that need OCR are rendered. Each render is bounded before it starts:

- one page at a time, at `VERA_EXTRACT_OCR_DPI` (default **300**);
- a pixel ceiling per page (default **12,000,000**). A larger page is rendered
  at the highest whole DPI that fits, never below **150**; a page that cannot
  fit the ceiling at 150 DPI fails with `page_too_large`;
- at most **20** OCR pages per document (`ocr_page_limit` beyond that);
- at most **30 s** for all OCR pages together and **15 s** for one page's
  render + OCR pair (`timeout`);
- a **64 MiB** image and an **8 MiB** TSV per page;
- the container's 512 MB memory, 64 PIDs and 1 CPU limits.

All limits fail the run with a typed reason. No partial result is stored: a
table read up to page 3 of 6 looks complete and is worse than an abstention.

## The evidence contract

The same schema is defined once in `backend/apicontract/extraction.go` and used
by the extractor, the stored result, the API and the desktop binding. Version 2
adds the per-page OCR evidence; a version-1 stored result remains readable (the
new fields decode as empty and the panel says nothing about OCR).

```jsonc
{
  "schemaVersion": 2,
  "engine": "poppler",
  "engineVersion": "25.03.0",
  "ocrEngine": "tesseract",          // present when OCR ran
  "ocrEngineVersion": "5.5.0",
  "pageCount": 2,
  "pagesWithText": 1,                // pages read from their own text layer
  "pagesWithoutText": 1,             // pages with no usable text layer (OCRed)
  "pagesWithOcr": 1,
  "durationMs": 1840,
  "pages": [{ "number": 1, "width": 595, "height": 842 }],
  "pageModes": [
    {
      "number": 1, "mode": "text",
      "textWords": 22, "textChars": 210,
      "ocrWords": 0, "confidence": null,
      "reason": "The page's text layer carries 22 words and 210 letters/digits ..."
    },
    {
      "number": 2, "mode": "ocr",
      "textWords": 1, "textChars": 2,
      "ocrWords": 31, "dpi": 300, "confidence": 88.4,
      "reason": "The page's text layer carries only 1 word and 2 letters/digits ..."
    }
  ],
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
  ],
  "rows": [
    {
      "lineNumber": { "value": "3", "status": "needs_review", "page": 2,
                      "bbox": { ... }, "confidence": 74 },
      "description": { ... },
      "quantity": { "value": "1", "status": "needs_review", "confidence": 58,
                    "note": "Read with OCR confidence 58%, below the 70% ..." },
      "uom": { ... }, "unitPrice": { ... }, "amount": { ... },
      "status": "needs_review",
      "page": 2,
      "bbox": { ... },
      "note": "The column header is on page 1; this row continues that table ..."
    }
  ],
  "abstentions": { "fields": 2, "rows": 1 }
}
```

- **Value** is normalised (an ISO date, a plain decimal with `.`, an uppercase
  currency code). **Raw** is what was printed.
- **Page/bbox** are in PDF points with the origin at the page's top-left.
  OCR pixels are converted with the DPI actually used, so text-layer and OCR
  evidence share one coordinate system. `pageModes[].dpi` records that DPI.
- **confidence** is the lowest word confidence behind a cell, 0–100, present
  only when the engine measured it. Text-layer values are `null`. A cell whose
  words are only partly measured claims no confidence.
- **Absent** is a deliberate answer: the label or value was not found (or the
  page had no readable words). It is never an empty string pretending to be a
  value.
- **needs_review** covers a value below the confidence floor, a value that
  cannot be normalised, an ambiguous date, a repeated label with a different
  value, a row missing a required cell, a continued row without a repeated
  header, and a duplicate or missing line-number risk. The note says why.
- **No inference:** an amount is never computed as quantity × price, a missing
  quantity is never filled, and a table without a readable header produces no
  rows.

### Uncertainty policy (#478)

- **Confidence floor is 70.** It is a release policy, not a measured optimum:
  the benchmark has not run. A cell whose lowest word confidence is below 70
  becomes `needs_review` with the measured percentage in the note; the row
  becomes `needs_review` too, including when the low-confidence cell is the
  amount or line number (which are not required for an "extracted" row).
- **A number split across words is not silently joined.** "1 250.00" is a
  grouping-shaped split and becomes `1250.00`; "1250" "00" or "10" "." "5" do
  not form a grouping and become `needs_review` with the printed tokens kept.
  Blindly removing spaces would turn 1,250.00 into 125000.
- **Rows continue across pages.** When a page has no repeated column header but
  the previous page ended inside a table, the column positions are carried.
  Every row read this way is `needs_review` and its note names the page whose
  header it used. A running page header above the continued table is not a row
  and is not appended to the previous line's description. A page with no header
  after the table already ended (a total/footer was read) produces no rows.
- **Duplicates and missing lines are reported, never repaired.** A repeated
  line number flags every occurrence; an identical line is flagged on both
  occurrences; a gap in numbered lines flags the row after the gap with the
  missing range. No row is deduplicated, merged, renumbered, dropped or
  invented.
- **OCR that finds no words is an honest empty page.** It is still recorded as
  an OCR page with `ocrWords: 0`, and no rows or fields are invented.

### Staleness is part of the envelope

A run stores the document id, version and SHA-256 it read. The store checks
that generation at finish and on every latest read:

- PDF replaced or removed → status `stale`, `result` discarded, `errorCode =
  document_changed`.
- Server restarted → status `failed`, `errorCode = interrupted` at startup.
- The API response also includes the active document generation, so the client
  can detect a mismatch even if it missed the replacement event.

Manual editor values are never touched by either transition. The replace and
stale guards are unchanged by OCR: a result computed from a replaced PDF is
discarded exactly as before.

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

`page_limit`, `ocr_page_limit`, `page_too_large`, `timeout`, `invalid_output`,
`output_too_large`, `extractor_unavailable`, `document_too_large`,
`document_missing`, `document_changed`, `interrupted`, `cancelled`,
`ocr_unavailable`, `engine_failed`.

Encrypted PDFs are rejected at upload by the store's PDF validator with the
existing unlock request; the extractor never sees a password. Invalid or
truncated files fail as `invalid_pdf`/`engine_failed` with a readable message,
and the stored original is unchanged.

The desktop binding (`StartPurchaseOrderExtraction`,
`GetPurchaseOrderExtraction`) decodes the result into typed DTOs; the raw JSON
never reaches the React code.

## Bounds

| Bound | Value | Where enforced |
| --- | --- | --- |
| PDF input | 20 MiB | upload validation, store schema, engine |
| Pages | 40 | `vextract -max-pages` (or `VERA_EXTRACT_MAX_PAGES`), parent validation |
| OCR pages | 20 | `vextract -max-ocr-pages` (or `VERA_EXTRACT_MAX_OCR_PAGES`) |
| Render resolution | 300 DPI default, 150 floor, 600 ceiling | `vextract -ocr-dpi` (or `VERA_EXTRACT_OCR_DPI`) |
| Rendered pixels | 12,000,000 per page | pipeline before render; decoded image after render |
| OCR wall time | 30 s total, 15 s per page | `vextract -ocr-timeout`, `-ocr-page-timeout` |
| Extractor wall time | 45 s | service context; engine context; process group killed |
| Rendered image | 64 MiB per page | OCR adapter |
| Tesseract TSV | 8 MiB per page | OCR adapter |
| Extractor stdout | 8 MiB | engine limited buffer |
| Poppler XML | 32 MiB | `vextract` capped buffer |
| Container memory | 512 MiB | wrapper/container flags |
| Container `/tmp` tmpfs | 256 MiB | wrapper/container flags |
| PIDs / CPU | 64 / 1 | wrapper/container flags |
| Network | none | `--network none` |
| Root filesystem | read-only | container flags |

Over a bound is a **failed run** with a typed reason, never a partial result.

## Security and operations

- **Production isolation is the dedicated container, not a child process.** A
  same-UID child in the server container is not isolated: it can read
  `/proc/<server-pid>/environ` and recover the database DSN or the signing key,
  so clearing its environment proves nothing. Extraction therefore runs only
  through the dedicated restricted image (`docker/po-extract`) — or an
  equivalent isolated worker/service that holds no database or signing
  credentials. `Dockerfile.veraserver` ships neither `vextract`, Poppler nor
  Tesseract, and without `VERA_EXTRACTOR_CMD` extraction is disabled (`503`),
  not silently degraded.
- **Restricted worker.** PDF bytes in, result JSON out. No DSN, no signing key,
  no session token, no caller-supplied path. The child gets a minimal
  environment (`PATH`, `HOME=/tmp`, `TMPDIR=/tmp`, `LANG`, the page cap, and the
  OCR settings when the operator set them), never `os.Environ()`. The container
  adds `--network none`, a read-only root, `--cap-drop ALL`,
  `--security-opt no-new-privileges`, and memory/PID/CPU limits. The OCR
  subprocesses write only to the bounded `/tmp` tmpfs and their files are
  removed before the page call returns.
- **Build context.** `.dockerignore` excludes `.env`, `test-data/*.pdf`,
  `frontend/node_modules`, `frontend/dist` and repository history, so local
  credentials and customer PDFs cannot enter an image layer.
- **Original preserved.** The stored PDF is never rewritten; extraction reads
  the stored bytes.
- **No external service.** No document bytes or derived text leave the host.
  Tesseract runs locally in the image.
- **Hostile input.** The store validator rejects malformed/encrypted PDFs on
  upload; Poppler, Tesseract and the renderer failures and timeouts become
  clean typed failures; page, pixel, time and output bounds are enforced before
  and after rendering.
- **Google hosting remains a subprocessor.** Server-side processing runs on
  Google infrastructure; this is not on-device processing.

### Image pinning, licensing and attribution

`docker/po-extract/Dockerfile` is the **reviewed configuration**. It pins both
bases by digest and every engine package by exact version:

- builder `golang:1.26-bookworm@sha256:d9c68c2c…` (matches the Go toolchain);
- runtime `debian:trixie-slim@sha256:62bd423a…`;
- `poppler-utils=25.03.0-5+deb13u4` (Poppler reports `pdftotext version 25.03.0`);
- `tesseract-ocr=5.5.0-1+b1` (Tesseract reports `tesseract 5.5.0`);
- `tesseract-ocr-eng=1:4.1.0-2` and `tesseract-ocr-ind=1:4.1.0-2` — the only
  two languages the documents use. Adding a language is a reviewed change.

An exact package pin is intentional: if the Debian suite rotates a package
away, the image build fails loudly instead of silently shipping a different
engine. Moving a pin is a reviewed change, not a rebuild side effect.

The Dockerfile records the installed engine versions in
`/etc/vextract-engine-version` and `/etc/vextract-ocr-version`; `vextract
-version` prints both, and every result carries `engineVersion` and, when OCR
ran, `ocrEngineVersion`, so a repeat run is comparable and a regression is
attributable to a version.

**License review basis (user-approved, 2026-10-21).**

- **Poppler is GPL.** Vera uses it as a separate server-side subprocess inside
  this dedicated image and does not link it into the desktop application or the
  server binary, so the desktop distribution is not conveying Poppler. The
  corresponding source for the pinned Debian package is obtainable from the
  Debian archive (`apt-get source poppler=25.03.0-5+deb13u4`) and from the
  matching Poppler 25.03.0 release; distributing a rebuilt image carries the
  usual source-offer duty for that GPL program, which the release notes must
  satisfy.
- **Tesseract is Apache-2.0.** The Debian `tesseract-ocr-eng` and
  `tesseract-ocr-ind` traineddata come from the Tesseract `tessdata` project
  and are distributed under Apache-2.0. Vera uses the binaries and data as
  separate server-side subprocesses in this image, not linked into the desktop
  application or the server binary. The attribution record is the Debian
  package copyright (`/usr/share/doc/tesseract-ocr*/copyright`,
  `/usr/share/doc/tesseract-ocr-eng/copyright`,
  `/usr/share/doc/tesseract-ocr-ind/copyright`); a rebuilt image carries the
  usual Apache-2.0 notice duty. The project attribution is Tesseract OCR,
  © Google and the Tesseract contributors.
- **The server image ships none of this.** `Dockerfile.veraserver` contains no
  Poppler, no Tesseract and no extractor.

Build and check:

```bash
scripts/build-po-extract-image.sh          # builds vera-po-extract:dev, prints both versions
```

In development the server reaches the image through
`scripts/vextract-docker.sh` on a Docker-capable host. That wrapper is a
convenience around the same isolation flags, not a production default. The
result's engine versions make the engines that produced it attributable.

### Environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `VERA_EXTRACTOR_CMD` | *(none)* | The isolated extractor command. The server splits on whitespace; no shell is invoked. Empty disables extraction and the start endpoint answers `503`; there is no in-process fallback. |
| `VERA_EXTRACT_MAX_PAGES` | `40` | Page cap read by `vextract`; the server also validates the result against its own cap. |
| `VERA_EXTRACT_OCR_DPI` | `300` | Render resolution for OCR pages (150–600). |
| `VERA_EXTRACT_MAX_OCR_PAGES` | `20` | OCR page cap for one document. |
| `VERA_EXTRACT_OCR_LANGS` | `eng+ind` | Tesseract language data. |

For local development, point the server at the container wrapper:

```bash
VERA_EXTRACTOR_CMD="$PWD/scripts/vextract-docker.sh" \
  go run ./cmd/veraserver
```

The wrapper adds `--network none`, `--read-only`, the `/tmp` tmpfs, memory/PID/
CPU limits and `--cap-drop ALL`. It forwards the page and OCR settings.

## UI behaviour

- **Extract and compare** is enabled whenever the draft/correction editor has
  an attached PDF. A saved view's panel explains that a correction is needed,
  because a saved record's values are protected.
- Progress is honest: “Queued for extraction…” then “Reading the PDF pages
  (text layers and scans)…”; no percentage is invented.
- The review shows every header field and row with its status, printed raw
  value, source page and a small relative region map.
- **OCR provenance is visible**: the engines and versions, one line per page
  with its mode (`text layer` or `OCR at 300 DPI (24 words, 88% mean
  confidence)`), and the confidence label on any OCR-read value. A value below
  the 70% floor says so; a row also names its low-confidence cells.
- Default selection is conservative: only an `extracted` header proposal into
  an **empty, untouched** editor field starts ticked. A field staff already
  filled is not ticked, whether equal or different; a field they cleared while
  the run was active waits for a choice; `needs_review` and `absent` wait for a
  tick. Every row starts unticked.
- **Apply selected** is the only path into the editor. Currency is evidence
  only — the PO's currency comes from the Quotation and is protected.
- **Manual edits during processing are reconciled, not overwritten (#479).**
  The editor records a baseline when tracking starts and compares every later
  value against it. An untouched empty field may fill in from an `extracted`
  proposal; a nonempty value that differs, or a field staff deliberately
  cleared, becomes an explicit **Keep draft / Use extracted** choice. Equal
  normalized values need no prompt (numbers compare semantically, text ignores
  surrounding and repeated whitespace without hiding a changed word), and a
  manual nonempty value is kept when extraction is empty. A `needs_review`
  proposal never auto-populates. Choices are committed only by Apply.
- **Line changes are protected too.** A line cell edited or cleared, or a line
  added or removed while the run was active, turns the row replacement into an
  explicit **Keep my lines / Use extracted lines** choice; while Keep stands the
  row checkboxes are disabled. Differing rows nobody touched remain the
  ordinary opt-in proposals, not conflicts.
- **Extract again** (the same “Extract and compare” action) never clears,
  overwrites or saves the editor. It keeps the working copy, the correction
  reason and the conflict choices; only stale row ticks are dropped. Replacing
  or removing the PDF discards the run, its tracked edits and its choices with
  the old document generation, so a late response cannot resurrect them.
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

This flow is deliberately separate from the PO-versus-Quotation difference
review on final Save (#471): extraction turns proposals into draft values,
whereas the final review chooses between the finished PO and its issued source.

## Tests

```bash
# Parsers, per-page decision, TSV conversion, the bounded pipeline, proposals,
# abstention/uncertainty rules, the command engine boundary and the
# production-isolation guards (no Docker, no Tesseract)
go test ./backend/extract ./cmd/vextract

# Store lifecycle, stale binding, interrupted recovery (disposable Postgres)
VERA_POSTGRES_TEST_DSN='postgres://…' go test ./backend/store

# API: start/read, 409 without a PDF, 503 when no isolated extractor is
# configured or its command is unavailable (nothing queued), stale
# replacement, typed failures, timeout, idempotent start, interrupted
# recovery, page-mode/OCR evidence round-trip, no PO writes
VERA_POSTGRES_TEST_DSN='postgres://…' go test ./backend/api -run Extraction -v

# Frontend pure decisions + DOM review/apply/provenance/confidence
npm --prefix frontend test

# Real Poppler + Tesseract, opt-in (the image must be built)
VERA_EXTRACTOR_DOCKER=1 go test ./backend/extract -run Docker -v
VERA_POSTGRES_TEST_DSN='postgres://…' VERA_EXTRACTOR_DOCKER=1 \
  go test ./backend/api -run Docker -v   # full server + store + container
```

The default suite uses a **stub text engine and a stub OCR engine**, so it needs
neither Poppler, Tesseract nor Docker. The stub OCR test asserts by call count
that a page with a usable text layer never invokes OCR and an image-only page
does; `cmd/vextract` repeats that assertion with fake `pdftoppm`/`tesseract`
executables while the real OCR binaries are absent.

The opt-in Docker tests build synthetic scans from the repository's text PDFs:
the pinned Poppler inside the image renders a page at 150 or 300 DPI, one
variant is skewed by 3°, and the raster is wrapped back into an image-only PDF.
They assert the mixed text-header/scanned-table case, the 150 DPI and skewed
cases, and a fully scanned document. They prove plumbing and the per-page
decision; **they say nothing about real-PO accuracy**.

Synthetic PDFs are generated in code (`backend/extract/pdfgen`); no customer
document and no binary fixture is committed.

## Manual tuning (never committed)

`test-data/*.pdf` is gitignored local material. Run the wrapper directly:

```bash
scripts/vextract-docker.sh < test-data/some-po.pdf | python3 -m json.tool
```

Read the result as evidence, not as a score: the extractor may abstain on
dates, right-aligned money, a table without a repeated header, a page whose
scan is too degraded for the confidence floor, or a multi-column layout. Record
what it did; do not turn one customer document into a fixture or an accuracy
claim.

## Known limitations (stated, not hidden)

- **Accuracy is unmeasured.** The synthetic tests prove plumbing and contract
  shape only; #476 §6 still blocks any real-PO accuracy claim. #476 §2.5
  scan/mixed thresholds are **not validated** — there is no ground truth and no
  holdout. The release position is this limitations list.
- The per-page decision is not a region model: an image table inside an
  otherwise text-rich page is not detected, so that page is read from its text
  layer only.
- No deskew or image cleanup. A skewed scan is passed to Tesseract as rendered;
  a 3° synthetic skew works, but no quality claim is made.
- No table-structure model. Rows use column positions and a readable header;
  a continued table without a repeated header is carried and flagged, not
  repaired.
- A single separator followed by three digits (`1.250`) is returned as
  `needs_review` rather than guessed as 1.25 or 1250.
- Slash dates where both components are ≤ 12 are `needs_review`.
- Right-aligned numeric columns and multi-column header blocks are heuristics;
  they are not a layout model.
- No PO totals are proposed in this slice.
- OCR language data is `eng` + `ind` only. A document in another language is
  outside the reviewed configuration.
