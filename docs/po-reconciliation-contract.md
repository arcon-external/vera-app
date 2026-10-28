# PO reconciliation contract — #502 / ticket 01

Contract version **1**, against `dca88904edd034875f33a071e9e669a31d9e39e4`.
This freezes the interface for tickets 04–06, not their implementation.
Authority: [the #502 spec](../.scratch/vera-502/spec.md),
[ticket 01](../.scratch/vera-502/issues/01-reconciliation-contract.md), and the
user's Q13–Q16 answers. Those answers supersede handover §§6.2–6.3/§8:
empty values are notices, there is no matched summary, and billing address is PO-only.

The shared, language-neutral examples are in
[`backend/testdata/po_reconciliation_contract.json`](../backend/testdata/po_reconciliation_contract.json).
`Fields` and `Shapes` are the machine-readable catalogue. Both consumers declare
their own field/key sets and compare them exactly against that file. Changing
the catalogue requires changing both consumers and this contract together.

## 1. Terms and the seam

The **difference review** is the staff workflow. A **reconciliation session** is
its retained record for one PO and exact source revision. A **notice** is
transient empty/matched feedback. A **review item** retains a difference, the
values checked and the decisions made. A matched notice is not an item status.

The primary feature seam remains the existing PO review Wails/HTTP interface.
The existing pure `ComparePurchaseOrder` / `comparePurchaseOrder` interfaces are
the parity seam for live versus authoritative comparison. The commit handler
will consume their commercial rules; the server independently revalidates on
Finalize. No second pricing engine or UI-specific server rule is introduced.

## 2. Comparable catalogue

`Field` is a catalogue ID, not a translated label. `Key` identifies a slot in a
session. Labels can track the displayed line number; keys cannot.

| Field | Item/check key | Value | When checked |
| --- | --- | --- | --- |
| `header:paymentTerms` | same | Trimmed text; label “Payment terms” | Commit |
| `header:incoterms` | same | Trimmed text; label “Incoterms” | Commit |
| `header:discountPercent` | same | Integer percent; label “Whole-PO discount” | Commit |
| `line:description` | `line:{lineKey}:description` | Trimmed text | Commit on a matched PO line |
| `line:uom` | `line:{lineKey}:uom` | Trimmed text; label “unit of measure” | Commit on a matched PO line |
| `line:quantity` | `line:{lineKey}:quantity` | Positive integer, quantity ×10,000 | Commit on a matched PO line |
| `line:unitPrice` | `line:{lineKey}:unitPrice` | Nonnegative integer, currency minor units | Commit on a matched PO line |
| `line:discount` | `line:{lineKey}:discount` | `{Enabled, Percent}` | Commit on a matched PO line |
| `line:match` | `line:{lineKey}:match` | Source line reference, or `null` | Line-set/reference change |
| `source:match` | `source:{sourceLineRef}:match` | Missing source reference, e.g. `"2.1"` | Line-set/reference change |
| `link:taxBasis` | same | `{TaxZone, TaxType, ServiceCategory}` | Link transaction only |
| `derived:totals` | same, **never an item** | Derived numbers and explanation | Derive after committed input changes |

Full labels and key templates are pinned in `Fields`. A line label is
“Line {lineNumber} description/quantity/unit of measure/unit price/discount”.
It names the current PO line, not the Quotation item number.

Comparison trims surrounding text whitespace, preserves internal whitespace
and case, and compares valid numeric inputs in stored integer units. Formatting
`3`, `3.0` and `3.0000` does not change quantity. Zero price is valid; zero
quantity is not. Rates are integer percent (0–100); PPN uses basis points.
A disabled line discount has no effect regardless of its dormant percentage;
keep `Enabled` in the value so the difference between disabled and enabled 0%
remains visible, as both current comparators do. This is not the Go comment's
claim that those two states match (see §8).

Line matching uses `(QuotationItemNo, QuotationSubNo)` in the exact revision.
References render as `"3"` or `"3.1"`. Priced sub-items follow the source's
customer-facing lines. Reorder and description similarity never establish a
match. A suggestion is a hint, not a match or choice. No reference or an
unknown reference is extra; an unclaimed source line is missing. Two PO lines
claiming one reference are a blocking duplicate, never an acknowledgeable extra.
An unknown reference is visible as extra during review, but still must satisfy
the existing saved-reference validator before Finalize (§8).

### Exclusions

Billing address is **PO-only**, even when copied from the Quotation. It never
gets a notice or review item from this comparison. Other PO-only values are:
customer PO number/date, received date, requested delivery date, delivery
address, ship-to code, contact email/phone, displayed line number, adjustment
and reason, confirmation number, customer PO document, state and version.
They retain their existing validation; “not comparable” does not mean optional.

Customer snapshot, currency, Quotation identity and source revision are
source-owned identity/constraints, not keep/use differences. A currency or
customer mismatch is refused, with no conversion. Cost, margin, PPH22 and
profit are absent from this interface and customer output.

Printed subtotal/discount/PPN/grand total are immutable source evidence, not
editable comparable inputs or decision slots. Derived line totals and PO totals
may be checked against that evidence, but no “use printed total” choice exists.

## 3. Stable identity

An item is unique by **(session ID, Key)**. A persisted PO line first entering
a session has a `LineIdentities.Key` such as `id:501`. A new editor line gets a
collision-resistant `client:<UUID>` key once, not an array index or the existing
mount-local integer key. The line's displayed number and source reference are
separate, mutable attributes.

The first save returns/persists a mapping from that client key to the line ID
in `LineIdentities`. The client key remains the item's `LineKey` for the lifetime
of that session: `line:client:<UUID>:quantity` does **not** become
`line:id:501:quantity`. This implements the spec's save mapping without violating
its no-re-key rule. The server must preserve the row ID on later edits, persist
the alias for reopen, and refuse another line claiming that alias.

Reorder changes only display order/labels. Edits upsert the same item. Correcting
a source reference does not change the PO line identity: prior values/decisions
stay in its history and newly affected comparisons need a current decision.
Missing-source keys use the immutable source reference within the session.

Removing a PO line marks its identity `Removed: true`, resolves its items with
`line_removed`, and retains their keys, values and history. Other lines never
inherit them. Recompute the line set: removing the only claimant may create a
separate missing-source item, which still needs acknowledgement/correction.
Retain the mapping after removal; do not cascade-delete review history.

**Implementation dependency:** existing replace-lines persistence destroys IDs
on every save. Ticket 01 does not change it; tickets 04–05 must provide identity
preservation before relying on persisted line IDs. See §8.

## 4. Commit and decision state machine

Run the handler on blur and on Finish/Continue/Final Save for the focused field,
before evaluating readiness. Typing changes editor input only. Repeating the
same commit is idempotent; it cannot append another item or duplicate event.
The server still checks the entire submitted order, including untouched fields.

| Event / precondition | Notice after commit | Item after commit | Save effect |
| --- | --- | --- | --- |
| Required value empty, no item | `empty`: “Quantity is empty” | None | Required-value gate blocks |
| Match, no item | `matched`: “Matches the Quotation” | None | No review blocker |
| Non-empty valid difference, no item | None | `unresolved`, resolution `null` | Blocks |
| Same unresolved value committed again | None | Same unresolved item | Blocks |
| Keep PO requested / confirmation cancelled | None | Remains unresolved | Blocks |
| Keep PO confirmed | None | `resolved / kept_po` | This item no longer blocks |
| Use Quotation | None | Update input, then `resolved / used_quotation` | This item no longer blocks |
| Existing item, committed value now matches | None | Same item, `resolved / values_now_match` | This item no longer blocks |
| Same kept value re-committed | None | Retain `kept_po` and its attribution | No new decision |
| Resolved item, a new differing value committed | None | Same item returns to unresolved; retain earlier decision in history | Blocks pending a new choice |
| Previously reviewed required value cleared | `empty` | Retain previous item/history | Required-value gate blocks, even if item was resolved |
| Missing / extra found | None | Unresolved line-match item | Blocks |
| Missing / extra acknowledged | None | `acknowledged_missing` / `acknowledged_extra` | Clears that acknowledgement blocker |
| Match corrected | None | `resolved / match_corrected`; recheck affected line set/inputs | Other blockers may remain |
| Duplicate merely acknowledged | None | Unresolved | Duplicate still blocks |
| PO line removed | None | `resolved / line_removed`, retained | Recheck for missing source lines |

Status is exactly `unresolved | resolved`. A non-null resolution accompanies a
resolved item. `matched` and `empty` are notice kinds, never persisted statuses.
No field with an item also has a matched notice. Empty notices may accompany
retained history; they do not invent another item. Match notices remain until
the next commit reevaluates that field, and are not stored or replayed on reopen.
Invalid non-empty input remains a validation error and blocks readiness; do not
turn a parse failure into zero or a reviewable commercial difference.

For re-edits, the general commit rule means a new differing value needs a new
choice, while the same committed value keeps its choice. An append-only
`History` on the existing item preserves earlier resolved decisions without
duplicating pane rows. This is the representation of the spec's retained
decision history, not a new user-visible workflow.

Missing/extra acknowledgements apply to the current line set, not arbitrary
client-supplied labels. A stale acknowledgement cannot acknowledge another line.
`Transitions.BlocksFinalSave` describes that transition's contribution to
readiness; it does not say that other fields or line-set blockers are clear.

## 5. Frozen JSON data shape

Keys are exact **PascalCase**, matching current Wails DTO conventions. All keys
listed in `Shapes` are present; nullable values use JSON `null`, arrays use `[]`,
and there is no `omitempty` alias. IDs, quantities and money transported as
JSON numbers must be safe integers (within ±9,007,199,254,740,991); production
validators retain their stricter existing limits. Revision `0` is a real source
revision, not “no source”. Timestamps are UTC RFC3339 strings.

| Shape | Exact members |
| --- | --- |
| Session | `ID`, `PurchaseOrderID`, `SourceQuotationID`, `SourceRevision`, `POVersion`, `Active`, `ReadOnly`, `CreatedBy`, `CreatedAt`, `UpdatedAt`, `LineIdentities`, `Items` |
| LineIdentity | `Key`, `ID` (number or null before first save), `Removed` |
| Item | `Key`, `Field`, `Kind`, `LineKey` (string or null), `SourceLineRef` (string or null), `Label`, `POValue`, `SourceValue`, `Status`, `Resolution` (string or null), `ActorID`, `UpdatedAt`, `History` |
| HistoryEvent | `Kind`, `SourceLineRef` (string or null), `POValue`, `SourceValue`, `Status`, `Resolution` (string or null), `ActorID`, `At` |
| Discount | `Enabled`, `Percent` |
| TaxBasis | `TaxZone`, `TaxType`, `ServiceCategory` |

`Kind` is `difference | missing_line | extra_line | duplicate_match | tax_basis`;
line-match kinds name the problem even after it has been resolved. Allowed kinds,
statuses, resolutions and notices are also pinned in `Enums`. Scalar text values are strings;
quantity, money and whole-PO percent values are integers, not display strings.
Line discount uses the Discount object; tax basis uses the TaxBasis object.
Line-match values are reference strings or null (absent). `Field` determines
the value shape; never compare formatted currency labels. A disabled discount
has canonical `Percent: 0` without changing the dormant editor value.

`SourceLineRef` is context for a line field/match and null for header/tax fields.
`LineKey` is null for header, tax and missing-source items. Source quotation and
revision are owned by the session; every item and history event belongs to that
exact revision through its session, not the latest Quotation revision.
`History` is ordered oldest-first, contains snapshots of recorded reference/state/value
changes, and retains the original actor/time of each decision. `ActorID` and
timestamps come from the authenticated server context, never trusted client data.

The fixture's `Session` is an example **read representation**. It is not
permission to accept a client replacement of history/actors. The existing PO
review response will gain this representation additively; existing fields and
older-client behaviour need an explicit implementation compatibility decision.
Ticket 04 must bind the test-only declarations to the real DTOs and freeze the
write command/envelope before wiring transport. This ticket changes no Wails
method, HTTP route or production DTO.

## 6. Session lifecycle, tax and totals

A never-linked draft has no session or invented source. Link/create-from-source
creates a session for the exact revision. Draft saves retain it and its items;
reopen returns the same IDs, mappings and history. All reads/writes are scoped
to the authenticated organisation and PO; org identity is not a writable DTO
field. At most one session is active for a PO (thus also one active per
PO/source pair). All writes use the PO's optimistic version; a stale write
conflicts. Readiness cannot trust an old POVersion or another session's decisions.

Finalize makes the session read-only. A source repair prepares a **new** session
with a full comparison, not just touched fields. The successful repair atomically
retains/deactivates the old session and installs the new session against the new
revision; the new session is read-only with the saved repaired PO. An abandoned
repair changes neither the old active session nor the PO.
Old sessions remain readable. A saved PO and its saved sessions cannot be edited
through the draft-session writer. Correction lifecycle ambiguity is recorded in
§8; do not relax read-only history to solve it.

Tax basis is the complete semantic triple, not just its PPN rate. Link review
records keep-PO or use-Quotation as one resolved `link:taxBasis` item; source-
created POs record adoption of that source basis. Normalise legacy `TaxCategory`
through the existing resolver before projecting the triple; keep the database
legacy field intact. Resolve PPN from the chosen triple in the link transaction.
After linking the item is read-only context. Changing basis later goes through
an authorised correction/repair path, never a field-level pane action. The
current correction path's limitations are an open conflict, not new authority.

The calculation contract is unchanged:

1. Gross = round-half-away-from-zero(quantityScaled × unitPriceMinor / 10,000).
2. Row discount = truncate(gross × enabled percent / 100); net = gross − discount.
3. Subtotal = sum of row nets; whole-PO discount = truncate(subtotal × percent / 100).
4. PPN = truncate((subtotal − discount) × basis points / 10,000), charged on top.
5. Grand total = subtotal − discount + PPN + signed adjustment; never negative.

An adjustment needs its existing reason validation. The source's unit price is
already the customer list price; PO reconciliation never reapplies supplier
cost or margin. Derived totals explain differences through input keys, line-set
changes, tax choice or adjustment. Printed evidence is not a target. A valid,
explained total difference creates no review item and adds no save blocker.

## 7. Final Save gate

Commit the focused input first. The server locks the current PO/version and
re-reads its exact live source and document state in the final transaction.
Finalize requires all of the following:

- A live source, matching authoritative customer/currency, customer PO number,
  received date, customer snapshot and at least one valid PO line.
- Existing input/date/length/range/reference/calculation rules; required values
  filled. Empty notices do not substitute for these readiness checks.
- Zero unresolved items, plus a current explicit choice for every difference
  recomputed from the submitted values. A previously kept value is not authority
  to accept a newly edited value. Unknown/duplicate decisions are refused.
- Every current missing/extra line acknowledged or corrected; no duplicates.
  “Use Quotation” must actually update the value before the save can succeed.
- The resolved link-time tax basis remains protected; no inline basis mutation.
- No-document confirmation when no active PDF exists; the optional confirmation
  number keeps its existing allocation/immutability rules.

Zero unresolved items alone is insufficient: empty required values, an untouched
difference, stale choices, invalid calculations or duplicate matches still
block. Save draft may retain unresolved items; this does not authorise storing
malformed numeric input. Archive reads do not mutate or recreate sessions.

## 8. Current conflicts and decisions still needed

These are findings against `dca8890`; ticket 01 leaves production unchanged.

| Surface | Evidence / consequence | Required follow-up |
| --- | --- | --- |
| Positional item keys | `backend/store/po_review.go` and `frontend/src/poFinalize.ts` emit `line:{lineNumber}:field`. | Tickets 04–05 need stable identities and a compatibility mapper; do not silently reinterpret old keys. |
| Line IDs not stable or exposed | `replacePurchaseOrderItems` deletes/reinserts every line; `PurchaseOrderItemDTO` / editor line input omit persisted IDs. | Preserve IDs and persist client-key mapping; migration 041 may need additive line support, not only session/item tables. |
| No session persistence | Current decisions/acknowledgements are final-save inputs, not a persisted commit history. | Ticket 04 supplies production types, migration, transport and server-authoritative history. Test-only DTOs are not evidence that this exists. |
| Price display drift | Go difference values are minor-unit strings (`"700"`); TS uses `"$7.00"`. | Fixture makes these legacy outputs explicit in `FrontendPriceValues`. Future Item values are canonical numbers; format in the view. |
| Duplicate comparable keys | Go pre-adds payment terms/Incoterms and appends them again. | Current parity checks compare a set for those slots. The new item uniqueness rule must not inherit those duplicates. |
| Discount comment disagrees | Go's comment says disabled 0% equals enabled 0%; both implementations distinguish `None` from `0%`. | Fixture pins executable behaviour. Product owner must rule before changing that commercial equality. |
| Tax basis projection | The general source DTO omits the triple, although draft-link review already exposes DraftTax/SourceTax and explicit tax choice. | Extend additively; retain the link resolver. Billing address stays excluded. |
| Tax correction wording | Spec allows later correction/repair, but current correction/update freezes linked basis; source repair adopts its source basis. | Product/backend owner must define the permitted correction command; this contract does not enable it. |
| Unknown source reference | Spec calls it extra/acknowledgeable; final saved-reference validation refuses a non-existent non-null reference. | Decide whether acknowledgement clears the reference or correction is required; do not promise acknowledgement alone can finalize it. |
| Required header values | Spec/ticket 04 say “empty required header” without naming an expanded set; current payment terms/Incoterms may be empty. | Retain existing requiredness. Additional required commercial fields need a product ruling; the conditional transition example does not make payment terms required. |
| Saved correction lifecycle | Ticket 06 says saved sessions are immutable and corrections are unaffected, but does not say which session records corrected values. | Decide session generation/attribution for corrections before wiring that path. Keep old decisions read-only meanwhile. |
| Revisit same source revision | “One session per PO and exact revision” versus a new session on every repair does not specify a later return to a previously used revision. | Freeze one active session now; decide historical generation uniqueness before a DB uniqueness constraint prevents that path. |
| Text whitespace edge set | Go TrimSpace and JS trim differ for NEL/BOM; current shared cases pin ASCII surrounding whitespace only. | Agree a cross-language Unicode trim policy before claiming all Unicode inputs are equal. |

The existing glossary's generic Choice includes “adjust”; the live field review
only offers keep/use. Adjustment remains a separate PO field, not a third
resolution button. The newly defined session/notice/item terms distinguish this
model without changing that older generic term or Quotation workflow.

## 9. What ticket 01 verifies

Go and TypeScript read **the same JSON file**. The executable comparison cases
cover every current commercial slot, text/number formatting, zero price,
discount-enabled semantics, exact scaled quantity, missing/extra/unknown/
duplicate references and reordered priced sub-items. Complete output/key-set
assertions catch one-sided field changes. The known price presentation exception
is explicit evidence of existing drift, not a claim of byte-identical values.

Independent test-only session/item types pin exact keys, nullability and nested
specimens. The transition, identity and gate examples specify the future
interface and are checked for shape/coverage/consistency. **They do not execute
a commit handler, session store, source repair or new Final Save gate.** Tickets
04–06 must run these examples against production behaviour and replace the
test-only wire declarations with production types, keeping the shared file.

Three executable `TotalsCases` also pin the calculation stages and an accepted
quantity/total difference; `ReasonFields` records the planned explanation inputs,
not a claim that the current Totals pane renders them. The existing calculation
parity fixtures remain the wider arithmetic suite;
no Quotation pricing, signing, migration history, production code, generated
Wails bindings or release workflow is changed by this ticket.

## 10. Ticket 01 verification record

Run in the shared worktree with Go/Node available from
`/home/tristan/.local/go/bin` and `/home/tristan/.nvm/versions/node/v22.23.4/bin`.
All final commands below exited **0** on the uncommitted ticket 01 files over
`dca8890`; this is scoped evidence, not a release/Judge verdict on the whole wave.

```sh
go test -race ./backend/store -run '^(TestPOReconciliation|TestComparePurchaseOrder|TestValidatePurchaseOrderDecisions|TestPurchaseOrderCalculationMatchesTheSharedFixture)' -count=1
npm --prefix frontend test -- src/poReconciliation.contract.test.ts src/poFinalize.test.ts src/purchaseOrder.test.ts
go vet ./backend/store
gofmt -l backend/store/po_reconciliation_contract_test.go
git diff --check
```

Vitest: **3 files / 90 tests passed**, including **17** new contract tests.
The scoped TypeScript check also exited 0: `tsc --noEmit --incremental false`
with a temporary config extending the frontend config, including only
`src/poReconciliation.contract.test.ts`, and `references: []`. That config was
removed after each run. No production build output was written.

Red evidence: both readers first failed on the absent shared fixture. Deliberate
one-sided mutations then failed (exit 1), naming a Go-only Item key, a TS-only
comparable field and a TS-only Item declaration key respectively. All three
mutations were restored and the final checks rerun successfully. This proves
the catalogue/runtime guard and the TypeScript declaration guard can fail.

No Postgres, UI/snapshot, full frontend build or full-repository gate was run;
the new lifecycle/transport does not exist yet, and the shared wave remains
active. The existing Vite warning about `__dirname` and a future native config
loader remains unchanged. No commit, branch change, push, merge or deploy was made.

Skills loaded: codebase-design (comparison seam), domain-modeling (three glossary
terms), code-review (standards/spec lens), tdd (red/drift proofs), and
writing-for-agents (this contract). The supplied to-spec/to-tickets skills were
read as context; the orchestrator owns publishing and the existing breakdown.
Deliberately not used: research/prototype/UI skills (no external fact or UI
work), diagnosing-bugs (no production bug fix), or pr (no PR change).
`karpathy-guideline` was not found in the installed skill paths. A formal
two-agent code-review/Judge pass is deferred to the quiescent team gate; this
ticket does not claim independent sign-off.
