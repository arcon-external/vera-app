# Changelog

Staff-facing release notes for Vera. Entries use plain language and only
describe user-visible changes; internal chores never appear here. Sections
per version: Added, Changed, Fixed, Security.

## [0.13.0] - 2026-10-15

### Added

- **Every settings tab saves itself.** Company settings' four sub-tabs each have
  their own save button — General saves the company profile, Units saves the
  unit list, Quotation saves the quotation template (its wording and its
  colours), Portal saves the portal colours. A save in one tab can no longer
  carry another tab's unfinished edits, and leaving a tab with unsaved changes
  says so and stays put.

### Changed

- **The Quotation tab's colours sit above the document wording**, so the order
  runs preview, colours, wording — the colours you change are the ones the
  preview beside them shows.
- **The document wording saves with its tab's button now**, instead of saving
  by itself a moment after you stop typing. "Save Quotation Template" saves the
  wording and the colours together.

### Fixed

- **The template preview no longer comes up blank.** The preview showed the
  document as a PDF inside a frame, which some systems refused to paint; it now
  shows the document's own HTML — the same template the print engine runs —
  and the Download sample still saves the real PDF.
- **The colour panels say what stays fixed.** The quotation's white page and
  green discount row, and the portal's verdict colours (valid, superseded,
  mismatch, not found), are not among the four editable colours — and the
  panels now say so, so no rebrand can wash out the paper or make one verdict
  look like another.

## [0.12.1] - 2026-10-14

### Fixed

- **The Quotation settings are one tab now, with the preview on top.** The wording
  editor, the four quotation colours and the document preview live under a single
  Quotation sub-tab, in the same shape as the Portal tab.
- **The template preview no longer comes up blank.** The preview could mount before
  the wording loaded and then never re-render; it now renders as soon as the wording
  arrives, and a failed render says why instead of showing nothing.
- **The company currency and canonical-unit dropdowns are styled** like the rest of
  the form. Their dropdown arrows now actually paint (the arrow image was missing a
  required attribute and rendered nothing, silently).
- **The units editor is labelled**: each row shows "Value — stored" and "Label —
  printed", and the row controls sit in a tidy footer on narrow windows.

## [0.12.0] - 2026-10-14

### Added

- **The Template tab now shows the real document while you edit the wording.** Change a
  label, leave the field, and the quotation PDF beside it re-renders with your words —
  the same document a customer receives, with a Download sample button. The sample is
  marked (a fake quote number, a code that verifies as nothing), and previewing can
  never affect a real quotation.

## [0.11.0] - 2026-10-14

### Added

- **Your quotation can now say what you want it to say.** A new **Template** tab
  in Company settings holds every customer-facing label — the title, the
  "Quotation to" heading, the terms heading, the four meta labels, the seven
  column headers, the four totals labels, the closing sentence and the four
  terms sentences. Each field is named after the wording it prints today, and
  empty a field to put that wording back. Type over a value and it saves by
  itself a moment after you stop typing, with a *Saved* readout; a value the
  system would refuse is held and the field is marked instead. Quotations that
  were already issued keep the wording they were exported with.

- **The verification page's colours can now be judged before they are saved.**
  The Portal tab shows the verification page beside its four colours, one state
  at a time — valid, superseded, mismatch, not found, unavailable — so a colour
  choice cannot make one state look like another. The page shown is the real
  one, rendered from the same template your customer sees, and it updates a
  moment after you stop adjusting a colour. The wording of the portal is
  deliberately fixed: "this document does not match our records" has to stay
  exactly that unambiguous.

- **Units of measure are yours to choose.** Company settings gains a **Units**
  tab: the list your quotations offer, plus a canonical unit that a package
  line is quoted in. Both are saved with the company, and every signed-in
  account sees the same list.

### Fixed

- **A line broken into sub-items no longer splits across a page break.** The
  whole line — its header and its parts — now moves together to the next page
  in both the native and the browser renderer, so a package's sub-items are
  never printed on one page while their parent sits on another.

## [0.10.4] - 2026-10-13

### Added

- **Lead time can now be quoted in years, months, weeks and days.** Four fields
  — years, months, weeks, days — all optional; the quotation prints exactly
  what was entered, skipping the empty units: "2 Years, 10 Months, 4 Weeks, and
  5 Days". With every field left at 0 the lead time reads "-" rather than
  "0 days". Existing quotations keep their day-only lead times.

