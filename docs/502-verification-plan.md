# #502 — PO reconciliation workbench: independent verification plan

**Status:** plan only. No suite was run to produce this document; the only commands
executed were read-only inspection commands.
**Author:** Frank (QA/verification). **Date:** 2026-10-28.
**Worktree:** `/var/lib/openbot/OpenBot/Shared/vera-502`.
**Baseline:** detached HEAD `dca88904edd034875f33a071e9e669a31d9e39e4`
(`feat/464-po-sprint`).
**Scope:** the #502 spec, tickets 01–07, Kaluna's frozen contract
(`docs/po-reconciliation-contract.md`), and Jessy's UX brief
(`docs/502-ux-brief.md`).

This plan says how the verdict will be produced. It is not a verdict, and nothing
in it may be quoted as a PASS.

## 0. Skills loaded, and deliberately not loaded

Loaded:

- **tdd** — red before green, test at an agreed seam, no implementation-coupled
  or tautological assertions. Every new guard in this plan must be seen failing
  before it is accepted.
- **code-review** — two axes (Standards and Spec), used on the final diff so a
  green suite cannot hide a spec miss. This plan carries the Spec traceability
  now; the Standards axis runs on the diff at the gate.
- **diagnosing-bugs** — red loop, minimise, prove the red, keep the regression
  test. Used for every defect found during verification, not only at the end.

Deliberately not loaded, with the reason:

- **pr** — QA opens no pull request and writes no PR body.
- **grilling**, **codebase-design**, **domain-modeling** — the spec, Q13–Q16
  answers and contract 01 already freeze the design and vocabulary. Re-opening
  them here would be a second design authority.
- **prototype**, **research** — no design fork and no fact that lives outside
  the repo.
- **shape**, **impeccable**, **adapt**, **animate**, **critique**, **polish** —
  design and polish skills belong to Jessy's design review. QA verifies
  behaviour and evidence, not taste.
- **wizard** — no credential or dashboard step is part of this plan. The
  Postgres DSN is an environment blocker, not a human onboarding flow.
- **writing-for-agents** — this is a verification record, not an `AGENTS.md`,
  skill or brief edit.
- **full-output-enforcement** — not a code-generation task.

If a skill's guidance conflicts with `AGENTS.md`, `AGENTS.md` wins and the
conflict is recorded in the report.

## 1. Target, staleness and the first valid verdict

At plan time:

- `git rev-parse HEAD` = `dca88904edd034875f33a071e9e669a31d9e39e4`.
- The tree is **not clean**. It carries ticket 01, ticket 02 and UX-brief work
  that is not committed (`GLOSSARY.md`, `frontend/src/App.tsx`,
  `frontend/src/components/AppShell.tsx`,
  `frontend/src/components/PurchaseOrderEditor.tsx`,
  `frontend/src/components/PurchaseOrderWorkspace.tsx`, `.scratch/`,
  `backend/store/po_reconciliation_contract_test.go`,
  `backend/testdata/po_reconciliation_contract.json`, `docs/502-ux-brief.md`,
  `docs/po-reconciliation-contract.md`, `frontend/src/pageHistory.*`,
  `frontend/src/poReconciliation.contract.test.ts`, `frontend/node_modules`).
- A result on this tree is a **pre-check**, not a verdict.

A verdict is valid only when all of these are recorded:

1. `git rev-parse HEAD` — the full 40-character SHA.
2. `git status --porcelain` — empty for a PASS. If not empty, the run is a
   pre-check and its tree fingerprint is `sha256(git status --porcelain)`.
3. The command, its **direct** exit code, and the decisive output lines.
4. The Postgres DSN used, identified as throwaway and dropped afterwards (never
   the password).
5. The snapshot directory and the PNG names actually read.
6. The hand-driven record where the criterion is hand-driven.

**Staleness rule.** A PASS on a different SHA is stale. A PASS on the same SHA
with a different tree is stale. A green CI run on a prior SHA is stale. A green
run whose integration tests skipped is not a PASS. After a fix, the whole
affected set is re-run on the new SHA; an old PASS is never re-used.

## 2. Seams

The seams are listed from highest (most product-visible, most integration
covered) to lowest. A behaviour is verified at the highest seam that can observe
it; it is not "proved" by a lower seam alone when a higher one exists.

| # | Seam | Existing interface | Catches | Misses |
| --- | --- | --- | --- | --- |
| S1 | HTTP API over a real store and real Postgres | `backend/api/server.go` routes for `/v1/purchase-orders/:id/review`, `/finalize`, `PUT /:id`, `/source-repair`, `/source-link`; `doJSON` tests | Auth, org scoping, HTTP status and typed error code, request binding, protected fields, optimistic-version conflict, response JSON keys | React, CSS, the Wails WebView bridge |
| S2 | Store public methods | `Store.CreatePurchaseOrderDraft`, `UpdatePurchaseOrderDraft`, `GetPurchaseOrderReview`, `FinalizePurchaseOrder`, `RepairPurchaseOrderSource`, plus the new session read/write | Transaction boundaries, DB constraints and triggers, immutable history, line-identity mapping, rollback on refusal | HTTP mapping, UI |
| S3 | Wails DTO boundary | `backend.Quotation` methods, `backend/apicontract` rows, `frontend/wailsjs/go/**`, `bindings_test.go` | Desktop method presence and generated-binding drift; `apicontract` tests cover JSON field names and nullability | Runtime bridge behaviour; React |
| S4 | Pure comparison and state machine | Go `ComparePurchaseOrder` / `ValidatePurchaseOrderDecisions`; TS `comparePurchaseOrder` / `evaluateReview`; the new commit handler; the shared fixture | Field parity, match/differ rules, notices vs items, resolutions, stable keys, totals derivation | Persistence, transport, rendering |
| S5 | React component in jsdom | The real editor, pane, sheet and list, with only `@backend/backend/Quotation` and `@backend/models` mocked at the module boundary | Blur/click/keyboard behaviour, notice content, keep/use, filters, count, focus, Escape, no API call when blocked | CSS layout, real browser, actual motion, screen reader |
| S6 | SSR snapshot harness | `frontend/src/ui-snapshot.harness.test.tsx` → HTML → PNG via `scripts/ui-snapshot.sh` | Static layout at 360 and 1280: pane/rail presence, widths, wrapping, no horizontal scroll, action-bar position | Any interaction, motion, 1024, real scrolling |
| S7 | Hand-driven desktop/browser | Real Wails build or Chrome at the stated viewport | Motion, focus order/restore, live regions with a real screen reader, sticky behaviour, 1024, 200% zoom, paste, app close | Repeatability; the record must be explicit |

**What I will not test at:** private helpers, reducer internals, CSS class names,
or component state that is not reachable through a user action. SQL is used
directly only to prove a **schema guard** (unique active session, immutable
saved session), and the same behaviour is also exercised through S1/S2; a raw SQL
check never substitutes for the store/API behaviour.

**Highest-seam choices for #502:**

- Any behaviour that writes a row, returns an error code, or gates Final Save:
  S1, with S2 for the invariant and S3 for the desktop contract.
- Any behaviour that is only a comparison rule: S4, plus S1 if it affects the
  save gate.
- Any behaviour a user sees: S5; layout only: S6; motion/focus/screen reader:
  S7.
- Ticket 01's contract is S4 only by design; it must not be described as proof
  of persistence or UI.

## 3. Fixtures

The verification fixtures are built through the **public store/API seams**, not
inserted by hand into the tables (the one exception is the SQL red proof for a
schema guard). Exact money values are pinned in the fixture file and copied into
the run log; the values below are the properties each fixture must have, not a
claim about the seeded numbers.

