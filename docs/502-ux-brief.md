# #502 — UX and interaction brief: the PO reconciliation workbench

**Status:** ready for Loki and Frank
**Author:** Jessy (UX/UI and interaction design)
**Worktree:** `/var/lib/openbot/OpenBot/Shared/vera-502` at `dca8890`
**Source of truth:** `.scratch/vera-502/spec.md`, tickets 01–07, `docs/po-reconciliation-contract.md`, `.impeccable.md`, and the current `PurchaseOrderEditor`, `PurchaseOrderFinalize`, `PurchaseOrderListPage` and `AppShell`
**Scope:** design and acceptance detail only. No production code. This brief does not add or remove a requirement, a field, a comparison, or a migration.

## 0. Skills used

All six named skills were loaded; none was deliberately skipped. **shape** (brief structure and state inventory), **impeccable** (design context, anti-patterns), **adapt** (1024/1280/360 behaviour) and **animate** (motion and reduced motion) shaped the brief. **critique** and **polish** were read for the review criteria and the final checklist, but not executed: they are post-build passes, and there is no built workbench to review yet. `.impeccable.md` already supplies the design context, so no `impeccable teach` run was needed.

| Ticket | Where this brief answers it |
|---|---|
| 01 Contract and parity fixtures | §4.5, §4.8, §4.11, §10.6, §11 |
| 02 Back stack, workspace state, action bar | §5, §7 |
| 03 PO-native docket list | §8 |
| 04 Live header reconciliation | §4.3–4.6, §5, §6 |
| 05 Lines, tax basis, totals | §3.1–3.2, §4.5, §4.8, §4.11 |
| 06 Archive read-only session and source repair | §4.12, §7 |
| 07 Motion, accessibility, polish | §9–§11, §15, §16 |

---

## 1. Design intent

**Three words:** ledger, proof, calm.

The workbench is where the customer's order is checked against the quotation we issued. Staff should never reach the last screen and discover a difference. The pane is not a report and not a modal; it is the other half of the form, present while they type.

**Primary action:** resolve the next difference, then Final Save. Everything else is supporting evidence.

**What success feels like:** "I can see what I have checked, what still differs, and what is left before I can save." Not "I hope this passes at the end."

**Non-negotiables**

- One deep teal action accent. Emerald is reserved for a true match or success. Amber is never used here; it means "internal only", not "warning". Red is reserved for blocking failures and the Source missing badge, never for a difference.
- A difference is a decision, not an error. No red cards, no warning stripes, no side-stripe borders over 1px.
- Numbers are tabular and right-aligned; labels are quiet; values are not truncated.
- Every state is explicit: empty, loading, ready, error, blocked, resolved, read-only.
- The evidence rail uses one surface and hairline separators. No card inside a card. No shadows on item rows.
- No new colours, fonts or spacing scale. Use the existing Vera tokens and the existing 5-step type scale.

---

## 2. Page frame and breakpoints

The shell header stays 64px at ≥640 and is sticky. Below 640 it stops being sticky and wraps to two rows; the reconciliation bar then sticks to `top: 0`. The page scrolls normally; the action bar is sticky at the bottom.

| Viewport | Outer padding | Content width | Form column | Gutter | Evidence rail | Notes |
|---|---:|---:|---:|---:|---:|---|
| 360 × 900 | 20px each side | 320px | 320px | — | Sheet, full content width | One column; action bar 2 buttons |
| 1024 × 768 | 32px each side | 960px | 616px | 24px | 320px | Two panes; line rows are two-row |
| 1280 × 900 | 32px each side | 1216px | 828px | 28px | 360px | Two panes; line rows are one row |
| 1440+ | 32px each side, max 1560 | up to 1560px | 964 at 1440; grows to 1084 at 1560 | 32px | 380px | Rail capped at 380px |

The page title block sits at the top of the form column; the evidence rail starts level with it. At 360 it spans the single column. At 360 the `h1` is `text-title` (19/26); at ≥768 it is `text-display` (26/32). The quote number is never wrapped mid-token. Meta lines may wrap.

### 2.1 1280 layout

```text
┌─ shell header, 64px, sticky ───────────────────────────────────────────────┐
│ ← Purchase Orders   Editor  Drafts  Archive        PT Arcon …  bell  user │
├────────────────────────────────────────────────────────────────────────────┤
│ ┌─ form 828 ──────────────────────────────┐  28  ┌─ rail 360 ────────────┐  │
│ │ Purchase Order              [Draft]     │      │ Reconciliation  3 to…│  │
│ │ QT-2026-018 · PT Maju Jaya · rev 02      │      │ Compared with QT-…    │  │
│ │ Source                                  │      │ [All][Unresolved][Res]│  │
│ │ Order details                           │      │ ┌ item ─────────────┐ │  │
│ │ Commercial terms                        │      │ │ Payment terms     │ │  │
│ │ Lines                                   │      │ │ PO …  Quotation … │ │  │
│ │ Discounts and adjustment                │      │ │ [Keep][Use Quot.] │ │  │
│ │ Totals                                  │      │ └───────────────────┘ │  │
│ │                                         │      │ Customer PO PDF  View │  │
│ │                                         │      │ Totals check  Match   │  │
│ └─────────────────────────────────────────┘      └───────────────────────┘  │
├─ action bar, sticky: status              Save draft       Final Save ───────┤
└────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 1024 layout

Same grid, `616 / 24 / 320`. The form column is 616px, so the line card uses the two-row layout in §3.2 and the address fields take the full width. No horizontal scroll is allowed at 1024. The rail is 320px wide; item values stack, never sit in two columns.

### 2.3 360 layout

```text
┌─ header row 1: ← Purchase Orders                         avatar ─┐
│  header row 2: Editor   Drafts   Archive                        │  (scrolls away)
├─────────────────────────────────────────────────────────────────┤
│ Reconciliation · 3 to resolve                              v    │  sticky, 56px
├─────────────────────────────────────────────────────────────────┤
│ Purchase Order                              [Draft]             │
│ QT-2026-018 · revision 02                                       │
│                                                                 │
│ Order details (single column)                                   │
│ Commercial terms                                                │
│ Lines (stacked line cards)                                      │
│ Discounts and adjustment                                        │
│ Totals                                                          │
├─────────────────────────────────────────────────────────────────┤
│ Resolve 3 items to enable Final Save                            │
│ [   Save draft   ]  [        Final Save        ]                │  sticky
└─────────────────────────────────────────────────────────────────┘
```

At 360 the sheet covers the content between the reconciliation bar and the action bar; it never covers the shell header or the action bar. See §6.

Wireframes in this brief use example values (`BP-1042`, `QT-2026-018`, `PT Maju Jaya`, `Rina`, dates) only to show layout and hierarchy. They are not real records, rates or names.

---

## 3. The form column

### 3.1 Section order

| Section | Contents | Comparison status |
|---|---|---|
| Source | Linked quotation number, revision, state; `Choose Quotation` / `Link Quotation` when absent | PO context; not compared |
| Order details | Billing address, delivery address, ship-to code, contact email, contact phone, customer PO number, PO date, received date, requested delivery date | PO-only. Billing address is never compared (Q14). |
| Commercial terms | Payment terms, Incoterms, whole-PO discount | Compared on commit |
| Source basis | Currency, tax zone, tax type, service category. Editable only before a link; after a link these are read-only context and live in the rail | Compared at link time only (Q15) |
| Lines | One card per line; description, UoM, quantity, unit price, row discount; add/remove | Compared per matched line |
| Discounts and adjustment | Signed adjustment and adjustment reason; a one-line reminder of the whole-PO discount applied in Commercial terms | Whole-PO discount is compared in Commercial terms; adjustment is PO-only |
| Totals | The PO's own calculation: line gross, row discount, subtotal, whole-PO discount, PPN, adjustment, grand total | Evidence in the form; the rail holds the check against the printed totals |

**Do not repeat the whole-PO discount field in two sections.** It belongs in **Commercial terms** only. The **Discounts and adjustment** section holds the signed adjustment and its reason, plus a one-line reminder of the whole-PO discount currently applied.

**Why split Order details from Commercial terms:** billing address and the other PO-only fields must not sit beside compared fields where staff will assume a check exists. The split makes the comparable set visible without putting a tag on every input.

**Nothing internal appears here.** Cost, margin, PPH22 and profit are absent from the workbench, the pane, the totals check and the list, by construction. Do not add an internal-only badge or a cost column to this feature.

**Field grid:** short fields are two columns when the form column is at least 560px; the two address textareas are full width below 720px of form width (1024) and side by side at 720px and above (1280). At 360 everything is one column. Use the form column's width, not the viewport.

**PO-only does not mean optional.** The existing required, date, length and reference validation still applies to every Order details field; the split only says it is not compared.

Each section uses the existing `SectionHeading`. The section caption is one line at 11.5px, only where it prevents a wrong assumption:

- Order details: `PO-only details. These are not compared with the Quotation.`
- Commercial terms: `Checked against the Quotation when you leave the field.`
- Lines: `{n} lines · matched by quotation reference, not by position.`
- Totals: `Calculated from this PO. The quotation's printed totals are evidence, not a target.`

### 3.2 Line card

Every line keeps a visible identity header, because stable identity is the point of the reconciliation:

```text
Line 1    [Quotation 1]                                      Remove
Description ...................................................
Quantity [ 12 ]   UoM [ PCS v ]   Unit price (IDR) [ 1.250.000 ]
[ ] Row discount  [ 2 ] %                         Line total IDR …
```

- **Line header:** `Line {n}` in 12.5 medium. At ≥1024 a match control sits at the right of the label: chip `Quotation 2.1` / `Extra line` plus `Change`. At 360 the header is two rows: `Line {n}` and the 44×44 Remove on row 1, then a full-width 44px match control on row 2. Activating it reveals a `Quotation match` select under the header with the source lines and `Extra line (no match)`; only one line's select is open at a time. Choosing a match recomputes the line set and items. The select is not a review decision and does not save by itself. Accessible name for the control: `Change quotation match for line {n}`.
- **Line header actions:** `Remove` at the right. At 360, Remove becomes a 44×44 icon button with the accessible name `Remove line {n}`; at ≥1024 it is a text button.
- **≥700px of form width** (1280 and up): one field row — description `minmax(0, 1fr)`, quantity 104px, UoM 120px, unit price 150px, Remove 72px, 12px gaps.
- **560–699px of form width** (1024): description on its own row; quantity, UoM and unit price on the next row (`1fr 1fr 1.2fr`), Remove at the end.
- **Below 560px** (360): description; quantity and UoM side by side; unit price full width with the 44px remove control at the end; discount strip wraps.
- **Discount strip:** checkbox + `Row discount` + percent input 72px + `%` + right-aligned `Line total {money}`. At 360 the line total wraps to its own line but stays tabular and right-aligned.
- **Typography:** field labels 12.5 medium; values 13.5 tabular; line total 12.5 tabular; match chip 10.5 uppercase.
- **No auto-focus** when a line is added. Focus the new line's description; announce `Line 3 added.` politely.
- **Remove:** immediate local removal, no modal. Announce `Line 2 removed. Its review items are kept as history.` If the line had unresolved items, they become resolved `Line removed` rows; they are never re-keyed. If focus was in the removed line, move it to the next line's description or the `Add line` button; never `body`.
- **Add line:** a full-width dashed button after the last line, 44px high, `Add line`. Do not duplicate it in the section heading.

