# 001 — Quotation-to-PO allocation model

- **Status:** Accepted
- **Date:** 2026-11-22
- **Supersedes:** the narrow “link saved PO lines” approach in #636

## Context

Vera issues a quotation as immutable revisions and records the customer’s
purchase order against it. The business does not order a quotation in one step:
one quotation can produce several POs, one quotation line can be split across
POs, and several quotation items can be presented to the customer as one
aggregate PO line.

The existing PO model assumed the opposite shape: one active PO per quotation,
one PO line pointing to at most one quotation line, and a PO that could carry
free-text lines and its own discounts. Production exposed the cost of that
mismatch: three finalized POs carried Rp151.230.000 of revenue with no
quotation allocation, so the report could not resolve their cost and showed
zero capital and margin.

The business also needs to distinguish three different outcomes that the old
model conflated:

- quantity that is not bought yet and remains orderable;
- a price that was agreed below the quotation;
- quantity that will not be bought because the quotation is closed.

## Decision

1. **A PO is a set of allocations against one issued quotation revision.** One
   quotation may have many POs. A quotation-backed PO line owns exactly one
   allocation: quotation, exact source revision, item/sub-item, source
   quantity, source and ordered unit, frozen quotation price baseline, actual PO
   revenue and frozen supplier cost.

2. **One aggregate PO line may group one or more quotation-backed or extra
   lines.** The aggregate is a customer-facing presentation only. Its children
   remain the priced truth. There is one aggregate level; an aggregate cannot
   contain another aggregate.

3. **A PO line may also be an explicit extra expense line.** It has no
   quotation source and no allocation. It is a pass-through: the customer
   charge equals the expense and its margin is zero. It never affects the
   quotation ghosts, Unsold, Pipeline or quotation variance.

4. **The quotation owns pricing and cost.** New quotations use row discounts
   only; whole-order quotation discounts are legacy and block new POs until the
   quotation is revised. The PO has no discount, margin or adjustment input.
   Quantity, unit label, description wording and the negotiated unit price are
   editable.

5. **Bought state is derived, not stored as a flag.** Remaining quantity is the
   quotation quantity minus saved PO allocations, matched across revisions by
   item/sub-item identity. Drafts are advisory only. A full quotation disables
   new POs until it is revised. Item/sub-item numbers are the stable identity
   and must not be silently reused for a different item.

6. **Unsold, price loss and closed loss are different things.**
   - A quantity decrease is **Unsold** while the quotation is open. It stays
     orderable and is never subtracted from realized variance.
   - A price decrease is an immediate **price loss** on the sold units; a price
     increase is **price extra income**.
   - A quantity increase is **quantity extra income**.
   - Only an explicitly **closed** quotation turns its remaining Unsold into
     **closed loss**.

7. **Supplier cost, margin, and the report-level variance and extra-expense
   buckets stay internal.** Supplier cost and margin never appear on the
   customer PO, in the availability response, or in workspace DTOs. The
   workspace does show the structural line kind (`quotation` or `extra`, with an
   “Extra” badge) and the customer-side comparison of the negotiated price with
   the frozen quotation price baseline (price loss or price extra income). Those
   compare customer-facing prices and carry no supplier cost. The report remains
   the only place where supplier cost, margin, and the variance and
   extra-expense buckets are calculated, and the customer PO still prints only
   customer-facing prices.

## Consequences

- New allocation, aggregate-group and line-kind records are required, with
  correction snapshots and a provable historical backfill.
- The Actual report resolves allocated lines first, then legacy quotation
  references, then uncosted lines; Capital still rounds once per PO.
- The PO workspace becomes an allocation workspace: remaining-item ghosts,
  normal and aggregate lines, and an explicit extra-item action.
- The one-active-PO-per-quotation rule is removed; concurrency moves to a
  quotation-first lock order and a remaining-quantity check.
- Existing POs that cannot be proven to reference a quotation line remain
  readable and uncosted; they are never guessed into an allocation.
- The three production POs are corrected through the audited PO correction
  path, not raw SQL or an automatic migration.

## Alternatives considered

- **Header-only source link with a warning.** Rejected: it leaves the PO line
  without a cost identity and only relabels the uncosted symptom.
- **Free-text PO lines as the normal path.** Rejected: it makes cost
  untraceable. Extra items remain allowed only as explicit zero-margin extra
  expense.
- **Inferring a match from description, price or position.** Rejected: a wrong
  cost is worse than an unknown cost.
- **Keeping the whole-order quotation discount engine.** Rejected for new
  quotations; row discounts are the commercial model and the whole total is
  the sum of rows.
- **Treating every partial order as a loss.** Rejected: unbought quantity is
  still orderable and is Unsold, not loss.
- **Carrying supplier cost into the PO workspace.** Rejected: it violates the
  customer-facing boundary and the existing Story 42 rule.