| Fixture | What it establishes |
| --- | --- |
| `fx502/base` | One issued Quotation revision `00` with at least four customer-facing lines, including one priced sub-item (`2.1`), a non-FTZ goods line (PPN applies) and a service line (tax basis is non-trivial). Payment terms, Incoterms and whole-quotation discount are set. |
| `fx502/diff` | A linked draft that deliberately reaches: differing payment terms, differing Incoterms, differing whole-PO discount, two differing line fields, one missing source line, one extra PO line (no reference), and one duplicate match (two PO lines claim one source reference). |
| `fx502/reorder` | The same lines with the display order changed after items exist; used to prove keys and decisions do not move. |
| `fx502/remove` | One matched PO line removed; used to prove `line_removed`, retained history, and the new missing-source item. |
| `fx502/totals-input` | Totals differ because a named input differs; the reason must name that input key. |
| `fx502/totals-unexplained` | Totals differ while every comparable input matches (the printed total carries something the PO does not); used to prove no invented reason and no forced equality. |
| `fx502/discount-edge` | One line with discount disabled at 0%, one with discount enabled at 0%; used to pin the contract's `None` vs `0%` distinction. |
| `fx502/saved` | A finalized record with a resolved session; used for the read-only archive and for a refused post-save write. |
| `fx502/repair` | Revision `01` of the same quotation with a changed line; used for source repair, the new session and the retained old one. |
| `fx502/roles` | An administrator and a non-admin in the same organisation; a second organisation for scoping. |
| `fx502/empty` | A never-linked draft (no source) for the pane's empty state and Final Save refusal. |

**Source of truth for expected values:** the shared fixture
`backend/testdata/po_reconciliation_contract.json` for comparison rules, and
worked examples in the ticket / contract for transitions. The fixture is not
edited to match the implementation; if a test needs a new case, the contract and
both readers change together.

## 4. Per-ticket test matrix

Levels: **pure** (S4), **store** (S2), **API** (S1), **component** (S5),
**snapshot** (S6), **hand** (S7). "Proof" names the test/command I expect; the
actual test name is recorded at run time and a run that matches no test is not
evidence.

### Ticket 01 — reconciliation contract and parity fixtures

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 01.1 | Every comparable field is named: payment terms, Incoterms, whole-PO discount, description, UoM, quantity, unit price, line discount, line match, tax basis (link-time), totals (derived) | pure | `TestPOReconciliationContractCatalogueAndWireKeys`; `poReconciliation.contract.test.ts` "pins every field and every wire key" | `--- PASS` lines; the 12 `Fields` entries; any field present in one language only |
| 01.2 | Exclusions are explicit: billing address, PO-only fields and printed totals, each with a reason | pure + review | Same tests; contract §2; grep the comparable set for `billingAddress` | Contract citations; assertion count; confirmation that `billingAddress` is absent from the comparable set and present only as PO-only |
| 01.3 | The state machine is written as states and transitions: empty, matched, differing, resolved-kept, resolved-used, acknowledged-missing, acknowledged-extra, match-corrected, line-removed, values-now-match | pure | `TestPOReconciliationFixtureCoversThePlannedStateMachine`; TS transition test | 27 transition examples and their PASS; the states actually asserted |
| 01.4 | Item identity is stable across edits, reordering and removal; a resolved item is never deleted or re-keyed | pure | `IdentityCases` reader; contract §3 | The three identity examples; the key forms (`id:`, `client:`) and the no-re-key assertion |
| 01.5 | The session/item JSON shape is frozen: exact PascalCase keys, nullability, nested specimens | pure | Go wire-key test; TS "pins every field and every wire key" | Shape names and key lists; null/array specimens; any `omitempty` alias found |
| 01.6 | One shared JSON file is read by both languages | pure | Both readers against `backend/testdata/po_reconciliation_contract.json` | File SHA-256; both PASS lines |
| 01.7 | A field or key added on one side without the other fails | pure | Deliberate one-sided mutations (Go-only Item key, TS-only comparable field, TS-only Item property) | The mutation diff, exit 1, the failing assertion, the restore, the green rerun |
| 01.8 | No production behaviour changed | review | `git diff --stat dca8890`; the touched-file list | Only docs, testdata and test files; no production Go/TS, no migration, no generated binding, no workflow |

**Verdict for ticket 01 is not a product verdict.** It proves the contract and the
fixture guard; contract §9 says the transition/gate examples are shape checks,
not an implemented state machine.

### Ticket 02 — back stack, workspace state and shell action bar

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 02.1 | Home → PO workspace → PO settings; Back goes settings → the same workspace → Home | component/hand | `App` DOM test driving the real shell; hand-driven Wails pass | The click sequence; the page reached after each Back; the exact workspace values on return |
| 02.2 | The same rule for the Quotation workspace | component/hand | Quotation workspace DOM test | Same record; confirm the rule is not PO-only |
| 02.3 | A dirty PO editor survives a settings round-trip: no discard prompt, no value lost | component/hand | DOM test: edit a line, open settings, Back, assert every edited value | The values before/after; the absence of the discard dialog; the hand-driven result |
| 02.4 | Drafts/Archive filters, page, scroll and active tab survive the round-trip | component/hand | List DOM test for filters/page/tab; scroll is hand-driven | Filter values, page number, active tab; `scrollTop` before/after in the hand record |
| 02.5 | The review/confirm step survives the round-trip | component/hand | DOM test: open review/confirm, settings, Back, assert the same step | The step name and the same decision state |
| 02.6 | Back never jumps straight to Home from a settings page | component/hand | DOM test at settings depth 2 | The page after Back is the workspace, not Home |
| 02.7 | Save draft and Final Save are in the shell action bar and reachable while the form scrolls | component/snapshot/hand | DOM role/name assertions; snapshots; hand scroll at 1280 and 360 | The action-bar buttons; their position at the top and bottom of the form; the hand record |
| 02.8 | Unsaved state is not retained after sign-out or app close | component/hand | DOM sign-out test; app close hand-driven | The stack/working copy after sign-out; the app-close record (or "not driven") |
| 02.9 | Tests cover each scenario; no new global store if the existing shell state can hold the stack | review | The test list + `git diff` for a new store module | Test names; the diff's state ownership; if a new store exists, the justification |

**Caveat:** jsdom has no layout. Scroll restoration is hand-driven only; a unit
test that asserts a stored `scrollTop` number is implementation-coupled and is
not accepted as proof (§11, HR-10).

### Ticket 03 — PO-native docket list

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 03.1 | Drafts and Archive stay separate tabs with the existing server paging | component/API | List DOM test; existing list API test; API response `page`/`total` | Tab state; page size 15; API request URL and body |
| 03.2 | A row reads as a docket: customer PO, source Quotation + revision, customer, received date, state, Source missing badge | component/snapshot | DOM row assertions; snapshots at 1280 and 360 | Each field's rendered text; the badge for a source-missing record |
| 03.3 | Exactly one Open action per row; it opens the draft or the saved record as appropriate | component | DOM test with a draft row and a saved row | One accessible Open control; the destination phase for each row |
| 03.4 | Admin Delete is in a separate menu, not beside Open | component/snapshot | DOM test as admin and non-admin; snapshot | Menu trigger name; Delete absent for non-admin; separate from Open |
| 03.5 | At 360 rows stack as cards; no `min-w-[900px]` table remains | snapshot | 360 PNGs of Drafts and Archive | The PNG path; card layout; absence of horizontal scroll; grep for the old class |
| 03.6 | No amount column; the list query is unchanged | API/review | Response key set; `git diff dca8890 -- backend/store/po_list.go` (or the list query file) | The response keys; the diff is empty; no money field in the query |
| 03.7 | Empty, loading and error states remain | component/snapshot | DOM tests and 360 snapshots | The state reached; the copy and action; the retry path |
| 03.8 | Snapshot cases at 1280 and 360 cover Drafts and Archive | snapshot | `PurchaseOrderDocket*` cases | Four PNGs read (Drafts/Archive × 1280/360), each described |