The line fields must use **container queries on the form column**, not viewport `sm:` classes. At 1024 the form is 616px while the viewport is 1024px; a viewport breakpoint would try to fit five columns into 616px and overflow.

### 3.3 Form totals, rail totals check

These are different objects and must not look duplicated:

- **Form `Totals`:** the PO's own calculation, shown as stages; this is what will be stored. Keep the existing `PurchaseOrderTotals` content, including the explanation of row discount, whole-PO discount, PPN on top, signed adjustment and reason.
- **Rail `Totals check`:** a derived comparison of the PO's calculated figures with the quotation revision's printed figures. It never asks for a decision and never blocks Final Save. See §4.11.

### 3.4 Cancel

The editor keeps one quiet **Cancel draft** / **Cancel correction** text button in the page title block, not in the form footer and not in the action bar. At ≥1024 it sits at the top right of the title block beside the badges; at 360 it sits under the meta line as a 44px text button. It closes the open record back to its list tab; the dirty guard is the same one the shell Back uses. It is not a primary action and never competes with Save draft.

---

## 4. The evidence rail

### 4.1 Order and sticky model

The rail is one `aside`, not three cards. Sections are separated by 1px hairlines and 16px spacing.

Order, top to bottom:

1. **Reconciliation pane** — its header carries the exact source (`Compared with QT-… · revision …`); it is the only scrolling section at normal heights.
2. **Customer PO document / source** — a compact pinned strip for the PDF and its version; the source identity stays in the pane header and the page title, not duplicated as a card.
3. **Totals check** — a compact pinned strip.

```text
rail (sticky, top 88px, width 320 / 360 / 380)
├ Reconciliation header        ← sticky inside rail
│  Reconciliation        [3 to resolve]
│  Compared with QT-2026-018 · revision 02
│  Checked when you leave a field.
├ Filters                      ← sticky inside rail
│  [ All 7 ][ Unresolved 3 ][ Resolved 4 ]
├ Item list                    ← the only scroll region
│  unresolved newest first, then resolved newest first
│  …
├ Customer PO PDF              ← pinned footer strip, 48px collapsed
│  PO-1042.pdf · 2 pages · v1            View
├ Totals check                 ← pinned footer strip, 48px collapsed
│  PO IDR 12.345.000 · Printed IDR 12.345.000   Matches
```

- **Rail position:** `position: sticky; top: 88px` (64px header + 24px gap), `max-height: calc(100dvh - 176px)` (88px above, 64px action bar + 24px below). At 1024×768 that leaves 592px of rail.
- **What is sticky:** the rail column while the form scrolls; inside it, the header, the count, the filters and the latest-notice strip when present. The document and totals strips are pinned to the bottom of the rail at normal heights.
- **What scrolls:** only the item list. `overscroll-behavior: contain` so a trackpad gesture at the end of the list does not throw the page.
- **Short viewports:** below 720px of viewport height, the whole rail becomes one scroll column; the header and filters still stick inside it, and the document/totals strips flow after the list instead of pinning.
- **Expanded strips:** only one of the document or totals strips may be expanded at a time; opening one collapses the other. If both are collapsed the list gets the space. The expanded totals strip grows upward (the list shrinks), never pushes the rail past its max height.
- **State on settings round-trip:** pane filter, list scroll, which strip is expanded, and the sheet state survive exactly. Returning from settings does not re-run the rail’s entrance motion.

### 4.2 Pane header

- Title `Reconciliation`, 15/23 semibold, ink-950.
- Count chip on the right: `3 to resolve` in accent-800 on accent-50, 11.5 tabular, 28px high; `1 to resolve` singular; at zero `Nothing to resolve` in ink-700 on paper-muted. Do not use success here: zero review items does not mean the required fields are complete, and Final Save readiness is separate.
- Source line: `Compared with QT-2026-018 · revision 02`, 11.5 ink-500.
- Teaching line: `Checked when you leave a field.`, 11.5 ink-400. This is the only explanation of commit-on-blur; do not repeat it in every item.
- The chip is a status at ≥1024, not a button. At <1024 the whole sticky bar is the button (see §6).

### 4.3 Empty states

| Situation | Visible result |
|---|---|
| No source linked | Title `Reconciliation`, empty body `Link a Quotation to start the comparison.` Caption `Currency and tax basis stay editable until then.` Action `Link Quotation` (opens the existing selector). Count chip is hidden; filters are hidden. |
| Linked, no items yet | `No differences yet` / `Values are checked when you leave a field.` Filters show their real counts (0/0/0 only when there are no items); if the link-time tax basis is already recorded, show that resolved row instead of the empty state. |
| Unresolved filter, none | `No unresolved items` / `No differences to resolve.` Action `Show all`. The count chip shows `Nothing to resolve`; Final Save readiness is decided separately by the required-field check. |
| Resolved filter, none | `No resolved items yet` / `Kept and accepted values will appear here.` Action `Show all`. |
| Saved record, no items | `No differences were recorded for this session.` Read-only, no filters. |
| Loading | Three skeleton rows, 44px, 12px apart; `aria-busy="true"`; visually hidden `Loading the reconciliation…`. No spinner. |
| Error | `The reconciliation could not be loaded.` / `Save draft still works. Final Save is blocked until it loads.` Action `Try again`. `role="alert"`. |
| Source missing (`sourceState=never`) | Same as “no source linked”, with the page badge `Source missing`. Final Save is blocked. |
| Former source (`sourceState=former`) | `The source quotation was deleted.` / `This reconciliation is read-only until a new source is linked.` Actions `Link new source` and, if the user is an admin, `Repair source`. Item actions are disabled. Final Save is blocked. |
| Currency mismatch | `This purchase order is in {X}; quotation {number} revision {n} is in {Y}.` / `Amounts in different currencies are never compared or converted.` Blocking banner (`role="alert"`), no comparison items, Final Save blocked. A customer mismatch uses the same blocking shape with the two customer names. |

Empty states never use an illustration, a big icon, or a centered marketing block. They are left-aligned, type-only, and one action at most.

### 4.4 Notices

A **notice** is inline feedback, not a review item, and is never persisted. It has two homes: under the committed field, and (for the most recent notice only) as a 32px strip under the filters.

**Placement and life**

- Inline, directly under the field, 16px line, 11.5px, 12px icon, 4px below the input. It does not change the input's height; the form may reflow by 16px when it appears.
- Pane strip, under the filters, only while the most recent commit produced an empty or matched notice. It replaces the previous strip and is one line, not a summary list; it never accumulates matches and is not a matched summary. A difference does not use this strip; it creates an item at the top of the list.
- Life: an empty notice stays until the field is filled and committed. A match notice stays until the next commit that reevaluates **that field**; a commit in another field does not clear it. Neither is persisted; both are gone after reopen or settings round-trip.
- The pane strip echoes the latest commit event; the inline notice's per-field lifetime above still governs the field.
- Requiredness is the existing validation's, not the comparable set's. Only a required field gets an empty notice. An optional comparable field is compared as its trimmed value (an empty whole-PO discount is 0%); both sides empty is not a difference, empty vs non-empty is a normal difference item, and comparing the field never makes it required. Do not expand requiredness in this brief.
- No **matched** notice for a field that has a review item, resolved or unresolved. An **empty** notice may appear on a field with retained item history: it blocks the existing required-value gate but creates no second item and does not delete the history.
- `role` of the visible text is none; the single live region in §11 makes the announcement. Do not mark the visible notice itself as a live region.

**Copy and colour**

| State | Copy | Colour |
|---|---|---|
| Empty required field | `{Field} is empty` e.g. `Quantity is empty` | ink-500 with a 6px hollow dot. Not red on blur. |
| Matched | `Matches the Quotation` | success-700 with a 12px check. |
| Invalid non-empty number | `Quantity must be greater than zero` / `Unit price must be a valid amount` / `Discount must be between 0 and 100` | danger-700, danger border once committed; blocks Final Save; never becomes a review item and never coerces to zero. |
| Difference | No notice. A quiet inline anchor appears: `Review · differs from the Quotation` with a `Show` button (accessible name `Show review item for {Field}`) that focuses the item in the pane. | accent-700 text, no border, no red. |
| Resolved, value still present | No notice and no matched notice. The field is clean; the resolved row in the pane is the record. | — |
| Resolved, value cleared | `{Field} is empty`; the resolved item and its history stay; the required-value gate blocks Final Save. | ink-500, danger only on a blocked save attempt. |

On a blocked Final Save attempt, the first empty required field gets a danger border and its notice changes to danger-700, and focus moves there. Until that attempt, an empty field is incomplete, not an error. Invalid non-empty input is an error on commit, not a review item; the pane never shows it as a difference and the value is never replaced with zero.

At <1024 the inline `Show` opens the reconciliation sheet and focuses the item; it does not navigate or close the editor.

### 4.5 Review item cards

Unresolved items are newest first. Resolved items follow, newest resolution first. A new item enters with the pane-item motion in §9; no item ever animates out.

```text
┌──────────────────────────────────────────┐
│ Payment terms                  Unresolved│
│ PO          50% advance, balance 30 days │
│ Quotation   30 days from invoice         │
│ [ Keep PO ]          [ Use Quotation ]   │
└──────────────────────────────────────────┘
```

