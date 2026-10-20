// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderReviewDTO } from "../poFinalize";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import { PurchaseOrderEditorPage } from "./PurchaseOrderEditor";

// Issue #471: the final-save flow in a real DOM. These are the interactions the
// SSR suite cannot reach: opening the review, choosing a side, watching the
// gating change, returning to entry, and the no-PDF confirmation before the
// one call that finalizes.
//
// The bindings are replaced at their module boundary, which is what the
// desktop provides; everything inside the editor and the review is real.

const { reviewMock, finalizeMock } = vi.hoisted(() => ({
  reviewMock: vi.fn(),
  finalizeMock: vi.fn(),
}));

vi.mock("@backend/backend/Quotation", () => ({
  GetPurchaseOrderReview: (...args: unknown[]) => reviewMock(...args),
  FinalizePurchaseOrder: (...args: unknown[]) => finalizeMock(...args),
  UpdatePurchaseOrderDraft: vi.fn(),
}));

vi.mock("@backend/models", () => ({
  backend: {
    PurchaseOrderFinalizeInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderUpdateInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
    PurchaseOrderItemInput: class {
      constructor(source: Record<string, unknown>) {
        Object.assign(this, source);
      }
    },
  },
}));

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true;

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const ORDER: PurchaseOrderDTO = {
  PODate: null,
  RequestedDeliveryDate: null,
  UUID: "uuid-471",
  State: "draft",
  QuoteNumber: "1502260042",
  CustomerName: "PT. Buyer",
  BillingAddress: "Jl. Billing",
  DeliveryAddress: "Jl. Delivery",
  ShipToCode: "",
  ContactEmail: "",
  ContactPhone: "",
  PONumber: "PO-471",
  ReceivedDate: "2026-10-02",
  Incoterms: "CIF",
  Currency: "USD",
  TaxZone: "FTZ",
  TaxCategory: "",
  AdjustmentReason: "",
  CreatedAt: "2026-10-02T10:00:00Z",
  UpdatedAt: "2026-10-02T10:00:00Z",
  Items: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineNumber: 1,
      Description: "Widget",
      QuantityScaled: 20000,
      UoM: "Pc",
      UnitPriceMinor: 11000,
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 22000,
      DiscountMinor: 0,
      NetMinor: 22000,
    },
    {
      QuotationItemNo: 3,
      QuotationSubNo: null,
      LineNumber: 2,
      Description: "Spare",
      QuantityScaled: 10000,
      UoM: "Lot",
      UnitPriceMinor: 5000,
      DiscountEnabled: true,
      DiscountPercent: 10,
      GrossMinor: 5000,
      DiscountMinor: 500,
      NetMinor: 4500,
    },
  ],
  ID: 7,
  QuotationID: 12,
  SourceRevision: 1,
  CustomerID: 2,
  PaymentTerms: "30 days",
  DiscountPercent: 0,
  PpnBps: 0,
  SubtotalMinor: 26500,
  DiscountMinor: 0,
  PpnMinor: 0,
  AdjustmentMinor: 0,
  GrandTotalMinor: 26500,
  PrintedSubtotalMinor: 24500,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 24500,
};

const REVIEW: PurchaseOrderReviewDTO = {
  Order: ORDER,
  DocumentState: "none",
  Source: {
    QuotationID: 12,
    RevisionNumber: 1,
    QuoteNumber: "1502260042",
    CustomerID: 2,
    CustomerName: "PT. Buyer",
    Currency: "USD",
    PaymentTerms: "45 days",
    Incoterms: "CIF",
    DiscountPercent: 0,
    PrintedSubtotalMinor: 24500,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 24500,
    Lines: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineLabel: "1",
        Description: "Widget",
        UoM: "Pc",
        QuantityScaled: 20000,
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
      },
      {
        QuotationItemNo: 3,
        QuotationSubNo: null,
        LineLabel: "3",
        Description: "Spare",
        UoM: "Lot",
        QuantityScaled: 10000,
        UnitPriceMinor: 5000,
        DiscountEnabled: true,
        DiscountPercent: 10,
      },
    ],
  },
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  reviewMock.mockReset();
  finalizeMock.mockReset();
  reviewMock.mockResolvedValue(REVIEW);
  finalizeMock.mockResolvedValue({
    ...ORDER,
    State: "saved",
    PaymentTerms: "45 days",
    GrandTotalMinor: 24500,
    PdfAbsentConfirmed: true,
  });
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