## [0.10.3] - 2026-10-13

### Changed

- **The discount percentage no longer prints under the After Disc. price.** The
  cell shows the price after the discount and nothing else; the percentage was
  noise beside a figure that already had the discount taken off.

- **More room for the item description.** The No., Qty. and price columns are
  slimmer and the Description column is wider, so long scope text fits on fewer
  lines. A price figure too long for its column now prints slightly smaller
  rather than running into its neighbour — it never wraps or crops.

## [0.10.2] - 2026-10-12

### Fixed

- **A line broken into sub-items no longer shows a 0% discount when its parts
  carry real ones.** With the sub-prices hidden, such a line printed "0%"
  underneath a price that had visibly come down — 52.100.000 against 46.380.000.
  The line's discount is now worked out from its parts, the same way its margin
  already was, so the figure under the price agrees with the price.

- **A line with no discount now reads "0%" rather than a dash.** The column is a
  discount percentage, so it states one.

## [0.10.1] - 2026-10-12

### Changed

- **The item table now shows both unit prices, so the columns add up at a
  glance.** A discounted line printed the price *after* its discount under
  "Unit Price IDR", with a discount percentage beside it — so the arithmetic
  appeared not to work, and a reader would take the percentage off a figure that
  already had it. **Unit Price IDR now shows the price before the discount**, and
  the column beside it, **After Disc. IDR**, shows the price after, with the
  discount percentage underneath. "Total IDR" is unchanged: the price after
  discount multiplied by the quantity. A line with no discount shows the same
  figure in both columns, and every quotation now carries the same columns
  rather than one that appeared only when something was discounted.

### Fixed

- **A services quotation no longer prints an internal code on the customer's
  copy.** The PDF carried a fifth term reading "Service category:
  NonConstruction" — a value from inside the app that means nothing to a
  customer, and one the on-screen preview never showed. Both now print the same
  four terms.

## [0.10.0] - 2026-10-12

### Changed

- **Your brand colours now reach the whole quotation, not part of it.** The four
  colours under Settings > Company used to recolour roughly a quarter of the
  document — the headings, the top ribbon, the accents — while everything else
  stayed blue, because those tints were fixed when the app was built. They are
  now worked out from your four colours, so the document follows the brand
  throughout. **The default look is unchanged for practical purposes:** the
  largest difference anywhere on a page is 7 parts in 255, measured against the
  old output.

- **Settings > Company is now three sections: General, Quotation and Portal.**
  Everything that was there is still there — the company profile and logo under
  General, the quotation's colours under Quotation, the verification page's
  colours under Portal — and one save button covers all three, so it is reachable
  from whichever section you are in. Nothing new to learn: the same settings,
  grouped by what they affect.

### Fixed

- **The totals box on a quotation no longer stretches to match the terms beside
  it.** When no PPN was charged — services under PPH22, or a free-trade-zone
  quotation — the box ended well above the bottom of its frame and left a block
  of empty space. It now ends where its figures end, and the rest of the height is
  filled with the same tinted bar that sits below the quotation details.

- **Cancel no longer creates the record.** On the new-client and new-account
  forms, Cancel was submitting the form on its way out: with a required field
  empty it refused and showed a validation message instead of closing, and with
  everything filled it created the record *and* closed. Cancel now cancels.

## [0.9.2] - 2026-10-11

### Fixed

- **A services quotation built from the `vera` command line is no longer
  rejected.** The tool's own reference listed the wrong values for a quotation's
  service category, so following it produced *"services require a category"* on
  the first attempt. It now names the values the server accepts, and a test keeps
  the two from drifting apart again.

### Security

- **Sign-in is now limited per network address, not only per account.** The lock
  that follows five failed attempts was keyed on the address typed into the form
  — a value the sender chooses — so it protected one account but left the
  endpoint itself unthrottled, and a flood of different addresses cost a database
  lookup each. **Nothing changes in normal use:** five failed attempts still lock
  an account for a minute, and the new limit is sixty attempts a minute from one
  address, which an office signing in together will never reach.

## [0.9.1] - 2026-10-10

### Fixed

