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
  Closed: false,
  ClosedAt: null,
  ClosedByName: "",
  ClosedReason: "",
  ValidUntil: null,
  ExpiredUnclosed: false,
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
      revisionOptions={[{ value: 1, label: "Revision 01 (latest)" }]}
      onRevisionChange={noop}
      onBack={noop}
      onEdit={noop}
      purchaseOrder={null}
      purchaseOrderPending={false}
      purchaseOrderAllBought={false}
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

  it("offers Create PO and Open PO draft while a draft exists and quantity remains", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "draft" },
    });
    expect(buttonTagFor(html, "Open PO draft")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Create PO from this Quotation")).not.toContain(
      " disabled="
    );
    expect(html).not.toContain("View PO");
  });

  it("offers Create PO and View PO while a saved record leaves remaining quantity", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "saved" },
    });
    expect(buttonTagFor(html, "View PO")).not.toContain(" disabled=");
    expect(buttonTagFor(html, "Create PO from this Quotation")).not.toContain(
      " disabled="
    );
    expect(html).not.toContain("Open PO draft");
  });

  it("disables creation with revision guidance when every item is bought", () => {
    const html = renderHeader({
      purchaseOrder: { id: 41, state: "saved" },
      purchaseOrderAllBought: true,
    });
    expect(html).not.toContain("Create PO from this Quotation");
    const guidance = buttonTagFor(html, "All bought — revise the quotation");
    expect(guidance).toContain(" disabled=");
    expect(buttonTagFor(html, "View PO")).not.toContain(" disabled=");
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
    Closed: false,
    ClosedAt: null,
    ClosedByName: "",
    ClosedReason: "",
    ValidUntil: null,
    ExpiredUnclosed: false,
    ...overrides,
  };
}