### Ticket 04 — live header reconciliation end to end

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 04.1 | Migration `041` adds org-scoped session/item tables, one active session per PO and source revision, immutability for saved records; `038` and `040` untouched | store | `TestMigration041...`; direct SQL guard proof | Table/index/trigger names; the unique violation on a second active session; the trigger refusing a saved-session update; `038`/`040` bytes unchanged |
| 04.2 | Creating a draft from a Quotation, or linking one, creates the session | store/API | Store test + API test for both entry paths | Session row count and `SourceRevision` for create-from-Quotation and for link |
| 04.3 | Saving a draft persists the session; reopening returns the same session and items | store/API | Round-trip test through `UpdatePurchaseOrderDraft` and `GetPurchaseOrderReview` | Session ID, item keys, resolutions and history before/after; no duplicate session |
| 04.4 | The Wails/API surface reads and writes the session; JSON matches contract 01 | API/S3 | API test + `apicontract` mapping test + `bindings_test.go` | Response JSON keys; generated binding method present; `--- PASS` for both directions |
| 04.5 | A stale session write conflicts on the PO version instead of overwriting | store/API | Two writers with the same `ExpectedVersion`; the loser gets the typed conflict | Both responses; the stored version; the session unchanged for the loser |
| 04.6 | At ≥1024 the pane is the right rail; below 1024 it is a full-height sheet; not a modal at either width | component/snapshot/hand | DOM matchMedia/width test; 1280 and 360 snapshots; 1024 hand-driven | Rail/card layout; the sheet's role (not `aria-modal`); the 1024 record |
| 04.7 | The pane has an empty state, All/Unresolved/Resolved filters, unresolved newest first, resolved retained, sticky count | component/snapshot | Pane DOM tests; 1280 snapshot | Each filter's count; the order of items; the count text; the resolved row retained |
| 04.8 | Committing an empty **required** header field shows a notice and creates no item | component | DOM test on the field that is actually required by the existing validation | The notice text; item count unchanged; the blocker named; see HR-01 if no header is required |
| 04.9 | Committing a matching header field shows a match notice and creates no item | component | DOM test | The notice text; item count unchanged; no persisted row |
| 04.10 | Committing a differing header field creates one unresolved item with both values | component/store | DOM test + store round-trip | One row; `POValue`/`SourceValue`; status `unresolved`; key stable |
| 04.11 | Keep PO asks "Is this correct?" and resolves on confirm; Use Quotation updates the field immediately and resolves | component | DOM test: click Keep, assert the question, Cancel then Confirm; click Use and assert the input value | The question and its focus; the unresolved state after Cancel; the resolved `kept_po`/`used_quotation`; the changed input value |
| 04.12 | Resolving shows no matched notice; re-committing the same field updates the same item | component | DOM test: resolve, blur again, assert no notice and one row | The notice count; the item key/ID; the history length |
| 04.13 | Final Save is blocked while any header item is unresolved; the count and blocker are visible | component/API | DOM test spying on the finalize binding; API refusal | The spy was not called; the first blocker focused; the server's typed refusal |
| 04.14 | Tests cover the commit handler, the pane, save/reopen and the Final Save gate; Postgres covers the schema | mixed | The test list; the Postgres run | Every named test's PASS; no skip in the Postgres run |

### Ticket 05 — lines, tax basis and totals

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 05.1 | Each matched line's description, UoM, quantity, unit price and line discount compares on commit with the same notice/item behaviour as the header | pure/component | Table-driven pure cases per field + one DOM test per field class | Field keys; expected notice/item; the DOM-visible row |
| 05.2 | Stable line identity: reorder does not re-key; removing a line resolves its items as `line_removed` and retains them; no other line inherits them | pure/store | Reorder and remove tests through the store | Item keys before/after; the `Removed` identity; the history; the new missing item |
| 05.3 | A line with no reference, or an unknown reference, is extra and must be acknowledged or corrected | pure/API | Comparison test; finalize refusal for an unacknowledged extra; the unknown-reference case | The `extra_line` key; acknowledgement; the saved-reference validator's answer (see HR-02) |
| 05.4 | A source line no PO line claims is missing and must be acknowledged or corrected | pure/API | Comparison test; finalize refusal | The `missing_line` key; the acknowledgement; the refusal naming the line |
| 05.5 | Two PO lines claiming one source line are a blocking duplicate until one match is corrected | pure/API | Duplicate test; acknowledgement does not clear; correcting one match clears | The duplicate item; the blocking status after acknowledgement; the resolved state after correction |
| 05.6 | Tax basis (zone, type, service category) is compared at link, recorded as a resolved link-time item, then read-only; no inline "use Quotation" action | store/API/component | Link test; post-link session read; DOM assertion; API refusal of a post-link basis write | The `link:taxBasis` item and triple; no Keep/Use controls; the count excludes it; the refused write |
| 05.7 | Totals are derived: matched/different + the input reason; never a decision item; never force the printed total | pure/component | Totals tests for input-difference and unexplained cases; DOM assertion | The PO's own grand total; the reason key; no item; no blocker (see HR-06) |
| 05.8 | The frontend source DTO exposes the tax basis; billing address is not added | API/S3 | DTO key test; `git diff` for the source DTO | `TaxZone`, `TaxType`, `ServiceCategory` present; `BillingAddress` not in the comparable set |
| 05.9 | Final Save refuses unresolved items, unacknowledged missing/extra and duplicates, naming the blockers | API/store | Three refusals + one acceptance | Typed code; blocker text; no state change; the version unchanged |
| 05.10 | Saved decisions remain readable; no resolved item is deleted | store/API/component | Save, reopen, read the session; remove a line and read history | Items, resolutions, actor, timestamp; history length |
| 05.11 | Tests cover every field, every line outcome, the link-time tax choice, the totals reason and the gate | mixed | The test list | A field/outcome coverage table with no blanks |

### Ticket 06 — archive read-only and source-repair history

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 06.1 | Opening a saved PO shows the session read-only: items, values, resolutions, actor, timestamp | API/component/snapshot | Saved-record API read; DOM/snapshot | The session JSON; the read-only rail; actor and UTC timestamp |
| 06.2 | A saved record cannot write to its session; the API refuses and the UI offers no edit affordance | store/API/component | Store refusal + API 4xx/typed code + DOM absence | The refusal body; the unchanged row; no button/control |
| 06.3 | Source repair creates a new session against the new revision and seeds a full comparison | store/API | Repair test | New session ID/revision; the item set; the old session still present |
| 06.4 | The old session is retained as history and reachable from the record | store/API/component | Read both sessions; DOM history disclosure | Old session ID/read-only; the disclosure's content |
| 06.5 | Repair records old/new source and repeats the difference review; the admin-only rule is unchanged | store/API | Repair test as admin and non-admin; old/new fields | The recorded link; the decisions; the 403/typed refusal for non-admin |
| 06.6 | An abandoned repair leaves the old session active and the record unchanged | store/API | Preview repair, then abandon; read the record and session | Same active session; same record version; no new session |
| 06.7 | Corrections and version history are unaffected by the reconciliation session | store/API | Correction test plus history read (see HR-04 for the open lifecycle) | Correction snapshot; version history; session unchanged |
| 06.8 | Tests cover the read-only read, the new-session repair, the old-session history and the abandoned repair | mixed | The test list | Every named test's PASS |

### Ticket 07 — motion, accessibility and polish

