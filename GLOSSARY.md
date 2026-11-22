# Vera Business Suite

Vera is the quotation and purchase-order system for PT Arcon Penta Persada. It prices and issues quotations, records and reviews customer purchase orders, and verifies every issued revision.

## Language

### Parties and access

**Customer**:
The party a quotation or purchase order is for.
_Avoid_: Client (for the party), buyer, account

**Client record**:
The company's directory entry for a customer, with an immutable client code, addresses and contacts.
_Avoid_: Customer row, account

**Client directory**:
The Admin Clients tab; the list of client records.
_Avoid_: Customers (for the admin list)

**Client code**:
The stable short code that identifies a client record; it never changes.
_Avoid_: Customer ID, customer number

**Recipient**:
The person at the customer named on a quotation.
_Avoid_: Contact (alone)

**Customer contact**:
The person at the customer named on a purchase order, with the contact email and phone.
_Avoid_: Recipient (the quotation's contact), user

**User**:
A person who signs in to Vera.
_Avoid_: Account, operator

**Admin**:
A user with the right to manage the team, clients, company profile and backups.
_Avoid_: Superuser, owner

**Staff**:
A user who can create quotations and purchase orders but cannot administer the company.
_Avoid_: Normal user, operator

**Team**:
The set of user accounts in the company; the Admin Team tab.
_Avoid_: Staff (the non-admin subset)

**Team member**:
One user account in the team.
_Avoid_: User account, seat

**Sign-in email**:
The email a user signs in with; it is an identity, not a delivery channel.
_Avoid_: Login email, account email, contact email

### Workspace learning

**Features Tutorial**:
A progressive, interactive guide through one workspace's features. It opens automatically the first time an account enters that workspace on this device and can be replayed from that workspace's settings.
_Avoid_: Onboarding, product tour, walkthrough, guided tour

### Company identity

**Company**:
PT Arcon Penta Persada, the single business whose data Vera holds.
_Avoid_: Organization, tenant

**Company profile**:
The company's name, address, contact details, tax ID and logo; only the name and address print on quotations.
_Avoid_: Company settings, organization

**Brand theme**:
The company's colors for the quotation document and the verification portal; the two palettes are set independently.
_Avoid_: Skin, palette, theme (alone)

**Company logo**:
The artwork printed on quotations; the profile logo takes precedence over the built-in fallback.
_Avoid_: Avatar, brand mark

**NPWP**:
The company's Indonesian tax identification number; it is not printed on quotations.
_Avoid_: Tax ID

### Quotations

**Quotation**:
A priced offer to a customer, with a subject, line items, terms and taxes; it can be edited as a draft and issued as immutable revisions.
_Avoid_: Quote (noun), proposal, estimate

**Quotation draft**:
An editable, unissued working copy of a quotation; it is not a revision.
_Avoid_: Quote, revision, version

**Revision**:
An immutable issued snapshot of a quotation; revision 00 is the first issue and each later edit issues the next.
_Avoid_: Version, copy

**Revision number**:
The two-digit issue sequence of a revision, starting at 00.
_Avoid_: Version number, rev

**Quote number**:
The customer-facing number of a quotation, shared by its revisions and allocated per customer and issue year.
_Avoid_: Quotation number, reference number

**Issue date**:
The date a revision is issued; its validity runs from this date.
_Avoid_: Print date, creation date

**Issue year**:
The year a quotation is created and its quote number is allocated; numbers are never backdated.
_Avoid_: Quotation year

**Latest revision**:
The most recent issued revision of a quotation; its verification code is valid.
_Avoid_: Current version

**Open quotation**:
An issued quotation with no explicit close, so its unbought quantity still reports as Unsold and stays orderable.
_Avoid_: Active quotation, live quotation

**Closed quotation**:
A quotation an admin explicitly closed with a reason; its unbought quantity moves from Unsold to **Closed loss** until an admin reopens it.
_Avoid_: Cancelled quotation, expired quotation

**Expired (unclosed)**:
A quotation past its validity that no one closed; it still carries Unsold with a report badge and is not an automatic loss.
_Avoid_: Lapsed, written off

**Prepare**:
The validation and pricing of a quotation before export; it produces the review figures but issues no revision.
_Avoid_: Validate, calculate

**Export**:
Issuing the prepared quotation as the next immutable revision and rendering its customer document.
_Avoid_: Save, download, finalize

**Quotation archive**:
The Archive tab of the Quotation workspace; the list of issued quotation revisions.
_Avoid_: History, records

**Quotation review**:
The staff check of the internal breakdown before export.
_Avoid_: Preview, approval

**Customer preview**:
The customer-facing HTML rendering of a quotation, showing list prices and PPN.
_Avoid_: Preview (alone), customer view

**Quotation PDF**:
The rendered customer document for an issued revision.
_Avoid_: Printout, document (alone)

**Internal breakdown**:
The staff-only view of cost, margin, PPH23 and nett profit.
_Avoid_: Review (the step), breakdown (the customer document)

**Validity**:
The number of days a quotation remains open to the customer, counted from its issue date.
_Avoid_: Expiry, term

**Payment terms (quotation)**:
The number of days the customer has to pay, printed on a quotation.
_Avoid_: Payment period, credit terms

**Incoterm**:
The trade term (EXW, FOB, CIF, etc.) that allocates delivery responsibilities.
_Avoid_: Delivery term, shipping term

**Lead time**:
The delivery estimate on a quotation, entered in years, months, weeks and days.
_Avoid_: Delivery time

**Subject**:
The short title of a quotation.
_Avoid_: Title, description

**Line item**:
One priced row of a quotation, with description, quantity, unit, cost, margin and discount.
_Avoid_: Row, product

**Sub-item**:
An optional child line that breaks down a line item's price.
_Avoid_: Child item, detail

**Quotation depth**:
A quotation's presentation level (item, sub-item or sub-sub-item), preserved by an aggregate PO line for its quotation-backed children.
_Avoid_: Level, indent

**Unit**:
The unit of measure for a line item, such as a lot or a piece; a new line defaults to the company's canonical unit. The workspace labels the field UoM; in prose, use unit.
_Avoid_: Measure, unit type

**Quantity**:
The number of units on a line, possibly fractional.
_Avoid_: Qty

### Money and tax

**Cost**:
The supplier cost per unit of a line item; the company's buying price.
_Avoid_: Buy price, supplier price

**Margin**:
The percentage added to a line's cost to produce its list price; it is optional per line.
_Avoid_: Markup

**Margin amount**:
The sell total minus the cost total; it is signed and can be negative when a discount cuts below cost.
_Avoid_: Profit (the nett profit is after PPH23)

**List price**:
The price a customer sees for a line before any discount; cost plus margin.
_Avoid_: Marginalized price, marked-up price

**Line discount**:
A percentage off a line's list price; it never changes the line's cost.
_Avoid_: Row discount, markdown

**Quotation price baseline**:
The row-discounted quotation unit price frozen on an allocation when its PO line is allocated, stored as `quoted_unit_price_minor`. It is the basis for price variance and for valuing extra and unbought quantity; it is neither the list price before the line discount nor the supplier cost.
_Avoid_: Quoted price, list price, unit cost

**Quotation discount**:
The retired whole-order percentage off the subtotal, applied after all line discounts; it never changes cost. New quotations use row-only discounts. A **Legacy header discount** is a stored instance on an older revision, and that stored form still blocks a new PO.
_Avoid_: Whole discount, global discount

**Row-only discount**:
The pricing model for new quotations: only line discounts exist. The quotation total and margin are the exact sums of the line totals and line margins; there is no whole-order discount input.
_Avoid_: Whole discount, header discount

**Legacy header discount**:
The stored whole-order quotation discount on an older issued revision. Its presence blocks a new allocation-backed PO until the quotation is revised to row-only pricing.
_Avoid_: Quotation discount (for new data), global discount

**Discount**:
A price reduction on a line; it never changes cost. A new row-only quotation has no whole-order discount; the whole-order form is legacy only.
_Avoid_: Markdown, rebate

**Sell price**:
The price a customer pays for a line after its line discount, before PPN.
_Avoid_: Net price

**Line total**:
The sell price of a line multiplied by its quantity.
_Avoid_: Net line

**Quotation total**:
The quotation's sell total before PPN. On a new row-only quotation it is the exact sum of the line totals; a legacy revision subtracts its stored whole-order discount below that sum.
_Avoid_: Grand total (that includes PPN), whole total

**Subtotal**:
The sum of line totals before any whole-order quotation discount; on a new row-only quotation it equals the quotation total and the sell total. Only a legacy revision's stored discount sits between its subtotal and its sell total.
_Avoid_: Sell subtotal

**Sell total**:
The quotation's amount before PPN, after any whole-order discount. A new row-only quotation has no such discount, so it equals the quotation total; a legacy revision subtracts its stored discount here.
_Avoid_: Net total

**Customer total**:
The sell total plus PPN; the amount the customer pays and the quotation prints as its total.
_Avoid_: Grand total (the sell total does not include PPN)

**Cost total**:
The sum of the line costs; discounts never reduce it.
_Avoid_: Original total

**Nett profit**:
The sell total minus PPH23 and the cost total; it is signed and never includes PPN.
_Avoid_: Profit, net income

**PPN**:
The Indonesian value-added tax (currently 11%) added on top of the customer price for non-FTZ goods; it is printed on the quotation and is never company profit.
_Avoid_: VAT, sales tax

**PPH23**:
The Indonesian final income tax on services (currently 2% for non-construction and 1.75% for construction), calculated on the sell total and kept internal; it never prints.
_Avoid_: Income tax, withholding tax

**Tax basis**:
The tax zone, tax type and service category that determine PPN and PPH23.
_Avoid_: Tax profile, tax settings

**Tax zone**:
FTZ or Non-FTZ; it determines whether PPN applies to goods.
_Avoid_: Tax region, area

**Tax type**:
Goods or Services; the two categories of quotation line for tax.
_Avoid_: Tax category

**Service category**:
Construction or Non-construction; it sets the PPH23 rate for services.
_Avoid_: Service type, construction class

**FTZ**:
The Free Trade Zone; goods in this zone are not charged PPN.
_Avoid_: Free zone, bonded zone

**Non-FTZ**:
Any tax zone outside the Free Trade Zone; goods here are charged PPN.
_Avoid_: Domestic, local

**Company currency**:
The single currency in which the company issues quotations and purchase orders; currently Rupiah.
_Avoid_: Local currency, currency (alone)

**Rupiah (IDR)**:
The Indonesian currency used for every amount in Vera.
_Avoid_: Dollar, USD

### Purchase orders

**Purchase order (PO)**:
Vera's record of a customer's order. A new PO is a set of allocations against an issued quotation revision plus any explicit extra expense lines; a legacy PO may still be source-missing and keep its own entered values.
_Avoid_: Order, customer order

**Customer PO**:
The purchase order the customer sent; the customer's own order, not Vera's record.
_Avoid_: PO (for the customer's document), order

**Customer PO document**:
The single PDF of the customer's purchase order attached to a PO; it has retained versions but only one is current.
_Avoid_: Attachment, supporting file

**Customer PO document version**:
One retained upload of the customer PO document; replaced and removed versions stay viewable.
_Avoid_: Upload, attachment version

**PDF pending**:
The saved record's derived status when no customer PO document is active; it names the next evidence step and is not stored.
_Avoid_: Missing PDF, no PDF

**PDF attached**:
The saved record's derived status when one customer PO document version is active.
_Avoid_: Uploaded PDF, PDF complete

**Document chain**:
A PO's customer PO document and all its retained versions; a deletion request can remove the chain.
_Avoid_: Attachments

**Customer PO number**:
The number printed on the customer's purchase order; it is not the confirmation number.
_Avoid_: PO number (for Vera's record), order number

**Confirmation number**:
The optional reference Vera issues to confirm the order, which the customer's system receives; once assigned it does not change.
_Avoid_: PO number, confirmation code

**Number warning**:
The flag raised when a quote or confirmation number is close to the end of its customer-year sequence.
_Avoid_: Overflow warning

**Customer-year sequence**:
The per-customer counter, reset each year, from which quote and confirmation numbers are drawn.
_Avoid_: Sequence number

**PO date**:
The date the customer issued the purchase order.
_Avoid_: Order date

**Received date**:
The date Vera received the customer's purchase order, in WIB.
_Avoid_: PO date

**Requested delivery date**:
The delivery date the customer asks for on the purchase order.
_Avoid_: Delivery date, lead time

**WIB**:
Western Indonesian Time (UTC+7), the timezone for Vera's dates.
_Avoid_: Local time, Jakarta time

**Ship-to**:
The customer's receiving location for the order, identified by a ship-to code and a delivery address.
_Avoid_: Delivery address (the address alone)

**Billing address**:
The customer's invoice address on the purchase order.
_Avoid_: Invoice address

**Payment terms (purchase order)**:
The free-text payment instruction the customer's purchase order carries; it may differ from the quotation's payment days.
_Avoid_: Payment period, credit terms

**PO draft**:
A purchase order that has not been finalized; it is editable and can be discarded.
_Avoid_: Draft PO, unsaved PO

**Finalized PO**:
A purchase order saved after the difference review; its values and totals are fixed, but its customer PO document stays attachable, replaceable and removable as versioned evidence.
_Avoid_: Saved PO, approved PO

**Finalize**:
To save a purchase order as a finalized record after the review.
_Avoid_: Save, approve, export

**PO archive**:
The Archive tab of the Purchase Order workspace; the list of finalized purchase orders.
_Avoid_: History, records

**PO line**:
One priced leaf of the customer's order: either quotation-backed (one allocation to a quotation item or sub-item) or an explicit extra expense line. It has a customer-facing description, ordered quantity, unit label and negotiated price. An **Aggregate PO line** is a presentation group of leaves, not a third priced kind.
_Avoid_: Item, row

**Quotation-backed PO line**:
A PO line that fulfils one quotation item or sub-item. It owns exactly one allocation and drives bought state, Unsold and quotation variance.
_Avoid_: Matched line, linked line

**Extra expense line**:
A PO line that was never on the quotation. It has no allocation, is a pass-through (customer charge equals expense), and has zero margin. It never affects ghosts, bought state, Unsold, Pipeline or quotation variance.
_Avoid_: Free line, manual line, extra item

**Allocation**:
The canonical link between one quotation-backed PO line and one quotation revision item or sub-item. It records the source quantity, the **Source unit** and **Ordered unit**, the **Quotation price baseline**, the actual PO revenue and the frozen supplier cost.
_Avoid_: Match, reference, link (alone)

**Bought state**:
The derived state of a quotation item after saved PO allocations are counted. It is not a manual flag and not affected by drafts.
_Avoid_: Ordered flag, purchased flag

**Remaining quantity**:
The latest issued revision's quotation quantity minus the saved PO allocation quantity for an item or sub-item, matched across revisions. It never goes below zero: an item removed from the latest revision is absent rather than negative, and when the latest revision's quantity is below the bought quantity the difference clamps to zero and reports as **Over-bought**. It is the quantity a ghost may offer for a later PO.
_Avoid_: Leftover, balance, unsold quantity

**Aggregate PO line**:
A customer-facing PO line that groups one or more quotation-backed lines of any quotation depth and optionally extra lines. It is a presentation group, not itself a priced leaf. The group has its own title and derived total; its children remain the priced truth. There is one aggregate level only.
_Avoid_: Aggregate display (the report layout), package, bundle

**Aggregate child**:
One priced leaf inside an aggregate PO line. A quotation-backed child keeps its own description, quantity, unit, price, **Quotation depth** and allocation; an extra child has no allocation and no quotation depth.
_Avoid_: Sub-item (a quotation depth), child line

**Source unit**:
The unit the quotation states for the source item or sub-item, stored on the allocation as the quotation-side label. Like the ordered unit it is a display label; quantity is the numeric truth and there is no conversion.
_Avoid_: Quotation UoM, original unit

**Ordered unit**:
The unit label on a PO line as the customer's order states it. Quantity is the numeric truth and the ordered unit is a display label only; there is no conversion between source and ordered units.
_Avoid_: Converted unit, PO UoM

**Ordered quantity**:
The quantity the customer ordered on a PO line; it may differ from the quotation.
_Avoid_: Order quantity, qty

**Ghost**:
A quotation item or sub-item offered in the PO workspace while it has remaining quantity; choosing it creates a pre-filled quotation-backed line. A draft does not remove one.
_Avoid_: Remaining item, shortcut

**All bought**:
The derived state of a quotation whose latest issued revision has no remaining quantity on any leaf; Create PO is disabled until the quotation is revised.
_Avoid_: Fully ordered, exhausted

**Over-bought**:
The state of an item or sub-item whose saved allocations exceed the latest revision's quantity; the excess is quantity extra income and a further PO may not buy it without a quotation revision.
_Avoid_: Over-order (alone), excess stock

**Source**:
The issued quotation revision a purchase order is linked to. A new allocation-backed PO cannot be created without one; entering a purchase order before a source exists is a legacy path. The customer PO document is attached evidence, not the source.
_Avoid_: Origin, reference, parent, customer PO document

**Source revision**:
The exact issued quotation revision a purchase order follows.
_Avoid_: Parent revision, linked quote

**Source selector**:
The page where staff choose the quotation revision for a purchase order, a source link or an admin source repair.
_Avoid_: Picker, dropdown

**Used revision**:
A quotation revision that already carries saved PO allocations. It stays selectable while any leaf has remaining quantity; it is not disabled merely because a PO points at it.
_Avoid_: Taken quotation

**Manual entry**:
Typing purchase-order values without a quotation source. It is a legacy path for historical POs and older desktop versions; a new PO is created from remaining quotation items, with an explicit extra expense line for anything not quoted.
_Avoid_: Manual PO, standalone PO

**Source missing**:
The state of a purchase order with no live source. It covers a draft that was never linked and a record whose former source quotation was deleted; the record keeps its own values, and only a formerly-linked record keeps a former source.
_Avoid_: Broken PO, orphan, unlinked PO

**Former source**:
The quote number and revision a source-missing purchase order used to follow.
_Avoid_: Old quote

**Source link**:
The Staff action that links a source-missing draft to an issued quotation revision through the difference review. It preserves the draft's entered values, needs no reason, keeps no repair history, and is not offered once a draft has a live source.
_Avoid_: Relink, source repair, attach

**Tax choice**:
The explicit decision a source link makes between keeping the draft's tax basis and adopting the quotation revision's. PPN is recomputed from the chosen basis.
_Avoid_: Tax override, tax sync

**Source repair**:
The admin action that links a finalized source-missing purchase order to a new quotation revision, with a reason and a repeated review.
_Avoid_: Relink, fix, source link

**PO version**:
The number that advances on every write to a purchase order; a save with a stale version is a version conflict.
_Avoid_: Revision, edit number

**Version conflict**:
A save whose purchase-order version is no longer current; the editor chooses to overwrite or reload.
_Avoid_: Collision, race

**Overwrite**:
The deliberate choice in a version conflict to replace the stored draft with the editor's values.
_Avoid_: Replace, force save

**PO correction**:
An edit to a finalized purchase order that advances its version, requires a reason and keeps the previous values in history.
_Avoid_: Edit, overwrite

**PO list page size**:
The number of purchase orders shown per page in the Drafts and Archive lists; it is a per-user default.
_Avoid_: Rows per page, limit

**PO defaults**:
The payment terms, incoterms, received date and list page size applied to a new purchase order where the source supplies no value.
_Avoid_: Preferences, presets

### PO review and evidence

**Reconciliation session**:
The retained difference review for one purchase order and its exact source revision, including the values checked and the staff decisions.
_Avoid_: Review run, reconciliation log

**Review notice**:
Temporary feedback that a checked purchase-order field is empty or matches the source; it is not a retained decision.
_Avoid_: Toast, review item

**Review item**:
A retained purchase-order difference or line-match issue, with its values and unresolved or resolved decision history.
_Avoid_: Notice, toast

**Difference review**:
The mandatory comparison of the source revision and the customer PO, where every differing value is chosen or adjusted before finalize.
_Avoid_: Reconciliation, comparison

**Line match**:
The legacy review-time name for the link between a quotation line and a customer PO line. New records persist that link as an **Allocation**, never as a line match.
_Avoid_: Mapping, pairing

**Difference**:
A field or line where the customer PO and the source quotation disagree.
_Avoid_: Discrepancy, conflict (the version conflict is separate)

**Choice**:
A staff decision on a difference: keep the quotation value or take the customer PO value. Adjusting is a legacy path; a new allocation-backed PO has no adjustment input.
_Avoid_: Selection, resolution

**Adjustment**:
A deliberate signed money correction on a legacy purchase order, with a written reason. A new allocation-backed PO has no adjustment input.
_Avoid_: Override, manual price, correction (the PO correction is the saved edit)

**PO totals**:
The finalized purchase order's own totals. For an allocation-backed PO they are derived from the lines (quantity × negotiated price, PPN and grand total) with zero discount or adjustment; legacy POs keep their stored totals. They are not copied from the quotation.
_Avoid_: Copied totals, quotation totals

**Printed totals**:
The subtotal, discount, PPN and grand total the source revision printed, kept for comparison with the PO's numbers.
_Avoid_: Source totals, original totals

**No-document confirmation**:
The deliberate acknowledgement, on a first final save with no customer PO PDF, that the order has no document.
_Avoid_: Skip upload, PDF waiver

### Purchase order reporting

**Actual**:
The reporting view of saved purchase orders: the revenue, cost, capital and margin actually recorded. It excludes drafts, Unsold and every estimate.
_Avoid_: Real revenue, booked revenue

**Pipeline**:
The reporting view of Unsold quantity on open quotations. It is still orderable and is never added to Actual.
_Avoid_: Forecast, expected revenue

**Quotation variance**:
The comparison of saved PO prices and quantities against the **Quotation price baseline**: quantity extra income, price extra income, price loss and closed loss. Open Unsold is excluded from its net total.
_Avoid_: PO variance, forecast error

**Business category**:
The reporting line a purchase order belongs to: FM (Facility Management), FAB (Fabrication), SUPPLY_GEN (Goods Supply & General) or OTHERS. It is chosen on the PO and required before final save; it is never inferred from the subject or the customer.
_Avoid_: Report category, cost category, tax category

**Unassigned**:
The report bucket for a saved purchase order with no business category: a record saved before the category existed, or a draft that has not chosen one. It is a visible bucket, not a missing value to drop.
_Avoid_: Unknown category, empty category

**PO amount**:
A purchase order's sell total for reporting: its subtotal less any legacy discount plus any legacy adjustment, excluding PPN. An allocation-backed PO has neither, so its PO amount is its subtotal. It is not the customer total, which includes PPN.
_Avoid_: Revenue, grand total, customer total

**Costed amount**:
The part of the PO amount carried by lines whose exact source cost resolved. An allocation-backed line keeps its frozen cost snapshot after its quotation is deleted, so its historical cost remains known. A live source that no longer matches the allocation is not trusted; a legacy or unresolved line with no allocation or no readable cost contributes no costed amount.
_Avoid_: Known revenue, matched amount

**Uncosted amount**:
The PO amount minus the costed amount and the extra expense. It is disclosed as incomplete evidence and is never counted as margin.
_Avoid_: Pure profit, missing cost, unknown revenue

**Capital**:
The pinned source revision's cost for the PO's ordered quantity: a line's original unit cost, or a sub-item's unit cost, multiplied by the PO's quantity and rounded once per PO. It is not the quotation's total, not its latest revision, and never a sell field.
_Avoid_: Cost total, quotation cost, supplier value

**PO margin**:
The costed amount minus Capital; it is signed and can be negative when the negotiated price falls below the cost. A new allocation-backed PO has no discount or adjustment, so no whole-order step sits between its line margins and the PO margin; PPH23 is not subtracted because this is a purchase-order margin, not the quotation's nett profit.
_Avoid_: Nett profit, gross profit, quotation margin

**PO margin %**:
PO margin divided by Capital, weighted across the period and expressed in basis points; it is null when Capital is zero.
_Avoid_: Markup, profit rate, average of line percentages

**Coverage**:
The costed amount divided by the non-extra PO amount: the PO amount minus extra expense, because extra expense is an intentional zero-margin pass-through. It shows how much of that sell value has a known cost, and it is null when the denominator is zero or below.
_Avoid_: Confidence, completeness score

**Unsold**:
The value and margin of quoted quantity not yet bought on an **Open quotation**, valued at the **Quotation price baseline** and its frozen cost. It stays orderable in the ghost list, is never added to Actual and is never a loss; a later PO may buy it.
_Avoid_: Loss, shortfall (while open), unbought profit

**Quantity extra income**:
The value and margin of quantity bought above the quotation quantity. The excess is valued at the **Quotation price baseline** and its frozen cost; the item becomes **Over-bought** and needs a quotation revision for more.
_Avoid_: Over-order profit, surplus revenue

**Price extra income**:
The amount by which the negotiated PO price of sold quantity exceeds the **Quotation price baseline**. It has no cost effect and is pure margin.
_Avoid_: Markup, discount reversal

**Price loss**:
The amount by which the negotiated PO price of sold quantity is below the **Quotation price baseline**. It is realized on those sold units and does not disappear if the remaining quantity is bought later.
_Avoid_: Discount, markdown

**Closed loss**:
The value and margin of unbought quantity after a quotation is explicitly closed, valued at the **Quotation price baseline** and its frozen cost. Unlike Unsold it is a loss, and it stays final until an admin explicitly reopens the quotation.
_Avoid_: Unsold, expired loss, automatic write-off

**Extra expense**:
The customer charge of an extra expense line, which is also its expense; margin is zero. It is reported separately and never enters quotation margin, Unsold, Pipeline or quotation variance.
_Avoid_: Extra income, free profit, unquoted margin

**Complete month**:
A calendar month that has ended before both the report end month and the as-of month in WIB. Months between the first observed PO month and the last complete month count even when they contain no orders; earlier months are not invented.
_Avoid_: Full month, closed month

**Current-period projection**:
The forecast for the month or year still in progress, derived from elapsed days or complete trailing months with a stated method and sample. It is an estimate, never actual evidence, and never enters an actual total.
_Avoid_: PO projection, forecast actual, trend, confidence interval

**Current month summary**:
The server-owned readout of the current calendar month: month-to-date actuals, recorded future-dated evidence, and the current-period remainder. Actual, recorded and estimated figures are separate quantities, and none is added to another.
_Avoid_: Monthly total, projected actual, forecast total

**Recorded future-dated evidence**:
A saved purchase order dated after the report's as-of day but inside its month. It is recorded evidence, not an estimate; it is shown separately and is never added to a forecast.
_Avoid_: Future actual, forecast, expected order

**Aggregate display**:
A reporting layout that shows the selected section's combined rows from the server-supplied snapshot. It is a display selection, not a newly calculated total and not a change to the money definitions. It is not the **Aggregate PO line**, which is a customer-facing PO grouping.
_Avoid_: Aggregate total, summed view, combined actual, aggregate PO line

### Verification and retention

**Verification**:
The public check of a printed verification code against the stored revision.
_Avoid_: Authentication, validation

**Verification code**:
The short code and QR printed on a quotation that identifies one revision; it keeps verifying across later revisions.
_Avoid_: Signature, hash, token

**Verification portal**:
The public web page that verifies a code and shows the customer document.
_Avoid_: Customer portal, website

**Valid**:
The verification outcome when the code matches the latest revision of its quotation.
_Avoid_: Authentic, genuine

**Superseded**:
The verification outcome when the code matches an older revision that a newer revision has replaced.
_Avoid_: Expired, invalid

**Mismatch**:
The verification outcome when the code resolves to a revision whose stored content no longer matches the code.
_Avoid_: Tampered (a cause, not the outcome)

**Not found**:
The verification outcome when the code resolves to no revision.
_Avoid_: Missing, unknown

**Backup**:
A restorable copy of the company's data taken for disaster recovery; the Admin Backups tab manages it.
_Avoid_: Archive, export

**Restore**:
Replacing current company data with a backup.
_Avoid_: Import, rollback

**Deletion request**:
A request to permanently delete a quotation, purchase order or document chain; an admin approves it.
_Avoid_: Delete, purge

**Mock fixture**:
A clearly labelled fictional record created only in an explicitly confirmed local database for manual testing; it is not an issued document and carries synthetic PDFs.
_Avoid_: Test data, demo data, sample data

### Workspaces and settings

**Home**:
The global landing page that chooses a workspace; it is outside every workspace and the only place to switch between them.
_Avoid_: Dashboard, main menu

**Workspace**:
A top-level area with its own brand and tabs: Quotation, Purchase Order or Admin Dashboard.
_Avoid_: Module, section

**Quotation workspace**:
The workspace for the quotation Editor, Drafts and Archive tabs.
_Avoid_: Quotation module

**Purchase Order workspace**:
The workspace for the PO Editor, Drafts and Archive tabs.
_Avoid_: PO module

**Admin Dashboard**:
The admin workspace containing the Client directory, Team, Company profile and Backups tabs; those tabs live in the shell's primary navigation, not in the page body.
_Avoid_: Admin (the role), dashboard

**Tab**:
A page inside a workspace; only the current workspace's tabs are shown.
_Avoid_: Page, screen

**Editor**:
The workspace tab where a new or active quotation or purchase order is entered.
_Avoid_: Form, entry screen

**Drafts**:
The workspace tab listing saved unissued quotations or unfinalized purchase orders.
_Avoid_: Queue, saved items

**Archive**:
The workspace tab listing issued quotation revisions or finalized purchase orders; it is a view, not a copy or a backup.
_Avoid_: History, backup

**Profile menu**:
The user menu with Account settings and Sign out everywhere. Inside Quotation or Purchase Order it also offers that workspace's own settings — Quotation settings or Purchase Order settings — and a settings page keeps the entry of the workspace it was opened from. The Admin Dashboard has no settings entry.
_Avoid_: User menu, account menu

**Account settings**:
The user's General and Keyboard preferences; it does not hold the quotation or PO defaults.
_Avoid_: Settings (alone), profile

**Quotation settings**:
The profile page where a user sets their Quotation defaults.
_Avoid_: Preferences, presets

**Quotation defaults**:
The validity, payment terms, tax basis and subject applied to a new quotation.
_Avoid_: Preferences, presets

**PO settings**:
The profile page where a user sets their PO defaults.
_Avoid_: Preferences, presets

### Product surfaces and future documents

**Desktop app**:
The staff application used to create and manage quotations and purchase orders.
_Avoid_: App, client

**CLI**:
The `vera` command-line interface for the same quotation and purchase-order operations, used for automation and local checks.
_Avoid_: Command line, tool

**API server**:
The shared service that the desktop app, CLI and verification portal use; it owns the company data.
_Avoid_: Backend (for the whole system)

**Delivery Order**:
A document authorizing delivery, planned to follow a purchase order; it is not implemented in this release.
_Avoid_: DO, delivery note

**Invoice**:
A request for payment, planned to follow a purchase order; it is not implemented in this release.
_Avoid_: Bill
