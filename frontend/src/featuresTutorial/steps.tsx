import type { FeaturesTutorialStep } from "../components/FeaturesTutorial";

// Issue #595: the two Features Tutorial scripts. They are deliberately
// workspace-shaped rather than a generic carousel: each step names a real
// control, and the final step always lands on that workspace's Features
// Tutorial card so replay is one click away.
//
// Every target is the smallest element that carries the idea. A whole card or
// column is only used when the step is about the whole card (totals, the
// customer document); a field, a button or a ghost is never highlighted with
// its surrounding row.

export const QUOTATION_TUTORIAL_STEPS: FeaturesTutorialStep[] = [
  {
    id: "quotation-tabs",
    target: '[data-tutorial="quotation-tabs"]',
    title: "One workspace, three views",
    body: (
      <p>
        <strong>Editor</strong> builds the quotation, <strong>Drafts</strong>{" "}
        hold unfinished work, and <strong>Archive</strong> keeps every issued
        revision. The tabs stay put while you move between them.
      </p>
    ),
  },
  {
    id: "quotation-details",
    target: '[data-tutorial="quotation-customer"]',
    page: "create",
    title: "Start with the customer",
    body: (
      <p>
        The client drives the printed details. Recipient, subject, validity,
        payment terms and tax sit around it, and all of them become a snapshot
        on the issued revision — later client edits cannot rewrite an old
        quotation.
      </p>
    ),
  },
  {
    id: "quotation-lines",
    target: '[data-tutorial="quotation-lines-header"]',
    page: "create",
    title: "Price every line in place",
    body: (
      <>
        <p>
          Each line carries its own description, quantity, cost, margin and row
          discount, with the sell price updating beside it.
        </p>
        <p className="mt-2">
          Split a line into priced sub-items for a package, add scope lines
          beneath a part, or paste a block from Excel.
        </p>
      </>
    ),
  },
  {
    id: "quotation-totals",
    target: '[data-tutorial="quotation-totals"]',
    page: "create",
    title: "Know the number before you issue it",
    body: (
      <p>
        The rail separates the customer subtotal and PPN from the internal
        margin, PPh22 and nett profit. Internal economics never print on the
        customer document.
      </p>
    ),
  },
  {
    id: "quotation-actions",
    target: '[data-tutorial="quotation-actions"]',
    page: "create",
    title: "Save a draft, or issue a revision",
    body: (
      <p>
        <strong>Save draft</strong> keeps the work editable.{" "}
        <strong>Export PDF</strong> stores a numbered, signed revision that can
        never change; the next export becomes the following revision.
      </p>
    ),
  },
  {
    id: "quotation-drafts",
    target: '[data-tutorial="quotation-drafts-list"]',
    page: "drafts",
    title: "Pick up unfinished work",
    body: (
      <p>
        Only you see your drafts. Opening one returns it to the editor;
        exporting it turns it into a real quotation and removes it from this
        list.
      </p>
    ),
  },
  {
    id: "quotation-archive",
    target: '[data-tutorial="quotation-archive-list"]',
    page: "archive",
    title: "Every issued revision lives here",
    body: (
      <p>
        Search or filter, then open a row for its revisions, the customer
        document and the verification code. Any record can start a purchase
        order with one action.
      </p>
    ),
  },
  {
    id: "quotation-settings-tutorial",
    target: '[data-tutorial="quotation-settings-tutorial"]',
    page: "quotation-settings",
    title: "Replay this tutorial anytime",
    body: (
      <p>
        This card lives in Quotation settings. <strong>Replay tutorial</strong>{" "}
        starts the tour again from the first step whenever you want it.
      </p>
    ),
    final: true,
  },
];

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
        Every purchase order follows one issued quotation revision. Use this
        link to choose the practice quotation — nothing is written to your real
        quotation or purchase-order lists.
      </p>
    ),
    advance: "click",
  },
  {
    id: "po-tutorial-selector",
    target: '[data-tutorial="tutorial-selector"]',
    title: "Choose the practice quotation",
    body: (
      <p>
        Only the practice quotation is available here. Select{" "}
        <strong>TUT-2026-001</strong>, keep revision 0, and confirm the source.
        The guide steps aside so the selector is fully usable.
      </p>
    ),
    // The selector is a modal with its own focus. Dimming a box around it and
    // stacking the guide on top made the two fight; the guide docks below the
    // modal and leaves every control clickable.
    spotlight: "none",
    advance: "event",
    advanceEvent: "po-tutorial-linked",
    onLeave: "close-tutorial-selector",
    action: {
      label: "Open the practice selector again",
      action: "open-tutorial-selector",
    },
  },
  {
    id: "po-header",
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
          yet; <strong>green</strong> means this field is using it.
        </p>
        <p className="mt-2">
          Focus Payment terms to compare both values and keep one side. Your own
          value leaves the field normal.
        </p>
      </>
    ),
  },
  {
    id: "po-ghost",
    target: '[data-quotation-ghost]',
    page: "po",
    title: "Quotation ghosts show the whole item",
    body: (
      <>
        <p>
          A ghost is the real quotation item, not a second form. This split item
          shows its parent, both priced parts and their scope lines.
        </p>
        <p className="mt-2">
          ✓ uses the quotation values. ✕ keeps yours and records the omission.
          Editing an accepted line brings its ghost back.
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
