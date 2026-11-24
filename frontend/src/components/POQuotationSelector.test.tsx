import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  initialPOSourceSelectorState,
  poSourceSelectorReducer,
  type POSourceQuotation,
  type POSourceSelectorAction,
  type POSourceSelectorState,
} from "../poSelector";
import { POQuotationSelectorView } from "./POQuotationSelector";

// Issue #467: the modal's markup, rendered without a DOM. renderToString
// prints the attributes React will actually set, so `disabled` below is the
// button's real state, not a source scan.
//
// What this suite cannot prove: clicks, focus trapping, escape handling, or how
// the dialog reads at 360/1280. Those are hand-driven and the snapshot case.

const QUOTATION: POSourceQuotation = {
  id: 12,
  quoteNumber: "1502260001",
  subject: "Electrical installation",
  customerName: "PT. Infineon Technologies Batam",
  latestRevision: 1,
  issuedRevisionCount: 2,
  activePO: null,
  allBought: false,
  leafCount: 2,
  remainingLeafCount: 2,
  savedPoCount: 0,
  draftPoCount: 0,
  legacyHeaderDiscount: false,
  closed: false,
  closedAt: null,
  closedReason: "",
  validUntil: null,
  expiredUnclosed: false,
};

const USED: POSourceQuotation = {
  id: 13,
  quoteNumber: "1502260002",
  subject: "Mechanical works",
  customerName: "PT. H-Tech Oilfield Equipment",
  latestRevision: 0,
  issuedRevisionCount: 1,
  activePO: { id: 21, state: "saved" },
  allBought: false,
  leafCount: 2,
  remainingLeafCount: 1,
  savedPoCount: 1,
  draftPoCount: 0,
  legacyHeaderDiscount: false,
  closed: false,
  closedAt: null,
  closedReason: "",
  validUntil: null,
  expiredUnclosed: false,
};

const DRAFT: POSourceQuotation = {
  id: 14,
  quoteNumber: "1502260003",
  subject: "Commissioning",
  customerName: "PT. Datalec Global Solution",
  latestRevision: 0,
  issuedRevisionCount: 1,
  activePO: { id: 20, state: "draft" },
  allBought: false,
  leafCount: 1,
  remainingLeafCount: 1,
  savedPoCount: 0,
  draftPoCount: 1,
  legacyHeaderDiscount: false,
  closed: false,
  closedAt: null,
  closedReason: "",
  validUntil: null,
  expiredUnclosed: false,
};

const ALL_BOUGHT: POSourceQuotation = {
  id: 15,
  quoteNumber: "1502260004",
  subject: "Completed supply",
  customerName: "PT. Fully Bought",
  latestRevision: 0,
  issuedRevisionCount: 1,
  activePO: { id: 22, state: "saved" },
  allBought: true,
  leafCount: 2,
  remainingLeafCount: 0,
  savedPoCount: 2,
  draftPoCount: 0,
  legacyHeaderDiscount: false,
  closed: false,
  closedAt: null,
  closedReason: "",
  validUntil: null,
  expiredUnclosed: false,
};

const TWO_REVISIONS = [
  { revision: 1, issuedAt: "2026-10-20T10:00:00+07:00" },
  { revision: 0, issuedAt: "2026-10-18T10:00:00+07:00" },
];

const AVAILABILITY = {
  quotationId: QUOTATION.id,
  revisionNumber: 1,
  latestRevision: 1,
  allBought: false,
  legacyHeaderDiscount: false,
  closed: false,
  closedAt: null,
  closedReason: "",
  validUntil: null,
  expiredUnclosed: false,
  savedPoCount: 1,
  draftPoCount: 0,
  items: [
    {
      quotationItemNo: 1,
      quotationSubNo: null,
      label: "1",
      description: "Electrical installation",
      uom: "Pc",
      quotedQuantityScaled: 100_000,
      boughtQuantityScaled: 40_000,
      draftQuantityScaled: 0,
      remainingQuantityScaled: 60_000,
      overBoughtQuantityScaled: 0,
      quotedUnitPriceMinor: 5_000_000,
    },
    {
      quotationItemNo: 2,
      quotationSubNo: null,
      label: "2",
      description: "Cabling",
      uom: "Lot",
      quotedQuantityScaled: 10_000,
      boughtQuantityScaled: 10_000,
      draftQuantityScaled: 0,
      remainingQuantityScaled: 0,
      overBoughtQuantityScaled: 0,
      quotedUnitPriceMinor: 2_500_000,
    },
  ],
};

