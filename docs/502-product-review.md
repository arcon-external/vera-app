# #502 product review — reconciliation workbench

**Status:** product review, read-only. No spec rewrite, no implementation.
**Worktree:** `/var/lib/openbot/OpenBot/Shared/vera-502` at `dca8890` (detached, unchanged).
**Read:** `.scratch/vera-502/spec.md`, tickets `01–07`, `docs/po-reconciliation-contract.md` (contract v1), `docs/502-ux-brief.md`, `GLOSSARY.md`, `backend/store/po_review.go`, `frontend/src/poFinalize.ts`.
**Not found in worktree:** `handover.md` (§7, §9 requested but absent — glossary + contract §8 used as fallback; any handover conflict outside Q13–Q16 is unverified).
**Skills:** loaded `domain-modeling` (term sharpening, GLOSSARY cross-check) and `grilling` (frontier questions, recommendations). Deliberately did not load `tdd`, `code-review`, `implement-spec`, `research`, `prototype`, UI-build or release skills — this review freezes criteria, it does not verify code or ship anything.

Q13–Q16 as decided (notices vs persisted items on commit/blur, billing PO-only, tax at link, derived totals) are taken as confirmed. The Q16 "forced equality vs same pipeline" confirmation noted in channel history is still open — see §5.

---

## 1. User stories — missing, ambiguous, untestable

References are `spec.md` story numbers.

| Story | Verdict | Problem + recommended wording |
|---|---|---|
| 9 "empty required field shows notice" | Ambiguous / not testable | Required set is never enumerated. Current code (`po_review.go`, `poFinalize.ts`) compares payment terms / Incoterms that may be empty today (contract §8). **Recommend:** "As staff, I want an empty notice only for fields the existing validation already requires, so comparing a field never makes it required. Acceptance: whole-PO discount empty (=0%) with both sides empty is not a difference; payment-terms empty vs non-empty is a normal difference item, not an empty notice, unless validation requires it." |
| 10 "matches the Quotation" | Ambiguous | Silent on: which commit (blur + Finish/Continue for focused field), lifetime, persistence. **Recommend:** append "The notice appears only on commit (never per keystroke), lasts until the next commit re-evaluates that field, and is never persisted or replayed on reopen." |
| 14 "never gains a second matched row" | Ambiguous | Overlaps spec §"Two feedback mechanisms" (`values now match` stays resolved, never a notice). A reader can read 14 as "no notice ever after an item exists" — but empty notices **may** coexist with retained history (contract §4, brief §4.4). **Recommend:** split into "no *matched* notice for a field with any item (resolved or not)" + "an *empty* notice may appear on a field with retained history; it blocks on requiredness and creates no second item." |
| 20 missing/extra/duplicate | Ambiguous, lumps three outcomes | Duplicate is blocking and **cannot** be acknowledged (spec §Two feedback, ticket 05), while missing/extra can. Story as written implies all three are "acknowledged or corrected". **Recommend:** three stories: missing → acknowledge-or-correct; extra (+unknown ref, see §4) → acknowledge-or-correct **subject to saved-reference validator**; duplicate → correct-only, acknowledgement does not clear. |
| 22 totals "names the reason" | Not testable | No reason format, no multi-cause rule, no pending rule. **Recommend:** "Totals shows Matched / Different · explained by {single input key, e.g. Whole-PO discount 2% vs 0%} / Different · inputs do not fully explain it / Pending (unparseable input). It never creates an item and never blocks. Printed totals are evidence, never a target." (Matches contract §6, brief §4.11.) |
| 23 Final Save blocked | Incomplete | Lists only review items + line acks. The real gate (contract §7, brief §5.2) also needs: live source, customer/currency match, required values, valid calculations, no duplicates, no-document confirmation, confirmation-number rules, stale-version refusal, protected tax basis. **Recommend:** enumerate the full gate or normatively reference contract §7 so 23 cannot pass with a partial gate. |
| 32 "feels like a deliberate arrival" | Untestable | Subjective. **Recommend:** replace with brief §9.2 sequence: review → confirmation → progress (≥300 ms min, 1.1 s sweep, start after 150 ms) → saved record with confirmation number above fold, focus on saved `h1`, one polite announcement. |
| 34 keyboard focus "predictable order" | Untestable as written | **Recommend:** normatively reference brief §10.2–10.5 (tab order, focus-after-action table, Escape never navigates, `aria-disabled` Final Save focus rule). |
| 28/29/30 docket list | Incomplete | 28 "at a glance" is vague; 29 omits non-admin invisibility and open-failure behaviour. **Recommend:** reference brief §8.1–8.5 (columns, 1024 no-`min-w-[900px]`, 360 cards, `Open purchase order {n}`, admin `⋯` omitted — not disabled — for non-admins, Opening…/failure restore). |