async function flush(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("Quotation detail legacy discount (issue #652)", () => {
  it("shows a stored whole-order discount read-only on the revision", async () => {
    mocks.getCatalogue.mockResolvedValue([IDR_ENTRY]);
    mocks.getOccupancy.mockResolvedValue({ ActivePO: null });
    mocks.getRevision.mockResolvedValue(
      makeQuotation({
        DiscountPercent: 10,
        MarginalizedDiscount: 4100000,
        RevisionNumber: 0,
        AvailableRevisions: [0],
        Closed: false,
        ClosedAt: null,
        ClosedByName: "",
        ClosedReason: "",
        ValidUntil: null,
        ExpiredUnclosed: false,
      })
    );

    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <ToastProvider>
          <QuotationDetailPage
            quotationID={1}
            revision={0}
            onBack={noop}
            onEdit={noop}
            onCreatePurchaseOrder={async () => {}}
            onOpenPurchaseOrder={noop}
            currency={IDR_ENTRY}
          />
        </ToastProvider>
      );
    });
    await flush();
    await flush();

    // The archived revision keeps its stored 10% visible; there is no input
    // in the reader to change it.
    expect(container.textContent).toContain("Discount 10% (on the price)");
    expect(
      container.querySelector(
        'input[aria-label*="quotation discount" i], input[aria-label*="whole-order" i]'
      )
    ).toBeNull();

    act(() => root.unmount());
  });
});

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
              Closed: false,
              ClosedAt: null,
              ClosedByName: "",
              ClosedReason: "",
              ValidUntil: null,
              ExpiredUnclosed: false,
            })
          : makeQuotation({
              RevisionNumber: 0,
              Note: "Revision one note",
              AvailableRevisions: [1, 0],
              Closed: false,
              ClosedAt: null,
              ClosedByName: "",
              ClosedReason: "",
              ValidUntil: null,
              ExpiredUnclosed: false,
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

// Issue #656: the header is where closure becomes visible and actionable. A
// closed quotation cannot create a purchase order; the reason and the actor
// stay on the page; an admin can reopen it, and an expired-unclosed quotation
// is a warning badge only.
describe("QuotationDetailHeader closure and expiry (issue #656)", () => {
  const closed = {
    closed: true,
    closedAt: "2026-11-20T03:04:05Z",
    closedByName: "Closure Admin",
    closedReason: "customer cancelled the remainder",
    expiredUnclosed: false,
  };

  it("marks a closed quotation and disables Create PO with the reopen reason", () => {
    const html = renderHeader({ closure: closed });
    expect(html).toContain("Closed");
    expect(html).toContain("customer cancelled the remainder");
    expect(html).toContain("Closure Admin");
    expect(html).not.toContain("Create PO from this Quotation");
    const guidance = buttonTagFor(html, "Closed — reopen to order");
    expect(guidance).toContain(" disabled=");
    // Closure is not "all bought": the remedy differs.
    expect(html).not.toContain("All bought — revise the quotation");
  });

  it("shows an expired-unclosed badge without disabling Create PO", () => {
    const html = renderHeader({
      closure: {
        ...closed,
        closed: false,
        closedAt: null,
        closedByName: "",
        closedReason: "",
        expiredUnclosed: true,
      },
    });
    expect(html).toContain("Expired (unclosed)");
    expect(buttonTagFor(html, "Create PO from this Quotation")).not.toContain(
      " disabled="
    );
  });

  it("offers Close quotation to an admin on an open quotation", () => {
    const html = renderHeader({ isAdmin: true, onCloseQuotation: noop });
    expect(buttonTagFor(html, "Close quotation")).not.toContain(" disabled=");
    expect(html).not.toContain("Reopen quotation");
  });

  it("offers Reopen quotation to an admin on a closed quotation", () => {
    const html = renderHeader({
      isAdmin: true,
      closure: closed,
      onReopenQuotation: noop,
    });
    expect(buttonTagFor(html, "Reopen quotation")).not.toContain(" disabled=");
    expect(html).not.toContain("Close quotation<");
  });

  it("hides both closure actions from staff", () => {
    for (const closure of [closed, { ...closed, closed: false }]) {
      const html = renderHeader({ isAdmin: false, closure });
      expect(html).not.toContain("Close quotation<");
      expect(html).not.toContain("Reopen quotation");
    }
  });
});

// Issue #656: the whole close path through the real page. The dialog's own
// behaviour is covered beside it; this proves the page wires it to the binding
// wrapper, merges the answer without changing the displayed revision, and
// leaves the dialog open when the server refuses.
describe("Quotation detail closure actions (issue #656)", () => {
  function typeReason(value: string) {
    const field = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="Close reason"]'
    );
    if (field === null) throw new Error("the close reason textarea is not rendered");
    const setter = Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value"
    )?.set;
    setter?.call(field, value);
    act(() => {
      field.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }

  function clickButton(label: string) {
    const element = Array.from(document.querySelectorAll("button")).find(
      (candidate) =>
        candidate.getAttribute("aria-label") === label ||
        candidate.textContent?.trim() === label
    );
    if (element === undefined) throw new Error(`no button labelled "${label}"`);
    act(() => element.click());
  }

  it("closes through the dialog and keeps the displayed revision", async () => {
    mocks.getCatalogue.mockResolvedValue([IDR_ENTRY]);
    mocks.getOccupancy.mockResolvedValue({
      ActivePO: null,
      AllBought: false,
      Closed: false,
    });
    mocks.getRevision.mockResolvedValue(
      makeQuotation({ RevisionNumber: 0, AvailableRevisions: [1, 0] })
    );
    const onClose = vi.fn(async () =>
      makeQuotation({
        RevisionNumber: 1,
        AvailableRevisions: [1, 0],
        Closed: true,
        ClosedAt: "2026-11-20T03:04:05Z",
        ClosedByName: "Ada Admin",
        ClosedReason: "customer cancelled the remainder",
      })
    );

    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <ToastProvider>
          <QuotationDetailPage
            quotationID={1}
            revision={0}
            onBack={noop}
            onEdit={noop}
            onCreatePurchaseOrder={async () => {}}
            onOpenPurchaseOrder={noop}
            currency={IDR_ENTRY}
            isAdmin
            onCloseQuotation={onClose}
            onReopenQuotation={async () => makeQuotation()}
          />
        </ToastProvider>
      );
    });
    await flush();
    await flush();

    clickButton("Close quotation");
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
    typeReason("customer cancelled the remainder");
    clickButton("Confirm close quotation");
    await flush();
    await flush();

    expect(onClose).toHaveBeenCalledWith(1, "customer cancelled the remainder");
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(container.textContent).toContain("Closed");
    expect(container.textContent).toContain("customer cancelled the remainder");
    // The revision on screen did not jump to the latest one the response
    // carried: only the quotation-level closure fields were merged.
    expect(container.textContent).toContain("Edit revision 00");
    expect(container.textContent).not.toContain("Edit revision 01");

    act(() => root.unmount());
  });

  it("keeps the dialog and the reason when the server refuses", async () => {
    mocks.getCatalogue.mockResolvedValue([IDR_ENTRY]);
    mocks.getOccupancy.mockResolvedValue({
      ActivePO: null,
      AllBought: false,
      Closed: false,
    });
    mocks.getRevision.mockResolvedValue(makeQuotation());
    const onClose = vi.fn(async () => {
      throw new Error("This quotation is already closed.");
    });

    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() => {
      root.render(
        <ToastProvider>
          <QuotationDetailPage
            quotationID={1}
            revision={0}
            onBack={noop}
            onEdit={noop}
            onCreatePurchaseOrder={async () => {}}
            onOpenPurchaseOrder={noop}
            currency={IDR_ENTRY}
            isAdmin
            onCloseQuotation={onClose}
            onReopenQuotation={async () => makeQuotation()}
          />
        </ToastProvider>
      );
    });
    await flush();
    await flush();

    clickButton("Close quotation");
    typeReason("customer cancelled");
    clickButton("Confirm close quotation");
    await flush();
    await flush();

    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
    expect(
      document.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="Close reason"]'
      )?.value
    ).toBe("customer cancelled");
    expect(document.body.textContent).toContain("This quotation is already closed.");

    act(() => root.unmount());
  });
});