const SOLE_REVISION = [{ revision: 0, issuedAt: "2026-10-18T10:00:00+07:00" }];

function reducerState(
  actions: POSourceSelectorAction[],
  start: POSourceSelectorState = initialPOSourceSelectorState
): POSourceSelectorState {
  return actions.reduce(poSourceSelectorReducer, start);
}

function stepOneState(overrides: Partial<POSourceSelectorState> = {}) {
  return {
    ...reducerState([
      {
        type: "rowsLoaded",
        page: {
          quotations: [QUOTATION, USED, DRAFT],
          page: 1,
          pageSize: 15,
          total: 17,
          totalPages: 2,
        },
      },
    ]),
    ...overrides,
  };
}

function atStepTwo(revisionList = TWO_REVISIONS) {
  return reducerState([
    {
      type: "rowsLoaded",
      page: {
        quotations: [QUOTATION],
        page: 1,
        pageSize: 15,
        total: 1,
        totalPages: 1,
      },
    },
    { type: "quotationSelected", quotation: QUOTATION },
    {
      type: "revisionsLoaded",
      quotationId: QUOTATION.id,
      revisions: revisionList,
    },
  ]);
}

function atStepThree(revision = 1) {
  return reducerState([
    { type: "rowsLoaded", page: { quotations: [QUOTATION], page: 1, pageSize: 15, total: 1, totalPages: 1 } },
    { type: "quotationSelected", quotation: QUOTATION },
    { type: "revisionsLoaded", quotationId: QUOTATION.id, revisions: TWO_REVISIONS },
    { type: "revisionSelected", revision },
    { type: "continue" },
    { type: "availabilityLoading" },
    {
      type: "availabilityLoaded",
      quotationId: QUOTATION.id,
      revision,
      availability: { ...AVAILABILITY, revisionNumber: revision },
    },
  ]);
}

function render(
  state: POSourceSelectorState,
  mode: "entry" | "repair" | "link" = "entry"
): string {
  // renderToString inserts comment nodes between dynamic text and its
  // neighbours ("Step <!-- -->2<!-- --> of 3"). They are not content, so the
  // assertions read the text as a user would.
  return renderToString(
    <POQuotationSelectorView
      state={state}
      dispatch={() => {}}
      onClose={() => {}}
      onConfirm={() => {}}
      onOpenDraft={() => {}}
      mode={mode}
    />
  ).replace(/<!-- -->/g, "");
}