- **A prepared quotation no longer carries our own figures in the part of it
  meant for the customer.** The data the app builds when you prepare a quotation
  held three of ours alongside the customer's — the margin a line earned, the tax
  we withhold, and the nett profit. **Nothing displayed them and nothing does
  now, so you will see no change at all.** It is fixed so that a screen added
  later cannot pick one up by accident and put a number that is ours alone in
  front of a customer. The figures themselves are unchanged and still where they
  belong, on the internal breakdown beside the customer's copy.

## [0.9.0] - 2026-10-10

### Added

- **A `vera` command-line tool, covering every feature.** Quotations, clients,
  drafts, the company record, staff accounts, approval requests and backups are
  all reachable from a terminal now. It talks to the same server the desktop does
  and renders the same PDF, so a quotation produced by a script is identical to
  one produced by clicking Export. `docs/cli.md` is the full guide.

- **`vera quotations export` files a quotation in one line.** It allocates the
  quote number, stores the signed revision, renders the customer's PDF and
  attaches it — the same sequence the desktop runs, with `--out` in place of the
  save dialog. A script can therefore file a real, verifiable quotation.

- **`vera quotations validate` and `vera quotations render`** price a quotation
  and produce its PDF without filing anything or consuming a quote number, which
  is the quick way to check a document before it is issued.

- **`vera verify` checks a printed document the way a customer would**, through
  the public verification page, and reports *valid*, *superseded*, *mismatch* and
  *not found* as separate results — so a script can tell an altered document from
  an out-of-date one without reading English.

- **Every command takes `--json`,** with documented exit codes (0 success,
  2 misuse, 3 signed out, 4 not found, 5 refused, 6 server). Automation branches
  on what happened rather than on the wording of a message.

- **`vera schema`** prints a worked quotation input file that runs as-is, with the
  field reference beside it.

- **`vera` is downloadable beside the desktop installers**, for Windows, macOS and
  Linux, with checksums. The download table is in `docs/cli.md`.

### Changed

- **The command-line tool keeps its sign-in token in
  `~/.config/vera/session.json`**, readable only by you, rather than the operating
  system's keyring — a server has no keyring. `VERA_TOKEN` overrides it entirely
  for automation that should not write one at all.

## [0.8.3] - 2026-10-10

### Fixed

- **Saving a draft no longer says the client has gone when it has not.** The
  message *"That client is no longer in the directory"* could appear while the
  client sat in the dropdown in front of you, on any client whose number did not
  happen to match a different client's internal row id. Drafts now pin the
  client you actually chose. A client that really has been removed is still
  refused, and now with the same clear message rather than a generic failure.
- **The Disc. column sits with the numbers around it.** It was aligned to the
  right while every other figure in the table is centred, which read as a
  mistake in an otherwise even table.
- **Exporting a quotation with no per-line discount could fail outright.** A
  quotation whose lines carry no discount at all — the common case — could stop
  the export dead. Fixed, and the test that should have caught it now renders
  the PDF rather than only describing it.

## [0.8.2] - 2026-10-10

### Fixed

- **The exported PDF now shows the Disc. column, like the preview does.** Since
  per-line discounts arrived, the quotation on screen and the verification
  snapshot printed a **Disc.** column whenever any line carried a discount — but
  the PDF the customer received did not. The price was correct, so no money was
  wrong; what was missing was the column saying a discount had been applied at
  all, and the pre-discount figure appeared nowhere on the customer's copy.
  The native PDF engine has been taught the column, so the preview, the snapshot
  and the PDF now print the same table. Quotations with no line discounts are
  unchanged: six columns, exactly as before.

### Changed

- **Updated the front-end build and test tooling** to clear nine security
  advisories raised against `vitest` and the copies of `vite`, `esbuild` and
  `tinypool` it bundled. Nothing about the quotation itself changes.

## [0.8.1] - 2026-10-10

### Fixed

- **A line discounted below what we paid for it could not be exported.** The
  quotation failed to save with a database error about
  `quotation_revision_items_margin_amount_check`, and nothing reached the
  archive. Such a line has a negative margin — that is the honest figure and the
  one the editor shows — but the column still required a margin of zero or more.
  It now accepts a loss, while still refusing a margin below what the line cost.