| ID | Behaviour | Level | Proof | Evidence I record |
| --- | --- | --- | --- | --- |
| 07.1 | Page/step transitions are enter-only: fade + 4–6 px rise, 180–220 ms, ease-out-quart | hand | Browser computed styles / recording at 1280 and 360 | The measured duration, easing and offset; the absence of an exit animation |
| 07.2 | The Final Save handoff has a calm progress state and a clear arrival | component/hand | DOM state test + hand-driven run | The progress state; the arrival; focus on the saved h1; one announcement |
| 07.3 | The rail fades/slides at 200 ms at ≥1024; the sheet enters and leaves cleanly below 1024 | hand | Browser recording at 1024/360 | Timing and direction; the close behaviour; see HR-08 for "leaves cleanly" |
| 07.4 | List rows have hover/press feedback only; no stagger | hand/snapshot | Pointer hover/press at 1280 and 360 | The observed feedback; no staggered entry |
| 07.5 | No bounce, confetti or perpetual animation | review/hand | Grep for animation libraries/loops; hand run | The absence; no repeated keyframes |
| 07.6 | Reduced motion removes transforms and keeps opacity/colour | hand | Browser with `prefers-reduced-motion: reduce` | The computed transform (none); opacity/colour still change |
| 07.7 | Keyboard order follows form → pane → sheet; focus is visible; Escape closes the sheet | component/hand | DOM focus tests + hand keyboard pass | Tab order; focus ring; Escape close and focus return; `inert` on the form |
| 07.8 | Notices are announced politely; blocking errors assertively | component/hand | DOM `role`/`aria-live` assertions + real screen-reader pass | The live-region attributes; one text change; the spoken result (see HR-09) |
| 07.9 | Snapshots at 1280 and 360 cover the editor, pane, sheet, list and archive | snapshot | The snapshot cases in §7 | Every PNG read and described; missing case named as missing |
| 07.10 | A hand-driven keyboard/focus/paste pass is recorded, with what was driven and what was not | hand | The hand record | Browser, viewport, keys/pointer, state reached, what was not driven |
| 07.11 | The design review names every state checked and every state it could not | review | Jessy's record cross-checked against the state inventory | The checked/unchecked list; no "looks fine" |

## 5. Red-loop cases

Every case below is written before the fix and **seen red**. The red proof is a
deliberate mutation of the production behaviour (or the absent behaviour), not
just "the test compiles". The mutation is reverted and the test re-run green.
Evidence for each: test name, first-run exit 1, the failing assertion line, the
mutation diff, the restore, the green exit 0.

### 5.1 Commit handler

| ID | Precondition | Action | Expected | Red proof (must fail before the fix) |
| --- | --- | --- | --- | --- |
| C-1 | A required line quantity is empty (ticket 05); for a header, see HR-01 | Commit the quantity field | Notice `Quantity is empty`; no item; readiness blocked | Classify empty as an item, or skip the notice → C-1 fails |
| C-2 | Linked draft, field equals source after trim | Commit | Notice `Matches the Quotation`; no item; count unchanged | Persist a `matched` item, or skip the notice → C-2 fails |
| C-3 | Field differs | Commit | One unresolved item with both values; count +1 | Create two items, or create none → C-3 fails |
| C-4 | Field differs | Type, do **not** blur | No notice, no item, count unchanged | Fire the handler on `change` per keystroke → C-4 fails |
| C-5 | An item exists for the field | Blur twice with the same value | Same item key; one row; no duplicate history | Append a new item on each commit → C-5 fails |
| C-6 | Text differs only by surrounding whitespace; another differs by internal whitespace | Commit both | Surrounding whitespace matches; internal whitespace differs | Remove `trim`, or collapse internal whitespace → C-6 fails |
| C-7 | Non-empty invalid number | Commit | Validation error naming the field; no item; no zero coercion; Final Save blocked | Turn the parse failure into `0`, or create an item → C-7 fails |

Observed at S5 for the user action, with the pure classification at S4. A DOM
test that calls the handler directly instead of blurring the field is not
accepted.

### 5.2 Resolution

| ID | Precondition | Action | Expected | Red proof |
| --- | --- | --- | --- | --- |
| R-1 | Unresolved differing item | Click Keep PO once | `Is this correct?` is shown; item remains unresolved; count unchanged | Resolve on the first click → R-1 fails |
| R-2 | The question is open | Confirm | `resolved / kept_po`; actor and UTC time; count −1; focus moves on | Leave it unresolved, or lose the actor → R-2 fails |
| R-3 | The question is open | Cancel or Escape | Question closes; item unresolved; focus returns to Keep PO | Treat Cancel as confirm → R-3 fails |
| R-4 | Unresolved differing item | Use Quotation | The input becomes the source value; `resolved / used_quotation`; no matched notice | Resolve without updating the input, or also show a notice → R-4 fails |
| R-5 | Resolved item | Edit the field back to the source value and commit | Same item, `resolved / values_now_match`; no notice; no new item | Delete the item, or create a notice → R-5 fails |
| R-6 | Resolved item | Edit to a new differing value and commit | Same item returns to `unresolved`; earlier decision retained in history; count +1 | Create a second row, or discard the history → R-6 fails |
| R-7 | A field with any item | Commit a matching value | No matched notice; the item remains the only row | Add a matched notice beside the item → R-7 fails |

### 5.3 Line matching

| ID | Precondition | Action | Expected | Red proof |
| --- | --- | --- | --- | --- |
| L-1 | Items and decisions exist for lines 1, 2, 3 | Reorder the lines in the editor and save | Item keys and decisions unchanged; only display numbers move | Key by array position (`line:{index}`) → L-1 fails |
| L-2 | A matched line has resolved items | Remove the line and save | Its items become `resolved / line_removed`, retained with history; no other line inherits them; the source line becomes missing and needs acknowledgement | Cascade-delete the items, or re-key them to the next line → L-2 fails |
| L-3 | PO line has no reference | Save | `extra_line` item; carries the line key; cannot finalize until acknowledged/corrected | Drop the line from the comparison → L-3 fails |
| L-4 | PO line has an unknown reference | Save | Extra item; the saved-reference validator still refuses a non-null unknown reference (see HR-02) | Promise that acknowledgement alone can finalize it → L-4 fails the API case |
| L-5 | Source line `2.1` is unclaimed | Save | `missing_line` item with the source reference; blocks until acknowledged/corrected | Ignore unclaimed source lines → L-5 fails |
| L-6 | Two PO lines claim reference `2` | Save | `duplicate_match`, blocking; acknowledgement does not clear it; correcting one match clears it | Treat the second as extra, or let acknowledgement clear it → L-6 fails |
| L-7 | A duplicate pair, one line removed | Save | The surviving line's state is defined by the frozen rule; the removed line's item is history | If the rule is undefined, stop and record HR-16 rather than invent one |

L-1 is the highest-value red proof: the current implementation keys by line
number (`po_review.go` and `poFinalize.ts`). The plan will not accept a test that
passes only because the fixture never reorders.

### 5.4 Tax basis at link

| ID | Precondition | Action | Expected | Red proof |
| --- | --- | --- | --- | --- |
| T-1 | A draft is being linked to revision `00` | Complete the link review with a tax choice | One `link:taxBasis` item with the chosen triple; `resolved`; actor/time | Omit the item, or store only the PPN rate → T-1 fails |
| T-2 | The link is committed | Open the pane | Tax row is read-only context; no Keep/Use; not in the unresolved count; present in Resolved | Expose a Keep/Use action, or count it unresolved → T-2 fails |
| T-3 | The basis is linked | Attempt a session write that changes `TaxZone`, `TaxType` or `ServiceCategory` | Store/API refuses; the persisted basis is unchanged | Accept the write → T-3 fails |
| T-4 | A saved record is source-repaired to revision `01` with a different basis | Complete the repair | A new `link:taxBasis` item for the new revision; the old one retained | Mutate the old item in place → T-4 fails |
| T-5 | A linked record | Try to change the basis inline | No affordance; no API route that treats it as a field-level "use" | Provide an inline action → T-5 fails |

T-5 is the only testable part of "changing it later is a correction or a source
repair"; the correction half is not implemented or defined (HR-03).

### 5.5 Derived totals

