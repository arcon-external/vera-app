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
