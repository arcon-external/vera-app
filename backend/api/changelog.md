# Changelog

Staff-facing release notes for Vera. Entries use plain language and only
describe user-visible changes; internal chores never appear here. Sections
per version: Added, Changed, Fixed, Security.

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
