# #502 — orchestrator rulings for implementation

**Status:** applied by the orchestrator on 2026-10-28 to unblock the implementation
wave. The user authorised `$implement-spec`; these are the defaults proposed before
the wave began. They are recorded here rather than absorbed silently, and they can be
overturned until the relevant ticket is merged.

| ID | Question | Ruling | Tickets |
| --- | --- | --- | --- |
| R-01 | Which header fields may show an "empty" notice (HR-01) | Only fields the existing validation already requires. Header requiredness is not widened in #502. The empty notice is proved on quantity; the header case is N/A, not a pass. | 04, 05 |
| R-02 | Unknown source reference (HR-02) | Correction is required. Acknowledgement alone does not clear an unknown reference and cannot finalize. | 05 |
| R-03 | Later tax-basis change (HR-03) | Out of scope for #502. The link-time choice is protected; later change remains through correction or source repair only. | 05, 06 |
| R-04 | Session write and `POVersion` (HR-05) | A session write bumps `POVersion`. The client sends the version it loaded; a stale write conflicts rather than overwrites. | 04, 05 |
| R-05 | "Full comparison" on source repair (HR-12) | Seed every current difference, every line-set outcome (missing/extra/duplicate) and the link-time tax item. Matching fields do not get items. | 06 |
| R-06 | Active-session uniqueness (HR-15) | Unique on `(po_id, source_revision_id, generation)`, so a repair back to a previously used revision is allowed. | 04, 06 |
| R-07 | Records saved before migration 041 (HR-18) | Show "no reconciliation recorded". No backfilled or synthetic session, and no attributed decisions. | 06 |
| R-08 | Duplicate resolution when a line is removed (HR-16) | Removing one of the duplicate lines resolves the duplicate item as "line removed"; the surviving line stays matched. | 05 |
| R-09 | Totals mismatch reason (HR-06) | Name the first differing input in document order. If no input differs, the reason is "no input difference explains the printed total". | 05 |
| R-10 | Disabled 0% vs enabled 0% discount (HR-07) | They are not commercially equal; the frozen fixture is the oracle. | 04, 05 |
| R-11 | Motion timing (HR-08) | The sheet closes with no exit animation and returns focus, per the brief. Timings are hand-measured, never asserted from a class name. | 07 |
| R-12 | Screen-reader announcements (HR-09) | DOM proves roles and one text mutation. The announcement itself is hand-driven and recorded as such. | 07 |

Open, not blocking the current wave: R-03's correction command name, the exact
wording of the totals reason, and whether a pre-041 record offers a repair action.