- **The Total IDR column on the customer quotation was not aligned like the
  figures around it.** Once a quotation had any per-row discount, the extra
  Discount column pushed everything after it along, and Total IDR quietly
  stopped being centred — it sat against the top-left of its cell while every
  other number sat in the middle. Every number in the table is now centred the
  same way, and only the description flows from the top.

### Changed

- The internal panel's PPN row reads **PPN collected**, without the
  parenthetical — it already sits inside a box marked *Internal only*.
- In Quotation Details, **Validity, Payment, Lead time and Incoterm now share
  the full width** of their row, closing the gap the removed Discount field left
  behind.

## [0.8.0] - 2026-10-10

### Fixed

- **Preparing a quotation could turn the window blank.** On any quotation with a
  line that had no sub-items — which is most of them — pressing **Prepare** made
  the whole screen go white, taking the quotation you were working on with it.
  Reloading was the only way back. Fixed, and the app can no longer go blank like
  that: if a screen ever fails again it says what went wrong and offers to carry
  on, instead of hiding everything.

### Added

- The internal panel now shows **PPN collected (not profit)** above the nett
  figure, the way it already showed PPH22. PPN is money held for the state, so
  it is shown leaving the total: **Grand total − PPN = Nett after Tax**. On a
  goods quotation the totals and the nett no longer look unrelated.

### Changed

- **Line items are laid out on two lines.** The description gets the full width
  of its own line, and quantity, unit, cost, margin, discount and the sell price
  sit underneath with their labels always visible. The column headings along the
  top are gone, so there is nothing to read across.
- Long descriptions and scope lines **wrap instead of running off the edge**, so
  what you typed is always fully readable.
- When a line has description lines, they now sit **beside** the controls rather
  than pushing them down the page.
- Buttons and switches finally show a **pointer cursor**, so it is obvious what
  can be clicked.
- **The quotation-wide Discount has been removed from Quotation Details.**
  Discounts are entered per line now, and a second one at the top silently
  re-priced every line at once. Quotations made with one keep it.
- A quotation without a whole-quotation discount no longer prints an empty
  **Discount (0%) Rp0** row on the customer document.

## [0.7.1] - 2026-10-09

### Fixed

- Opening an **older** quotation — one exported before per-row discounts
  existed — could show a made-up negative discount amount in its detail view.
  Nothing on screen was wrong; the figure was carried internally and would have
  surfaced the moment anyone displayed it. A line with no discount of its own now
  reports no discount money, whatever it was exported before the feature.

Quotations already exported are otherwise untouched.

## [0.7.0] - 2026-10-09

### Added

- **A discount can now sit on a single line.** Use **Disc.** next to the margin
  on any line, and on any sub-item of a split line, and that row alone is
  discounted. A line that needs 15% off and a line that needs none are now
  different lines, instead of the discount having to be spread across the whole
  quotation or the price quietly back-solved so the total came out right.
- The line shows what the discount came off — **was 120.000 · −12.000** — so the
  customer-facing price is never a mystery.
- Sub-items discount independently of the line they belong to.
- The quotation keeps its own discount as well, applied on top as before.

### Changed

- The margin column now reads **Margin / Disc.** and carries both controls,
  stacked. A new column would have squeezed the description at the app's own
  window width.
- A line discounted below what you paid for it now shows a **negative margin**.
  That is a real answer and it was previously either hidden or, worse, shown as a
  large positive number.

### Fixed

- The customer quotation grows a **Disc.** column **only when a row actually
  uses one**, so the description is not narrowed on every document in order to
  leave the column empty on most of them. Rows without a discount show a dash.

Quotations already exported keep their figures. Reopening one to edit it brings
in no per-row discount, which reads as "no row has a discount", and exporting
that edit follows all the current rules. Nothing stored is rewritten.

## [0.6.0] - 2026-10-09

### Added

- **You can save a quotation before it's finished.** Use **Save draft** in the
  editor and the work is kept — a client called with a partial item list, a
  quote waiting on a supplier's price, a line abandoned to take a phone call.
  Find it under the new **Drafts** tab, reopen it to carry on, or delete it.
  The tab shows how many you have, so you know something is waiting.
- Drafts are **yours alone**. Nobody else — including an administrator — can
  see, open or delete a draft that isn't theirs.
- A draft is **not a quotation**: it has no quotation number, it is not
  signed, and it cannot be checked online. Exporting it turns it into a real
  quotation, and the draft then disappears on its own — there is nothing left
  for it to be.