- Card: white, 1px rule, 8px radius, 12px padding, no shadow.
- Label 12.5 medium ink-900; status pill 10.5 uppercase, neutral border and ink-500 text. Unresolved is a state, not a warning.
- Values: two rows, 11.5 label column 72px + 12.5 value. Values wrap; never truncate. An empty side reads `(empty)`, not a dash. Money and quantities are tabular. At 320px the label column may drop above the value if the value needs the width.
- Actions: 32px high at ≥1024, 44px at <1024. `Keep PO` is secondary; `Use Quotation` is secondary with accent text (`text-accent-700`, accent-200 border, accent-50 hover). Neither is preselected and neither is primary.
- Accessible names include the field: `Keep PO entry for Payment terms`, `Use Quotation value for Payment terms`. Visible labels stay short.
- Line items use the same card with the contract label `Line 2 quantity` (and `description`, `unit of measure`, `unit price`, `discount`) and a secondary line `Quotation 2.1`. A line’s five fields are five items, not one grouped card; this keeps newest-first ordering honest and the count exact. The displayed line number is a label only; the item key is the stable line identity from the contract, never the position.
- A line discount value reads `No discount` when disabled and `{n}%` when enabled. Disabled 0% and enabled 0% are different values; do not collapse them to the same label.
- **Use Quotation** updates the field value immediately, resolves the item, and moves focus to the next unresolved item’s first action. If none remain, focus moves to the count/status element (`tabIndex="-1"`). Under the `Unresolved` filter the resolved row leaves the list; compute the next target before removal. The form does not scroll; the value changes where it is.
- **Keep PO** does not resolve immediately; it opens the compact confirmation in §4.6.
- **Line removed** items are resolved rows with the chip `Line removed`; they are never re-keyed onto another line. Recompute the line set after removal: if removing the last claimant creates a missing source line, that new missing item appears and needs its own acknowledgement or correction.

Comparison rules the card must reflect: text is trimmed at the ends, case-sensitive, and internal whitespace is preserved; quantity and money compare in stored integer units (`3` and `3.0000` match as quantity; formatted currency is never compared); a disabled line discount is `No discount`, distinct from an enabled 0%. Do not normalise the displayed text beyond the trim.

**Line-set cards**

| Kind | Label and body | Actions |
|---|---|---|
| Missing | `Missing line · Quotation 3` + the source description | `Add line from Quotation` (inserts the line, seeds its items, focuses the new description) and `Acknowledge omission` |
| Extra | `Extra line · Line 4` + the PO description. An unknown reference is labelled `Unknown reference · {reference}` | `Match to Quotation…` (scrolls that line into view and opens its `Quotation match` select; no new picker) and `Acknowledge extra`. For an unknown reference, acknowledgement is offered as the spec requires, but the card must also show the server’s saved-reference blocker if acknowledgement alone cannot finalise; never promise a green path the server will refuse. |
| Duplicate | `Duplicate match · Quotation 2` + `Lines 3 and 5 both claim this line.` + `Blocking` pill | `Fix match` (scrolls the first duplicate line into view and opens its match select). It stays unresolved until one match is corrected. |

`Acknowledge omission` and `Acknowledge extra` resolve with the chips `Acknowledged missing` and `Acknowledged extra`. They are decisions, not checkboxes inside the card; use buttons so the resolution is explicit and attributable.

### 4.6 The compact “Is this correct?” keep-PO step

Clicking **Keep PO** replaces the two action buttons with one inset confirmation in the same card. It is not a dialog, not a popover, and not a page-level modal. Only one keep confirmation can be open at a time; opening another cancels the first without resolving it.

```text
┌──────────────────────────────────────────┐
│ Payment terms                  Unresolved│
│ PO          50% advance, balance 30 days │
│ Quotation   30 days from invoice         │
│ ┌ Is this correct? ────────────────────┐ │
│ │ Keeping the PO value records a       │ │
│ │ deliberate difference.               │ │
│ │ [ Yes, keep PO entry ]   [ Cancel ]  │ │
│ └──────────────────────────────────────┘ │
└──────────────────────────────────────────┘
```

- On opening: focus moves to `Yes, keep PO entry`; the live region says `Keeping the PO value for Payment terms. Is this correct?`
- `Yes, keep PO entry` resolves as `Kept PO`, updates the actor and time, collapses the card to the resolved row, and moves focus to the next unresolved item’s first action (or the count when none remain). Under the `Unresolved` filter the resolved row leaves the list, so determine and focus the next target before the row is removed; focus must never fall to `body`.
- `Cancel` returns to the two actions and focuses `Keep PO`.
- `Escape` cancels the step and returns focus to `Keep PO`. The keep step consumes the key so it does not also close the sheet. It does not navigate.
- `Tab` order inside the step: `Yes, keep PO entry`, then `Cancel`, then the next card. No focus trap.
- There is no timeout. The step stays until confirmed, cancelled, or superseded.
- The button is `Yes, keep PO entry`, not `OK` or `Confirm`; it states exactly what will be recorded.

### 4.7 Resolved rows

A resolved item is one 44px row, expandable to show the values read-only.

```text
Payment terms                        Kept PO   v
Kept by Rina · 24 Oct 2026, 14:32
```

- Chips: `Kept PO`, `Used Quotation`, `Values now match`, `Acknowledged missing`, `Acknowledged extra`, `Match corrected`, `Line removed`. These are display labels for the contract's resolutions `kept_po`, `used_quotation`, `values_now_match`, `acknowledged_missing`, `acknowledged_extra`, `match_corrected`, `line_removed`.
- Secondary line: `{resolution} by {actor} · {time}`. If a later edit makes the values match again, the chip becomes `Values now match` and the secondary line reads `Values now match · {time}`; the item is not deleted and the earlier resolution remains in the session history.
- If a resolved item is edited to a **new differing value** and committed, the same item returns to `Unresolved`; the earlier decision stays in its history and the count goes up again. This is one item, not a duplicate row. The row shows the new values and the two actions.
- Re-committing the same kept value does not ask again and does not add a row; the original `Kept PO` decision and attribution stand.
- The summary row is a button with `aria-expanded`. Expanded content shows the PO value, the Quotation value, the source revision and the resolution time, all read-only. No action buttons.
- Resolved rows remain in `All` and `Resolved`; they never appear in `Unresolved`.

### 4.8 Tax basis context

After linking, the tax basis is a read-only row, not a review item with actions:

```text
Tax basis · linked at revision 02        lock
{tax zone} · {tax type} / {service category}
```

- 11.5 label, 12.5 values, 12px lock glyph, no buttons.
- It is a resolved link-time item, so it appears in `Resolved` as history. It never appears in the unresolved count and has no inline “Use Quotation”.
- Before linking, the editable Source basis card is in the form, not the rail. The link-time choice is made in the existing source-link review, not in this pane.
- Changing the basis later is a correction or source repair, never a rail action.

### 4.9 Filters and the sticky count

- A three-tab segmented control, `role="tablist"`, one tabpanel:
  `All {n}` · `Unresolved {n}` · `Resolved {n}`. Counts are totals, independent of the selected filter.
- 32px high, full width, `bg-paper-muted`, 3px padding, active tab white with a 1px rule and ink-900 text.
- Keyboard: Left/Right moves and activates, Home/End jump to the ends. One tab stop for the group. `aria-selected` on the active tab; `aria-controls` points at the item list.
- Changing the filter resets the item list scroll to the top and announces `Showing 3 unresolved items.` The page scroll does not move.
- The count chip always shows the total unresolved count, regardless of the filter. It includes every unresolved review item: header and line differences, missing/extra lines, duplicates and reopened items. `1 to resolve` is singular; `1 item` in the action bar, not `1 items`. At zero it shows `Nothing to resolve`; the `Unresolved` tab remains available and shows its empty state. At ≥1024 the chip/status element is programmatically focusable (`tabIndex="-1"`) with a visible focus ring, so a resolved-last action can land on it without adding a tab stop. At <1024 the sheet header's count/status is the focus target while the sheet is open; the sticky bar is the target when the sheet is closed.
- The count is not a live region itself. The announcements in §11 include the new count.

### 4.10 Customer PO document strip

- Collapsed: `Customer PO PDF` (11.5 uppercase label), then one of `No document attached` or `{filename} · {size} · {version}`, then `Attach` or `View`.
- Attached actions: `View` (opens the existing preview), and a `⋯` menu with `Replace` and `Remove`. Remove uses the existing inline confirmation, not a new modal. The menu is not placed beside `View` if that crowds the strip; at 320px it wraps to a second row.
- Expanded: the strip shows the existing document panel content — version, uploaded date, earlier-version count, absence confirmation — in the rail, without the surrounding `Card` chrome. One strip expanded at a time.
- Loading: `Loading the customer PO PDF…` with `role="status"`, 44px.
- Error: `The document could not be loaded.` + `Try again`, `role="alert"`, inside the strip.
- No second `Customer PO PDF` section in the form. The rail is the one home for the document.

### 4.11 Totals check strip

- Collapsed: `Totals check` label, then `PO {grand total} · Printed {printed grand total}`, then `Matches`, `Different`, or `Pending`. Minimum 48px; it may grow to 64px if the totals wrap. Money is never truncated.
- Expanded: a compact 3-column table at 320px width: label, `PO`, `Printed`. Rows: Subtotal, Discount, PPN, Adjustment, Grand total. A stage row appears only when it applies (no zero-only rows); Grand total always. 11.5 labels, 12.5 tabular values. Grand total is 13.5 semibold. No horizontal scroll; a value that does not fit wraps under its column label rather than truncating.
- `Matches the printed totals` in success-700.
- `Different · explained by {the input that explains it}` in ink-700; the explanation names one input difference, e.g. `Whole-PO discount 2% vs 0%`. If no single input explains it: `Different · the inputs do not fully explain it`, with the caption `Printed totals are evidence; they do not block Final Save.`
- `Pending` in ink-500 while any input cannot be parsed: `Totals check pending. Fix the highlighted input.`
- With no live source, the strip says `Totals check unavailable until a Quotation is linked.` It has no rows and never blocks. With a former source, the printed totals remain evidence but no new comparison is claimed until a new source is linked.
- The printed figures are evidence. Never show a red “mismatch” or require the PO to equal them. This strip never creates a review item and never changes the count.
- It never asks “Is this correct?”; there is no decision here.

### 4.12 Saved record and session history

- On a saved PO the pane is read-only: no action buttons, no keep step, no inline anchors from fields.
- Count chip shows `{n} resolved`; filters are `All` and `Resolved` only (`Unresolved` is absent, not disabled).
- The header source line includes `Saved session · revision 02`.
- A `Session history` disclosure sits under the totals strip: each previous session shows `Revision {n} · {resolved} resolved · {date}` and opens read-only in place. The current session is not listed twice.
- If the saved record has no session (older data), the pane says `No reconciliation session was recorded for this purchase order.` Do not invent one.
- After a successful admin source repair, the rail shows the new session’s full comparison against the new revision; the previous session moves to `Session history`. An abandoned repair leaves the rail and the record unchanged.
- Corrections do not reopen or write the saved session (ticket 06). The correction editor shows the rail read-only with the caption `The saved reconciliation is history. This correction does not change it.` This consequence is flagged in §14 for Yara.

---

## 5. Shell action bar

The action bar is the only home for Save draft and Final Save. The form footer has no duplicate buttons.

### 5.1 Shape

