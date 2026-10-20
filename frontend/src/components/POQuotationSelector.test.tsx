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
  hasActivePO: false,
};

const USED: POSourceQuotation = {
  id: 13,
  quoteNumber: "1502260002",
  subject: "Mechanical works",
  customerName: "PT. H-Tech Oilfield Equipment",
  latestRevision: 0,
  issuedRevisionCount: 1,
  hasActivePO: true,
};

const TWO_REVISIONS = [
  { revision: 1, issuedAt: "2026-10-20T10:00:00+07:00" },
  { revision: 0, issuedAt: "2026-10-18T10:00:00+07:00" },
];

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
          quotations: [QUOTATION, USED],
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
  ]);
}

function render(state: POSourceSelectorState): string {
  // renderToString inserts comment nodes between dynamic text and its
  // neighbours ("Step <!-- -->2<!-- --> of 3"). They are not content, so the
  // assertions read the text as a user would.
  return renderToString(
    <POQuotationSelectorView
      state={state}
      dispatch={() => {}}
      onClose={() => {}}
      onConfirm={() => {}}
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

  it("makes a used Quotation non-interactive with a reason and the disabled cursor", () => {
    const html = render(stepOneState());
    const usedTag = buttonTagFor(html, "Quotation 1502260002, PT. H-Tech Oilfield Equipment");
    expect(usedTag).toContain(" disabled=");
    expect(usedTag).toContain('aria-disabled="true"');
    expect(usedTag).toContain("cursor-not-allowed");

    // A disabled button's descendants are not uniformly exposed, so the
    // reason lives outside the button and the button points at it.
    const describedBy = /aria-describedby="([^"]+)"/.exec(usedTag)?.[1];
    expect(describedBy).toBeTruthy();
    expect(html).toContain(`id="${describedBy}"`);
    expect(html).toContain("Already has a purchase order");
    const usedElement = buttonElementFor(
      html,
      "Quotation 1502260002, PT. H-Tech Oilfield Equipment"
    );
    expect(usedElement).not.toContain("Already has a purchase order");

    // The available row is the contrast that makes the guard meaningful.
    const available = buttonTagFor(html, "Quotation 1502260001, PT. Infineon Technologies Batam");
    expect(available).not.toContain(" disabled=");
    expect(available).not.toContain("cursor-not-allowed");
    expect(available).not.toContain("aria-describedby=");
  });

  it("puts keyboard focus on a dialog the trap can own", () => {
    const html = render(stepOneState());
    // The container is a programmatic focus target (negative tabindex), so
    // opening the modal moves focus into it; the Tab handler keeps it there.
    const dialogTag = /<div[^>]*role="dialog"[^>]*>/.exec(html)?.[0];
    expect(dialogTag).toBeTruthy();
    expect(dialogTag).toContain('tabindex="-1"');
  });

  it("hides used rows when the filter is on, and shows the checked filter", () => {
    const html = render(stepOneState({ hideUsed: true }));
    expect(html).not.toContain("1502260002");
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