- Saving is deliberately forgiving. A quotation that isn't finished enough to
  price still saves, because refusing to save is the one thing a draft must
  never do. **Prepare** still checks everything properly when you export.
- Drafts disappear if your account is deleted.

### Changed

- **What's New now reads like release notes.** Bold, italics, code and links
  render, a bullet that was written across several lines in the source reads
  as one bullet, and the dismiss button stays put instead of sitting at the
  bottom of a long read.
- The navigation gained a fifth tab. At the app's own window width the header
  was wrapping onto two lines, so the small "Quotation Generator" tagline now
  waits until there is room for it.

Quotations already exported are untouched by all of this.

## [0.5.1] - 2026-10-09

### Fixed

- **Our own profit figure was wrong on any quotation with a discount.** Two
  mistakes were adding up in the same direction. A discount was being taken off
  what we pay suppliers as well as off the price we charge — but giving a
  customer a lower price does not make our purchase cheaper, so profit was
  being flattered by the discount. And the 11% PPN we charge on non-FTZ goods
  was being counted as earnings, when it is collected for the state and handed
  straight back. Both are corrected. Quoted customer totals do not change: the
  discount, the PPN and the grand total all still come off the price exactly as
  before. Only the internal view moves.
- The review screen no longer prints the same cost figure twice under two
  different names, and the dead cost-side discount row is gone rather than
  showing zero.

### Changed

- The two service tax rates are now labelled **2% PPh22** and **1.75% PPh22**
  everywhere, replacing "2% withholding" and "1.75% final". Same rates, the name
  staff actually use on the paperwork.

### Added

- The public verification page now shows **the PPN the customer was charged**,
  under the grand total, so the figures on the page match the figures printed
  on the quotation. The line appears only when PPN was charged.

Quotations already exported keep the figures stored with them. Only new
quotations use the corrected profit.

## [0.5.0] - 2026-10-09

### Added

- **A line can now be a main item built from priced sub-items.** Split a line
  and each sub-item carries its own quantity, unit, cost and margin; the main
  item becomes their sum — always quantity 1 Lot — and its margin is shown as
  the **overall margin** of the whole line. What used to be one lump price can
  now be quoted the way the work is actually composed: the control panel, the
  wiring, the commissioning, each priced on its own.
- **Choose what the customer sees on an itemized line.** One switch picks
  between the sub-items printing as their own priced rows under a title-only
  main item, or the full price staying on the main item with the sub-items
  listed as unbilled scope lines. The total is identical either way; only the
  presentation changes.
- **Sub-sub-items.** A sub-item can spell out what it includes with plain
  detail lines beneath it — no numbers, just the words.
- **Paste a block of sub-items from Excel.** One row per sub-item —
  description, qty, UoM, unit cost, margin % — straight into the line.
- Sub-item quotations carry the same verification code as every other
  document: every printed part, down to the detail lines, is covered.
- The line's overall margin is shown live as the sub-items change. A margin
  that comes out to a round figure is shown as-is; anything else is rounded
  and marked with **≈**.

### Fixed

- Exports of FTZ quotations no longer print a `PPN 11% Rp0` row that the
  preview does not show.
- On longer quotations, the closing sections — the totals cards, the closing
  note and the verification block — now break across pages exactly like the
  preview, instead of moving together and occasionally adding an extra page;
  the totals card also spaces its rows to match the preview.
- In table rows whose description wraps to a second line, the numbers beside
  it (Qty., UoM., Unit price, Total) now sit vertically centered rather than
  at the top of the row.

## [0.4.1] - 2026-10-07

### Added

- The unit-of-measure menu now covers how jobs are actually quoted. Alongside
  **Lot, Pc, EA, LS, set, meter**, durations — **Hour, Day, Week, Month, Year,
  Quarter** — are now first-class choices, and **m2** and **m3** have been
  added for areas and volumes: coatings, concrete and excavation are quoted
  per square meter and per cubic meter.
- Pasted spreadsheets are understood more forgivingly: `hours` becomes Hour,
  `mtr` becomes meter, and area/volume spellings such as `sqm`, `m²` and `m³`
  become m2 and m3, so a real unit no longer falls back to Lot during a paste.

## [0.4.0] - 2026-10-02