- Sticky bottom, `z-30`, 1px top rule, `bg-paper/95`, backdrop blur, existing `shadow-rail`.
- ≥1024: 64px high; left status, right actions. Content max 1560px, 32px side padding.
- <1024: auto height, minimum 64px; status line above a 2-column button grid; 44px buttons.
- 360: 20px side padding; `Save draft` and `Final Save` each `1fr`; 8px gap. Status line is one line, truncated with the full text in `title`.
- Focus ring inside the bar must not be clipped (`focus-visible:ring-inset` if needed).
- The bar is a landmark: `role="region" aria-label="Purchase order actions"`.

### 5.2 States and copy

| State | Left status | Right actions | Notes |
|---|---|---|---|
| Clean, persisted, no issues | `All changes saved` | `Save draft` disabled; `Final Save` enabled | `Save draft` is disabled only when persisted and clean. |
| Dirty | `Unsaved changes` | `Save draft` enabled; `Final Save` enabled only if ready | The status is the single save readout. |
| Review items open | `Resolve 3 items to enable Final Save` | `Save draft` enabled; `Final Save` aria-disabled | Activating Final Save focuses the first action of the first unresolved item (or opens the sheet below 1024). |
| Required fields empty, no items | `Complete 2 required fields to enable Final Save` | `Save draft` enabled; `Final Save` aria-disabled | Count may say `Nothing to resolve`; readiness is separate. |
| Source missing | `Link a Quotation to enable Final Save` | `Save draft` enabled; `Final Save` aria-disabled | Includes former-source repair case: `Repair the source to enable Final Save`. |
| Currency mismatch | `Currency mismatch — Final Save is blocked` | `Save draft` enabled; `Final Save` aria-disabled | No amounts are converted or compared. A customer mismatch uses the same shape. |
| Reconciliation unavailable | `Reconciliation unavailable — Final Save is blocked` | `Save draft` enabled; `Final Save` aria-disabled | Load error state. |
| Saving draft | `Saving draft…` | Both disabled; 2px indeterminate line on the bar's top edge | No layout shift. |
| Review step | `Reviewing QT-2026-018 · revision 02` | `Back to editing` secondary; `Continue` primary | The form is replaced by the review view; the bar stays in the same place. |
| Confirmation step | `Ready to save` | `Back to review` secondary; `Save purchase order` primary | Existing confirmation content, including the no-PDF acknowledgement when needed. |
| Saving final | `Saving purchase order…` | Buttons replaced by a disabled `Saving…`; 2px progress line | The line starts after 150ms if still pending; stops on response. |
| Save failed | `Not saved — {short reason}` | `Save draft` or `Try again`; `Final Save` as appropriate | The detailed reason is in the page banner with `role="alert"`. |
| Correction | `Correcting saved record {id}` | `Save correction` primary only | No Final Save in correction mode. |
| Saved read-only | No action bar | — | The saved view owns its own actions. |

One status is shown at a time. Priority: saving or save error > blocking reason (unresolved items, required fields, source, reconciliation) > `Unsaved changes` > `All changes saved`. At 360 the blocking reason wins the one line, because it is the action the user must take next.

### 5.3 Blocked Final Save

- `Final Save` is not `disabled`. It uses `aria-disabled="true"`, keeps its focusability, and on activation does not call the API. Instead it:
  1. announces `Final Save is blocked. Resolve 3 items first.` assertively;
  2. at ≥1024, focuses the first action of the first unresolved item and scrolls it into view in the rail;
  3. at <1024, opens the reconciliation sheet and focuses the first action of the first unresolved item.
- If the blocker is validation rather than a review item, it focuses the first invalid field, opens the field's inline error, and announces the same blocking sentence with the field name.
- If the blocker is a missing or former source, it focuses the `Link Quotation` / `Link new source` / `Repair source` action in the form or pane, whichever is visible.
- If the blocker is an unavailable reconciliation, it focuses the pane's `Try again`.
- The server still refuses the save; `aria-disabled` is a UI promise, not the gate.
- This pattern is chosen so keyboard and screen-reader users can discover why the button does not act. `disabled` would remove it from the tab order and hide the reason.

### 5.4 Save draft

- Enabled when there are unsaved changes or the draft has never been persisted (`orderId === 0`).
- Disabled when the record is persisted and clean, with `title="No unsaved changes"`.
- Saving a draft with unresolved items is allowed. That is the point of a draft. Malformed numeric input is still refused by the existing validation; the draft is not a way to store it.
- A session change is an unsaved change: a new item, a keep/use resolution, an acknowledgement, a corrected match or a line-removal resolution makes the editor dirty. Transient empty/matched notices do not. The dirty flag and the discard guard include the session, not only the form values.
- The draft save persists the reconciliation session (ticket 04). A failed save leaves the local session and values intact and shows the error. A stale session write conflicts on the PO version and opens the existing conflict prompt; never retry silently against a newer version.
- The action-bar status is the primary save readout. Do not also announce the same sentence through a toast; if the existing toast is kept, it must not duplicate the live-region message.

### 5.5 The final review is a receipt, not a second workbench

The live pane is the only place a difference is decided. The Final Save review must not ask the same question twice. The pane replaces the final-save card wall as the decision surface; the review remains the final gate and receipt.

- When Final Save is activated, run the commit handler once for the field that currently has focus, then evaluate readiness against the whole current form and session with the same pure comparison; do not trust the existing count alone. If anything is unresolved or invalid, the review does not open; the pane focuses the first blocker as in §5.3.
- If ready, the review opens with a read-only **Decisions** card. Example structure, not fixed counts:
  - `2 kept from the PO` — field and both values for each;
  - `1 updated from the Quotation`;
  - `1 line acknowledged as missing`;
  - `1 line acknowledged as extra`;
  - `1 match corrected`.
- There are no Keep/Use buttons in the review. The only actions are `Back to editing` and `Continue`. Decisions are changed by going back to the pane, not by re-deciding in the review. A decision row may carry one `Change` text link that returns to the entry view and focuses that item; it is navigation, not a second decision control.
- If the server revalidation returns a blocker the client missed, show it as a blocking banner at the top of the review and return focus to the matching field or item; do not silently drop it and do not present a second choice wall.
- The confirmation step still owns the no-PDF acknowledgement and the confirmation-number choice.
- This keeps the existing #471 review as the final gate without duplicating the live pane. A review that repeats every question is a defect, not a safety feature.

---

## 6. The sheet below 1024

The pane is never a modal. Below 1024 it is a full-content-height sheet with the shell header and action bar still visible and usable. It is a disclosure panel (`role="region"`), not a dialog: `aria-modal` is never set, there is no scrim, and the shell header and action bar are never inert.

### 6.1 Trigger

- A sticky bar directly below the shell header, 56px, full content width, `bg-paper/95`, 1px bottom rule, `z-20`.
- Whole bar is a button: left `Reconciliation` 12.5 medium, optional source line `QT-… · rev 02` at 11.5 (hidden at 360), right count chip and a chevron.
- At zero unresolved the chip is `Nothing to resolve`; the bar still opens so resolved history can be read.
- With no source, the bar reads `Reconciliation · Link a Quotation` and opens the no-source empty state; there is no count. On a former source or saved record it reads `Reconciliation · read-only`; it still opens the history.
- Accessible name: `Reconciliation, 3 items to resolve. Open panel.` At zero: `Reconciliation, nothing to resolve. Open panel.` No source: `Reconciliation, no source linked. Open panel.`
- `aria-expanded` and `aria-controls` point at the sheet.
- Sticky offset: `top: 64px` at 640–1023; `top: 0` below 640, where the shell header scrolls away.
- The bar has `scroll-margin-top: 64px`; the form gets top padding so the bar never covers the first field.

### 6.2 Shape

- **Below 1024:** a full-content-width sheet, full height of the content area between the reconciliation bar and the action bar. It covers the form but not the shell header or action bar. No scrim, no dimming, 1px top rule.
- Sheet header 48px: `Reconciliation` 15 semibold, close button `Close reconciliation` (X icon + accessible name). Count chip and source line follow in the body header.
- The sheet body scrolls. While it is open the page behind does not scroll; only the sheet body does. This is a scroll lock, not a focus trap: the shell header and action bar remain interactive. The pane header, count and filters stick at the top of the sheet. Document and totals strips flow at the end of the sheet rather than pinning, because the sheet is already a scroll context.
- Open at the top on first open. Closing and reopening in the same editing session restores the last sheet scroll and the last focused item; a settings round-trip preserves both.
- When the sheet is open, the form column is `inert` at every width below 1024. The shell header and action bar remain interactive. This prevents focus landing on a covered field and keeps the sheet non-modal with respect to the shell.
- `Escape` closes the sheet and returns focus to the trigger. The close button does the same.
- `Tab` order inside the sheet: close → tabs → items → document → totals → action bar. There is no focus trap; the shell header and action bar remain reachable.
- The sheet does not pause a save in flight, discard anything, or change the review step. It is a view of the same state; closing it unmounts only the view, not the session, count or items.

### 6.3 Opening from a blocked Final Save

At <1024, activating a blocked Final Save opens the sheet and focuses the first action of the first unresolved item. It does not merely focus the trigger. At ≥1024 it focuses the same action in the rail. In both cases the announcement in §5.3 fires.

---

## 7. Back stack and page state

Navigation is a page stack of at most three entries. It is not browser history and there is no router.

```text
[ Home ]  →  [ Purchase Order workspace ]  →  [ Purchase Order settings ]
              (any tab, record, review step)      (or Account settings when opened here)

Back: settings → workspace, exactly as it was
Back: workspace → Home
Back: Home → nothing; no back control is rendered
```

### 7.1 Depth rules

- **Home** is depth 0. It has no back arrow.
- **Workspace** is depth 1. All of its tabs (`Editor`, `Drafts`, `Archive`), an open record, a review step, a confirmation step, a sheet and a list filter are substates at depth 1. Moving between them is lateral: it adds no stack entry, and each tab keeps its own state.
- **Settings** is depth 2. It is the only deeper entry from the workspace. Account settings opened from Home is depth 1 from Home.
- **Back** pops exactly one level. It never goes settings → Home directly, and it never skips the workspace.
- **No other control pops the stack.** The workspace tabs, Home shortcut, list rows and record actions are lateral. `Alt+0` / `Alt+4` / `Alt+6` are direct jumps, not Back; they replace the destination rather than pretending to be a stack pop. An in-page `Back to archive` on a saved record and an editor `Cancel` close the record but stay at depth 1; only the shell back arrow pops.
- **Escape does not navigate.** It closes a sheet, a keep step, a menu or the discard prompt; it never leaves a page.
- **Sign-out and app close** reset the stack to Home and clear all workspace state. Unsaved state is not retained after sign-out or app close, and no draft is written on close.

### 7.2 What the workspace snapshot holds

While the app is open, the depth-1 snapshot is retained in memory (not in localStorage, not in the server) and restored exactly:

