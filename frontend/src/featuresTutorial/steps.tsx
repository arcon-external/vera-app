import type { FeaturesTutorialStep } from "../components/FeaturesTutorial";

// Issue #595: the two Features Tutorial scripts. They are deliberately
// workspace-shaped rather than a generic carousel: each step names a real
// control, and the final step always lands on that workspace's Features
// Tutorial card so replay is one click away.

export const QUOTATION_TUTORIAL_STEPS: FeaturesTutorialStep[] = [
  {
    id: "quotation-tabs",
    target: '[data-tutorial="quotation-tabs"]',
    title: "One workspace, three views",
    body: (
      <>
        <p>
          The Quotation workspace has an <strong>Editor</strong> for the
          document you are building, <strong>Drafts</strong> for unfinished
          work, and <strong>Archive</strong> for every issued revision.
        </p>
        <p className="mt-2">
          The tabs stay here while you move between them.
        </p>
      </>
    ),
  },
  {
    id: "quotation-details",
    target: '[data-tutorial="quotation-details"]',
    page: "create",
    title: "Start with the commercial details",
    body: (
      <>
        <p>
          Choose the client, then set the recipient, subject, validity, payment
          terms, Incoterm and tax basis.
        </p>
        <p className="mt-2">
          These values become the snapshot on the issued revision; later client
          edits cannot rewrite an old quotation.
        </p>
      </>
    ),
  },
  {
    id: "quotation-lines",
    target: '[data-tutorial="quotation-lines"]',
    page: "create",
    title: "Price every line in place",
    body: (
      <>
        <p>
          Each line carries its own description, quantity, unit, cost, margin
          and row discount, with the sell price updating beside it.
        </p>
        <p className="mt-2">
          Split a line into priced sub-items when the customer needs a package,
          add scope lines beneath a part, or paste a block from Excel.
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
      <>
        <p>
          The summary separates the customer subtotal, PPN charged on top, and
          the internal margin, PPh22 and nett profit.
        </p>
        <p className="mt-2">
          Internal economics never print on the customer document. The printed
          source totals are evidence, not a target.
        </p>
      </>
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
          can never be changed afterwards.
        </p>
        <p className="mt-2">
          A new export becomes the next revision, and the archive keeps every
          one.
        </p>
      </>
    ),
  },
  {
    id: "quotation-drafts",
    target: '[data-tutorial="quotation-drafts"]',
    page: "drafts",
    title: "Pick up where you left off",
    body: (
      <p>
        Drafts are unfinished quotations. Open one to continue editing; the
        editor remembers the values you last saved.
      </p>
    ),
  },
  {
    id: "quotation-archive",
    target: '[data-tutorial="quotation-archive"]',
    page: "archive",
    title: "Every issued revision lives here",
    body: (
      <>
        <p>
          Search or filter the archive, then open a row to see its immutable
          revisions, the rendered document and its verification code.
        </p>
        <p className="mt-2">
          The same record can start a purchase order with one action.
        </p>
      </>
    ),
  },
  {
    id: "quotation-settings-tutorial",
    target: '[data-tutorial="quotation-settings-tutorial"]',
    page: "quotation-settings",
    title: "Replay this tutorial anytime",
    body: (
      <p>
        This card lives in Quotation settings. Use{" "}
        <strong>Replay tutorial</strong> whenever you want the tour again; it
        always starts from the first step.
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
        The Purchase Order workspace has an <strong>Editor</strong> for entry,{" "}
        <strong>Drafts</strong> for unfinished orders, and{" "}
        <strong>Archive</strong> for finalized records and corrections.
      </p>
    ),
  },
  {
    id: "po-link",
    target: '[data-tutorial="po-link-button"]',
    page: "po",
    title: "A purchase order begins with a quotation",
    body: (
      <>
        <p>
          A purchase order follows one issued quotation revision. Open the
          source selector and choose the practice quotation for this tutorial.
        </p>
        <p className="mt-2">
          In the practice, only one mock quotation is available — nothing is
          written to your real quotation or purchase-order lists.
        </p>
      </>
    ),
    advance: "click",
  },
  {
    id: "po-tutorial-selector",
    target: '[data-tutorial="tutorial-selector"]',
    title: "Choose the practice quotation",
    body: (
      <>
        <p>
          This selector is showing the single practice quotation created for
          the tutorial. Select it, keep revision 0, and confirm the source.
        </p>
        <p className="mt-2">
          The practice PO is not saved; it exists only while this tour is open.
        </p>
      </>
    ),
    advance: "event",
    advanceEvent: "po-tutorial-linked",
    action: {
      label: "Open the practice selector again",
      action: "open-tutorial-selector",
    },
  },
  {
    id: "po-header",
    target: '[data-tutorial="po-payment-terms"]',
    page: "po",
    title: "Quotation values appear inline",
    body: (
      <>
        <p>
          The quotation values are already beside your fields. A{" "}
          <strong>blue</strong> border means the quotation has a value you have
          not used yet; <strong>green</strong> means this field is using it.
        </p>
        <p className="mt-2">
          Focus Payment terms to see the bar: use the quotation value or keep
          yours. Your own value leaves the field normal.
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
          A ghost is the real quotation item, not a second form. This split
          ghost shows the parent, both priced parts and their scope lines.
        </p>
        <p className="mt-2">
          ✓ replaces your line with the quotation values. ✕ keeps your values
          and records the omission. Edit or remove an accepted line and its
          ghost returns.
        </p>
      </>
    ),
  },
  {
    id: "po-extra",
    target: '[data-tutorial="po-lines"]',
    page: "po",
    title: "Your own lines stay yours",
    body: (
      <p>
        A manual line with no quotation match is an extra line. It stays on the
        order and is acknowledged automatically; unmatched quotation items wait
        below, ready to add or ignore.
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
        The calculated subtotal, PPN and grand total belong to this purchase
        order. The printed quotation totals are shown beside them as source
        evidence, never as a total the PO must force itself to match.
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
      <>
        <p>
          Attach the customer PO PDF first when you have it. Final Save is the
          one-way transition to a saved record; if no PDF is attached, Vera asks
          you to confirm that omission deliberately.
        </p>
        <p className="mt-2">
          The optional confirmation number is issued at this point. This
          practice is not saved.
        </p>
      </>
    ),
  },
  {
    id: "po-drafts",
    target: '[data-tutorial="po-drafts"]',
    page: "po-drafts",
    title: "Drafts are unfinished orders",
    body: (
      <p>
        Your real purchase-order drafts live here. The practice PO from this
        tour is deliberately not in the list.
      </p>
    ),
  },
  {
    id: "po-archive",
    target: '[data-tutorial="po-archive"]',
    page: "po-archive",
    title: "Saved records and corrections",
    body: (
      <p>
        Finalized purchase orders live in the Archive. Open one to see its
        saved values, source revisions, PDF and correction history.
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
        This card lives in Purchase Order settings. Use{" "}
        <strong>Replay tutorial</strong> whenever you want the tour again; it
        always starts from the first step.
      </p>
    ),
    final: true,
  },
];