### Added
- Profit margin per line now goes up to **1000%**, up from 100%. Low-cost items
  sold well above cost — a fitting on a service visit, a consumable — can now
  carry a real number. A 1000% margin means selling at eleven times cost.
  Discount percentage is unchanged and still stops at 100%.

### Changed
- **Exports are dramatically faster and no longer depend on Google Chrome.**
  Printing a quotation used to take about half a minute and needed a working
  Chrome installation on the machine. Vera now builds the document itself and
  finishes in well under a second, on any machine, with nothing else installed.
  Staff who were told to keep Chrome installed for this can stop.
- The quotation preview and the finished PDF now always use the same typeface.
  The preview had been quietly falling back to a substitute font while the
  export used the intended one, so what you approved and what the customer
  received could differ. They now match.
- Repeated lines of an item list are no longer given a rounded top edge. That
  rounding belonged only to the first page, where the column headings are.
- Spacing around the QUOTATION heading is now even above and below.

### Fixed
- A spreadsheet paste could put the wrong text in the unit-of-measure column —
  a line would occasionally show its own description there, and in the finished
  PDF that sentence would run into the price beside it. Pasted values are now
  checked, and anything that is not a real unit is corrected before it can reach
  a customer's document. If a stored quotation already carries a bad value, it
  now shows as unrecognised in the editor instead of quietly displaying
  something else, and preparing it is refused with a message naming the line.

## [0.3.5] - 2026-09-24

### Fixed
- Logo sizing: breathing room in the desktop icon, tighter navbar
  lockup, true-size marks on sign-in and activation.

## [0.3.4] - 2026-09-24

### Changed
- Refined Vera artwork across the app icon, header, and sign-in screens.

### Fixed
- Modal backdrops dim the page again behind What's New and shortcuts help.

## [0.3.3] - 2026-09-24

### Fixed
- Company Logo card shows the real built-in logo, file details for
  uploads, and a proper Upload logo button.

## [0.3.2] - 2026-09-24

### Fixed
- Settings cards align edge-to-edge in every row at every window size.
- Keyboard shortcut rows breathe with comfortable spacing.

## [0.3.1] - 2026-09-24

### Fixed
- Settings General tab rebalanced: no more dead empty space, cards pair
  evenly at every window size.
- Keyboard shortcuts moved to their own Settings tab, visible to everyone.

## [0.3.0] - 2026-09-24

### Added
- Keyboard shortcuts everywhere: Excel-style grid navigation in the editor
  (arrows, Tab, Enter, Shift to reverse), one-key export, prepare, and page
  jumps, arrow-key archive browsing, and a shortcut overlay on ?.
- Settings Keyboard section: rebind any shortcut, reset single keys or all.
- What's New dialog after every update, so changes are never a surprise.
- Default export folder: the save dialog opens where you choose.
- Portal checks list only real mismatches, with clearer hints and its own
  verification card.
- New Vera artwork across the app icon, header, and sign-in screens.

### Changed
- Editor column reads Unit Price everywhere it is shown.

### Fixed
- Long subjects in export filenames break at a word boundary instead of
  mid-word.

## [0.2.2] - 2026-09-23

### Added
- Company settings fill the width: profile, logo, and both color sets in one balanced grid.
- Sign-in and invite-code screens show the Vera mark.

### Changed
- App Updates card distributes its content instead of pooling at the top.
- Desktop icon ships on a white background, visible on dark taskbars and docks.

## [0.2.1] - 2026-09-23

### Added
- Windows ARM64 installer for Snapdragon machines; the updater serves the right binary per device.
- Settings General restacked: App Updates sits under Sessions.
- Settings Company split into form and logo columns.

### Changed
- Vera mark is the desktop app icon and the shell header.

## [0.2.0] - 2026-09-23

### Added
- Public verification portal: every exported quotation carries a signed QR code clients can scan.
- Company profile, uploadable logo, and brand color themes for quotations and the portal.
- Team page owns all approval queues; notification bell is notify-only.
- Windows installer and macOS disk image built by release CI.
- Server-driven update check with banner and Settings re-check.
- Per-customer yearly quotation numbering with UUID identity.

## [0.1.0] - 2026-09-15

### Added
- First desktop release: quotation editor with per-line margins, tax zones, PDF export, archive with immutable revisions, staff accounts with admin roles, backups.
