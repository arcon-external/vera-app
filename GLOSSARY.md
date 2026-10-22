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
The staff-only view of cost, margin, PPH22 and nett profit.
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

**Unit**:
The unit of measure for a line item, such as a lot or a piece; a new line defaults to the company's canonical unit.
_Avoid_: UoM, measure

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
_Avoid_: Profit (the nett profit is after PPH22)

**List price**:
The price a customer sees for a line before any discount; cost plus margin.
_Avoid_: Marginalized price, marked-up price

**Line discount**:
A percentage off a line's list price; it never changes the line's cost.
_Avoid_: Row discount, markdown

**Quotation discount**:
A percentage off the subtotal, applied after all line discounts; it never changes cost.
_Avoid_: Whole discount, global discount

**Discount**:
A price reduction, either per line or on the quotation; it never changes cost.
_Avoid_: Markdown, rebate

**Sell price**:
The price a customer pays for a line after its line discount, before PPN.
_Avoid_: Net price

**Line total**:
The sell price of a line multiplied by its quantity.
_Avoid_: Net line

**Subtotal**:
The sum of line totals before the quotation discount.
_Avoid_: Sell subtotal

**Sell total**:
The subtotal after the quotation discount, before PPN.
_Avoid_: Net total

**Customer total**:
The sell total plus PPN; the amount the customer pays and the quotation prints as its total.
_Avoid_: Grand total (the sell total does not include PPN)

**Cost total**:
The sum of the line costs; discounts never reduce it.
_Avoid_: Original total

**Nett profit**:
The sell total minus PPH22 and the cost total; it is signed and never includes PPN.
_Avoid_: Profit, net income

**PPN**:
The Indonesian value-added tax (currently 11%) added on top of the customer price for non-FTZ goods; it is printed on the quotation and is never company profit.
_Avoid_: VAT, sales tax

**PPH22**:
The Indonesian final income tax on services (currently 2% for non-construction and 1.75% for construction), calculated on the sell total and kept internal; it never prints.
_Avoid_: Income tax, withholding tax

**Tax basis**:
The tax zone, tax type and service category that determine PPN and PPH22.
_Avoid_: Tax profile, tax settings

**Tax zone**:
FTZ or Non-FTZ; it determines whether PPN applies to goods.
_Avoid_: Tax region, area

**Tax type**:
Goods or Services; the two categories of quotation line for tax.
_Avoid_: Tax category

**Service category**:
Construction or Non-construction; it sets the PPH22 rate for services.
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
Vera's record of a customer's order; it always follows a source (an issued quotation revision or a customer PO document) and is finalized after review.
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

**Document chain**:
A PO's customer PO document and all its retained versions; a deletion request can remove the chain.
_Avoid_: Attachments

**Customer PO number**:
The number printed on the customer's purchase order; it is not the confirmation number.
_Avoid_: PO number (for Vera's record), order number

**Confirmation number**:
The optional reference a customer's system returns to confirm the order; once assigned it does not change.
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
A purchase order saved after the difference review; its totals and document state are fixed.
_Avoid_: Saved PO, approved PO

**Finalize**:
To save a purchase order as a finalized record after the review.
_Avoid_: Save, approve, export

**PO archive**:
The Archive tab of the Purchase Order workspace; the list of finalized purchase orders.
_Avoid_: History, records

**PO line**:
One line of the customer's order, matched to a quotation line or added, with an ordered quantity and price.
_Avoid_: Item, row

**Ordered quantity**:
The quantity the customer ordered on a PO line; it may differ from the quotation.
_Avoid_: Order quantity, qty

**Source**:
The issued quotation revision or customer PO document a purchase order is built from.
_Avoid_: Origin, reference, parent

**Source revision**:
The exact issued quotation revision a purchase order follows.
_Avoid_: Parent revision, linked quote

**Source selector**:
The page where staff choose the quotation revision for a purchase order or a source repair.
_Avoid_: Picker, dropdown

**Used revision**:
A quotation revision already linked to a purchase order; the source selector shows it disabled.
_Avoid_: Taken quotation

**Manual entry**:
Typing purchase-order values after choosing a source, as opposed to extracting them from a customer PO PDF.
_Avoid_: Manual PO, standalone PO

**Source missing**:
The state of a purchase order whose source quotation or customer PO document is no longer available; its own values and former source remain.
_Avoid_: Broken PO, orphan

**Former source**:
The quote number and revision a source-missing purchase order used to follow.
_Avoid_: Old quote

**Source repair**:
The admin action that links a source-missing purchase order to a new quotation revision, with a reason and a repeated review.
_Avoid_: Relink, fix

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

**Difference review**:
The mandatory comparison of the source revision and the customer PO, where every differing value is chosen or adjusted before finalize.
_Avoid_: Reconciliation, comparison

**Line match**:
The link between a quotation line and a customer PO line during the difference review.
_Avoid_: Mapping, pairing

**Difference**:
A field or line where the customer PO and the source quotation disagree.
_Avoid_: Discrepancy, conflict (the version conflict is separate)

**Choice**:
A staff decision on a difference: keep the quotation value, take the customer PO value, or adjust.
_Avoid_: Selection, resolution

**Adjustment**:
A deliberate signed money correction to a purchase order, with a written reason.
_Avoid_: Override, manual price, correction (the PO correction is the saved edit)

**PO totals**:
The finalized purchase order's own subtotal, discount, PPN, adjustment and grand total; they are not copied from the quotation.
_Avoid_: Copied totals, quotation totals

**Printed totals**:
The subtotal, discount, PPN and grand total the source revision printed, kept for comparison with the PO's numbers.
_Avoid_: Source totals, original totals

**No-document confirmation**:
The deliberate acknowledgement, on a first final save with no customer PO PDF, that the order has no document.
_Avoid_: Skip upload, PDF waiver

### Extraction and review

**Extraction**:
Reading a customer PO PDF into a purchase-order draft; it may use text extraction or OCR.
_Avoid_: Import, parsing

**OCR**:
The text recognition used for scanned customer PO pages.
_Avoid_: Scanning, auto-OCR

**Extracted value**:
A value read from the customer PO that must be reviewed before it becomes part of the purchase order.
_Avoid_: Imported value, parsed value

**Extraction confidence**:
How reliably a value was read from the PDF; low-confidence values always need review.
_Avoid_: Score, certainty

**Extraction conflict**:
A value the extractor cannot resolve safely, so staff must choose or correct it.
_Avoid_: Parse error, mismatch

**Extraction review**:
The mandatory check of every extracted value; there is no auto-accept mode.
_Avoid_: Auto-OCR, auto-accept

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
The user menu with Account settings and Sign out everywhere. Inside a workspace it also offers that workspace's own settings — Quotation settings in Quotation, Purchase Order settings in Purchase Order — and a settings page keeps the entry of the workspace it was opened from.
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