- active tab (`Editor`, `Drafts`, `Archive`);
- open record id and phase (`entry`, `review`, `confirm`, `saved`, `correction`, `repair`);
- all unsaved form values, including the transient blank draft;
- review decisions, acknowledged missing/extra lines, document confirmation, confirmation-number choice, and the confirmation payload built at Continue;
- pane filter, item-list scroll, open keep-PO step, which strip is expanded, sheet open/closed and sheet scroll;
- per-tab list state: filters, page, page size, list scroll, filter-panel open state, and the search text;
- page scroll position and the focus descriptor of the last focused control inside `main`;
- `settingsOrigin` (which workspace the settings page belongs to).

The snapshot is used on a tab switch, a Back from settings, and a Back from workspace to Home. A dirty editor is preserved across a settings round-trip without a discard prompt, because it was never unmounted or discarded.

### 7.3 Exact scenarios

The back control's accessible name states its destination: `Back to Home` in a workspace, `Back to Purchase Orders` in PO settings, `Back to Quotation` in Quotation settings, and the workspace name in Account settings when it was opened from one.

| From | Action | Result | Focus and scroll |
|---|---|---|---|
| Home | Open Purchase Orders | Workspace opens on the last active tab and restores the last open record if it was clean; on first use it opens `Editor` with no record. After a discard, the record is closed. | Page title `h1`; page top. |
| Workspace, any tab | Back arrow | Home. A dirty editor or unsaved review decisions open the discard prompt first. Clean state is retained for the session. | Home heading; Home scroll position. |
| Workspace, dirty editor | Back arrow | `Discard unsaved changes?` dialog. `Keep editing` stays; `Discard changes` goes Home and drops the working copy. | Dialog focus is trapped; on discard, Home. |
| Workspace | Click another tab with a dirty editor | The same discard dialog. `Keep editing` stays on the current tab; `Discard changes` switches and drops the working copy. | Dialog focus; on discard, the destination tab. |
| Workspace | Profile → Purchase Order settings | Settings page, depth 2. The workspace stays alive in memory. | Settings `h1`; settings top. |
| Saved record | `Back to archive` (in-page) | Archive tab with its list state; stack depth stays 1. This is not the shell Back. | Previous list scroll and row focus. |
| Purchase Order settings | Back arrow | The same workspace: same tab, record, values, review step, pane filter, sheet state, scroll and focus. No discard prompt. | The last focused control inside `main`; if it no longer exists, the page `h1`. Scroll restored. |
| Purchase Order settings | `Alt+4` | Direct jump to the Purchase Order workspace, not a Back. The settings entry is replaced; Back from the workspace goes Home. | Workspace `h1`. |
| Quotation settings | Back arrow | The Quotation workspace, same rule. | Last focused control or `h1`. |
| Account settings opened from the PO workspace | Back arrow | The PO workspace, same rule. | Last focused control or `h1`. |
| Account settings opened from Home | Back arrow | Home. | Home heading. |
| Any workspace | Sign out | The discard guard runs first if the editor is dirty. On sign-out, login screen; all workspace snapshots are cleared. | Login focus. |
| Any workspace | App close | Nothing is retained and nothing is saved. | — |

### 7.4 Discard copy

The existing dialog assumes “The draft was saved earlier.” Add the never-persisted case:

- Never persisted (`orderId === 0`): `This purchase order has not been saved. Its entries will be lost.`
- Persisted with changes: `The draft was saved earlier. Changes since then will be lost.`
- Review decisions changed but values saved: the persisted copy; the review decisions are part of the unsaved state.
- Correction mode: `The correction has unsaved changes. The saved record will keep its previous values.`

### 7.5 Focus restoration

- Before opening settings, capture a stable focus descriptor from the last focused element inside `main` (field `aria-label`, item key + action, filter tab, or list row id). Do not capture the profile-menu trigger as the return target.
- On return, focus that descriptor if it still exists and is visible. Otherwise focus the page `h1` (`tabIndex="-1"`).
- Restore the page scroll after the first paint, then the rail/sheet scroll. If the list has fewer pages on return, clamp to the last valid page and show the normal page-1 empty state.
- Announce nothing for a plain return; the focused heading or field is the context. If the state could not be restored (e.g. the record was deleted elsewhere), show the normal load error and focus its heading.

---

## 8. Docket list

The list reads as order dockets, not a generic table. Drafts and Archive remain separate tabs with server paging. No amount column and no money field in the query.

### 8.1 At ≥1024: the docket table

| Column | Content | Width guidance at 1024 |
|---|---|---|
| Customer PO | `BP-1042` 13.5 medium tabular; `Not entered` in ink-400 when blank; currency code `IDR` 10.5 ink-400 below | 24% |
| Quotation | `QT-2026-018` 13.5 tabular; `Revision 02` 11.5 below; `Former quotation · revision 02` or `No quotation linked` when applicable; `Source missing` badge below when set | 26% |
| Customer | Customer name, up to two lines | 22% |
| Received | `12 Oct 2026`, tabular, one line | 14% |
| State | `Draft` accent badge or `Saved` success badge | 9% |
| Actions | `Open` secondary 64×32 | 80px fixed |
| Admin | `⋯` 32×32 in its own column, after a 1px separator and 16px gap; omitted entirely for non-admins | 48px fixed |

- Header 40px, rows 56px, 1px row rules, hover `bg-paper-subtle/60` for 150ms. No row click; the only open target is the button.
- Column percentages are approximate; `Open` is fixed at 80px and the admin `⋯` column at 48px (admin only), and the five data columns share the rest. At 1024 the table must fit without horizontal scroll.
- No `min-w-[900px]` and no horizontal scroll at 1024. If the content does not fit, the quotation/customer cells wrap; the table never scrolls sideways.
- `Open` is a secondary button, not a link and not a whole-row click. It is labelled `Open purchase order {PO number or id}`. On activation it shows `Opening…` and disables until the record loads; on failure re-enable and show the existing toast.
- Admin `⋯` has the accessible name `More actions for purchase order {n}` and one menu item, `Delete purchase order…`, in danger text. Non-admins see no `⋯` at all, not a disabled one.
- No amount, no margin, no totals in the list. The currency code is the only financial context and stays a quiet subline.

### 8.2 Below 1024: stacked cards

```text
┌─────────────────────────────────────────────┐
│ BP-1042 · IDR                      [Draft] ⋯│
│ PT Maju Jaya                                │
│ QT-2026-018 · Revision 02                   │
│ [Source missing]                            │
│ Received 12 Oct 2026                        │
│ [                 Open                    ] │
└─────────────────────────────────────────────┘
```

- 12px padding, 1px rule, 8px radius, white; 12px gap between cards; no shadow.
- Line 1: PO number 13.5 medium + currency 10.5 + state badge; admin `⋯` at the far right, 44×44.
- Line 2: customer 13.5; line 3: quotation + `Revision 02` 11.5; Source missing badge below; received date 11.5.
- `Open` is a full-width 44px secondary button at the bottom, separate from the `⋯`.
- At 360 the card is 320px wide with 20px page padding; it must not scroll horizontally.
- No row/card click; only Open opens.

### 8.3 Filters

- ≥1024: search full width on the first row; state is pinned by the tab (Drafts/Archive) and not shown; received-from, received-to, source filter, customers, active filter count and Reset filters as today.
- <1024: search stays visible; a `Filters` button with an active-count badge toggles the rest. The panel contains received-from/to, source, customers; `Reset filters` inside. Active filters appear as removable chips under search (`Received 1–31 Oct ×`, `Customer: PT Maju ×`). The panel open state and the chips survive a settings round-trip.
- Filtering is immediate with the existing debounce; no Apply button.
- Search placeholder: `PO number, quotation number, or customer`.
- Source filter labels stay exactly `Any source`, `Source missing only`, `Source present only`.
- Do not add an amount filter or a state filter at <1024; state is the tab.

### 8.4 List states

| State | Visible result |
|---|---|
| Loading, no rows | Four skeleton rows 48px (cards at <1024); `aria-label="Loading purchase orders"`; no spinner. |
| Loading, rows present | Keep the rows; dim the table to 60% opacity and disable paging; no full skeleton. |
| Empty, no filters | `No draft purchase orders` / `Start one from an issued quotation revision.` + primary `Add Purchase Order from a Quotation`; Archive: `No saved purchase orders yet` / `Saved records appear here after a draft is finalized.` |
| Empty, filters active | `No purchase order matches these filters` / `Clear the filters to see the whole archive.` + `Reset filters`. |
| Error | `role="alert"` with the server message or `The purchase orders could not be loaded. Try again.` + `Try again`. |
| Paging | `{total} purchase orders · page {n} of {m}` 11.5; `Previous` / `Next`, disabled at the ends. At 360 both 44px. |
| Opening a row | `Opening…` on that row's button; others remain enabled; failure restores the button and shows the toast. |

### 8.5 State retention

Drafts and Archive each keep their own filters, page, page size, scroll and filter-panel state while the app is open. Opening a record and returning to the list restores the list state. A settings round-trip restores it exactly, including the 360 filter panel and the chips.

---

## 9. Motion

Motion is enter-only. Nothing animates out, nothing staggers, and nothing animates height.

### 9.1 Spec

| Moment | Enter | Duration / easing | Reduced motion |
|---|---|---|---|
| First entry to a workspace from Home | opacity 0→1 + translateY(4px→0) | 200ms, ease-out-quart | opacity only, 0ms or ≤80ms |
| Rail entrance at ≥1024, first editor mount only | opacity 0→1 + translateX(6px→0) | 200ms, ease-out-quart | opacity only |
| New pane item | opacity 0→1 + translateY(4px→0) | 180ms, ease-out-quart | opacity only |
| Inline notice | opacity 0→1, no movement | 120ms, ease-out-quart | opacity only |
| Resolved row replaces a card | opacity 0→1 + translateY(2px→0) | 180ms, ease-out-quart | opacity only |
| Sheet open at <1024 | opacity 0→1 + translateY(12px→0) | 220ms, ease-out-quart | opacity only |
| Sheet close | removed immediately, no exit animation | 0ms | same |
| Review / confirmation view | opacity 0→1 + translateY(6px→0) | 200ms, ease-out-quart | opacity only |
| Error banner | opacity 0→1 + translateY(4px→0) | 180ms, ease-out-quart | opacity only |
| Row hover / button press | colour/background; button active scale 0.98 | 120–150ms | colour only; no scale |
| Final-save progress line | 2px line, one calm sweep using `transform`, not width | 1.1s per cycle, only while the request is pending; starts after 150ms | static 2px line at 40% opacity |
| Saved record arrival | opacity 0→1 + translateY(6px→0) | 220ms, ease-out-quart | opacity only |

**Rules**