| ID | Precondition | Action | Expected | Red proof |
| --- | --- | --- | --- | --- |
| D-1 | Every input matches the source | Open the totals check | `Matches` (or the brief's exact wording); no item; count unchanged | Create a totals item → D-1 fails |
| D-2 | A quantity differs, so the calculated total differs | Open the totals check | `Different` + the reason names the quantity input key; no item; no blocker | Hard-code a reason, or omit the key → D-2 fails |
| D-3 | Every comparable input matches but the printed total differs | Open the totals check | `Different` + an explicit "no input difference explains this" (or the agreed wording); no invented reason; no item | Invent a reason, or force the printed total → D-3 fails |
| D-4 | A printed total differs | Finalize with all review decisions | The PO keeps its own calculated grand total; the printed total is evidence only | Overwrite the PO total with the printed total → D-4 fails |
| D-5 | A totals check was shown | Change an input and commit | The derived figures recompute; the old reason is not shown | Keep the stale figure → D-5 fails |
| D-6 | Unresolved items exist | Finalize | The totals check contributes no blocker and no item; the item blockers still gate | Count totals as unresolved → D-6 fails |

D-3 is the case the spec does not define (HR-06); until it is defined, D-3 is
blocked, not waived.

### 5.6 Final Save gate

| ID | Precondition | Action | Expected | Red proof |
| --- | --- | --- | --- | --- |
| F-1 | One unresolved item | Finalize | Typed refusal (4xx) naming the item; no row change; version unchanged; session still draft | Remove the unresolved check → F-1 fails |
| F-2 | A missing or extra line is unacknowledged | Finalize | Refusal naming the line(s) | Remove the acknowledgement check → F-2 fails |
| F-3 | A duplicate exists, even if acknowledged | Finalize | Refusal; duplicate still blocks | Let acknowledgement clear a duplicate → F-3 fails |
| F-4 | An empty required value or an invalid calculation | Finalize | Readiness refusal, independent of item count | Let zero items override readiness → F-4 fails |
| F-5 | All items resolved, all lines acknowledged, values valid | Finalize | 200; saved record; session read-only; items retained | Leave the session writable → F-5 fails |
| F-6 | A stale PO version | Finalize | Typed conflict (`409`-class); no overwrite; session not advanced | Ignore the version → F-6 fails |
| F-7 | The pane shows an unresolved item | Click Final Save in the UI | No finalize API call; first blocker focused; assertive announcement | Call finalize anyway → F-7 fails |
| F-8 | A saved record | Attempt a session write | Refusal; no row change | Accept the write → F-8 fails |
| F-9 | The persisted session says resolved, but the finalize payload omits the decision | Finalize | The server recomputes from the submitted values and refuses; the UI must never produce this state | Trust the session alone, or trust the payload alone → F-9 fails |

F-9 is the integration bite Jessy flagged: the live pane's decisions must reach
the finalize payload, not only the session. The test asserts both the refusal
(server is authoritative) and the UI's payload construction (the refusal is not
reachable through the real path).

## 6. Postgres integration cases and commands

### 6.1 DSN requirement

The store/API/CLI integration tests skip without `VERA_POSTGRES_TEST_DSN`, and
`go test` exits 0 with the tests skipped. A green suite without the DSN is **not
evidence**. The DSN must point at a **throwaway** Postgres 18 database; never the
production database, never a shared development database.

- CI supplies Postgres 18 and
  `VERA_POSTGRES_TEST_DSN=postgres://vera:secret@localhost:55432/quotes?sslmode=disable`.
- Locally this agent cannot use the Docker socket (`/var/run/docker.sock` is
  root:docker, the agent is not in the group), and no local Postgres is
  installed. A local Postgres 18 DSN must be supplied, or docker access granted,
  before the persistence verdict can be produced.
- The database major must be 18. A newer dump/restore client against an older
  server fails with `unrecognized configuration parameter "transaction_timeout"`.
- The tests drop schemas (`resetTestSchema`). Point the DSN only at a database
  whose loss is acceptable; drop the database/container afterwards.

### 6.2 Cases

| ID | Case | Level | Expected |
| --- | --- | --- | --- |
| PG-01 | `041` applies on a fresh schema; session/item tables, indexes, uniqueness and immutability triggers exist | store | Migration ledger has `41`; tables and constraints present; `038` and `040` bytes unchanged |
| PG-02 | `041` applies to a schema already at `040` with existing POs | store | Existing records remain readable; whether pre-041 saved POs get a session is a recorded decision (HR-18), not an assumption; ledger has `41` |
| PG-03 | One active session per PO and source revision | store | A second active insert is refused by the unique index/constraint |
| PG-04 | A saved record's session is immutable | store + SQL | Direct `UPDATE`/`DELETE` is refused by the trigger; the store write is refused too |
| PG-05 | Session creation from an issued revision | store | Session row with the exact `SourceRevision`, org, PO; no session for a never-linked draft |
| PG-06 | Session creation at link | store | The same session shape after `LinkPurchaseOrderDraftSource`; the revision is the linked one |
| PG-07 | Draft save round-trip | store | Items, keys, resolutions and history survive `UpdatePurchaseOrderDraft`; reopen returns the same session ID |
| PG-08 | Line identity across replace-lines | store | Persisted line IDs are preserved; the client-key alias maps once; reorder does not re-key; removal retains history |
| PG-09 | Stale session write | store | The second writer gets the typed conflict; the first writer's session is intact |
| PG-10 | Org scoping | store | Another organisation's context cannot read or write the session |
| PG-11 | Finalize refuses unresolved | store | Typed refusal; no saved row; session still draft |
| PG-12 | Finalize refuses unacknowledged missing/extra and duplicates | store | Refusal names each blocker; no saved row |
| PG-13 | Finalize accepts a fully resolved record | store | Saved row; session read-only; items retained; version advanced |
| PG-14 | Saved session refuses a later write | store | Typed refusal; row and history unchanged |
| PG-15 | Source repair creates a new session and retains the old | store | New session for revision `01`; old session inactive/read-only but readable |
| PG-16 | Abandoned repair changes nothing | store | Same active session; same record version; no new session |
| PG-17 | Correction does not mutate the saved session | store | Correction snapshot and history written; session unchanged (if the correction lifecycle is still undefined, this case is blocked, not waived — HR-04) |
| PG-18 | Repeated repair to the same revision | store | Defined by the uniqueness rule; if undefined, blocked (HR-15) |

### 6.3 Exact commands

All commands run from `/var/lib/openbot/OpenBot/Shared/vera-502` on a quiescent
tree. Redirect to a log, then read the exit code directly; do not pipe the test
command into `head`/`tail` and read the pipe's status.

```sh
export PATH=/home/tristan/.local/go/bin:/home/tristan/.nvm/versions/node/v22.23.4/bin:$PATH
cd /var/lib/openbot/OpenBot/Shared/vera-502

# Throwaway Postgres 18. Only where the Docker socket is accessible; never
# against production or a shared development database.
docker run -d --name vera-pg-test-502 \
  -e POSTGRES_USER=vera -e POSTGRES_PASSWORD=vera -e POSTGRES_DB=veratest \
  -p 127.0.0.1:55432:5432 postgres:18-alpine
docker exec vera-pg-test-502 pg_isready -U vera -d veratest

export VERA_POSTGRES_TEST_DSN='postgres://vera:vera@127.0.0.1:55432/veratest?sslmode=disable'

# 1. Prove the integration guard actually runs, not skips.
env -u VERA_POSTGRES_TEST_DSN go test -count=1 ./backend/store \
  -run '^TestMigration041' -v > /tmp/opencode/502-verify/skip.log 2>&1
echo $?                                  # 0 even when skipped
grep -c -- '--- SKIP' /tmp/opencode/502-verify/skip.log   # must be >= 1

# 2. Store cases.
VERA_POSTGRES_TEST_DSN="$VERA_POSTGRES_TEST_DSN" go test -race -count=1 \
  ./backend/store \
  -run '^(TestMigration041|TestReconciliation|TestFinalize.*Session|TestSourceRepair.*Session|TestPurchaseOrder.*Reconciliation)' \
  -v > /tmp/opencode/502-verify/store.log 2>&1
echo $?                                  # must be 0
grep -- '^--- PASS' /tmp/opencode/502-verify/store.log  # every expected top-level test must appear
grep -- '^--- SKIP' /tmp/opencode/502-verify/store.log  # must print nothing

# 3. API cases.
VERA_POSTGRES_TEST_DSN="$VERA_POSTGRES_TEST_DSN" go test -race -count=1 \
  ./backend/api \
  -run '^(TestPurchaseOrderReconciliation|TestPurchaseOrderFinalize)' \
  -v > /tmp/opencode/502-verify/api.log 2>&1
echo $?
grep -- '^--- PASS' /tmp/opencode/502-verify/api.log
grep -- '^--- SKIP' /tmp/opencode/502-verify/api.log   # must print nothing

# 4. Full persistence package set (the CI shape).
VERA_POSTGRES_TEST_DSN="$VERA_POSTGRES_TEST_DSN" go test -race -count=1 \
  ./backend/store ./backend/api ./cmd/vera \
  > /tmp/opencode/502-verify/persistence.log 2>&1
echo $?

# The database is throwaway: drop it when the run is recorded.
docker rm -f vera-pg-test-502
```

