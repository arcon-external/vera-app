# Quotation → PO sprint verification (#482)

Status: **partially verified — one required gate is BLOCKED and one user-facing
copy defect is open.** No release, merge, push or production change was made.

- Branch: `feat/464-po-sprint`
- Verified commit: `8809adb` (`merge: resolve manual extraction conflicts and retry
  safely (#479)`), clean tree before the verification.
- Database: disposable local `vera-pg-test` (PostgreSQL 18.6),
  `postgres://vera:vera@127.0.0.1:55432/veratest?sslmode=disable`, and the
  rehearsal schema `vera_sprint_upgrade`. Production was never contacted.
- Date: 2026-10-22.

## Verdict at a glance

| # | Acceptance criterion | Status |
| --- | --- | --- |
| 1 | Manual and PDF workflows, both entry paths, selector steps, lists/filters, drafts, finalization, correction | **PASS** (behaviour); one copy defect, D1 |
| 2 | Optional numbering, race/overwrite paths, deletion challenges, source survival/repair, permissions end to end | **PASS** |
| 3 | Production-shaped upgrade/rollback rehearsal; old signature still verifies | **PASS** (local binary + schema rehearsal, not a production deploy) |
| 4 | Real assembled pages, modal keyboard/focus, errors, progress, evidence at narrow/wide | **PASS** for jsdom + SSR snapshots; real Wails interactions **HAND-DRIVEN** |
| 5 | Extraction holdout results against the agreed quality/resource/security/license gates | **BLOCKED — user input required** |
| 6 | Full repository gates on a quiescent tree; Judge before merge; orchestrator revalidation | Gates **PASS**; Judge and orchestrator revalidation **HAND-DRIVEN / pending** |
| 7 | No release while sprint issues/PRs are unresolved; an unresolved quality gate blocks completion | **BLOCKED** by criterion 5 (and PR #484 is still a draft) |

**#482 cannot be closed and #464 cannot be declared fully verified** until the
extraction holdout inputs below are supplied, or the user explicitly accepts
assisted extraction without accuracy claims. The rest of the sprint is verified.

## 1. Manual and PDF workflows

### Entry paths

- Home page: `frontend/src/components/DocumentHomePage.test.tsx` — four
  destinations, Quotation and PO enabled, DO/Invoice truly disabled with
  "Coming later". Snapshot `DocumentHome-360.png` / `DocumentHome-1280.png`.
- Quotation detail: `frontend/src/components/QuotationDetailPage.test.tsx` — "Add
  Purchase Order from a Quotation" is an enabled action and Edit/identity are
  unchanged.
- Both buttons are wired in `App.tsx` to `setPage("po")` and
  `setPOSelectorOpen(true)`. **The click itself is not covered by an automated
  test**; it is a two-line state setter and remains hand-driven in Wails.

### Selector (three steps, 15/page, search, occupancy)

- Store/API: `TestPOSourceListPaginatesIssuedQuotationsFifteenPerPage`,
  `TestPOSourceSearchCoversNumberSubjectCustomerAndItemText`,
  `TestPOSourceListCountsEveryIssuedRevisionAndShowsTheLatest`,
  `TestPOSourceListExcludesUnsavedDrafts`,
  `TestPOSourceListReportsTheActivePOAndHonoursTheHideFilter`,
  `TestPOSourceRevisionsAreIssuedOnlyNewestFirst`,
  `TestPOSourceSelectionRevalidatesAtConfirmTime`,
  `TestPOSourceRevalidationRequiresAnExplicitRevision` — all PASS.
- Views: `POQuotationSelector.test.tsx` (step 1/2/3, sole-revision
  auto-selection, disabled Continue/Confirm, revalidation error) and
  `POQuotationSelector.dom.test.tsx` (focus entry/restore/trap, Escape, Cancel
  and unmount never deliver a stale selection) — all PASS.
- Snapshots `POSelectorStepOne/Two/Three-{360,1280}.png` and
  `POSelectorRepairMode-{360,1280}.png` read; the list scrolls, the 360 footer
  clips the next row at the scroll boundary rather than overflowing.
### Lists and filters

Store `TestListPurchaseOrdersPaginatesFifteenPerPageNewestReceivedFirst`,
`TestListPurchaseOrdersSearchesPONumberQuoteNumberAndCustomerName`,
`TestListPurchaseOrdersFiltersByCustomersReceivedRangeAndState`,
`TestListPurchaseOrdersSourceMissingPredicateOnLiveRows`,
`TestListPurchaseOrdersRejectsInvalidFilters`; API
`TestPurchaseOrderListPaginatesFifteenPerPage`,
`TestPurchaseOrderListSearchCoversTheThreePromisedFields`,
`TestPurchaseOrderListFiltersByCustomerReceivedRangeAndState`,
`TestPurchaseOrderListSourceMissingPredicateOnLiveRows`,
`TestPurchaseOrderListRejectsMalformedFilters`; frontend
`PurchaseOrderListPage.test.tsx` and `.dom.test.tsx` (filters are sent, reset
works, empty state honest). Snapshots `PurchaseOrderArchive-{360,1280}.png`,
`PurchaseOrderArchiveNoMatch-{360,1280}.png`. The 360 table scrolls
horizontally by design (`min-w-[900px]` inside `overflow-x-auto`).

### Drafts

Store `TestPurchaseOrderDraftCreatesFromTheExactIssuedRevision`,
`TestPurchaseOrderDraftDefaultsReceivedDateAndAcceptsAnOverride`,
`TestPurchaseOrderDraftUpdateReplacesHeaderAndLines`,
`TestPurchaseOrderDraftOneActiveRecordPerQuotationUnderConcurrency`,
`TestPurchaseOrderDraftRoundTripsCommercialPaymentWording`; API
`TestPurchaseOrderDraftCreateReopenAndUpdate`,
`TestPurchaseOrderDraftRefusesASecondActiveRecord`,
`TestPurchaseOrderDraftRejectsInvalidLinesAndSavedWrites`,
`TestPurchaseOrderOccupancyFeedsTheSelector`; frontend editor view/dirty-guard
and discard tests (`PurchaseOrderEditor.test.tsx`, `.dom.test.tsx`). Snapshots
`PurchaseOrderEditor-{360,1280}.png`, `PurchaseOrderTabDiscard-{360,1280}.png`.

### Finalization and correction

- Finalize/difference review: store `TestFinalizePurchaseOrderSavesTheRecordAndKeepsTheQuotation`,
  `...RequiresTheFinalSaveMinimum`, `...RequiresExplicitDifferenceChoices`,
  `...RequiresMissingAndExtraAcknowledgement`, `...RequiresTheNoPDFConfirmation`,
  `...KeepsThePurchaseOrdersOwnTotal`; API `TestPurchaseOrderReviewAndFinalizeOverTheAPI`,
  `TestPurchaseOrderFinalizeEnforcesTheReviewRules`; frontend
  `PurchaseOrderFinalize.test.tsx` / `.dom.test.tsx` (no preselected choice,
  recalculation, no-PDF confirmation, optional confirmation number).
- Correction/overwrite: store `TestPurchaseOrderEveryWriteBumpsItsVersionAndAStaleSaveIsNamed`,
  `TestPurchaseOrderCorrectionKeepsThePreviousValuesAndRequiresAReason`,
  `TestPurchaseOrderCorrectionPreservesProtectedIdentity`,
  `TestPurchaseOrderCorrectionConflictPreservesBothHistories`; API
  `TestPurchaseOrderStaleSaveReturnsTheNamedConflict` (and the overwrite test
  named in §2); frontend `PurchaseOrderCorrection.dom.test.tsx`,
  `PurchaseOrderHistoryPanel.test.tsx`, `PurchaseOrderWorkspace.dom.test.tsx`.
- Snapshots read: `PurchaseOrderDifferenceReview-{360,1280}.png`,
  `PurchaseOrderFinalConfirmation-360.png`,
  `PurchaseOrderCorrectionEditor-1280.png`,
  `PurchaseOrderVersionConflict-360.png`.

### PDF attach, replace, remove and extraction

- Attach/replace/history: store `TestAttachPurchaseOrderDocumentStoresExactBytesAndMetadata`,
  `TestReplacingAPurchaseOrderDocumentRetainsThePriorVersion`,
  `TestRemovingThePurchaseOrderDocumentRetainsHistoryAndNeedsConfirmation`,
  `TestAttachPurchaseOrderDocumentRefusesInvalidEncryptedAndOversized`; API
  `TestPurchaseOrderDocumentUploadReadReplaceAndHistory` and the removal /
  no-PDF-finalize tests; frontend `PurchaseOrderDocumentPanel.test.tsx` / `.dom.test.tsx`.
- Extraction: `go test ./backend/extract ./cmd/vextract` and
  `backend/api -run Extraction` (pipeline, per-page text/OCR decision, typed
  failures, stale-after-replacement, idempotent start, OCR provenance), plus
  frontend `poExtraction*.test.ts`, `PurchaseOrderExtractionPanel.dom.test.tsx`
  and `PurchaseOrderEditor.dom.test.tsx` conflict/apply tests.
- Real engines, opt-in: `VERA_EXTRACTOR_DOCKER=1 go test ./backend/extract -run Docker -v`
  → 4 PASS (text, mixed header + scanned table, 150 DPI + 3° skew, fully
  scanned). Full server + store + container: API `TestDockerExtractionEndToEnd`
  → PASS. These use **synthetic** PDFs and prove plumbing only.
- Snapshots read: `PurchaseOrderDocumentAttached-360.png`,
  `PurchaseOrderExtractionReview-360.png`,
  `PurchaseOrderExtractionConflicts-{360,1280}.png`.

## 2. Numbering, races, deletion, source survival, permissions

All commands below ran with the disposable DSN and `-race -count=1`; the
package exits are recorded in §6.

- **Confirmation numbering** (prefix 23, per customer/year, 8000 warn, widen
  past 9999, null until assigned, never reused): `TestPOConfirmationNumberFormatAndWIBYear`,
  `TestFinalizeAssignsOptionalConfirmationNumbersPerCustomerAndYear`,
  `TestAssignConfirmationNumberUsesItsOwnCustomerYearCycle`,
  `TestAssigningTwiceReturnsTheExistingNumber`,
  `TestConfirmationNumberCannotBeChangedClearedOrRegenerated`,
  `TestDeletedConfirmationNumberIsNeverReissued`,
  `TestPOConfirmationWarnWidenAndExhaust`,
  `TestUnrequestedOrFailedSavesConsumeNoConfirmationNumber`,
  `TestPOConfirmationAllocationRollsBackWithItsTransaction`,
  `TestAssignConfirmationNumberRefusesADraft`; API
  `TestPurchaseOrderConfirmationNumbersOverTheAPI` — all PASS.
- **Race/overwrite paths:** `TestPurchaseOrderDraftOneActiveRecordPerQuotationUnderConcurrency`,
  `TestFinalizePurchaseOrderNumberRaceHasOneWinner`,
  `TestConfirmationAllocationIsAtomicUnderConcurrency`,
  `TestLaterAssignmentIsIdempotentUnderConcurrency`,
  `TestAttachPurchaseOrderDocumentKeepsOneActiveUnderARealRace`,
  `TestRepairPurchaseOrderSourceLosesTheRaceForAnOccupiedTarget`,
  `TestChainDeletionRaceWithACorrectionNeverCascadesUnconfirmed`,
  `TestConcurrentChainDeletionsRunOnce`,
  `TestConcurrentExtractionStartsShareOneActiveRun`; API
  `TestPurchaseOrderDraftOverwriteOverTheAPIKeepsTheReplacedDraftAndAttachment`,
  `TestPurchaseOrderStaleSaveReturnsTheNamedConflict` — all PASS.
- **Deletion challenges:** `TestDeleteQuotationChainNeedsTheChallengeAndRemovesEverything`,
  `TestDeleteChainStopsWhenTheChainChangedAndRenewsTheChallenge`,
  `TestDeletionChallengesExpire`, `TestExpiredChallengeIsRemovedWhenAnotherChallengeDeletesTheChain`,
  `TestDeletionChallengeLettersAvoidLookalikes`,
  `TestDeletionRemovesEveryChallengeItsOwnWriteInvalidated`,
  `TestExpiredChallengesArePurgedByEveryDeletionObservation`,
  `TestPausedIssuanceFindsTheChainGoneAndStoresNoFingerprint`; API
  `TestDeleteChainOverAPIRequiresTheTenLetterChallenge`,
  `TestDeleteChainOverAPIStopsOnAChangedChainAndRenews`,
  `TestDeleteRejectsAReasonOrTypedNumberMismatchOverAPI`, and the
  source-missing fingerprint tests — all PASS.
- **Source survival/repair:** `TestDeletePurchaseOrderRecordKeepsTheQuotationAndTheCounter`,
  `TestDeleteQuotationRecordKeepsItsPOAsSourceMissingAndReadable`,
  `TestSourceMissingDeletionFingerprintCoversADocumentUploadedAfterPreview`,
  `TestSourceMissingDeletionFingerprintCoversHistoryAddedAfterPreview`,
  `TestRepairSourceMissingPurchaseOrderLinksTheSurvivorAndKeepsItsIdentity`,
  `TestRepairSourceMissingPurchaseOrderRepeatsTheSavedEligibilityRules`,
  `TestRepairPurchaseOrderSourceKeepsAConfirmationNumberAndRecordsTheMove`,
  `TestRepairPurchaseOrderSourceMovesCustomerOnlyBeforeAssignment`,
  `TestRepairPurchaseOrderSourceRefusesAnOccupiedTarget`,
  `TestListPOSourceQuotationsRepairModeFreesOnlyTheRepairedRecord`; API
  `TestSourceMissingPurchaseOrderIsFoundAndRepairedOverTheAPI`,
  `TestPurchaseOrderSourceRepairProtectsTheConfirmationNumberOverTheAPI`,
  `TestSourceMissingDraftIsReadableButCannotBeSaved` — all PASS.
- **Permissions end to end:** `TestDeletionIsAdminOnly`,
  `TestRepairPurchaseOrderSourceRequiresAnAdministrator`; API
  `TestDeletionEndpointsRefuseStaff`,
  `TestPurchaseOrderSourceRepairIsAdminOnlyOverTheAPI`,
  `TestPurchaseOrderEndpointsRequireASession`,
  `TestPOSourceEndpointsRequireASession`,
  `TestExtractionRequiresASessionAndAKnownRecord`; frontend
  `PurchaseOrderWorkspace.dom.test.tsx` admin/staff action visibility and
  `DeletionDialog.dom.test.tsx` 403 handling — all PASS.

### CLI e2e coverage

`TestEndToEndThroughTheCLI` PASS with the DSN (exit 0). It covers login,
clients, validate, export (real PDF + signed revision), verify, archive,
drafts and logout. **It does not cover any PO path**: the `vera` CLI has no
purchase-order commands in this sprint, so PO end-to-end evidence is the
authenticated API suite above, not the CLI test.

## 3. Upgrade / rollback and old signatures

Rehearsed on the disposable schema `vera_sprint_upgrade`; no production access.

1. Built `v0.14.3` (`main`) binaries from a detached worktree and pointed them
   at the empty schema. They applied migrations **1–28**.
2. Through the old CLI/server: signed in, created a client, exported a legacy
   quotation with a row discount and nested sub-items (real PDF + revision
   `00`, verification code `ac0eae0c45ce4e00`), and saved a Quotation draft.
   Old server verified the code: `valid: true`.
3. Started the sprint server on the same schema. It applied **29–38** (verified
   `max(version) = 38`; 8 `purchase_order*` tables, `deletion_*` and
   `organizations` present). The sprint server verified the old code:
   `valid: true`, and read the old quotation back (`1507260001`, one revision,
   one stored document, signature length 86).
4. Rollback: started the **previous (v0.14.3) binary against the upgraded
   schema**. It started, verified the old code (`valid: true`) and read the old
   quotation. This is the harder direction; restoring the pre-upgrade data
   state and running the previous image is strictly safer.

The score was `REHEARSAL OK`; the schema was left intact for inspection and is
dropped after this record. Migration 031's backfill of a pre-calculation PO
draft cannot exist in a pre-sprint database (PO tables are new), so that path
is covered by `TestPurchaseOrderMigration031BackfillsAPreCalculationDraft` and
`TestPurchaseOrderMigration031RefusesToFabricatePrintedTotals`, not by the
rehearsal. No Cloud Run revision/image rollback was performed; that is a
deployment action outside this verification's authority.

## 4. Assembled pages, keyboard/focus, narrow/wide

- `scripts/ui-snapshot.sh` exit 0; 48 cases rendered to 96 PNGs in
  `/tmp/opencode/ui-snap-sprint/` (local evidence, not committed).
- Read at both widths: DocumentHome, POSelector Step 1/3, PurchaseOrderEditor,
  PurchaseOrderDifferenceReview, PurchaseOrderExtractionConflicts,
  PurchaseOrderRepairReview, PurchaseOrderArchive.
- Read at 360 only: PurchaseOrderSavedSourceMissing,
  PurchaseOrderFinalConfirmation, PurchaseOrderVersionConflict,
  PurchaseOrderExtractionReview, PurchaseOrderDocumentAttached,
  DeletionDialogChallengePanel.
- Read at 1280 only: DeletionDialogChain, PurchaseOrderCorrectionEditor.
- No clipped controls, horizontal overflow, or missing status/evidence labels
  were seen; the archive table and the selector list scroll as designed.
- Modal keyboard/focus, Escape, Tab trapping, focus restore and stale-callback
  rejection are covered in jsdom
  (`POQuotationSelector.dom.test.tsx`, `PurchaseOrderEditor.dom.test.tsx`,
  `PurchaseOrderFinalize.dom.test.tsx`, `DeletionDialog.dom.test.tsx`).
- **Not browser-verified:** the snapshots are `renderToString` plus a headless
  Chrome screenshot, so effects, live progress transitions, native focus rings,
  file pickers, PDF preview and a real Wails webview were not exercised. Those
  stay hand-driven.

## 5. Extraction holdout gate — BLOCKED

`docs/po-extraction.md` and `docs/po-extraction-benchmark.md` state it
explicitly: the accuracy thresholds in #476 §2.5 have **not been measured**.
The sprint handoff records only three authorised samples, with no ground truth;
#476 records that no authorised holdout or threshold signoff exists. Reporting
extraction as validated would be inventing a number. The licence review for
Poppler/Tesseract as separate server-side subprocesses is approved
(2026-10-21); the *measurement* gate is not.

**Exact inputs required from the user (or an explicit written acceptance of
assisted extraction without accuracy claims):**

1. Authorised customer PDFs, stratified per #476 §2.1: at least 3 customer
   layouts, 30 POs, 5 scanned/mixed documents — or a written statement of what
   was actually obtained with thresholds re-scoped accordingly.
2. Human-corrected ground truth per PO, recorded independently of any engine
   output: header fields, every line (description, quantity to 4 decimals, UoM,
   unit price), printed totals, and source page/region.
3. A tuning / frozen-holdout split by layout, with the holdout unseen until the
   acceptance run.
4. Signoff on the §2.5 bars before measuring: critical-field accuracy ≥ 95 %,
   complete-row accuracy ≥ 90 %, zero silent monetary/quantity errors, zero
   silent missing/duplicate rows, correction time below the manual baseline,
   p95 latency inside the async UX budget, and zero crashes/hangs on hostile,
   oversized, encrypted or invalid input.
5. The §2.4 measurements themselves (accuracy, abstentions, latency p50/p95,
   peak RAM, failures, cost) on that holdout.

Until then: #476 stays open, #482 stays open, #464 is not fully verified, and
PR #484 stays a draft. This is a documented blocker, not an assumption.

## 6. Repository gates on the quiescent tree

Tree was clean at `8809adb`; no worker was running.

| Command | Exit | Result |
| --- | --- | --- |
| `gofmt -l .` | 0 | no output |
| `go vet ./...` | 0 | clean |
| `VERA_POSTGRES_TEST_DSN=… go test -race -count=1 ./...` | 0 | 16 packages `ok`; `backend/api` 260.6 s, `backend/store` 166.0 s, `cmd/vera` 5.8 s |
| `npm --prefix frontend run build` | 0 | 88 modules, 711.82 kB JS / 41.80 kB CSS |
| `npm --prefix frontend test` | 0 | 62 files passed, 1 skipped; 828 tests passed, 48 skipped (the 48 are the opt-in UI snapshot harness) |
| `go test ./cmd/vera -run '^TestEndToEndThroughTheCLI$'` (DSN) | 0 | PASS |
| Targeted store PO suite (`-race`, DSN) | 0 | 132 PASS |
| Targeted API PO suite (`-race`, DSN) | 0 | 74 PASS, 1 Docker test skipped (run separately) |
| Targeted frontend PO suite (30 files) | 0 | 390 passed |
| `VERA_EXTRACTOR_DOCKER=1 go test ./backend/extract -run Docker` | 0 | 4 PASS |
| `VERA_EXTRACTOR_DOCKER=1 … go test ./backend/api -run '^TestDockerExtractionEndToEnd$'` | 0 | PASS |
| `scripts/ui-snapshot.sh` | 0 | 48 cases, 96 PNGs |
| Upgrade/rollback rehearsal | 0 | `REHEARSAL OK` |

Signature regression tests also pass by name: 20 canonical/signing tests in
`backend` and 15 `TestVerify*` tests in `backend/api` with the DSN.

**Judge before merge and orchestrator revalidation are process roles, not
commands, and have not happened in this session.** This record is the evidence
they must consume; no merge or release was performed.

## 7. Defects found

### D1 — stale extraction copy says scanned pages are not read

`frontend/src/purchaseOrderDocument.ts:229` (and the empty-state hint at
`:216`) still says:

> Reads the selectable text layer only; scanned pages are not read.

That is #477 wording. #478 shipped per-page OCR, and the review panel on the
same screen reports "1 of 2 pages had no usable text layer and were read with
OCR (tesseract 5.5.0)". The stale hint is visible in the freshly rendered
`PurchaseOrderDocumentAttached-360.png`; the pure test
(`purchaseOrderDocument.test.ts:100`) only asserts the hint contains
`"text layer"`, so nothing catches the contradiction. The component's header
comment at `frontend/src/components/PurchaseOrderExtractionPanel.tsx:44`
carries the same stale claim ("A scanned page is reported as an abstention,
never OCRed here").

No product code was changed here. Recommended fix wave: update both hints and
the comment, and strengthen `purchaseOrderDocument.test.ts` so the wording
cannot regress to "scanned pages are not read" after OCR exists.

The `tdd` lens found the reason the existing test misses it:
`purchaseOrderDocument.test.ts:100` asserts only that the hint contains
`"text layer"`, which the stale sentence still satisfies. A correct regression
seam exists and is cheap — `extractAndCompareAvailability` is a pure function —
so the fix should assert the shipped scan behaviour in the wording (and the
absence of the "not read" claim) before changing the string. The
`diagnosing-bugs` lens confirmed the same boundary: the minimised repro is the
rendered `DocumentAttached` panel, where the hint and the OCR evidence line
contradict each other on one screen.

No functional failure was found: every gate above passed, and the OCR pipeline
behaves as documented in the Docker and API tests.

## 8. Remaining unverified / hand-driven

- **Extraction accuracy, latency, memory, cost and hostile-PDF behaviour on real
  documents** — blocked, see §5. Stock hostile-input handling is covered by
  synthetic tests only.
- **Real desktop Wails interactions** — clicking either entry path, typing,
  native focus/keyboard in the webview, file picker, PDF preview, window
  resize. jsdom substitutes for focus/keyboard logic; it is not a browser.
- **Live browser rendering** — snapshots are static SSR HTML screenshots; no
  effect, transition, animation or live progress state was observed.
- **Production deployment/rollback** — only a local schema + binary rehearsal.
  No Cloud Run revision was deployed or rolled back and no production data was
  read.
- **Legacy PO rows** — none can pre-exist this sprint, so 031's backfill is
  proven by tests, not by production-shaped data.
- **`cmd/vera` PO path** — the CLI has no PO commands; PO end-to-end evidence
  is the API suite.
- **Judge verdict and orchestrator revalidation** — pending; PR #484 is still a
  draft and remains unmodified.
- Administrative state observed, not altered: #465–#482 are open until the
  integration PR merges; #483's fix is present and its regression tests pass,
  but the issue itself is still open.

## 9. Process record

- Skills loaded and what each changed:
  - `code-review` — the Spec axis compared the shipped slices against #464 and
    #482 (this table is the result); the Standards axis checked the repo's
    AGENTS.md rules. The skill's parallel-subagent dispatch was not available
    to this verification worker, so both axes were run directly rather than
    delegated.
  - `tdd` — used to check whether each claimed guard can actually fail. It
    found D1's guard is too weak: the test asserts a substring the stale copy
    still contains.
  - `diagnosing-bugs` — used for D1: the repro is the assembled panel where
    the stale hint and the OCR evidence line disagree; the correct regression
    seam is the pure `extractAndCompareAvailability` function. No fix was
    applied, per the brief.
  - Deliberately not loaded: `research` (the primary documents are in-repo),
    `prototype` (no design fork), and the design/copy skills (no product
    change).
- Every claim above names its command or file; the piped-exit-code trap was
  avoided by writing outputs to files and reading the shell's own exit.
- No code, AGENTS.md, pricing, signing, release or PR was changed. The only
  committed artifact is this document.
