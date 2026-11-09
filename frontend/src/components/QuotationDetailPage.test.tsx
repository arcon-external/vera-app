// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ArchivedQuotation } from "../archive";
import { ToastProvider } from "../toast";
import { NoteCard } from "./NoteCard";
import {
  QuotationDetailHeader,
  QuotationDetailPage,
  type QuotationDetailHeaderProps,
} from "./QuotationDetailPage";

// The detail page reads the desktop bindings; the note-card tests mount the
// real page with only those calls replaced, so the note is proven to follow
// the displayed revision rather than a fixture.
const mocks = vi.hoisted(() => ({
  getRevision: vi.fn(),
  getOccupancy: vi.fn(),
  getCatalogue: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/Quotation")>();
  return {
    ...actual,
    GetQuotationRevision: (...args: unknown[]) => mocks.getRevision(...args),
    GetPOSourceOccupancy: (...args: unknown[]) => mocks.getOccupancy(...args),
  };
});

vi.mock("@backend/backend/AdminAPI", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@backend/backend/AdminAPI")>();
  return {
    ...actual,
    GetCurrencyCatalogue: (...args: unknown[]) => mocks.getCatalogue(...args),
  };
});

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

afterEach(() => {
  document.body.innerHTML = "";
});

// Issue #499: the issued-Quotation detail action. It is no longer a second
// entry point into the general selector -- it acts on the revision on screen.
// The header is rendered without a DOM (the page fetches in an effect), so
// these pins prove the three labels exist exclusively, and that the action is
// disabled while the occupancy answer is pending or the revision is switching.
// The click-through lives in quotationDetailPOApp.dom.test.tsx.

const QUOTATION = {
  QuoteNumber: "1502260001",
  RevisionNumber: 1,
  AvailableRevisions: [1, 0],
  CustomerName: "PT. Infineon Technologies Batam",
  Subject: "Electrical installation",
} as ArchivedQuotation;

const noop = () => {};

function buttonTagFor(html: string, label: string): string {
  const button = html
    .split("<button")
    .slice(1)
    .find((chunk) =>
      chunk.slice(0, chunk.indexOf(">")).includes(`aria-label="${label}"`)
    );
  if (!button) throw new Error(`no <button aria-label="${label}"> in: ${html}`);
  return button.slice(0, button.indexOf(">") + 1);
}

function renderHeader(
  overrides: Partial<QuotationDetailHeaderProps> = {}
): string {
  return renderToString(
    <QuotationDetailHeader
      quotation={QUOTATION}
      activeRevision={1}
      revisionOptions={[{ value: 1, label: "Revision 01 — latest" }]}
      onRevisionChange={noop}
      onBack={noop}
      onEdit={noop}
      purchaseOrder={null}
      purchaseOrderPending={false}
      onCreatePurchaseOrder={noop}
      onOpenPurchaseOrder={noop}
      {...overrides}
    />
  ).replace(/<!-- -->/g, "");
}

describe("QuotationDetailHeader from-this-Quotation action (issue #499)", () => {
  it("offers exactly Create PO from this Quotation when the Quotation is free", () => {
    const html = renderHeader();
    const action = buttonTagFor(html, "Create PO from this Quotation");
    expect(action).not.toContain(" disabled=");
    expect(html).toContain("Create PO from this Quotation");
    expect(html).not.toContain("Open PO draft");
    expect(html).not.toContain("View PO");
  });

  it("offers Open PO draft, and no creation attempt, when a draft occupies it", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "draft" },
    });
    expect(buttonTagFor(html, "Open PO draft")).not.toContain(" disabled=");
    expect(html).not.toContain("Create PO from this Quotation");
    expect(html).not.toContain("View PO");
  });

  it("offers View PO when the occupying record is finalized", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "saved" },
    });
    expect(buttonTagFor(html, "View PO")).not.toContain(" disabled=");
    expect(html).not.toContain("Create PO from this Quotation");
    expect(html).not.toContain("Open PO draft");
  });

  it("disables creation until the occupancy answer arrives", () => {
    const html = renderHeader({ purchaseOrderPending: true });
    expect(buttonTagFor(html, "Create PO from this Quotation")).toContain(
      " disabled="
    );
  });

  it("disables creation while the displayed revision is being replaced", () => {
    const html = renderHeader({ revisionSwitching: true });
    expect(buttonTagFor(html, "Create PO from this Quotation")).toContain(
      " disabled="
    );
  });

  it("keeps the existing Edit action and quotation identity", () => {
    const html = renderHeader();
    expect(html).toContain("Edit revision 01");
    expect(html).toContain("1502260001");
    expect(html).toContain("PT. Infineon Technologies Batam");
    expect(html).toContain("Electrical installation");
  });
});