A `-run` regex that matches no test also exits 0. The `--- PASS` count is the
guard against that; the expected count is written in the run log before the
command is run.

## 7. Snapshots and what must be hand-driven

### 7.1 Snapshot cases

`scripts/ui-snapshot.sh` builds the frontend, renders the harness with
`UI_SNAPSHOT=1`, then screenshots every HTML at **360×900** and **1280×900**. It
only proves layout. The PNGs are local evidence and are never committed.

```sh
CHROME_DIR=/tmp/opencode/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64 \
UI_SNAPSHOT_DIR=/tmp/opencode/ui-snap-502 \
scripts/ui-snapshot.sh > /tmp/opencode/502-verify/snapshot.log 2>&1
echo $?
```

The script itself always screenshots through Docker (`docker run ... vera-chrome-go:latest`).
At plan time the pinned directory is absent, no host Chrome is installed, and the
Docker socket is inaccessible, so snapshot coverage is **blocked**. A manual
headless-Chrome run against the generated HTML is a fallback only if Docker
cannot be granted; record it as a manual run, not as the script's evidence. A
missing PNG is reported as missing, never as a pass.

| Case | Viewport | What must be visible |
| --- | --- | --- |
| `PurchaseOrderWorkbench1280` | 1280 | Two panes; rail 360; pane with unresolved + resolved + tax context + totals; action bar; no horizontal scroll |
| `PurchaseOrderWorkbench360` | 360 | One column; no rail; sticky `Reconciliation · N to resolve`; two 44px action-bar buttons; no horizontal scroll |
| `PurchaseOrderWorkbenchSheet360` | 360 | Sheet open between the reconciliation bar and action bar; header and action bar visible; document and totals at the end. If the harness cannot render the open state without a snapshot-only prop, mark this case hand-driven and say so; do not add the prop |
| `PurchaseOrderDocketDrafts1280` | 1280 | Docket table: PO, Quotation + revision, Customer, Received, State, Open; no amount column; admin menu separated |
| `PurchaseOrderDocketDrafts360` | 360 | Stacked cards; state badge; full-width Open; admin menu at top right |
| `PurchaseOrderDocketArchive1280` | 1280 | Same as Drafts, archive state and Source missing badge |
| `PurchaseOrderDocketArchive360` | 360 | Same as Drafts, archive state |
| `PurchaseOrderDocketEmpty360` | 360 | Empty state with one action |
| `PurchaseOrderDocketError360` | 360 | Alert + Try again |
| `PurchaseOrderPaneLines1280` | 1280 | Missing, extra and duplicate cards; duplicate `Blocking`; named values |
| `PurchaseOrderSavedRail1280` | 1280 | Read-only rail; `{n} resolved`; no actions; session history disclosure |
| `PurchaseOrderFinalReceipt1280` | 1280 | Read-only `Decisions` receipt; kept/used/acknowledged rows; no Keep/Use; `Back to editing` / `Continue` |
| `PurchaseOrderWorkbenchError1280` | 1280 | Error banner with the server reason, Try again, Back; no editor, no stale record |

Existing cases that must also be read for regressions: `PurchaseOrderEditor`,
`PurchaseOrderDifferenceReview`, `PurchaseOrderSaved`, `PurchaseOrderDrafts`,
`PurchaseOrderArchive`, `PurchaseOrderTotals`, `PurchaseOrderSettings`,
`AppShellPurchaseOrder`.