- Do not re-run the page, rail or sheet entrance when returning from settings, changing a tab, or resolving an item. Only the new content enters.
- No exit animation anywhere, including sheet close, filter changes, resolved rows and route changes. Removing an item is instantaneous; the next item moves up. The only fade-out is the progress line itself when the save response arrives; no content animates out.
- Never animate height, width, top, left, margin or padding. For an expandable strip use an instant show/hide or a grid-row transition only if it can be done without layout jank; the default is instant.
- No stagger, no bounce, no elastic, no confetti, no parallax, no perpetual animation. The only repeating motion is the in-flight save line, and it stops on the response.
- No animation on typing, on the count changing, or on the initial page load before the content is ready.

### 9.2 Final Save handoff

The signature moment is a three-step arrival, not a spinner:

1. **Editor → review.** Final Save (when ready) enters the read-only receipt described in §5.5 with 200ms fade + 6px rise. The action bar swaps to `Back to editing` / `Continue` in the same position; no layout jump. Focus moves to the review `h1`. The form scroll position is remembered for Back.
2. **Review → confirmation.** Continue enters the confirmation view with 200ms fade + 6px rise. Focus moves to the confirmation `h1`. If the no-PDF acknowledgement is required, it is the first control after the heading.
3. **Confirmation → saved record.** `Save purchase order` puts the action bar into the progress state: status `Saving purchase order…`, buttons replaced by a disabled `Saving…`, and a 2px accent line sweeping once per 1.1s, starting after 150ms if the request is still pending. There is no fake percentage. Keep the progress state visible for at least 300ms so the arrival does not flash, and never delay longer than that artificially. On response:
   - Success: the progress line completes and fades 200ms; the saved record enters with 220ms fade + 6px rise; focus moves to the saved `h1`; the confirmation number block is visible above the fold; the live region announces the result politely.
   - Failure: the saved record does not enter. The confirmation view returns with an 180ms error banner, focus moves to the banner, and the action bar restores its previous actions. The working copy is intact.
   - Conflict: the existing conflict prompt opens; do not animate the page underneath a modal.
4. **Arrival.** The saved record's top block shows `Purchase order saved`, the confirmation number (or `No confirmation number assigned`), and `Read-only`. No confetti, no checkmark burst, no sound. Calm is the point.

### 9.3 Reduced motion

The current `index.css` blanket rule sets `animation-duration` and `transition-duration` to `0.01ms` for every element. That also removes opacity and colour transitions, which the spec wants to keep. Replace it with targeted rules:

- Remove `transform` transitions and animations; disable `active:scale`.
- Keep opacity, background-color, border-color and color changes, at ≤80ms, or make them instant if the user prefers.
- Keep focus rings and state colours; never remove a focus indicator.
- The progress line becomes a static 2px line at 40% opacity plus the `Saving purchase order…` text.
- No `scroll-behavior: smooth`.
- The saved arrival is an opacity change only; no rise.

This is a required change in ticket 07, not an optional polish.

### 9.4 List motion

Rows and cards have hover and press feedback only. No entrance stagger, no row transform, no hover lift. Paging replaces the rows instantly; no crossfade of the whole table.

---

## 10. Focus and keyboard

### 10.1 First focusables

- Add a `Skip to main content` link as the first focusable element in the app shell; it targets `<main id="main-content" tabIndex="-1">` and is visually hidden until focused.
- At ≥1024, add a `Skip to reconciliation` link immediately after the page title, before the form. It targets the pane heading. It is visually hidden until focused and is the fastest keyboard path from the title to the decisions.
- At <1024 the sticky reconciliation bar is already before the form in DOM order, so no second skip link is needed; the bar is the target.

### 10.2 Tab order at ≥1024

1. Skip to main content.
2. Shell: Back, Editor/Drafts/Archive tabs, notifications, profile.
3. Main: page `h1` is not a tab stop; `Skip to reconciliation` is.
4. Source action (`Choose Quotation` / `Link Quotation`) when present.
5. Order details fields in visual order.
6. Commercial terms: payment terms, Incoterms, whole-PO discount.
7. Lines, in visual order: each line's match control (and the select when open), description, quantity, UoM, unit price, row-discount checkbox, percent, Remove; then `Add line`.
8. Adjustment, adjustment reason.
9. Form totals (no focusable content).
10. Rail: pane heading (only when skipped to), filter tabs (one stop), item actions in list order, document actions, totals disclosure.
11. Action bar: Save draft, Final Save.

The DOM order is form → rail → action bar. The visual order matches at ≥1024. Do not use positive `tabindex`.

### 10.3 Tab order at <1024

1. Skip to main content.
2. Shell header.
3. Sticky reconciliation bar (one stop, opens the sheet).
4. Form fields, in the same order as above.
5. Action bar.

When the sheet is open:

1. Sheet heading (`tabIndex="-1"`, focused on open).
2. Close.
3. Filter tabs.
4. Item actions in list order.
5. Document actions.
6. Totals disclosure.
7. Action bar (header remains reachable with Shift+Tab; there is no focus trap).

The form is `inert` while the sheet is open. The shell header and action bar are not.

### 10.4 Keys

| Key | Where | Result |
|---|---|---|
| `Tab` / `Shift+Tab` | Everywhere | Natural order above; focus ring always visible. |
| `Enter` | Item action | Keep PO opens the confirmation; Use Quotation resolves; Yes resolves; a disclosure toggles. |
| `Space` | Buttons, checkboxes, disclosure | Standard activation/toggle. |
| `Escape` | Keep step | Cancel the step, return focus to Keep PO. |
| `Escape` | Sheet | Close the sheet, return focus to the trigger. |
| `Escape` | Menu / discard prompt | Close / keep editing; the prompt already traps focus. |
| `Escape` | Anywhere else | No navigation. |
| `Left` / `Right` | Filter tabs | Move and activate the previous/next filter; wrap. |
| `Home` / `End` | Filter tabs | First/last filter. |
| `Enter` in a text field | Form | Moves to the next field only if the form already does so; it must never save or resolve. |
| `Alt+0`, `Alt+4`, `Alt+6` | Global | Existing direct jumps. They are not Back and do not restore the stack. |
| `?` | Global | Existing shortcuts overlay. It takes focus as a modal and does not resolve or save anything. |

Do not add a keyboard shortcut for Keep PO, Use Quotation or Final Save. Decisions must be deliberate and must not be triggerable by a stray key in a text field.

### 10.5 Focus after actions

| Action | Focus moves to |
|---|---|
| Commit a field (blur) | Stays on the field / next field; the notice is announced, not focused. |
| Use Quotation | First action of the next unresolved item; if none, the count/status element (`tabIndex="-1"`). |
| Keep PO → Yes | First action of the next unresolved item; if none, the count/status element (`tabIndex="-1"`). |
| Keep PO → Cancel / Escape | The same item's Keep PO. |
| Filter change | The active tab (focus stays); list scroll resets to top. |
| Open sheet | Sheet heading. |
| Close sheet / Escape | The sticky trigger. |
| Blocked Final Save | First action of the first unresolved item (sheet opens at <1024) or first invalid field. |
| Successful final save | Saved record `h1`. |
| Final save failure | Error banner (`tabIndex="-1"`), then the first action. |
| Review `Change` link | Back to entry, focus that item and its first action. |
| Settings return | Last focused control or page `h1`. |
| Delete dialog close | The `⋯` trigger when the row still exists; after a successful delete, the next row's `Open` button, or the list heading/empty state when none remain. |

### 10.6 Focus visibility and target size

- Focus ring: 2px accent-500 with 2px offset; on an accent-filled control use a white inner ring so it remains visible. Never `outline: none` without a replacement.
- 44px minimum target at <1024 and on the sheet; 32px minimum on the desktop rail with 8px between actions. The WCAG 2.2 minimum is 24px; the desktop pane may use 32px because it is dense and pointer-first, but not smaller.
- Sticky bars must not cover a focused control: `scroll-padding-bottom: 96px` on the page/scroll container. Use `scroll-margin-top: 88px` at ≥1024 (64px header + 24px gap); at <1024 use `scroll-margin-top: 136px` (64px header + 56px reconciliation bar + 16px gap), or 72px at 360 where the header has scrolled away.

---

## 11. Live regions and announcements

Use one polite reconciliation status region and the existing alert banners; do not make the whole list `aria-live`.

| Event | Announcement | Region |
|---|---|---|
| Empty commit | `Quantity is empty.` | polite |
| Invalid number commit | `Quantity must be greater than zero.` | polite |
| Match commit | `Payment terms: matches the Quotation.` | polite |
| Difference commit | `Payment terms: differs from the Quotation. 3 items to resolve.` | polite |
| Keep step opens | `Keeping the PO value for Payment terms. Is this correct?` | polite |
| Keep resolved | `Payment terms kept from the PO. 2 items to resolve.` | polite |
| Use Quotation | `Payment terms updated from the Quotation. 2 items to resolve.` | polite |
| Missing acknowledged | `Quotation line 3 acknowledged as missing. 4 items to resolve.` | polite |
| Extra acknowledged | `Line 4 acknowledged as extra. 4 items to resolve.` | polite |
| Filter change | `Showing 3 unresolved items.` | polite |
| All items resolved | `All differences resolved.` If required fields are empty: `All differences resolved. 2 required fields are still empty.` | polite |
| Line removed | `Line 2 removed. Its review items are kept as history.` | polite |
| Draft saved | `Draft saved.` | polite (action-bar status) |
| Correction saved | `Correction saved.` | polite (action-bar status) |
| Blocked Final Save | `Final Save is blocked. Resolve 3 items first.` or `Final Save is blocked. Complete 2 required fields first.` | assertive |
| Duplicate created | `Lines 3 and 5 both claim Quotation 2. Correct one match before Final Save.` | assertive |
| Save failure | `The purchase order was not saved. {reason}` | assertive (banner) |
| Reconciliation load failure | `The reconciliation could not be loaded.` | assertive (banner) |
| Final save success | `Purchase order saved. Confirmation number {n}.` or `Purchase order saved.` | polite (saved view status) |

Rules:

- The polite region is visually hidden, `aria-atomic="true"`, and lives once at the workbench root, not inside the sheet or the pane, so commit/resolve announcements still fire while the sheet is closed at <1024. It is not rendered when the workbench itself is not mounted.
- At ≥1024 the same region serves the inline pane; at <1024 it serves the closed sheet and the sheet when open.
- The visible notice is not itself a live region; this avoids double announcements.
- Do not announce the initial load, the initial count, or a count that changed only because a saved draft was rehydrated. Those are context, not events.
- Do not announce every focus move. Focus movement plus a focused heading is enough.
- The count chip is not a live region. Every event that changes the count includes the new count in its one sentence.
- The action-bar save-status text is a polite status region for `Draft saved` / `Correction saved`; it is not an alert, and the rest of the bar is not live.
- Save success uses one channel. If the existing toast remains, it must not repeat the same sentence as the saved view's polite status line; focus to the saved heading is the primary context.

