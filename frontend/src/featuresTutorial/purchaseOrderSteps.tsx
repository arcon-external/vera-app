import type { FeaturesTutorialStep } from "../components/FeaturesTutorial";
import {
  GHOST_ACTIONS_ANCHOR,
  GHOST_RESOLVED_EVENT,
  QUOTE_BAR_CHOICE_REVEAL,
  QUOTE_BAR_REVEAL,
  QUOTE_BAR_RESOLVED_EVENT,
  TUTORIAL_PAYMENT_TERMS,
} from "./tutorialData";

// Issue #595: the Purchase Order Features Tutorial script. It is deliberately
// workspace-shaped rather than a generic carousel: each step names a real
// control, and the final step always lands on the workspace's Features
// Tutorial card so replay is one click away.
//
// Every target is the smallest element that carries the idea. A whole card or
// column is only used when the step is about the whole card (totals, the
// customer document); a field, a button or a ghost is never highlighted with
// its surrounding row.
//
// Issue #602: this script lives on its own so later tickets can change the
// Purchase Order tour without touching the Quotation tour.

export const PURCHASE_ORDER_TUTORIAL_STEPS: FeaturesTutorialStep[] = [
  {
    id: "po-tabs",
    target: '[data-tutorial="po-tabs"]',
    title: "One workspace, three views",
    body: (
      <p>
        <strong>Editor</strong> enters the order, <strong>Drafts</strong> hold
        unfinished work, and <strong>Archive</strong> keeps finalized records
        and their corrections.
      </p>
    ),
  },
  {
    id: "po-link",
    target: '[data-tutorial="po-link-button"]',
    page: "po",
    title: "A purchase order begins with a quotation",
    body: (
      <p>
        Every purchase order follows one issued quotation revision. Click{" "}
        <strong>Link a Quotation</strong> to choose the practice quotation —
        nothing is written to your real quotation or purchase-order lists.
      </p>
    ),
    advance: "click",
    beaconLabel: "Click",
  },
  // Issue #603: the selector is guided one real click at a time instead of
  // explained as a block. Each micro-step docks the guide, keeps the modal
  // fully usable and waits for its own control. `onLeave` closes the selector
  // only when the tour leaves a step *without* its action (Back, Next), so a
  // fulfilled click keeps the modal open for the next micro-step.
  {
    id: "po-selector-quotation",
    target: '[data-tutorial="tutorial-quotation-row"]',
    title: "Select the practice quotation",
    body: (
      <p>
        Choose <strong>TUT-2026-001</strong>. It is the only quotation in this
        sandbox, so nothing real is touched.
      </p>
    ),
    // The selector is a modal with its own focus. Dimming a box around it and
    // stacking the guide on top made the two fight; the guide docks in the
    // corner and leaves every control clickable.
    spotlight: "none",
    advance: "click",
    beaconLabel: "Select",
    onLeave: "close-tutorial-selector",
    action: {
      label: "Open the practice selector again",
      action: "open-tutorial-selector",
    },
  },
  {
    id: "po-selector-continue",
    target: '[data-tutorial="tutorial-selector-continue"]',
    title: "Keep revision 0 and continue",
    body: (
      <p>
        The practice quotation has one issued revision, already selected.{" "}
        <strong>Continue</strong> moves to the final check.
      </p>
    ),
    spotlight: "none",
    advance: "click",
    beaconLabel: "Click",
    // Back closes the selector, so it returns to the rail link rather than
    // to a micro-step whose control is gone with the dialog.
    backTo: "po-link",
    onLeave: "close-tutorial-selector",
    action: {
      label: "Open the practice selector again",
      action: "open-tutorial-selector",
    },
  },
  {
    id: "po-selector-confirm",
    target: '[data-tutorial="tutorial-selector-confirm"]',
    title: "Confirm the source",
    body: (
      <p>
        <strong>Confirm source</strong> links the practice order to this exact
        revision. The selector closes and the source values appear beside your
        fields; nothing is saved.
      </p>
    ),
    spotlight: "none",
    advance: "event",
    advanceEvent: "po-tutorial-linked",
    beaconLabel: "Click",
    backTo: "po-link",
    onLeave: "close-tutorial-selector",
    action: {
      label: "Open the practice selector again",
      action: "open-tutorial-selector",
    },
  },
  {
    id: "po-payment-terms",
    target: '[data-tutorial="po-payment-terms"]',
    page: "po",
    // Above the field keeps the source-basis card (the blue quotation values)
    // visible; below it the guide would cover the thing it is explaining.
    placement: "top",
    title: "Quotation values appear inline",
    body: (
      <>
        <p>
          A <strong>blue</strong> border is a quotation value you have not used
          yet. Press <strong>Payment terms</strong> to compare it with yours and
          choose a side.
        </p>
        <p className="mt-2">
          This is the practice order; nothing is written to your real records.
        </p>
      </>
    ),
    advance: "focus",
    beaconLabel: "Press",
  },
  // Issue #604: the bar is explained, then chosen from. Both of these steps
  // carry the reveal so the bar stays open while the guide is visible — a
  // focus-driven bar would close as soon as the guide takes focus for Next.
  {
    id: "po-quote-bar",
    target: '[data-tutorial="po-quote-bar"]',
    page: "po",
    title: "Value from quotation",
    body: (
      <>
        <p>
          The bar shows the value this quotation carries:{" "}
          <strong>{TUTORIAL_PAYMENT_TERMS}</strong>.{" "}
          <strong>Use quotation value</strong> fills the field with it;{" "}
          <strong>Use my value</strong> keeps the value you entered.
        </p>
        <p className="mt-2">
          The guide keeps the bar open while you read it, even after Next.
        </p>
      </>
    ),
    reveal: QUOTE_BAR_REVEAL,
  },
  {
    id: "po-quote-choice",
    target: '[data-tutorial="po-quote-bar-actions"]',
    page: "po",
    // Right of the action row keeps the bar's header and the field visible.
    placement: "right",
    title: "Choose the value to keep",
    body: (
      <p>
        Either button answers this step. The next step explains what the
        field&rsquo;s border says about the value you kept.
      </p>
    ),
    advance: "event",
    advanceEvent: QUOTE_BAR_RESOLVED_EVENT,
    beaconLabel: "Select",
    reveal: QUOTE_BAR_CHOICE_REVEAL,
  },
  {
    id: "po-quote-result",
    target: '[data-tutorial="po-payment-terms"]',
    page: "po",
    placement: "top",
    title: "The border is the answer",
    body: (
      <>
        <p>
          <strong>Green</strong> means this field is using the quotation&rsquo;s
          value. <strong>White</strong> means your own value is kept.
        </p>
        <p className="mt-2">
          Editing the field brings the choice back, so nothing is locked in.
        </p>
      </>
    ),
  },
  // Issue #606: the ghost stops being a picture and becomes a decision. The
  // intro explains the card, the choice waits for the real ✓/✕ pair, and the
  // result points at the line that owned the ghost, whether it was accepted or
  // ignored. The practice order is the only record involved.
  {
    id: "po-ghost",
    target: '[data-quotation-ghost]',
    page: "po",
    title: "A ghost is the whole quotation item",
    body: (
      <>
        <p>
          This split ghost shows its <strong>parent</strong>, both{" "}
          <strong>priced parts</strong> and their <strong>scope lines</strong> —
          exactly what the quotation carries.
        </p>
        <p className="mt-2">
          It waits here because your line&rsquo;s values differ from it. The
          next step lets you accept or ignore it.
        </p>
      </>
    ),
  },
  {
    id: "po-ghost-choice",
    target: `[data-tutorial="${GHOST_ACTIONS_ANCHOR}"]`,
    page: "po",
    title: "Accept or ignore the ghost",
    body: (
      <p>
        <strong>✓</strong> copies the quotation&rsquo;s parent, parts and scope
        onto your line. <strong>✕</strong> keeps the values you entered and
        records the omission. Either answer moves the guide on.
      </p>
    ),
    advance: "event",
    advanceEvent: GHOST_RESOLVED_EVENT,
    beaconLabel: "Choose",
  },
  {
    id: "po-ghost-result",
    // The split package is editor item 2, the line that owned the first ghost.
    // Its description input is a stable anchor even after the ghost card is
    // gone, and it is where staff see the accepted (or kept) values.
    target:
      '[data-tutorial="po-lines"] input[aria-label="Description for line 2"]',
    page: "po",
    title: "The line now carries your choice",
    body: (
      <>
        <p>
          Accepted, the line uses the quotation values. Ignored, it keeps the PO
          values you entered and the omission is recorded.
        </p>
        <p className="mt-2">
          Editing or removing an accepted line brings its ghost back, so you can
          decide again.
        </p>
      </>
    ),
  },
  {
    id: "po-extra",
    target: '[data-tutorial="po-add-line"]',
    page: "po",
    title: "Your own lines stay yours",
    body: (
      <p>
        A line with no quotation match is an extra. It stays on the order and is
        acknowledged automatically; unmatched quotation items wait below, ready
        to add or ignore.
      </p>
    ),
  },
  {
    id: "po-totals",
    target: '[data-tutorial="po-totals"]',
    page: "po",
    title: "The PO keeps its own totals",
    body: (
      <p>
        Subtotal, PPN and grand total belong to this purchase order. The printed
        quotation totals sit beside them as source evidence, never as a number
        the PO must force itself to match.
      </p>
    ),
  },
  {
    id: "po-document",
    target: '[data-tutorial="po-document"]',
    page: "po",
    title: "Keep the customer's PDF with the order",
    body: (
      <p>
        Attach the customer's purchase-order PDF when you have it. It stays with
        the record as evidence; a final save without one asks you to confirm the
        omission deliberately.
      </p>
    ),
  },
  {
    id: "po-final-save",
    target: '[data-tutorial="po-final-save"]',
    page: "po",
    title: "Final Save confirms the order",
    body: (
      <p>
        This is the one-way transition to a saved record. The optional
        confirmation number is issued here, and the practice order is never
        saved.
      </p>
    ),
  },
  {
    id: "po-drafts",
    target: '[data-tutorial="po-drafts-list"]',
    page: "po-drafts",
    title: "Drafts are unfinished orders",
    body: (
      <p>
        Your real purchase-order drafts live here. The practice order from this
        tour is deliberately not in the list.
      </p>
    ),
  },
  {
    id: "po-archive",
    target: '[data-tutorial="po-archive-list"]',
    page: "po-archive",
    title: "Saved records and corrections",
    body: (
      <p>
        Finalized purchase orders live in the Archive. Open one for its saved
        values, source revisions, PDF and correction history.
      </p>
    ),
  },
  {
    id: "po-settings-tutorial",
    target: '[data-tutorial="po-settings-tutorial"]',
    page: "po-settings",
    title: "Replay this tutorial anytime",
    body: (
      <p>
        This card lives in Purchase Order settings.{" "}
        <strong>Replay tutorial</strong> starts the tour again from the first
        step whenever you want it.
      </p>
    ),
    final: true,
  },
];
