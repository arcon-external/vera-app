import type { FeaturesTutorialStep } from "../components/FeaturesTutorial";

// Issue #595: the Quotation Features Tutorial script. It is deliberately
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
// Quotation tour without touching the Purchase Order tour.

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
    id: "quotation-note",
    target: '[data-tutorial="quotation-note"]',
    page: "create",
    title: "Write the note, then format it",
    body: (
      <p>
        Focus the practice note. A formatting toolbar appears above it, and
        everything you type stays in this sandbox.
      </p>
    ),
    advance: "focus",
  },
  {
    id: "quotation-note-toolbar",
    target: '[data-tutorial="quotation-note-toolbar"]',
    page: "create",
    title: "Bold, italic and lists",
    body: (
      <>
        <p>
          <strong>B</strong> toggles bold, <em>I</em> italic, and the two list
          buttons start a bullet or numbered list. The same commands work from
          the keyboard.
        </p>
        <p className="mt-2">
          The note prints under the line items on the customer quotation.
        </p>
      </>
    ),
  },
  {
    id: "quotation-subject",
    target: '[data-tutorial="quotation-subject"]',
    page: "create",
    title: "Paste the details from Excel",
    body: (
      <p>
        The subject prints above the lines. Focus it, or paste a row from Excel.
        In the real editor one paste fills the subject, recipient and the
        rest of the details in a single step.
      </p>
    ),
    advance: "focus",
  },
  {
    id: "quotation-details",
    target: '[data-tutorial="quotation-customer"]',
    page: "create",
    title: "Start with the customer",
    body: (
      <p>
        The customer drives the printed details. Recipient, subject, validity,
        payment terms and tax sit around it, and all of them become a snapshot
        on the issued revision. Later customer edits cannot rewrite an old
        quotation.
      </p>
    ),
  },
  {
    id: "quotation-line-cost",
    target: '[data-tutorial="quotation-line-cost"]',
    page: "create",
    title: "Type a cost and watch the sell price",
    body: (
      <p>
        The first line is a practice service. Type any cost into{" "}
        <strong>Unit cost</strong> (try 7500000) and the line's sell price
        updates beside it as you type. Then press <strong>Next</strong> to keep
        going. Nothing leaves this workspace.
      </p>
    ),
    advance: "next",
    beaconLabel: "Type",
  },
  {
    id: "quotation-line-sell",
    target: '[data-tutorial="quotation-line-row"]',
    page: "create",
    title: "Cost, margin and discount make the sell price",
    body: (
      <p>
        A line's sell price starts at its cost, adds that line's margin, then
        takes off any row discount. The Summary rail adds every line together,
        so each practice edit moves the totals.
      </p>
    ),
  },
  {
    id: "quotation-line-margin",
    target: '[data-tutorial="quotation-line-margin"]',
    page: "create",
    title: "Margin is set per line",
    body: (
      <p>
        This line carries a 15% margin. Click its switch to turn the margin
        off: the sell price drops at once. Turn it back on whenever you like.
        One line's margin never applies to another.
      </p>
    ),
    advance: "click",
  },
  {
    id: "quotation-line-discount",
    target: '[data-tutorial="quotation-line-discount"]',
    page: "create",
    title: "Discount comes after margin",
    body: (
      <p>
        Cost plus margin is the list price, and a row discount then comes off
        it. Click the switch (this line has 10% ready) and the sell price
        drops while what we pay the supplier stays the same.
      </p>
    ),
    advance: "click",
  },
  {
    id: "quotation-line-split",
    target: '[data-tutorial="quotation-line-split"]',
    page: "create",
    title: "Split a line into priced parts",
    body: (
      <p>
        <strong>Split into sub-items</strong> turns the line into a package.
        The parent price moves out, so enter each part&rsquo;s cost, margin and
        discount. The line becomes their sum at quantity 1. The rail follows
        along.
      </p>
    ),
    advance: "click",
  },
  {
    id: "quotation-totals",
    target: '[data-tutorial="quotation-totals"]',
    page: "create",
    // The rail is against the right edge; place the guide in the main column
    // so it never covers the totals it is explaining.
    placement: "left",
    title: "Know the number before you issue it",
    body: (
      <p>
        The rail separates the customer subtotal and PPN from the internal
        margin, PPh23 and nett profit. Internal economics never print on the
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
      <>
        <p>
          <strong>Save draft</strong> keeps the work editable.{" "}
          <strong>Export PDF</strong> stores a numbered, signed revision that
          can never change; the next export becomes the following revision.
        </p>
        <p className="mt-2">
          There is nothing to press on this step: the practice quotation is
          never saved, so pointing here creates no draft and no revision.
        </p>
      </>
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