**Missing stories (no story covers them; tickets cover some but stories are the freeze surface):**

1. **Tax choice at link.** Story 21 says tax is "compared at link then protected" but no story makes the choice itself: keep-PO vs use-Quotation recorded as one resolved `link:taxBasis` item, PPN recomputed from the chosen triple, source-created POs adopt the source basis. **Recommend new story:** "As staff linking a source, I want to choose keep-PO or use-Quotation tax basis once, so PPN is recomputed from a recorded decision and the pane shows it as read-only context afterwards."
2. **Never-linked draft.** No story for creating/typing before a source exists (no session, currency/tax editable, pane shows "Link a Quotation", Final Save blocked). Ticket 04 assumes session-on-link; the pre-link state has no story.
3. **Source-missing / former-source.** No story for never-linked vs deleted-source (`Source missing` badge, read-only until link, admin repair vs staff wait). Brief §4.3/S20 and `po-source-missing.md` describe it; spec stories do not.
4. **Correction of a saved PO.** Story 26 covers read-only view; nothing covers correcting values (rail stays read-only history, correction does not write the session — brief §4.12/§14.1, contract §8). Needs a product ruling, not a UI detail.
5. **Abandoned repair.** Ticket 06 checkbox exists ("abandoned leaves old session active") but no story. Needs a story so Frank can test the no-op path.
6. **Adjustment (signed money + reason).** Adjustment is PO-only and feeds grand total (contract §6), but no story enters it or explains its totals-check treatment. Recommend a story: "As staff I want to enter a signed adjustment with a reason, so a deliberate money correction explains a totals difference without becoming a review item."
7. **Reordered lines are a no-op.** Spec §Stable identity says reorder changes nothing; no story pins it. Recommend explicit story to prevent position-key regression (contract §8 conflict).
8. **Currency / customer mismatch refusal.** Contract §7 + brief §4.3/P18 block with no conversion/comparison; no story. Recommend a story — this is the only guard against comparing across customers/currencies.
9. **No-document confirmation + confirmation number.** Part of the Final Save gate (contract §7) with no story. Recommend a story each, or explicitly mark them as existing behaviour reused unchanged.
10. **Version conflict on stale write.** Contract §6–7 + brief §5.4/I15b require conflict-not-overwrite; no story.
11. **Audit attribution.** Story 35 wants actor+timestamp; no story says actor comes from the authenticated server context and history is append-only (contract §4–5). Recommend tightening 35: "attributed from server auth, never trusted client data; earlier decisions retained in item history."

---

## 2. Acceptance criteria that cannot be verified as written

| Location | Criterion | Why it fails + fix |
|---|---|---|
| 04 | "Committing an empty required header field shows a notice" | See §1 story 9. **Fix:** name the required set (existing validation only) or reference contract §4 transition table + brief §4.4. |
| 04 | "Final Save is blocked while any header item is unresolved" | Necessary but insufficient; omits required values, line set, duplicates, source, document, version. **Fix:** reference the full contract §7 gate. |
| 04 | "reopening the same draft returns the same session and items" | Omits mapping + history + notices-gone. **Fix:** "same session ID, same item keys (client key retained across save, not re-keyed to `id:`), same resolutions/actor/time/history; transient notices are gone." (Contract §3.) |
| 05 | "Final Save refuses … with the specific blockers named" | "Specific" is not enumerated. **Fix:** list: each unresolved item key, each unacknowledged missing/extra ref, duplicate refs, empty required field name, invalid field name, missing/former source, currency/customer mismatch, no-document confirmation, stale version. |
| 05 | "The frontend source DTO exposes the tax basis fields" | Additive change with no shape. **Fix:** reference contract §5 `TaxBasis{TaxZone,TaxType,ServiceCategory}` + link resolver retaining legacy `TaxCategory` in DB. |
| 05 | "Saved decisions remain readable" | Readable where, by whom, for how long? **Fix:** "saved session read-only via existing review surface; old sessions reachable as history; no resolved item deleted." |
| 06 | "Corrections and version history are unaffected" | Directly contradicts the open question both contract §8 and brief §14.1 flag: which session (if any) records corrected values? **Fix:** freeze one reading now — recommended: "corrections do not write the saved session; rail shows `The saved reconciliation is history` (brief §4.12)" — and mark any reopen-reconciliation as a spec change. |
| 06 | "Source repair … seeds a full comparison" | "Full" vs touched-fields is decided in contract §6 but the ticket does not say resolvable-vs-readonly on seed, nor revision-return behaviour. **Fix:** "new session, full comparison, all items unresolved; old session retained; return to a previously used revision is out of scope until the uniqueness rule is decided (contract §8)." |
| 02 | "Save draft and Final Save … remain reachable while the form scrolls" | No viewport/position rule. **Fix:** reference brief §5.1 (sticky bottom bar, `z-30`, 64px ≥1024 / 44px buttons <1024). |
| 07 + spec §Motion | Durations/easings stated, but "signature moment" has no timing/focus/announcement gate | **Fix:** reference brief §9.2 + §11 + acceptance I14 (progress ≥300 ms, focus `h1`s, one polite announcement, failure restores working copy). |
| All tickets | "Tests cover …" without the seam | **Fix:** keep spec §Testing Decisions seam (Wails/API boundary; fake backend in components; pure comparison + commit handler directly). Ticket 07 already needs the 1024 hand-driven note (harness renders only 360/1280 — brief §16.4). |