---

## 12. State inventory

| # | Surface | State | What is visible |
|---|---|---|---|
| S1 | Workbench | Opening a record | Skeleton page or the list row's `Opening…`; no stale record. |
| S2 | Workbench | Load failure | Error banner + `Try again` + `Back`; no editor. |
| S3 | Pane | No source | `Link a Quotation to start the comparison.` + `Link Quotation`; no filters/count. |
| S4 | Pane | Loading | Three skeletons + visually hidden `Loading the reconciliation…`. |
| S5 | Pane | Load error | Alert, `Try again`, `Save draft still works. Final Save is blocked until it loads.` |
| S6 | Pane | Linked, no items | `No differences yet` + teaching line; real filter counts; the link-time tax row appears if recorded. |
| S7 | Pane | Unresolved filter empty | `No unresolved items` / `No differences to resolve.` + `Show all`. |
| S8 | Pane | Resolved filter empty | `No resolved items yet` + `Show all`. |
| S9 | Pane | Empty notice | Inline `{Field} is empty`; latest-notice strip; no item, no count change. |
| S10 | Pane | Matched notice | Inline `Matches the Quotation`; latest-notice strip; no item. |
| S11 | Pane | Unresolved item | Card with label, `Unresolved`, both values, Keep PO / Use Quotation. |
| S12 | Pane | Keep confirmation | `Is this correct?` inset, `Yes, keep PO entry` / `Cancel`; only one at a time. |
| S13 | Pane | Resolved | 44px row + chip + actor/time; expandable read-only values. |
| S14 | Pane | Values now match / re-opened | Resolved chip `Values now match`; a new differing commit returns the same item to unresolved with its history retained. |
| S15 | Pane | Missing / extra / unknown / duplicate | Line-set card with the named action(s); duplicate is `Blocking`; unknown reference also names the saved-reference blocker. |
| S16 | Pane | Tax basis | Read-only locked row, link-time values, no actions. |
| S17 | Pane | Totals check | Collapsed `Matches` / `Different` / `Pending`; expanded 5 rows; never a decision. |
| S18 | Pane | Document none / attached / loading / error | Compact strip with Attach/View/menu, skeleton, or alert. |
| S19 | Pane | Saved read-only | No actions; `{n} resolved`; All/Resolved; history disclosure. |
| S20 | Pane | Source missing / former / currency mismatch | Link/repair state or blocking mismatch; Final Save blocked; no conversion. |
| S21 | Action bar | Clean / dirty / blocked / saving / review / confirm / saving final / failed / correction | §5.2 table. |
| S22 | Sheet | Closed / open below 1024 | 56px trigger; full-width content-height sheet; form inert; Escape closes. |
| S23 | List | Loading / loaded / filtered empty / empty / error / opening a row | §8.4. |
| S24 | List | Non-admin | No `⋯`, no delete menu, no repair action. |
| S25 | Settings | Opened from workspace, dirty editor behind | Settings page; Back returns to the intact workspace. |
| S26 | Global | Reduced motion | No transforms; opacity/colour only; static progress line. |
| S27 | Global | 200% zoom / 1280 effective 640 | Single column and cards; no horizontal scroll; controls reachable. |
| S28 | Global | Long payment terms / 20 lines / long customer name | Wraps in cards and rail; list scrolls; no truncation of compared values. |
| S29 | Review | Final receipt | Read-only `Decisions` card; no Keep/Use buttons; `Back to editing` / `Continue`; server blockers as a banner. |

---

## 13. Implementation notes for Loki

- **Two-pane:** viewport breakpoint at `lg` (1024) decides the grid; a container query on the form column decides the line-card and field layout. Do not use viewport `sm:` inside the form column.
- **Rail:** one `aside`, sticky, own scroll only on the item list. Do not nest `Card` components; use hairlines.
- **Sheet:** one view component used at both sheet widths; `role="region"` (not `aria-modal`), focus to heading, form `inert`, Escape, focus return.
- **Action bar:** extend the existing `AppShell` `actionBar` slot rather than creating a second fixed bar. Keep the existing quotation action bar untouched.
- **State:** the stack and workspace snapshot belong in the existing shell/workspace controller; no new global store and no router (ticket 02).
- **List:** render a table at ≥1024 and cards below; do not make the table responsive with `display:block` and lose table semantics. Remove `min-w-[900px]`.
- **Motion:** add named classes/animations only for the moments in §9; fix the blanket reduced-motion rule in `index.css`.
- **No new colours or fonts.** Use `accent`, `success`, `ink`, `paper`, `rule`, `danger`; no amber in this feature.
- **Accessible names:** include the field in every repeated action (`Keep PO entry for Payment terms`); visible labels stay short. Field item titles use the contract labels (`Payment terms`, `Line 2 unit of measure`, `Whole-PO discount`); do not invent a parallel label that the fixture and server do not share.
- **Test seams:** the pane state and commit handler are testable through the existing pure comparison and reducer paths; component tests use the fake backend. Do not add a new seam inside the pane if the pure functions can be tested directly (spec §Testing Decisions).
- **Stable identity:** never key a pane item by the displayed line number or array position. Use the contract’s stable line key; the displayed number may change. Format money, quantity and percent only in the view; compare stored numbers, not formatted strings.
- **Final receipt:** §5.5 means the review’s decisions are read from the session or pre-filled from it, not asked again. If the existing finalize input cannot carry a kept-PO resolution, raise it before building a second choice wall.

---

## 14. Open questions and flags

These do not block the layout brief; they block the relevant slice being called done.

1. **Correction mode and the session.** Ticket 06 says a saved record’s session is read-only and corrections do not change it. The rail therefore shows history read-only during a correction, even if the correction changes a compared value. Confirm with Yara that this is the intended audit story; if a correction should reopen reconciliation, that is a spec change, not a UI detail.
2. **Former-source actions.** The brief offers `Link new source` and, for admins, `Repair source`. Confirm the exact action set and whether a non-admin can do anything but wait.
3. **`aria-disabled` Final Save.** The pattern keeps the reason discoverable. Confirm Loki/Kaluna are happy with a non-disabled button that does not call the API; the server remains authoritative.
4. **1024 line layout.** The two-pane at 1024 depends on container queries. If the form column cannot reflow its line cards, the two-pane cannot start at 1024 without overflow. That would be a spec conflict, not a reason to move the breakpoint silently.
5. **Reduced-motion global rule.** The current `index.css` rule suppresses every transition. The brief requires targeted rules that keep opacity and colour. This is a ticket 07 change.
6. **Saved session history placement.** The rail’s `Session history` disclosure is the proposed home. If Clark prefers it on the saved page outside the rail, keep the same content and read-only rule; do not create a second editable surface.
7. **No mobile client.** 360 is a narrow desktop window. Do not add touch-only gestures, a bottom navigation bar, or a mobile route.
8. **Live-to-final decision mapping.** §5.5 treats the Final Save review as a read-only receipt. The finalize API must receive the live session’s resolutions (kept PO, acknowledged missing/extra, corrected match), or the server will re-ask or refuse. Confirm with Angkasa and Kaluna. If the API cannot carry them, the review is pre-filled from the session rather than a second blank card wall, and that is a ticket 04/05 change, not a silent UI workaround.
9. **Requiredness stays as it is.** The contract records that payment terms and Incoterms may be empty today. The pane shows empty notices only for fields the existing validation requires; comparing a field does not make it required. Confirm with Yara before anyone “tightens” this in the UI.
10. **Unknown reference acknowledgement.** The spec treats an unknown reference as extra and acknowledgeable; the contract records that the saved-reference validator may refuse a non-existent non-null reference. The UI shows both the acknowledgement and the server blocker; do not claim it finalises until Yara/Angkasa rule which action clears it.
11. **Tax-basis correction command.** The contract says the current correction path freezes a linked basis and source repair adopts the source basis. The pane offers no inline change. The permitted correction command remains a product/backend decision; do not add a rail action for it.

---

## 15. Final polish checklist for the built surface

Run this after Loki’s implementation and before Frank signs off; this is where the loaded **critique** and **polish** skills are actually used.

- [ ] No horizontal scroll at 360, 1024 or 1280.
- [ ] Rail is one surface, not nested cards; separators are 1px; no side-stripe borders.
- [ ] Every repeated action has a unique accessible name including its field/line.
- [ ] Difference is never red; match is the only green; amber is absent.
- [ ] Numbers are tabular; money and quantities are not truncated.
- [ ] Empty/loading/error/blocked/resolved states all present and distinct.
- [ ] No duplicate Save/Final buttons in the form.
- [ ] Focus ring visible on every control, including inside the action bar and on the sheet.
- [ ] Escape behaviour is correct and does not navigate.
- [ ] No exit animation, no stagger, no height animation.
- [ ] Reduced motion keeps opacity/colour and removes transforms.
- [ ] The 1024 line card reflows at 616px without overflow.
- [ ] The sheet never covers the shell header or action bar.
- [ ] Back never goes settings → Home directly.
- [ ] A dirty editor survives settings and asks before leaving the workspace.
- [ ] The action bar is reachable while the form scrolls at 360, 1024 and 1280.
- [ ] The saved arrival has one announcement and one focus target.
- [ ] No toast repeats a live-region sentence.

## 16. Acceptance surface for Frank

Use a draft created from an issued quotation revision with at least: one differing payment term, one differing Incoterm, one differing whole-PO discount, two differing line fields, one missing line, one extra line, and one duplicate match. A smaller fixture is acceptable if each state is reached deliberately; do not fake a state with a snapshot-only prop if the real path can be driven.

**Viewports:** 1280×900 (snapshot + hand-driven), 1024×768 (hand-driven; the snapshot harness only renders 360 and 1280), 360×900 (snapshot + hand-driven). The 360 view is a narrow desktop window, not a mobile client.

### 16.1 Layout and static states (snapshots allowed)

