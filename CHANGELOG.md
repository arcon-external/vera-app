# Changelog

Staff-facing release notes for Vera. Entries use plain language and only
describe user-visible changes; internal chores never appear here. Sections
per version: Added, Changed, Fixed, Security.

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