---

## 3. Field-classification errors and risks

Spec §Comparable-field contract direction is **correct** on Q14/Q15/Q16, but the freeze needs these tightenings. Sources: `po_review.go:16–32,65–101`, `poFinalize.ts:50–66`, contract §§2/6, brief §3.1.

1. **Billing address — correct, but fragile.** Spec rightly excludes it (Q14) even when copied from the Quotation. Risk: `PurchaseOrderSource` still carries `BillingAddress` (`po_review.go:76`) and the draft-link path must keep the PO-owned value on every link/customer-change (store tests pin this). **Recommend:** add "even when the values are text-identical, no notice/item is ever created for billing address; link/customer-change must not overwrite the typed PO address" to the spec's exclusion line.
2. **Tax basis — correct as link-time only, but the DTO is split-brained today.** Draft-link review already exposes `DraftTax/SourceTax` + explicit choice; the general source DTO omits the triple (contract §8). Ticket 05's "expose tax basis fields" must be additive (keep legacy `TaxCategory` + resolver) and must not add an inline pane action. **Recommend wording:** "Tax basis is the semantic triple `{TaxZone,TaxType,ServiceCategory}`; compared once at link, recorded as one resolved `link:taxBasis` item; afterwards read-only context. Changing it later is a correction/repair command (product/backend to define — contract §8 open), never a field-level Keep/Use."
3. **Totals — correct as derived, Q16 caveat open.** Spec + contract + brief agree: same pipeline, reason-naming check, printed totals are evidence never a target, valid explained difference creates no item and no blocker. The channel flags "calculation overall should be same — confirm if forced equality meant". **Do not freeze** until that is answered: if forced equality was meant, spec §Totals + ticket 05 + brief §4.11 all need a rewrite. Recommended freeze wording if the pipeline reading holds: "The PO calculates with the same pipeline as the Quotation (gross → row discount → subtotal → whole-PO discount → PPN on top → signed adjustment; contract §6). The check names the explaining input; it never forces the printed total."
4. **Line-match rules — unknown reference is the hole.** Spec: extra = acknowledged-or-corrected; ticket 05: extra includes unknown ref; contract §8: saved-reference validator **refuses** a non-existent non-null reference. Acknowledgement alone may not finalise. **Recommend:** "An unknown reference is shown as `Unknown reference · {ref}` extra; acknowledgement is offered but Final Save still requires the saved-reference validator to pass — i.e. correction may be required. Never promise a green path acknowledgement alone cannot deliver (brief §4.5)." Also pin: duplicate is correct-only (acknowledgement never clears); missing/extra ack applies to the current line set only (contract §4).
5. **Line discount `Enabled` semantics — spec is silent, code comment is wrong.** Both comparators distinguish disabled vs enabled 0% (contract §8; brief §4.5: `No discount` vs `0%`); the Go comment claims they match. **Recommend:** freeze "disabled 0% ≠ enabled 0%; canonical dormant value `Percent: 0` with `Enabled` retained in the value (contract §5)" and fix the Go comment in implementation, not here.
6. **PO-only list — one over-broad entry.** "Line number" is PO-only as a comparable (correct — identity is the stable key, number is a label), but the displayed number still appears in every item label (`Line {n} …`). **Recommend:** "displayed line number is a label only, never an item key" (contract §2–3). Contact email/phone as PO-only is correct per glossary (customer contact ≠ recipient) — keep, but do not let a future "compare contacts" slip in without a spec change.
7. **Missing from the comparable/excluded lists.** Customer name, currency, customer snapshot, source identity/revision are neither comparable nor listed as PO-only — they are **refusal guards** (contract §2, §7). **Recommend:** add one line: "Customer/currency mismatch is refused with a blocking banner; no comparison items are produced and no conversion is performed."
8. **PPH22/cost/margin/profit — correctly absent.** Keep the explicit exclusion; brief §3.1 "nothing internal appears" is the right guard. No change, just do not let totals-check rows reintroduce them.