const IDR_ENTRY = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before" as const,
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

function makeQuotation(
  overrides: Partial<ArchivedQuotation> = {}
): ArchivedQuotation {
  return {
    ID: 1,
    QuoteNumber: "1502260001",
    RevisionNumber: 0,
    CustomerName: "PT. Infineon Technologies Batam",
    CustomerCode: 4,
    CustomerAddress: "",
    RecipientName: "",
    RecipientAddress: "",
    Note: "",
    Subject: "Electrical installation",
    Currency: "IDR",
    CreatedAt: "",
    GrandTotal: 0,
    DisplayGrand: "",
    Incoterm: "EXW",
    ValidityDays: 30,
    PaymentTermDays: 90,
    DisplayLeadTime: "",
    LeadTimeYears: 0,
    LeadTimeMonths: 0,
    LeadTimeWeeks: 0,
    LeadTimeDays: 0,
    DiscountPercent: 0,
    TaxZone: "FTZ",
    TaxCategory: "",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxRateBps: 0,
    OriginalSubtotal: 0,
    OriginalDiscount: 0,
    OriginalGrand: 0,
    MarginalizedSubtotal: 0,
    MarginalizedDiscount: 0,
    MarginalizedGrand: 0,
    PpnAmount: 0,
    DisplayPpn: "",
    TaxAmount: 0,
    MarginalizedAfterTax: 0,
    ProfitAmount: 0,
    NettProfit: 0,
    DisplayOriginalSubtotal: "",
    DisplayOriginalDiscount: "",
    DisplayOriginalGrand: "",
    DisplayMarginalizedSubtotal: "",
    DisplayMarginalizedDiscount: "",
    DisplayMarginalizedGrand: "",
    DisplayTax: "",
    DisplayMarginalizedAfterTax: "",
    DisplayProfit: "",
    DisplayNett: "",
    Items: [],
    HTMLSnapshot: "",
    AvailableRevisions: [1, 0],
    ...overrides,
  };
}

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("Quotation detail note card (#535)", () => {
  it("renders the formatted note as customer-facing elements", () => {
    const html = renderToString(
      <NoteCard
        source={
          "Delivery is **ex-works**.\n- Two copies\n- One original\n\n1. Notify security\n2. Log the visit"
        }
      />
    ).replace(/<!-- -->/g, "");
    expect(html).toContain("Note");
    expect(html).toContain("<strong>ex-works</strong>");
    expect(html).toContain("<ul");
    expect(html).toContain("<li");
    expect(html).toContain("<ol");
    expect(html).toContain("Notify security");
    expect(html).not.toContain("**");
  });

  it("renders nothing for an empty or whitespace-only note", () => {
    expect(renderToString(<NoteCard source="" />)).toBe("");
    expect(renderToString(<NoteCard source={"  \n\t\n"} />)).toBe("");
  });

  it("switches the note with the displayed revision", async () => {
    mocks.getCatalogue.mockResolvedValue([IDR_ENTRY]);
    mocks.getOccupancy.mockResolvedValue({ ActivePO: null });
    mocks.getRevision.mockImplementation(
      async (_id: number, revision: number) =>
        revision === 1
          ? makeQuotation({
              RevisionNumber: 1,
              Note: "Revision two note",
              AvailableRevisions: [1, 0],
            })
          : makeQuotation({
              RevisionNumber: 0,
              Note: "Revision one note",
              AvailableRevisions: [1, 0],
            })
    );

    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const render = (revision: number) => {
      act(() => {
        root.render(
          <ToastProvider>
            <QuotationDetailPage
              quotationID={1}
              revision={revision}
              onBack={noop}
              onEdit={noop}
              onCreatePurchaseOrder={async () => {}}
              onOpenPurchaseOrder={noop}
              currency={IDR_ENTRY}
            />
          </ToastProvider>
        );
      });
    };

    render(0);
    await flush();
    await flush();
    expect(container.textContent).toContain("Revision one note");
    expect(container.textContent).not.toContain("Revision two note");

    render(1);
    await flush();
    await flush();
    expect(container.textContent).toContain("Revision two note");
    expect(container.textContent).not.toContain("Revision one note");

    act(() => root.unmount());
  });
});