function buttonElementFor(html: string, label: string): string {
  const button = html
    .split("<button")
    .slice(1)
    .find((chunk) => chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`));
  if (!button) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return button.slice(0, button.indexOf("</button>") + "</button>".length);
}

function buttonTagFor(html: string, label: string): string {
  const button = buttonElementFor(html, label);
  return button.slice(0, button.indexOf(">") + 1);
}

describe("POQuotationSelectorView step 1 (issue #467)", () => {
  it("renders a dialog with search, page position and both rows", () => {
    const html = render(stepOneState());
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain('aria-label="Select a quotation for the purchase order"');
    expect(html).toContain("Step 1 of 3");
    expect(html).toContain('aria-label="Search quotations"');
    expect(html).toContain("1502260001");
    expect(html).toContain("PT. Infineon Technologies Batam");
    expect(html).toContain("Electrical installation");
    expect(html).toContain("Page 1 of 2");
    expect(buttonTagFor(html, "Previous page")).toContain(" disabled=");
    expect(buttonTagFor(html, "Next page")).not.toContain(" disabled=");
  });

  it("keeps a partly bought Quotation selectable and shows its Partial badge", () => {
    const html = render(stepOneState());
    const partialTag = buttonTagFor(html, "Quotation 1502260002, PT. H-Tech Oilfield Equipment");
    expect(partialTag).not.toContain(" disabled=");
    expect(partialTag).not.toContain("cursor-not-allowed");
    expect(html).toContain("Partial");
    expect(html).toContain("1 of 2 left");
    expect(html).toContain("1 saved");
  });

  it("disables an All bought Quotation with a revision reason", () => {
    const state = stepOneState({
      rows: [ALL_BOUGHT],
      total: 1,
      totalPages: 1,
    });
    const html = render(state);
    const rowTag = buttonTagFor(html, "Quotation 1502260004, PT. Fully Bought");
    expect(rowTag).toContain(" disabled=");
    expect(rowTag).toContain('aria-disabled="true"');
    expect(rowTag).toContain("cursor-not-allowed");
    const describedBy = /aria-describedby="([^"]+)"/.exec(rowTag)?.[1];
    expect(describedBy).toBeTruthy();
    expect(html).toContain(`id="${describedBy}"`);
    expect(html).toContain("All bought");
    expect(html).toContain("Revise the quotation to order more.");
  });

  it("lets repair and link select an All bought quotation", () => {
    const state = stepOneState({
      rows: [ALL_BOUGHT],
      total: 1,
      totalPages: 1,
    });
    for (const mode of ["repair", "link"] as const) {
      const html = render(state, mode);
      const row = buttonTagFor(html, "Quotation 1502260004, PT. Fully Bought");
      expect(row).not.toContain(" disabled=");
      expect(html).not.toContain("Revise the quotation to order more.");
      expect(html).toContain("All bought");
    }
  });

  it("shows Open, not Partial, for an advisory draft", () => {
    const html = render(stepOneState());
    const draft = buttonElementFor(
      html,
      "Quotation 1502260003, PT. Datalec Global Solution"
    );
    expect(draft).toContain("Open");
    expect(draft).not.toContain("Partial");
    expect(draft).toContain("1 in draft");
  });

  it("offers Open draft for a draft while the row stays selectable for another PO", () => {
    const html = render(stepOneState());
    const draftRow = buttonTagFor(html, "Quotation 1502260003, PT. Datalec Global Solution");
    expect(draftRow).not.toContain(" disabled=");
    expect(html).toContain("1 in draft");
    const action = buttonTagFor(
      html,
      "Open draft for Quotation 1502260003"
    );
    expect(action).not.toContain(" disabled=");
    // A saved record never offers the draft action.
    expect(html).not.toContain("Open draft for Quotation 1502260002");
  });

  it("puts keyboard focus on a dialog the trap can own", () => {
    const html = render(stepOneState());
    // The container is a programmatic focus target (negative tabindex), so
    // opening the modal moves focus into it; the Tab handler keeps it there.
    const dialogTag = /<div[^>]*role="dialog"[^>]*>/.exec(html)?.[0];
    expect(dialogTag).toBeTruthy();
    expect(dialogTag).toContain('tabindex="-1"');
  });

  it("hides All bought rows when the filter is on, and shows the checked filter", () => {
    const html = render(
      stepOneState({
        hideUsed: true,
        rows: [QUOTATION, USED, DRAFT, ALL_BOUGHT],
      })
    );
    expect(html).not.toContain("1502260004");
    expect(html).toContain("1502260002");
    expect(html).toContain("Hide quotations already used");
    expect(html).toMatch(/type="checkbox"[^>]*checked/);
  });

  it("shows a loading state and an error state without inventing rows", () => {
    const loading = render(stepOneState({ rowsLoading: true, rows: [] }));
    expect(loading).toContain("Loading quotations");
    const failed = render(
      stepOneState({ rowsLoading: false, rows: [], rowsError: "Network is unreachable" })
    );
    expect(failed).toContain("Network is unreachable");
    expect(failed).not.toContain("<table");
  });
});

describe("POQuotationSelectorView step 2 (issue #467)", () => {
  it("disables Continue until a revision is chosen", () => {
    const html = render(atStepTwo());
    expect(html).toContain("Step 2 of 3");
    expect(buttonTagFor(html, "Continue to confirmation")).toContain(" disabled=");

    const chosen = render(reducerState([{ type: "revisionSelected", revision: 0 }], atStepTwo()));
    expect(buttonTagFor(chosen, "Continue to confirmation")).not.toContain(" disabled=");
  });

  it("shows and auto-selects a sole revision", () => {
    const html = render(atStepTwo(SOLE_REVISION));
    expect(html).toContain("Only issued revision");
    expect(html).toContain("selected automatically");
    expect(buttonTagFor(html, "Continue to confirmation")).not.toContain(" disabled=");
  });

  it("offers Back to step 1", () => {
    const html = render(atStepTwo());
    expect(buttonTagFor(html, "Back")).not.toContain(" disabled=");
  });
});

describe("POQuotationSelectorView step 3 (issue #467)", () => {
  it("shows the exact pair and enables Confirm when complete", () => {
    const html = render(atStepThree(1));
    expect(html).toContain("Step 3 of 3");
    expect(html).toContain("1502260001");
    expect(html).toContain("Revision 1");
    expect(buttonTagFor(html, "Confirm source")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Back")).not.toContain(" disabled=");
  });

  it("keeps Confirm disabled when the pair is incomplete", () => {
    // The guard stated directly: a step-3 screen without a revision must not
    // be able to commit a link.
    const broken: POSourceSelectorState = { ...atStepThree(1), selectedRevision: null };
    const html = render(broken);
    expect(buttonTagFor(html, "Confirm source")).toContain(" disabled=");
  });

  it("shows the server revalidation failure beside the choice", () => {
    const failed = reducerState(
      [
        { type: "confirmStarted" },
        {
          type: "confirmFailed",
          error: "This quotation no longer exists. Go back and choose another source.",
        },
      ],
      atStepThree(1)
    );
    const html = render(failed);
    expect(html).toContain("no longer exists");
    expect(html).toContain('role="alert"');
  });
});

// Issue #656: closure and expiry share the row's badge area but mean different
// things. A closed quotation is disabled with its own reason; an expired but
// unclosed one stays selectable because its remainder is still Unsold.
const CLOSED_ROW: POSourceQuotation = {
  ...QUOTATION,
  id: 21,
  quoteNumber: "1502260021",
  customerName: "PT. Closed Customer",
  closed: true,
  closedAt: "2026-11-20T03:04:05Z",
  closedReason: "customer cancelled the remainder",
  validUntil: "2026-12-01",
};

const EXPIRED_ROW: POSourceQuotation = {
  ...QUOTATION,
  id: 22,
  quoteNumber: "1502260022",
  customerName: "PT. Expired Customer",
  validUntil: "2026-01-01",
  expiredUnclosed: true,
};

describe("POQuotationSelectorView closure and expiry (issue #656)", () => {
  function oneRowState(row: POSourceQuotation): POSourceSelectorState {
    return stepOneState({ rows: [row], total: 1, totalPages: 1 });
  }

  it("disables a closed Quotation with its own reopen reason", () => {
    const html = render(oneRowState(CLOSED_ROW));
    const rowTag = buttonTagFor(html, "Quotation 1502260021, PT. Closed Customer");
    expect(rowTag).toContain(" disabled=");
    expect(rowTag).toContain('aria-disabled="true"');
    const describedBy = /aria-describedby="([^"]+)"/.exec(rowTag)?.[1];
    expect(describedBy).toBeTruthy();
    expect(html).toContain(`id="${describedBy}"`);
    expect(html).toContain("Closed");
    expect(html).toContain("Reopen it before ordering.");
    // The all-bought reason stays distinct: a closed quotation is not
    // "fully bought", and its remedy is a reopen, not a revision.
    expect(html).not.toContain("Revise the quotation to order more.");
  });

  it("keeps an expired-unclosed Quotation selectable with its badge", () => {
    const html = render(oneRowState(EXPIRED_ROW));
    const rowTag = buttonTagFor(html, "Quotation 1502260022, PT. Expired Customer");
    expect(rowTag).not.toContain(" disabled=");
    expect(html).toContain("Expired (unclosed)");
    expect(html).not.toContain("Reopen it before ordering.");
  });

  it("lets repair and link select a closed quotation", () => {
    for (const mode of ["repair", "link"] as const) {
      const html = render(oneRowState(CLOSED_ROW), mode);
      const row = buttonTagFor(html, "Quotation 1502260021, PT. Closed Customer");
      expect(row).not.toContain(" disabled=");
      expect(html).toContain("Closed");
      expect(html).not.toContain("Reopen it before ordering.");
    }
  });

  it("disables Confirm when the availability read says the source is closed", () => {
    const state = atStepThree();
    const closed: POSourceSelectorState = {
      ...state,
      availability:
        state.availability === null
          ? null
          : { ...state.availability, closed: true, closedReason: "customer cancelled" },
    };
    const html = render(closed);
    expect(html).toContain("This quotation is closed. Reopen it before ordering.");
    expect(buttonTagFor(html, "Confirm source")).toContain(" disabled=");
  });
});
