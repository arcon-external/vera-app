# Changelog

Staff-facing release notes for Vera. Entries use plain language and only
describe user-visible changes; internal chores never appear here. Sections
per version: Added, Changed, Fixed, Security.

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