function button(label: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`
  );
  if (!found) throw new Error(`button ${label} not found`);
  return found;
}

function pressed(label: string): string | null {
  return button(label).getAttribute("aria-pressed");
}

function disabled(label: string): boolean {
  return button(label).disabled;
}

function checkbox(label: string): HTMLInputElement {
  const found = document.querySelector<HTMLInputElement>(
    `input[aria-label="${label}"]`
  );
  if (!found) throw new Error(`checkbox ${label} not found`);
  return found;
}

function render(onSaved = vi.fn()) {
  act(() =>
    root.render(
      <PurchaseOrderEditorPage
        order={ORDER}
        currency={USD}
        uomOptions={[{ value: "Pc", label: "Pc" }]}
        incotermOptions={[{ value: "CIF", label: "CIF" }]}
        onSaved={onSaved}
        onCancel={() => undefined}
      />
    )
  );
  return onSaved;
}

describe("final save from the editor (issue #471)", () => {
  it("opens the review with no choice preselected and recalculates after a choice", async () => {
    render();
    await act(async () => {
      button("Final Save").click();
    });

    expect(document.body.textContent).toContain("Review differences before saving");
    expect(button("Use PO entry for Line 1 unit price")).toBeTruthy();
    expect(button("Use Quotation for Line 1 unit price")).toBeTruthy();
    expect(pressed("Use PO entry for Line 1 unit price")).toBe("false");
    expect(pressed("Use Quotation for Line 1 unit price")).toBe("false");
    // No choice anywhere is preselected: both sides of both differences are
    // unpressed when the review opens.
    expect(
      document.querySelectorAll('button[aria-pressed="true"]')
    ).toHaveLength(0);
    expect(disabled("Continue final save")).toBe(true);

    await act(async () => {
      button("Use Quotation for Line 1 unit price").click();
    });
    expect(pressed("Use Quotation for Line 1 unit price")).toBe("true");
    expect(pressed("Use PO entry for Line 1 unit price")).toBe("false");
    // The wording difference is independent; the price alone does not enable
    // the save.
    expect(disabled("Continue final save")).toBe(true);
    await act(async () => {
      button("Use Quotation for Payment terms").click();
    });
    expect(pressed("Use Quotation for Payment terms")).toBe("true");
    // The applied quotation values reproduce the printed total, so the
    // remaining difference is gone and Continue is enabled.
    expect(disabled("Continue final save")).toBe(false);
    expect(document.body.textContent).not.toContain("-$20.00");
    expect(document.body.textContent).toContain("$0.00");
  });

  it("returns to entry with the entered values still there", async () => {
    render();
    await act(async () => {
      button("Final Save").click();
    });
    await act(async () => {
      button("Back to entry").click();
    });
    const terms = document.querySelector<HTMLInputElement>(
      'input[aria-label="Payment terms"]'
    );
    expect(terms?.value).toBe("30 days");
    expect(document.body.textContent).toContain("Final Save");
  });

  it("requires the no-PDF confirmation before the one finalize call", async () => {
    const onSaved = render();
    await act(async () => {
      button("Final Save").click();
    });
    await act(async () => {
      button("Use Quotation for Line 1 unit price").click();
    });
    await act(async () => {
      button("Use Quotation for Payment terms").click();
    });
    await act(async () => {
      button("Continue final save").click();
    });

    expect(document.body.textContent).toContain("Final save");
    expect(disabled("Final save")).toBe(true);
    expect(finalizeMock).not.toHaveBeenCalled();

    await act(async () => {
      checkbox("Confirm saving without the customer PDF").click();
    });
    expect(disabled("Final save")).toBe(false);
    await act(async () => {
      button("Final save").click();
    });

    expect(finalizeMock).toHaveBeenCalledTimes(1);
    const payload = finalizeMock.mock.calls[0][0] as Record<string, unknown>;
    expect(payload.PONumber).toBe("PO-471");
    expect(payload.PaymentTerms).toBe("45 days");
    expect(payload.DocumentConfirmed).toBe(true);
    expect(payload.Decisions).toEqual([
      { Key: "header:paymentTerms", Choice: "quotation" },
      { Key: "line:1:unitPrice", Choice: "quotation" },
    ]);
    expect(payload.Decisions).not.toEqual(
      expect.arrayContaining([{ Key: "line:1:unitPrice", Choice: "po" }])
    );
    expect((payload.Items as Array<{ UnitPriceMinor: number }>)[0].UnitPriceMinor).toBe(10000);
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect((onSaved.mock.calls[0][0] as PurchaseOrderDTO).State).toBe("saved");
  });
});