---

## 4. Edge cases — recommendation for each requested case

| Edge case | Spec today | Recommendation (freeze this) |
|---|---|---|
| Never-linked draft | No story; brief §4.3 S3/S7-adjacent; contract §6 "no session" | **No session exists.** Currency/tax editable with the existing Source-basis card; pane shows link empty state, no filters/count; Final Save blocked with `Link a Quotation to enable Final Save`. Linking creates the session + full comparison + tax choice (§1.1). |
| Source missing (never vs former) | Stories silent; brief S20/P17; `po-source-missing.md` | Keep the glossary split: never-linked = blank link state; former (deleted source) = `The source quotation was deleted`, rail read-only, actions `Link new source` + admin-only `Repair source`; non-admin waits. Final Save blocked in both. |
| Source repair | Story 27 + ticket 06; contract §6; brief §4.12 | **New session, full comparison, all unresolved; old session retained as history; success makes the new session read-only with the saved record; repair records old+new source + reason; admin-only unchanged.** |
| Abandoned repair | Ticket 06 checkbox only | **Changes neither the old active session nor the PO.** Recommend a story + Frank case: start repair, abandon, assert old session ID/items/record unchanged and rail unmoved. |
| Correction (saved PO edit) | Open conflict (contract §8, brief §14.1, §2 ticket 06 above) | **Recommended freeze:** corrections do not write or reopen the saved session; rail shows read-only history + `The saved reconciliation is history. This correction does not change it.` Any "correction reopens reconciliation" is a spec change with a new session/generation rule. Needs Yara + backend ruling before ticket 06 is called done. |
| Duplicate lines | Spec + ticket 05 correct-only | **Blocking; `Fix match` scrolls to the first claimant and opens its match select; stays unresolved until one match is corrected; acknowledgement never clears; assertive announcement** (brief §11). Two PO lines claiming one ref never become two items on one key. |
| Reordered lines | Spec §Stable identity only | **No-op:** display order/labels change; item keys, decisions, history unchanged; no new/moved items. Add the story from §1.7 so a position-key regression fails product, not just contract tests. |
| Zero/negative adjustment | PO-only + reason-gated (contract §6); no story | Enter signed adjustment + required reason; grand total `= subtotal − discount + PPN + adjustment`, never negative (contract §6 rule 5); totals-check names it as the explaining input when it accounts for the gap. Malformed input is validation, never coerced to zero, never a review item (brief §4.4). |
| Reopened draft | Stories 24–25; ticket 04 | **Same session ID/keys/mapping/history/actor/time; notices gone (never persisted); match notices do not replay; session dirty flag participates in Save/Back guards** (brief §5.4/§7). |
| Archive read | Story 26; ticket 06; brief S19/P16 | **Read-only rail, no actions/anchors/keep-step; filters All/Resolved only; `{n} resolved`; `Session history` disclosure; `Saved session · revision {n}` in header; older data with no session says `No reconciliation session was recorded…` — never invent one.** Archive reads never mutate or recreate sessions (contract §7). |
| Extra: unknown reference | §3.4 above | Show as extra + server-blocker notice; acknowledgement offered, correction may still be required. |
| Extra: empty vs invalid | Brief §4.4 P3/P3b | Empty required → ink notice, required-gate blocks; invalid non-empty → danger message + border on commit, blocks, never a review item, never coerced to zero. Blocked-save attempt focuses the first empty/invalid field (danger) per §5.3. |
| Extra: stale version | Contract §6–7, brief I15b | Stale session/draft write conflicts on PO version; existing conflict prompt; never silent retry/overwrite; working copy intact. |
| Extra: no-document + confirmation number | Contract §7, brief §5.5 | Keep existing confirmation ownership: no-PDF acknowledgement gates Continue; confirmation-number allocation/immutability unchanged. |

---

