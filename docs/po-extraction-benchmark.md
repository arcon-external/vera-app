# PO PDF extraction — decision and benchmark protocol (#476)

Status: **proposed, not yet validated on real POs.** No authorized customer
PDFs exist in this environment, so no accuracy, latency, or memory number in
this document is a measurement. Every threshold in §2 is a proposal awaiting
pilot signoff. #477/#478 must not claim reliability until the protocol below
has run on authorized samples.

## 1. Decision: proposed pipeline and candidates

### Proposed pipeline (to be validated)

1. **Native text extraction first, page by page.** If a page yields selectable
   text with usable word positions, parse tables/layout from that — no OCR.
2. **Selective OCR only for pages that need it.** A page with no (or
   near-no) extractable text is rendered to an image and OCRed. Text pages
   are never re-OCRed.
3. **Table/layout parsing as a separate stage** operating on either text-layer
   words or OCR words, so the OCR engine and the table parser can be
   benchmarked independently.
4. **Abstain rather than guess.** Anything below the confidence policy in §5
   is returned as `needs_review` with its provenance, never as confirmed
   fact (parent spec user stories 19–20).

### Candidates (not commitments)

| Stage | Candidate | Why it is on the list |
| --- | --- | --- |
| Text layer | Poppler (`pdftotext` family) | Mature text + word-position extraction; the baseline every challenger must beat. |
| OCR for scans | Tesseract | Mature, offline, no per-page fee; the default OCR hypothesis. |
| Table challenger | PaddleOCR (+ table/layout models) | Structure-aware alternative if Tesseract + heuristics fail on real tables. |
| Optional preprocess | OCRmyPDF (deskew/clean before OCR) | Only if measured OCR gains justify the extra dependency and its license chain (§4). |

Selection or rejection happens **only on measured benchmark results plus
license review** — never on reputation. There is no universal-parsing promise:
PO layouts vary, so the documented outcome may be "assisted extraction with
explicit abstentions", not "reliable fully-automatic extraction".

### What is genuinely decided vs blocked

- Decided: the text-first/selective-OCR shape, the candidate list, the metric
  set, the restricted-worker shape, the provenance contract.
- Blocked: every empirical claim — accuracy, latency, RAM, cost, engine
  choice, and all numeric limits. Blocked on authorized sample PDFs (see §6).

## 2. Benchmark protocol

### 2.1 Sample requirements (authorized only)

- **Authorization first.** Only customer PDFs the customer has explicitly
  authorized for this testing. Never production PDFs taken without consent,
  never public-service processing, never committed fixtures (per ticket scope:
  no committed customer fixtures).
- **Stratification.** Cover, as available: per-customer layouts; text vs
  scanned vs mixed PDFs; Indonesian vs English (vs mixed-language) documents;
  clean vs degraded scans (skew, low DPI, stamps/handwriting overlay);
  single- vs multi-page POs; single- vs multi-line-item tables; flattened vs
  nested line layouts (parent spec story 39: customer line numbers are
  independent of Quotation item order).
- **Minimum scale (proposal).** At least 3 distinct customer layouts, at
  least 30 POs total, at least 5 scanned/mixed documents — or a written
  statement of what was actually obtained if authorization yields less, with
  thresholds re-scoped accordingly.

### 2.2 Ground truth

