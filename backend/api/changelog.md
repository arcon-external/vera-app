# Changelog

Staff-facing release notes for Vera. Entries use plain language and only
describe user-visible changes; internal chores never appear here. Sections
per version: Added, Changed, Fixed, Security.

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