## 5. Confirmed requirements vs assumptions; scope creep

**Confirmed (Q13–Q16 + channel decisions, as built in spec/contract/brief):**
- Q13 two mechanisms: transient empty/matched notices (commit-only, unpersisted) vs persisted review items with keep/use; no matched notice on a field with an item; resolved history retained; commit handler idempotent, blur + Finish/Continue, never per keystroke.
- Q14 billing address PO-only, never compared.
- Q15 tax basis at link, then protected; later change via correction/repair only.
- Q16 pipeline reading (derived totals, reason-naming, printed totals as evidence) — **with the forced-equality confirmation still outstanding.** Do not call Q16 fully frozen until the user answers.
- Back stack Home → workspace → settings, settings never jumps to Home; dirty editor survives settings; shell action bar owns Save/Final Save; docket list without amounts; enter-only motion + Final Save handoff; `aria-disabled` (not `disabled`) blocked Final Save; sheet `region` (never modal), form `inert` <1024.

**Assumptions the spec absorbs without a ruling (each is a freeze blocker or an explicit accepted risk):**
1. Requiredness unchanged — comparing a field never makes it required (contract §8, brief §14.9). Any "tightening" is a spec change.
2. Existing line-ID destruction (`replacePurchaseOrderItems` delete/reinsert, no IDs in editor DTO) will be fixed additively with client-key mapping (contract §3, §8). Migration 041 may need line support, not only session/item tables.
3. Unknown-reference acknowledgement vs validator refusal (§3.4); tax-correction command (§3.2); correction session generation (§4 correction); return-to-previously-used-revision uniqueness (contract §8) — all still need product/backend rulings. Ticket 06 cannot be called done on 1 and 4 without them.
4. Text trim = ASCII surrounding-only; money/quantity compare in stored integer units; Unicode NEL/BOM and cross-language trim policy undecided (contract §8). Current fixtures pin ASCII only — correct scope, but do not claim full Unicode parity.
5. `index.css` blanket `0.01 ms` rule will be replaced with targeted reduced-motion rules (brief §9.3/§14.5) — a global CSS change smuggled inside a feature slice; keep it explicit in ticket 07.
6. Finalize API must carry live session resolutions (kept/ack/corrected) or the receipt review re-asks/refuses (brief §14.8). If it cannot, tickets 04/05 change — not a silent UI workaround.

**Scope the spec has absorbed (record explicitly before freeze; none is judged here, but silent absorption is the risk the task names):**
- Quotation-workspace back-stack parity (spec §Back stack, ticket 02) — same rule for a second workspace doubles the test surface.
- PO-native docket list (ticket 03) — a list redesign inside a reconciliation slice; amount-column removal + query-unchanged constraint is correct but is list scope, not pane scope.
- 1024 two-pane with container-query line cards + 360 sheet + full motion/a11y system (brief §§2/6/9–11) — a responsive + motion + screen-reader programme inside one feature.
- `Session history` disclosure, saved read-only rail, correction-history caption — new audit surface with the open correction question attached.
- Final review as read-only receipt (§5.5) — reverses the existing card-wall decision surface; correct against double-asking, but it is a workflow change, not a restyle.

**Handover caveat:** with `handover.md` absent from the worktree, supersession ("spec supersedes handover where they differ") is verified only for Q13–Q16 via the contract. Any §7/§9 conflict outside those questions is unverified — note it in the freeze record rather than assuming coverage.

---

## 6. What to change before freeze (minimal list)

1. Answer the Q16 forced-equality question; if pipeline reading holds, keep §3.3 wording above.
2. Rule on correction-vs-session (§4 correction) and unknown-reference clearing (§3.4); both block ticket 06/05 done.
3. Split story 20 into missing/extra/duplicate; add stories for tax choice, never-linked, former-source, abandoned repair, adjustment, reorder no-op, mismatch refusal, no-document/confirmation-number, version conflict, server-attributed audit (§1).
4. Replace untestable criteria with the contract/brief references in §2 (especially 04-empty-required, 04/05 Final Save gate, 06 correction, 07 motion timings + 1024 hand-driven).
5. Add the classification lines in §3 (billing-identity note, tax-triple wording, unknown-ref rule, discount-enabled rule, customer/currency refusal line).
6. Record the absorbed scope + the five assumptions in §5 in the spec's Further Notes so a later reader knows what "done" included.

*No code, spec, or branch changed by this review. Skills: `domain-modeling`, `grilling` loaded; `tdd`, `code-review`, build/release/research skills deliberately not loaded.*