- Human-corrected field/row values per PO, recorded independently of any
  engine output (correct the document, not the parser's guess).
- Ground truth records: PO header fields (number, dates, currency, payment
  terms), every line (description, quantity with up to 4 decimals, UoM, unit
  price), printed totals, and the source page/region per value where
  applicable.
- Money and quantities transcribed exactly (no floats); quantity precision
  follows the PO contract (max 4 decimals), currency precision follows the
  supported-currency rules.

### 2.3 Tuning vs frozen holdout

- Split layouts (not just documents) into **tuning** and **frozen holdout**:
  parser heuristics may be tuned on tuning layouts only.
- Holdout layouts stay unseen until the final acceptance run. Report tuning
  and holdout numbers separately; acceptance is judged on holdout.
- Any post-hoc tuning after seeing holdout results invalidates the run — note
  it and re-freeze a new holdout.

### 2.4 Exact metrics (per engine configuration)

- **Critical-field accuracy:** fraction of header fields (PO number, dates,
  currency, printed totals) exactly correct.
- **Complete-row accuracy:** fraction of ground-truth lines extracted with
  description, quantity, UoM, and unit price all correct.
- **Missing rows** (ground-truth lines with no extracted row) and
  **duplicate rows** (one source line extracted twice), counted separately.
- **Monetary errors** and **quantity errors:** counts plus magnitude bands
  (e.g. off-by-rounding vs off-by-orders-of-magnitude); any sign error is
  reported explicitly.
- **Abstentions:** fields/rows returned as `needs_review` — counted as
  neither correct nor wrong, reported as a rate.
- **Correction time:** staff minutes per PO from extraction output to
  review-complete entry (manual-entry baseline measured the same way, so
  assistance can show a real saving or none).
- **p50/p95 latency** per page and per document; **peak RAM** per run;
  **failure rate** (crash/timeout/rejection counts by cause); **cost per
  page** (compute only — no external API is permitted, so this is infra
  cost, expected to be ~zero marginal).
- Scoring rule (proposal): exact numeric equality after whitespace
  normalization; descriptions compared whitespace-insensitively per the
  parent spec (whitespace ignored, meaningful changes not hidden).

### 2.5 Proposed acceptance thresholds (agree before measuring)

| Metric | Proposed bar (holdout) |
| --- | --- |
| Critical-field accuracy | ≥ 95% |
| Complete-row accuracy | ≥ 90% |
| Monetary/quantity errors | 0 silent (every one must be an abstention or flagged diff) |
| Missing/duplicate rows | 0 silent (same rule) |
| Correction time | strictly below manual-entry baseline |
| p95 latency | within the async UX budget set by #477 (extraction must not block manual entry) |
| Failures | 0 crashes/hangs on hostile/oversized/encrypted/invalid inputs (clean rejection instead) |

These are proposals for pilot signoff, not agreed bars. If the pilot misses
them, the decision is scoped assistance + abstentions — not threshold shopping.

## 3. Security and operations

- **Restricted worker.** Extraction runs in a worker with no production
  database credentials and no signing keys. It receives the PDF bytes (or a
  content-addressed handle) and returns the result shape in §5 — nothing else.
- **Bounded processing.** Enforce concrete limits before release, calibrated
  from benchmark measurements: max upload size, max pages, max render pixels
  per page, max wall-time, max peak memory, max output size. Anything over a
  limit is rejected with a clear message, never partially applied.
- **No external AI/OCR API.** No document bytes or derived text leave for a
  third-party document-processing service. Local models/binaries only.
- **Original preserved.** The uploaded PDF is stored unchanged; extraction
  never mutates it. Replaced PDFs stay in correction history until deliberate
  permanent deletion/retention policy applies (parent spec).
- **Hostile-PDF handling.** Reject invalid files; ask for an unlocked copy of
  encrypted PDFs; never store PDF passwords. Fuzz/oversize inputs must
  produce clean rejections (bounded time/memory), covered by negative tests
  in #477/#478 — without using real customer documents.
- **Google hosting remains a subprocessor.** Server-side processing runs on
  Google infrastructure; do not describe it as on-device. This affects
  customer-facing wording, not the pipeline choice.

## 4. Licensing — review before distribution

The table below states the obligations **to be reviewed with the actual
artifacts before adoption**, not legal conclusions. Pin exact versions first;
a different build (shared lib vs static vs CLI subprocess vs container image)
can change what applies.

| Candidate | License signal | Review point |
| --- | --- | --- |
| Poppler | GPL (v2/v3 family) | Strong copyleft: how the GPL-covered code ships (library link vs subprocess), what source offer is owed, and what that means for a distributed desktop binary. |
| Tesseract | Apache-2.0 | Permissive; confirm NOTICE/attribution handling and traineddata licensing for shipped language data. |
| PaddleOCR (code + models) | Apache-2.0 (code); verify per-model terms | Permissive code, but each model file's terms and the inference runtime's dependency tree (PaddlePaddle weight) must be checked separately — size and dependencies may rule it out before licensing does. |
| OCRmyPDF | MPL-2.0 (file-level copyleft) | Changed OCRmyPDF files must stay open; check interaction with the rest of the binary. |
| Ghostscript (via OCRmyPDF chain) | AGPL-3.0 | Network copyleft: server-side use can trigger source obligations; confirm whether the chosen OCRmyPDF path actually invokes it and under what terms. |

**Server-versus-desktop distinction.** Desktop distribution conveys binaries
to staff machines, which is where GPL/AGPL source-offer duties bite hardest;
pure server-side use changes (but does not remove) the analysis, and AGPL
code reachable over the network needs its own review. The benchmark must
record, per candidate: exact version, artifact (lib/CLI/image/model),
linkage, and the reviewer's signoff — before #477 wires anything in. No
dependency is added by this ticket.

## 5. Integration contract (for #477/#478)

Extraction results are evidence, never facts. Later tickets consume:

- **Per-field result:** normalized value, source page number, source region
  (bbox where the engine provides it, null otherwise), confidence where the
  engine provides it, and status `extracted` | `needs_review` | `absent`.
- **Per-row result:** the same, per cell, plus the customer line number as
  printed (independent of any Quotation match — matching is #471-equivalent
  review logic, not the extractor's job).
- **Run envelope:** engine name + pinned version, document generation (the
  PDF version the result was computed from), page count, timings, abstention
  counts — so a stale result (replaced/deleted source) can be detected and
  discarded.
- **Uncertainty policy:** below-threshold confidence → `needs_review`;
  conflicting duplicate detections → `needs_review`, never silent dedup;
  empty extraction → `absent` (a nonempty manual value always wins over
  `absent` per parent spec story 23).
- **Version pinning:** engine + model versions are part of the stored result
  so repeat runs are comparable and regressions are attributable.

### Runnable synthetic smoke-test plan (no customer data)

1. Generate synthetic POs in-code (Go test helper, owned by #477): text PDFs
   with known header fields, multi-row tables, flattened vs reordered lines,
   multi-page, plus scanned equivalents rendered at two DPIs with one skewed
   variant — all invented data, committed as code, never as binary fixtures.
2. Run the text-first path: assert text pages never invoke OCR (e.g. via a
   stubbed/counted OCR call), header fields score 100%, complete-row
   accuracy scores 100% on clean synthetic text.
3. Run the selective-OCR path on the synthetic scans: assert only image
   pages are OCRed and every low-confidence cell arrives as `needs_review`.
4. Negative cases: encrypted PDF (rejected with unlock message), truncated
   file (clean rejection), oversized input (limit rejection), stale
   generation (result discarded, manual entry preserved).
5. This plan proves plumbing and contract shape only — it says nothing about
   real-PO accuracy, which waits on §6.

## 6. Blocker: what is required from the user

#477 (text-PDF extraction) and #478 (scanned/mixed) cannot claim accuracy
until the user provides **authorized sample customer PDFs** meeting §2.1
(stratified text/scan/mixed, ground-truth correctable, holdout-capable), plus
**signoff on the §2.5 thresholds**. Until then the honest status of this
ticket is: decision proposed, protocol ready, validation blocked.
