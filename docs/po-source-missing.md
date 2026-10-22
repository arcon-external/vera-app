# Purchase Orders with a missing source (#481)

A Purchase Order follows one exact issued Quotation revision. Deleting that
Quotation does not delete the order: #480 releases the link and leaves a
readable survivor with a **former-source reference**. This note records how that
state is shown, filtered and repaired, and the downstream rule that applies
until it is repaired.

## What the state is

| Row shape      | `quotation_id` | `former_*` columns | Archive |
| -------------- | -------------- | ------------------ | ------- |
| Live link      | set            | all null           | normal  |
| Former source  | null           | all set            | `Source missing` badge |
| Never linked   | null           | all null           | `Source missing` badge; draft only |

`Source missing` covers both a draft that was never linked (#496) and a record
whose former source quotation was deleted; the latter keeps its former-source
reference. It is **independent of Draft/Saved** for a formerly-linked record.
A never-linked row is **draft-only** at the schema level: finalizing it or
reviewing it requires a live source first. A linked revision `00` is a real live
link, not an absence.

The row keeps its customer PO number, customer snapshot, received date, currency,
lines and totals. A formerly-linked row's `quote_number` and `source_revision`
name the **former** source only; a never-linked row has no source identity at
all.

The archive finds the row by the former Quotation number because search reads
`COALESCE(live quote number, former quote number)`. The independent filter is
`sourceMissing=only` / `sourceMissing=exclude` (default `any`); it counts as one
active filter and Reset clears it with the rest.

## Draft link: Link a Quotation (Staff, #497)

A **draft** with no live source is linked by Staff through the Source link panel
at the top of the editor:

1. Staff picks the exact issued revision with the shared **entry** selector. The
   selector is not repair mode: no admin gate and no `excludePOId`.
2. The editor saves the working copy as an ordinary draft first, so the review
   and the link compare the values the record will actually keep.
3. The review repeats the #471 matching and difference rules. A manual line is
   extra and an unclaimed source line is missing until it is explicitly matched
   or acknowledged; matches are by quotation reference, never by position.
4. Source identity (customer, quote number, revision, quotation line references,
   printed totals) comes from the revision. Every PO-owned value — number, dates,
   addresses, ship-to, contact, payment terms, Incoterm, lines, prices,
   discounts, adjustment — stays as entered. A changed customer also brings the
   revision's billing address, exactly as the saved-record repair does.
5. The draft and revision must be in one currency; a mismatch is `422`
   (`po_currency_mismatch`) and is never converted. A never-linked draft's
   currency can be edited before retrying; a formerly-linked draft's basis is
   frozen, so another revision in its currency must be chosen.
6. A differing tax basis needs an explicit `keep`/`source` choice (`422`
   `po_tax_choice_required` when missing). The server recomputes PPN from the
   chosen basis, and the basis is frozen again after the link.
7. Occupancy and the version are rechecked in the transaction: an occupied
   target is `409` (`po_target_occupied`), a stale version is `409`
   (`po_version_conflict`), and no live draft is re-linked. A saved record is
   refused as `409` (`po_not_draft`), not by the admin gate.
8. A draft link is an ordinary draft write. It needs no reason and writes no
   correction snapshot or `036` repair satellite. It is **not** a finalize
   review: Finalize still requires a live source and its own completed review.

## Finalized repair: Link Quotation (Admin)

1. An **admin** opens the saved record. Staff see the banner and no action; the
   API answers a non-admin with `403` and `code: "po_repair_forbidden"` — the
   route gate and the store return the same typed refusal.
2. The admin picks the new source through the same three-step selector used by
   ordinary entry, in repair mode. Used Quotations stay visible but disabled;
   the record being repaired has no live Quotation to exempt.
3. The saved record's **current values are the starting point**. The review
   repeats the #471 matching and difference rules against the target revision
   (no choice preselected), and the admin can correct line matches.
4. A **reason** is required. The write records the replaced values, the actor
   and time, the old and new source and every difference choice.
5. An **assigned confirmation number is preserved** for a same-customer repair.
   A different customer is refused once the number exists, because the number
   carries the original customer's code.
6. The target is rechecked inside the transaction: an occupied target is a
   `409` (`po_target_occupied`), a stale version is `409`
   (`po_version_conflict`), and the unique index is the race backstop.

After the repair the former columns are cleared, the record is a live link
again, and the archive's `sourceMissing=only` no longer returns it.

## Downstream rule (not implemented in this release)

**No new Delivery Order or Invoice can be created from a Purchase Order whose
source is missing. The source must be repaired first.** The saved view states
this beside the banner. Vera does not create Delivery Orders or Invoices in
this release; the rule is recorded here so the later modules inherit it rather
than inventing their own answer.

## Seams

- Store: `PreviewPurchaseOrderDraftLink`, `LinkPurchaseOrderDraftSource`,
  `ReviewPurchaseOrderSource`, `RepairPurchaseOrderSource`,
  `ListPurchaseOrders` in `backend/store`.
- HTTP: `POST /v1/purchase-orders/:id/source-link/review`,
  `POST /v1/purchase-orders/:id/source-link`,
  `POST /v1/purchase-orders/:id/source-repair/review`,
  `POST /v1/purchase-orders/:id/source-repair`,
  `GET /v1/purchase-orders?sourceMissing=only|exclude`.
- UI: `PurchaseOrderListPage` (badge, former reference, Source filter),
  `PurchaseOrderSourceLinkPanel` and `PurchaseOrderDraftLinkReviewPanel` (Staff
  draft link), `PurchaseOrderSavedView` (banner, Link Quotation), and the shared
  `PurchaseOrderRepairPage` / `PurchaseOrderSourceChange` flow.