| ID | Setup | Expected visible result | Mode |
|---|---|---|---|
| L1 | 1280, linked draft with items | Two panes: form 828, 28px gutter, rail 360. Pane visible; action bar visible; no horizontal scroll. | Snapshot 1280 |
| L2 | 1024, same draft | Two panes: form 616, 24px gutter, rail 320. Line cards are two-row; fields do not overflow; no horizontal scroll. | Hand-driven 1024 |
| L3 | 360, same draft | One column, no rail; sticky `Reconciliation · 3 to resolve` bar below the header; action bar with two 44px buttons; no horizontal scroll. | Snapshot 360 |
| L4 | 360, sheet open | Sheet fills between the reconciliation bar and action bar; document and totals at the end; header/action bar visible. | Snapshot 360 if the harness exposes the open-sheet view; hand-driven always |
| L5 | 1280, Drafts and Archive lists (two snapshots) | Docket table with PO, Quotation + revision, Customer, Received, State, Open; no amount column; admin `⋯` separated from Open. | Snapshot 1280 |
| L6 | 360, Drafts and Archive lists (two snapshots) | Stacked cards; state badge; quotation/revision; Source missing badge when set; full-width Open; admin `⋯` at the top right. | Snapshot 360 |
| L7 | 360, list empty/loading/error | Empty state with one action; four card skeletons; alert + Try again. | Snapshot 360 |
| L8 | 1280, saved record | Read-only rail; `{n} resolved`; no action buttons; session history disclosure. | Snapshot 1280 |
| L9 | 1024, form scrolled to lines | Rail stays at top 88 while the form scrolls; action bar stays at the bottom. | Hand-driven 1024 |
| L10 | 360, 20 lines + long payment terms | Cards and values wrap; list scrolls; no truncation of compared values; no horizontal scroll. | Hand-driven 360 |
| L11 | 1280, final review with decisions | Read-only `Decisions` receipt; kept/used/acknowledged rows; no Keep/Use buttons; action bar `Back to editing` / `Continue`. | Snapshot 1280 |
| L12 | Workbench load failure | Error banner with the server reason, `Try again`, `Back`; no editor; no stale record. | Snapshot 1280 + hand-driven |

### 16.2 Pane states (snapshot and DOM)

| ID | Setup | Expected visible result | Mode |
|---|---|---|---|
| P1 | No source | `Link a Quotation to start the comparison.` + `Link Quotation`; no count/filters. | Snapshot 1280 + DOM |
| P2 | Linked, no items | `No differences yet` + teaching line; real filter counts; the link-time tax row if recorded. | Snapshot 1280 + DOM |
| P3 | Empty required field, committed | Inline `{Field} is empty`; latest-notice strip; no item; count unchanged. | DOM + hand-driven |
| P3b | Invalid non-empty number, committed | Inline danger message and border; no review item; Final Save blocked with the field named. | DOM + hand-driven |
| P4 | Matching field, committed | Inline `Matches the Quotation`; latest strip; no item; count unchanged. | DOM + hand-driven |
| P5 | Differing field, committed | One unresolved card with both values; count increments once; no duplicate on a second blur. | DOM + hand-driven |
| P6 | Keep PO clicked | `Is this correct?` inset; `Yes, keep PO entry` focused; the two original actions gone; one confirmation only. | Snapshot 1280 + hand-driven |
| P7 | Keep confirmed | Resolved row `Kept PO` + actor/time; count decrements; focus next item. | DOM + hand-driven |
| P8 | Use Quotation | Field updates; resolved row `Used Quotation`; no matched notice; focus next item. | DOM + hand-driven |
| P9 | Resolved then edited to source | `Values now match`; item retained; no notice. | DOM |
| P9b | Resolved then edited to a new difference | The same item returns to `Unresolved`; earlier decision remains in history; count increments; no duplicate row. | DOM + hand-driven |
| P10 | All/Unresolved/Resolved | Counts are totals; active tab; Unresolved empty says `No unresolved items`; Resolved empty says `No resolved items yet`. | Snapshot 1280 + DOM |
| P11 | Missing/extra/duplicate | Named cards; duplicate has `Blocking`; acknowledgement resolves; duplicate stays until a match is corrected. | Snapshot 1280 + DOM |
| P11b | Change a line match | Inline `Quotation match` select opens under the line header; choosing recomputes the line set and items; no modal. | Hand-driven + DOM |
| P12 | Tax basis after link | Read-only locked row; no buttons; absent from unresolved count; present in Resolved. | Snapshot 1280 |
| P13 | Totals check | Matches / Different + reason / Pending; no decision buttons; no count change. | Snapshot 1280 + DOM |
| P14 | Document none/attached | Compact strip with Attach or filename + View; expandable; no second document section in the form. | Snapshot 1280 |
| P14b | Document loading/error | `Loading the customer PO PDF…`; on failure `The document could not be loaded.` + `Try again`, inside the strip. | DOM + hand-driven |
| P15 | Pane loading / error | Three skeletons / alert + Try again; form still usable; Final Save blocked. | DOM + hand-driven |
| P16 | Saved read-only | No actions; All/Resolved; history; no invented session. | Snapshot 1280 |
| P17 | Source missing / former | Link/repair state; Final Save blocked; former source explains read-only. | Snapshot 1280 + hand-driven |
| P18 | Currency or customer mismatch | Blocking banner names both sides; no comparison items; Final Save blocked; no conversion. | Snapshot 1280 + hand-driven |

### 16.3 Interaction and state (hand-driven or DOM, never snapshot-only)

| ID | Setup | Expected visible result | Mode |
|---|---|---|---|
| I1 | Type in Payment terms without leaving | No notice, no item, count unchanged. | Hand-driven + DOM |
| I2 | Tab out of a differing field | One item appears at the top; count increments; latest strip does not duplicate it. | Hand-driven + DOM |
| I3 | Blur the same field twice | Same item key; no second card; values update in place. | DOM |
| I4 | Keep PO → Escape | Step closes; focus returns to Keep PO; item unresolved; count unchanged. | Hand-driven |
| I5 | Keep PO → Cancel | Same as Escape. | Hand-driven |
| I6 | Use Quotation while the field is offscreen | Field updates; the form does not scroll; focus moves to the next item. | Hand-driven |
| I7 | Resolve the last item at ≥1024 | Count becomes `Nothing to resolve`; focus moves to the count/status; announcements match §11. | Hand-driven |
| I8 | Resolve the last item at 360 | Sheet stays open; count says nothing to resolve; focus moves to the status; Close returns to the trigger. | Hand-driven |
| I9 | Filter with arrow keys | Tabs switch and announce `Showing …`; list scroll resets; page scroll does not move. | Hand-driven |
| I10 | Open sheet at 360 | Focus on the sheet heading; Tab order close → tabs → items → document → totals → action bar; form is inert; Escape closes and returns focus. | Hand-driven |
| I11 | Blocked Final Save at 1280 | No API call; assertive announcement; focus the first action of the first unresolved item; rail scrolls it into view. | Hand-driven + network log |
| I12 | Blocked Final Save at 360 | Sheet opens; focus the first action of the first unresolved item; announcement. | Hand-driven |
| I13 | Save draft with unresolved items | Draft saves; items persist; reopen shows the same session, items and decisions; notices are gone. | Hand-driven + DOM |
| I13b | Resolve an item without saving | Action bar shows `Unsaved changes`; Back/settings treat it as dirty; Save draft persists the decision; reopen keeps it. | Hand-driven + DOM |
| I14 | Final Save handoff | Review enters 200ms/fade+rise; Continue enters confirmation; save shows the progress state; saved record arrives with the confirmation number above the fold; focus on its h1; one polite announcement. | Hand-driven |
| I15 | Final save failure | Confirmation view returns; error banner focused; action bar restored; working copy intact. | Hand-driven + fake backend |
| I15b | Stale version on draft save or final save | Existing conflict prompt opens; working copy intact; no silent overwrite. | Hand-driven + fake backend |
| I16 | Final save with no document | Confirmation requires the no-PDF acknowledgement; Continue stays blocked until checked. | Hand-driven + DOM |
| I17 | Final save with an attached document | No no-PDF checkbox; save proceeds. | DOM |
| I18 | Settings round-trip with a dirty editor | Values, tab, record, review step, pane filter, sheet state, scroll and focus are identical on return; no discard prompt. | Hand-driven |
| I19 | Back stack | Home → workspace → settings → Back returns to the exact workspace; Back returns Home; never settings → Home. | Hand-driven |
| I20 | Back from a clean workspace | Goes Home, no prompt. | Hand-driven |
| I21 | Back from a dirty workspace | Discard prompt; Keep editing stays; Discard goes Home and drops the working copy. | Hand-driven |
| I22 | List filters/page/scroll/tab | Change all four, open settings, Back: all four restored; 360 filter panel open/chips restored. | Hand-driven |
| I23 | Sign out / app close | Workspace state is cleared; reopening starts clean; nothing unsaved is written. | Hand-driven |
| I24 | Paste into Payment terms | Paste is native; one commit on blur; one item, not one per paste event. | Hand-driven |
| I25 | Admin vs non-admin list | Admin sees `⋯` with Delete; non-admin sees no menu and no repair action. | Hand-driven + snapshot |
| I26 | 200% zoom at 1280 | Effective 640: one column, cards, no horizontal scroll, action bar and sheet reachable. | Hand-driven |
| I27 | Reduced motion | No transforms, no rise, no scale; opacity/colour remain; progress is static; sheet opens with opacity only. | Hand-driven |
| I28 | Final review after all decisions | Review shows a read-only receipt, not a second set of Keep/Use choices; no decision is asked twice; `Change` returns to that item; Back returns to editing. | Hand-driven + DOM |
| I29 | Keyboard only, start to saved record | From page load, Tab/Skip links reach the form and pane; Keep/Use and the keep confirmation work; Final Save and the handoff complete without a mouse; focus is never lost to `body`. | Hand-driven |
| I30 | Screen reader announcements | Each event in §11 is announced once, polite or assertive as specified; no initial-load or focus-move noise; no toast repeats the same sentence. | Hand-driven |

### 16.4 What must be hand-driven rather than snapshot-tested

Snapshots only prove layout at 360 and 1280. They do not prove any of the following, and Frank must drive them:

- focus order, focus restoration, visible focus, and the `inert` form while the sheet is open;
- Escape and the keep-PO confirmation, including focus return;
- live-region content and politeness, and the absence of double announcements;
- sticky rail/action-bar behaviour while scrolling, and the 1024 two-pane at 768px height;
- settings round-trip with a dirty editor, list filters/page/scroll/tab, review step and sheet state;
- Back stack depth, including “never settings → Home”;
- Final Save progress, arrival, failure and conflict;
- reduced motion, hover/press, and the absence of exit animations;
- 200% zoom, long content, and the 360 filter panel;
- blocked Final Save with no API call, and the server’s refusal behind it.

Also hand-drive the 1024 layout: `scripts/ui-snapshot.sh` renders only 360×900 and 1280×900, so L2 and L9 are browser checks at 1024×768, recorded with what was seen. If the harness cannot render the open-sheet view, say so and hand-drive L4; ticket 07 still expects a sheet snapshot, so add the harness case if it can be expressed without a new seam.

The design review record names every state checked and every state that could not be reached, with the reason. A missing state is not a pass. The hand-driven pass records browser, viewport, keys and pointer actions used, the state reached, and what was not driven; "looks fine" is not a record.

---

*End of brief. Design only; no production code was written, no branch was changed, and nothing was committed.*