**Snapshot evidence** is the PNG path, the viewport, and what was actually seen
(e.g. "rail occupies 360 px, action bar fixed at the bottom, no horizontal
scroll, count reads `3 to resolve`"). Listing a path without reading the image is
not evidence.

### 7.2 Hand-driven

The following are not provable by snapshot or jsdom and are hand-driven at the
stated viewport. The record names the browser/build, viewport, keys/pointer,
state reached, and what was not driven.

- 1024×768 two-pane layout, and the 1024 boundary (the harness renders only
  360/1280).
- Sticky rail and action bar while the form scrolls; 20 lines and long payment
  terms; no horizontal scroll.
- Focus order, focus restoration, visible focus, `inert` form while the sheet is
  open; Escape closes the sheet and returns focus.
- Keep PO → Escape/Cancel; focus returns to Keep PO; item remains unresolved.
- Use Quotation while the field is offscreen: the form does not scroll; focus
  moves to the next item.
- Resolving the last item at 1280 and at 360; count/status and focus.
- Filter with arrow keys; list scroll resets; page scroll does not move.
- Blocked Final Save: no API call (network log), first blocker focused, rail
  scrolls it into view.
- Final Save handoff: progress, arrival, focus on the saved h1, one polite
  announcement; failure returns to the confirmation view with the error focused.
- Stale version on draft save or final save: conflict prompt; working copy
  intact; no silent overwrite.
- Settings round-trip with a dirty editor; list filters/page/scroll/tab; review
  step; sheet state.
- Back stack: Home → workspace → settings → workspace → Home; never settings →
  Home; dirty workspace prompt.
- Sign out and app close: unsaved workspace state is cleared, nothing unsaved is
  written.
- Paste into payment terms: one commit on blur, one item, not one per paste
  event.
- 200% zoom at 1280: effective 640; one column; no horizontal scroll; sheet and
  action bar reachable.
- Reduced motion: no transforms, no rise/scale; opacity/colour remain; sheet
  opens with opacity only.
- Keyboard-only start to saved record; focus never lost to `body`.
- Screen reader: each announcement once, polite/assertive as specified; no
  initial-load or focus-move noise; no repeated toast.

## 8. Gate commands

Run only on a quiescent tree: no worker is editing, and nothing else writes
`frontend/dist` while the build runs (`AGENTS.md` §12.4). The order is deliberate:
cheap checks first, the shared-output build after the Go suite, snapshots last.

```sh
export PATH=/home/tristan/.local/go/bin:/home/tristan/.nvm/versions/node/v22.23.4/bin:$PATH
cd /var/lib/openbot/OpenBot/Shared/vera-502
mkdir -p /tmp/opencode/502-verify

git rev-parse HEAD > /tmp/opencode/502-verify/sha.txt
git status --porcelain > /tmp/opencode/502-verify/tree.txt   # must be empty for a PASS

gofmt -l . > /tmp/opencode/502-verify/gofmt.log 2>&1; echo $?  # exit 0 is not enough: the log must be empty
go vet ./... > /tmp/opencode/502-verify/vet.log 2>&1; echo $?
git diff --check > /tmp/opencode/502-verify/whitespace.log 2>&1; echo $?

# Pure Go comparison/contract (no DSN needed).
go test -race -count=1 ./backend/store \
  -run '^(TestPOReconciliation|TestComparePurchaseOrder|TestValidatePurchaseOrderDecisions|TestPurchaseOrderCalculation)' \
  -v > /tmp/opencode/502-verify/pure-go.log 2>&1; echo $?

# Postgres set: see §6.3; a run without the DSN is not a gate.
# Full Go suite with the DSN:
VERA_POSTGRES_TEST_DSN="$VERA_POSTGRES_TEST_DSN" \
  go test -race -timeout 30m -count=1 ./... \
  > /tmp/opencode/502-verify/go-all.log 2>&1; echo $?

# Frontend tests and build.
npm --prefix frontend test > /tmp/opencode/502-verify/frontend-test.log 2>&1; echo $?
npm --prefix frontend run build > /tmp/opencode/502-verify/frontend-build.log 2>&1; echo $?

# Wails bindings must be regenerated, not hand-edited.
go test . -run '^(TestGeneratedBindingsCoverEveryBoundMethod|TestGeneratedBindingsHaveNoOrphans)$' \
  > /tmp/opencode/502-verify/bindings.log 2>&1; echo $?

# Snapshots (see §7.1; blocked without Chrome/docker).
CHROME_DIR="$CHROME_DIR" UI_SNAPSHOT_DIR=/tmp/opencode/ui-snap-502 \
  scripts/ui-snapshot.sh > /tmp/opencode/502-verify/snapshot.log 2>&1; echo $?
```

`npm run build` type-checks the test files too; a test that stops compiling must
fail the build, not lie in the suite. `go test -race ./...` without the DSN is
explicitly admitted by `AGENTS.md` §7 to pass with skips, so it is never recorded
as the persistence gate.

## 9. Evidence format

Every criterion gets one record. The format is fixed so two runs can be compared
without prose.

```text
ID:            04.13 / F-1 / PG-07 / L-1 ... (this plan's ID)
Criterion:     the ticket bullet, quoted or paraphrased
Verdict:       PASS | FAIL | BLOCKED | NOT RUN
SHA:           <40-char git rev-parse HEAD>
Tree:          clean | dirty, sha256(git status --porcelain) = <hash>
Command:       <verbatim, including env vars except the DSN password>
Exit code:     <direct exit status, read after redirect; never a pipe's status>
Expected:      <the observable result>
Actual:        <the decisive lines; for a FAIL, the failing assertion and line>
Reproduction:  1. ... 2. ... 3. ...
Artifacts:     <log path, PNG path, SQL output path>
Hand-driven:   <browser/build, viewport, keys/pointer, state, not driven> (when applicable)
Severity:      P0 | P1 | P2 | P3 (FAIL only)
```

Severity:

- **P0** — data loss, wrong money, auth/org bypass, or a saved record that cannot
  be recovered.
- **P1** — an acceptance criterion fails; Final Save is wrongly allowed or
  wrongly blocked; the audit trail is wrong.
- **P2** — a user-visible behaviour is wrong but recoverable; an error/empty state
  is missing; a non-critical path is inconsistent.
- **P3** — copy, spacing, or polish with no data effect.

A FAIL record is not a code review and does not propose a fix. It names the
criterion, the command, the expected/actual, the smallest reproduction, and the
severity. The implementer fixes; I re-run on the new SHA.

## 10. What I will not accept as evidence

1. A green `go test -race ./...` without `VERA_POSTGRES_TEST_DSN`. The store, API
   and CLI integration tests skip, and `go test` still exits 0.
2. A piped exit code (`cmd | head`, `cmd | tail`, `cmd | grep`) reported as the
   command's status. `AGENTS.md` §12.5 records a false "tsc clean" caused by
   exactly this.
3. A `-run` regex that matched zero tests. It exits 0; only the `--- PASS` lines
   prove the tests ran.
4. The implementer's own test as the only evidence of the implementer's code. I
   reproduce the behaviour at the public seam and record both results.
5. A shared fixture edited to fit the implementation. The fixture is the oracle;
   a change requires the contract and both readers to change together.
6. One-sided parity: Go-only or TypeScript-only proof of a rule that both
   languages implement.
7. Snapshot-only proof of interaction, "looks fine", or a PNG listed but not
   read. Snapshots prove layout at 360/1280 and nothing else.
8. A screenshot at a different viewport or zoom claimed as 1024 or 200%.
9. A DOM test that calls the commit handler directly, or sets pane state through
   a prop the real path cannot reach, instead of blurring/clicking the real
   component.
10. Direct SQL inserts or updates used to make a Final Save pass, or to create a
    session the product path would not create. SQL is only for the schema-guard
    red proof.
11. A guard that has never been seen failing, or a mutation that was "restored"
    without the diff and the failing output.
12. "Existing tests still pass" as proof of a new acceptance criterion.
13. Stale or hand-edited `frontend/wailsjs` bindings. `bindings_test.go` proves
    method presence, not runtime behaviour; a real desktop run is still needed
    for the bridge.
14. A PASS on a different SHA, or on a dirty tree whose fingerprint was not
    recorded. A CI green on a prior SHA is stale.
15. A CI check that is pending, skipped, cancelled or missing counted as green.
16. "Wails works" inferred from a compiled binding test or a mocked module.
17. Postgres below 18, a shared development database, or a production DSN.
18. "No visual regression" when the page composition changed and the assembled
    page was never rendered.
19. Ticket 01's test-only session/item types described as production
    persistence. Contract §9 says they are shape checks, not an implemented
    state machine or store.
20. A full-suite run on a tree another worker is writing to. The build writes
    shared output; a concurrent wave corrupts the gate (`AGENTS.md` §12.4).

## 11. Highest-risk areas

Ranked by the chance of a wrong PASS and the cost of the defect.

1. **Live-to-final decision mapping (ticket 04/05, F-9).** The pane persists
   decisions in a session; Finalize recomputes the comparison from the submitted
   values and requires a choice for every difference. If the UI resolves an item
   but does not send that decision in the finalize payload, the user has a green
   pane and a refused save. If the server trusts the session instead of the
   payload, a stale session can authorize a value the record does not contain.
   The test must assert both sides and the real UI path, not either alone.
2. **Line identity across replace-lines persistence (ticket 05, L-1/L-2).** The
   current `replacePurchaseOrderItems` deletes and reinserts every line, and both
   comparators key by line number. A test that never reorders will pass while the
   first reorder duplicates or loses every decision. This is the highest-value
   red proof in the plan.
3. **Tax-basis lifecycle (ticket 05/06, T-3/T-5, HR-03).** "Protected after link"
   is testable. "Changing it later is a correction" is not: the correction path
   currently freezes the linked basis, and no authorized command is defined. A
   PASS must not claim the later-change story.
4. **Unknown-reference acknowledgement (ticket 05, L-4, HR-02).** The spec says
   acknowledge or correct; the saved-reference validator refuses an unknown
   non-null reference. Acknowledgement alone cannot finalize. The contradiction
   is a product/backend decision, not a test detail.
5. **Migration 041 uniqueness and immutability (ticket 04/06, PG-01..PG-04,
   PG-18, HR-15).** "One active session per PO and source revision" and "a new
   session on every repair" collide if a repair returns to a previously used
   revision. The unique index can block the repair path. The immutability trigger
   must be proved by direct SQL, not only by the store's own check.
6. **Dual comparator drift (ticket 01/05, 01.1/05.1, HR-14).** Go and TypeScript
   already format price differences differently (`"700"` vs `"$7.00"`). The
   shared fixture pins the declared catalogue, but a production field added
   without the fixture will not fail it. Parity claims must be limited to
   classification and keys, not display strings, until the canonical-number
   change lands.
7. **Snapshot-only states and the 1024 gap (ticket 07, 07.9, §7).** The harness
   renders only 360/1280. The 1024 two-pane, the open sheet, sticky scrolling and
   every motion claim are hand-driven. A snapshot-only prop added to reach an
   interactive state is a fake state and is not accepted.
8. **The dirty, uncommitted tree.** A PASS now would be a PASS on `dca8890` plus
   someone's working copy. The first valid verdict waits for a commit and a clean
   tree.
9. **Pre-041 saved records (ticket 06, PG-02, HR-18).** A record saved before
   migration `041` has no session. The archive view and the API must define what
   it shows; a synthetic backfilled session would be an invented audit trail.
   This is a decision, not a test detail.

## 12. Untestable or ambiguous criteria as written

These are named so the verdict does not quietly invent a rule. "Blocked" means the
criterion cannot be given a PASS/FAIL until the named decision is made; it does
not mean the work is accepted.

| ID | Criterion (source) | Why it is not testable as written | Decision needed |
| --- | --- | --- | --- |
| HR-01 | Ticket 04: "committing an empty **required** header field shows a notice" | No comparable header field is required today: payment terms and Incoterms are optional (contract §8, "Required header values"); the store's required-value checks are a positive line quantity, a saved record's customer PO number and a received date, none of which is a comparable header field. The criterion is vacuous for headers. | Name the required header set, or scope the notice to the fields the existing validation requires. Until then, test the empty notice on quantity and mark the header case N/A, not PASS. |
| HR-02 | Ticket 05: an unknown reference is "extra and must be acknowledged or corrected" | The saved-reference validator refuses an unknown non-null reference at Finalize (contract §8, "Unknown source reference"). Acknowledgement cannot clear it. | Decide: acknowledgement clears the reference, or only correction can finalize. The UI must not promise a green path. |
| HR-03 | Spec §Tax basis: "Changing it later is a correction or a source repair" | The correction path freezes the linked basis, and the source-repair path adopts the new source basis. No permitted correction command is defined (contract §8, "Tax correction wording"). | Product/backend ruling on the permitted correction command, or an explicit "not in #502". |
| HR-04 | Ticket 06: "Corrections and version history are unaffected by the reconciliation session" | Which session records corrected values, and with which actor, is undefined (contract §8, "Saved correction lifecycle"). | Define whether a correction creates a new session generation or leaves the saved session as the only record. |
| HR-05 | Ticket 04: "A stale session write conflicts on the PO version" | A session-only write may not bump the PO version; two session writers at the same version are not defined. The write envelope is not frozen (contract §5/§6). | Freeze whether session writes bump `POVersion`, and which version the client sends. |
| HR-06 | Ticket 05/Spec §Totals: "names the input difference that explains a mismatch" | No precedence when several inputs differ; no defined wording/outcome when totals differ with no input difference. | Define the reason precedence and the "no input explains it" outcome. Until then, test "at least one differing key is named" and "no item", not exact copy. |
| HR-07 | Contract §2 discount: disabled 0% vs enabled 0% | The Go comment says they are equal; both implementations distinguish `None` from `0%`; the contract pins executable behaviour but says a product ruling is needed (contract §8). | Product ruling on commercial equality. Until then, the fixture is the oracle; a change to the ruling is a contract change. |
| HR-08 | Ticket 07: page/step motion 180–220 ms, ease-out-quart, 4–6 px; rail 200 ms; sheet "enters and leaves cleanly" | jsdom does not compute transitions or layout; snapshots are static. "Leaves cleanly" contradicts the spec's "no exit animation" and the brief's "instant close" (spec §Motion, brief §9). | Define "leaves cleanly" as no exit animation + focus return, or specify a timed exit. Motion timing/easing must be hand-measured in a browser, not asserted from a class name. |
| HR-09 | Ticket 07: "Notices are announced politely; blocking errors assertively" | DOM can assert `role`/`aria-live` and a single text change; it cannot prove what a screen reader says or that it does not double-announce. | Hand-driven screen-reader pass required. Automatable part: attributes and one text mutation. The rest is recorded as hand-driven, not PASS by DOM. |
| HR-10 | Ticket 02: "scroll" survives the round-trip; "unsaved state is not retained after app close" | jsdom has no layout/scroll, and there is no app-close lifecycle in a test. A stored `scrollTop` assertion is implementation-coupled. | Hand-driven only; record `scrollTop` and app-close behaviour. |
| HR-11 | Ticket 02: "no new global store is introduced" | Architectural, not behavioural; a test cannot distinguish a justified store from an unjustified one. | Code review of the state ownership in the diff. Record as a review finding, not a test. |
| HR-12 | Ticket 06: source repair "seeds a full comparison of every comparable field" | Read literally, a match would create an item, contradicting "a match creates no item" (spec §Two feedback mechanisms, contract §4). | Define "full comparison" as every current difference + every line-set outcome + the link-time tax item, not an item for every matching field. |
| HR-13 | Spec §Reconciliation session: "Each item records ... the source revision it belongs to" | The frozen `Item` shape has no `SourceRevision`; only the `Session` does (contract §5). | Fix the wording to "belongs through its session", or add the field to the contract and both readers. Do not let a test assert a field that is not in the shape. |
| HR-14 | Spec §Comparable-field contract / ticket 01: Go and TypeScript "must produce the same result" | Current price differences are minor-unit strings in Go and formatted currency in TS (contract §8, "Price display drift"). Byte parity is false today. | Define parity as the same classification and keys over canonical numbers; display formatting is excluded. |
| HR-15 | Ticket 04/05: "one active session per PO and exact source revision" + ticket 06 "a new session on every repair" | A later repair back to a previously used revision collides with the unique active rule (contract §8, "Revisit same source revision"). | Define historical generation uniqueness before the DB constraint lands. |
| HR-16 | Ticket 05: duplicate match "until one match is corrected" | Removing one of the duplicate lines is not defined: does the surviving line's duplicate item resolve, or stay blocking? | Define the line-removal interaction with `duplicate_match`. |
| HR-17 | Ticket 07/UX brief: 1024 layout and the open sheet | `scripts/ui-snapshot.sh` renders only 360/1280; some sheet states may not be reachable in SSR without a snapshot-only prop. | Provision a 1024 browser pass, and render the sheet through its real props or mark it hand-driven; do not add a harness-only state. |
| HR-18 | Ticket 06: "Opening a saved PO shows the reconciliation session read-only" | Records saved before migration `041` have no session. The spec does not say whether they show "no session recorded" or receive a backfilled synthetic session. | Define the pre-041 archive story. Do not invent decisions or attribute them to a user who never made them. |

## 13. Order of work and what I need to start

1. **Quiescent tree.** No worker editing; `frontend/dist` not being written.
2. **A committed SHA.** The wave must be committed so a verdict is reproducible.
   A pre-check on the current dirty tree can start earlier, but it cannot be
   quoted as PASS.
3. **A throwaway Postgres 18 DSN**, or Docker access to run one. Without it,
   PG-01..PG-18 are BLOCKED and no persistence verdict is issued.
4. **Chrome for snapshots** (pinned `CHROME_DIR` or Docker access to
   `vera-chrome-go:latest`). Without it, S6 is BLOCKED and ticket 07 cannot be
   passed on snapshots.
5. **Product rulings** on HR-01, HR-02, HR-03, HR-04, HR-05, HR-06, HR-07,
   HR-08, HR-12, HR-13, HR-14, HR-15, HR-16, HR-18 before those criteria can be
   PASS/FAIL. Until then they are BLOCKED, not waived.
6. **A Wails desktop run** for S7 (motion, focus, screen reader, 1024, app
   close). CI cannot prove these.

## 14. Reporting

- The verdict is **PASS** only when every criterion in tickets 01–07 has a
  record with a direct exit code and the evidence above, on one SHA with a clean
  tree.
- A **FAIL** names the criterion, the command, the direct exit code, the failing
  assertion/line, expected vs actual, the smallest reproduction, and severity
  P0–P3. It does not propose a fix and does not soften the verdict.
- A **BLOCKED** names the missing decision, environment or evidence. It is not a
  PASS and it is not a FAIL of the code; it is a failure of the verification to
  conclude.
- A **NOT RUN** is reported as not run. It is never described as "looks fine".
- The report separates: what I ran, what I saw, what I did not run, and what I
  could not test. The implementer's claims and the worker's green tests are
  supporting evidence, not the verdict.
- No code is edited, no branch is changed, no commit, push, merge or deploy is
  made by QA.

---

*Plan only. No production code, test, migration, binding or workflow was changed
to write this document; no commit was made. The worktree's uncommitted ticket
01/02/UX files were read, not modified.*
