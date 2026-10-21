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
| Source missing | null           | all set            | `Source missing` badge |

`Source missing` is **independent of Draft/Saved**. The stored entry state is
still the record's own state, and the archive shows both. The row keeps its
customer PO number, customer snapshot, received date, currency, lines and
totals; `quote_number` and `source_revision` name the **former** source only.

The archive finds the row by the former Quotation number because search reads
`COALESCE(live quote number, former quote number)`. The independent filter is
`sourceMissing=only` / `sourceMissing=exclude` (default `any`); it counts as one
active filter and Reset clears it with the rest.

## Repair: Link Quotation

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

- Store: `ReviewPurchaseOrderSource`, `RepairPurchaseOrderSource`,
  `ListPurchaseOrders` in `backend/store`.
- HTTP: `POST /v1/purchase-orders/:id/source-repair/review`,
  `POST /v1/purchase-orders/:id/source-repair`,
  `GET /v1/purchase-orders?sourceMissing=only|exclude`.
- UI: `PurchaseOrderListPage` (badge, former reference, Source filter),
  `PurchaseOrderSavedView` (banner, Link Quotation), and the shared
  `PurchaseOrderRepairPage` / `PurchaseOrderSourceChange` flow.
