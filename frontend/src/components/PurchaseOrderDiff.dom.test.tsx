// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderSourceDTO } from "../poFinalize";
import {
  initialPurchaseOrderEditorState,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import { PurchaseOrderDifferenceReviewView } from "./PurchaseOrderFinalize";

// Issue #582: the two-sided review is presentation-only, so these tests hold
// the seam that matters: the existing callbacks still fire, the chosen side is
// the only one marked "Will be used", the other stays readable, and a recorded
// acknowledgement becomes a chip. No comparison or payload code is involved.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function order(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-diff",
    State: "draft",
    QuoteNumber: "1502260042",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing",
    DeliveryAddress: "Jl. Delivery",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
    PONumber: "PO-582",
    ReceivedDate: "2026-10-02",
    Version: 1,
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
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 20000,
        DiscountMinor: 0,
        NetMinor: 20000,
      },
    ],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTerms: "30 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 20000,
    PrintedSubtotalMinor: 24500,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 24500,
    ...overrides,
  };
}

const SOURCE: PurchaseOrderSourceDTO = {
  QuotationID: 12,
  RevisionNumber: 1,
  QuoteNumber: "1502260042",
  CustomerID: 2,
  CustomerName: "PT. Buyer",
  Currency: "USD",
  PaymentTerms: "45 days",
  Incoterms: "CIF",
  DiscountPercent: 0,
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
      DiscountEnabled: false,
      DiscountPercent: 0,
    },
  ],
  PrintedSubtotalMinor: 24500,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 24500,
};

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

interface Options {
  paymentTerms?: string;
  decisions?: Record<string, "po" | "quotation">;
  acknowledgedMissing?: string[];
  acknowledgedExtra?: number[];
  items?: PurchaseOrderDTO["Items"];
  onDecision?: (key: string, choice: "po" | "quotation") => void;
  onToggleMissing?: (label: string) => void;
  onToggleExtra?: (lineNumber: number) => void;
}

function render(options: Options = {}) {
  const state = initialPurchaseOrderEditorState(
    order({
      PaymentTerms: options.paymentTerms ?? "30 days",
      ...(options.items === undefined ? {} : { Items: options.items }),
    }),
    USD
  );
  act(() =>
    root.render(
      <PurchaseOrderDifferenceReviewView
        state={state}
        source={SOURCE}
        currency={USD}
        quoteNumber="1502260042"
        documentState="none"
        decisions={options.decisions ?? {}}
        acknowledgedMissing={options.acknowledgedMissing ?? []}
        acknowledgedExtra={options.acknowledgedExtra ?? []}
        onDecision={options.onDecision ?? (() => undefined)}
        onToggleMissing={options.onToggleMissing ?? (() => undefined)}
        onToggleExtra={options.onToggleExtra ?? (() => undefined)}
        onMatchChange={() => undefined}
        onBack={() => undefined}
        onContinue={() => undefined}
      />
    )
  );
}

function side(name: "po" | "quotation"): HTMLElement {
  const found = document.querySelector<HTMLElement>(`[data-side="${name}"]`);
  if (!found) throw new Error(`side ${name} not found`);
  return found;
}

describe("two-sided difference review (#582)", () => {
  it("records a choice through the unchanged callback names", () => {
    const onDecision = vi.fn();
    render({ onDecision });
    act(() =>
      document
        .querySelector<HTMLButtonElement>(
          'button[aria-label="Use Quotation for Payment terms"]'
        )!
        .click()
    );
    expect(onDecision).toHaveBeenCalledWith("header:paymentTerms", "quotation");
  });

  it("marks only the chosen side as the one that will be used", () => {
    render({ decisions: { "header:paymentTerms": "quotation" } });
    expect(side("quotation").getAttribute("data-selected")).toBe("true");
    expect(side("quotation").textContent).toContain("Will be used");
    expect(side("quotation").textContent).toContain("45 days");
    expect(side("po").getAttribute("data-selected")).toBe("false");
    expect(side("po").textContent).not.toContain("Will be used");
    // The losing side stays readable, not hidden.
    expect(side("po").textContent).toContain("30 days");
  });

  it("uses the whole card for both values and keeps one hint on the unselected side (#587)", () => {
    const onDecision = vi.fn();
    render({ decisions: { "header:paymentTerms": "quotation" }, onDecision });
    const quotationSide = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Use Quotation for Payment terms"]'
    );
    const poSide = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Use PO entry for Payment terms"]'
    );
    expect(quotationSide).not.toBeNull();
    expect(poSide).not.toBeNull();
    expect(quotationSide?.tagName).toBe("BUTTON");
    expect(poSide?.tagName).toBe("BUTTON");
    // The selected side keeps the ring and "Will be used" and has no action
    // affordance; the unselected side offers one, and both remain clickable.
    expect(quotationSide?.getAttribute("aria-pressed")).toBe("true");
    expect(quotationSide?.textContent).toContain("Will be used");
    expect(quotationSide?.textContent).not.toContain("Use this value");
    expect(quotationSide?.querySelector("button")).toBeNull();
    expect(poSide?.getAttribute("aria-pressed")).toBe("false");
    expect(poSide?.textContent).toContain("Use this value");
    expect(poSide?.querySelector("button")).toBeNull();
    act(() => quotationSide?.click());
    act(() => poSide?.click());
    expect(onDecision).toHaveBeenNthCalledWith(1, "header:paymentTerms", "quotation");
    expect(onDecision).toHaveBeenNthCalledWith(2, "header:paymentTerms", "po");
  });

  it("accepts keyboard activation because each side is a native button", () => {
    render();
    for (const label of [
      "Use PO entry for Payment terms",
      "Use Quotation for Payment terms",
    ]) {
      const card = document.querySelector<HTMLButtonElement>(
        `button[aria-label="${label}"]`
      );
      expect(card?.type).toBe("button");
      expect(card?.getAttribute("tabindex")).toBeNull();
    }
  });

  it("keeps a formatted money value on one line and gives the changed side the accent", () => {
    const base = order();
    render({ items: [{ ...base.Items[0], UnitPriceMinor: 12000 }] });
    const row = document.querySelector<HTMLElement>(
      '[role="group"][aria-label="Line 1 unit price"]'
    );
    const poValue = row?.querySelector<HTMLElement>('[data-side="po"] [data-value]');
    const quotationValue = row?.querySelector<HTMLElement>(
      '[data-side="quotation"] [data-value]'
    );
    expect(poValue).not.toBeNull();
    expect(quotationValue).not.toBeNull();
    // Money never wraps; free text is covered by the next test.
    expect(poValue?.className).toContain("whitespace-nowrap");
    expect(quotationValue?.className).toContain("whitespace-nowrap");
    // The changed value is stronger than the chip alone: the quotation side
    // carries the accent and both sides are emphasised over body copy.
    expect(quotationValue?.className).toContain("text-accent-800");
    expect(quotationValue?.className).toContain("font-semibold");
    expect(poValue?.className).toContain("font-semibold");
  });

  it("lets a free-text value wrap", () => {
    render();
    const poValue = side("po").querySelector<HTMLElement>("[data-value]");
    expect(poValue).not.toBeNull();
    expect(poValue?.className).not.toContain("whitespace-nowrap");
  });

  it("shows an empty side as a dashed (empty) placeholder", () => {
    render({ paymentTerms: "" });
    const placeholder = side("po").querySelector<HTMLElement>(
      '[data-empty="true"]'
    );
    expect(placeholder).not.toBeNull();
    expect(placeholder?.textContent).toBe("(empty)");
    expect(placeholder?.className).toContain("border-dashed");
  });

  it("acknowledges a missing line through the unchanged callback and then chips it", () => {
    const onToggleMissing = vi.fn();
    const base = order();
    const items = [base.Items[0]];
    render({ items, onToggleMissing });
    act(() =>
      document
        .querySelector<HTMLInputElement>(
          'input[aria-label="Acknowledge omission of quotation line 3"]'
        )!
        .click()
    );
    expect(onToggleMissing).toHaveBeenCalledWith("3");

    render({ items, acknowledgedMissing: ["3"] });
    expect(document.body.textContent).toContain("ACKNOWLEDGED");
    expect(document.body.textContent).toContain("Quotation line 3 — Spare");
  });

  it("acknowledges an extra line through the unchanged callback and then chips it", () => {
    const onToggleExtra = vi.fn();
    const base = order();
    const items = [
      base.Items[0],
      {
        ...base.Items[0],
        LineNumber: 2,
        QuotationItemNo: null,
        QuotationSubNo: null,
        Description: "Freight",
        UnitPriceMinor: 0,
      },
    ];
    render({ items, onToggleExtra });
    act(() =>
      document
        .querySelector<HTMLInputElement>(
          'input[aria-label="Acknowledge line 2 as extra"]'
        )!
        .click()
    );
    expect(onToggleExtra).toHaveBeenCalledWith(2);

    render({ items, acknowledgedExtra: [2] });
    expect(document.body.textContent).toContain("ACKNOWLEDGED");
  });

  it("counts differences, missing and extra lines in the summary", () => {
    const base = order();
    render({
      items: [
        base.Items[0],
        {
          ...base.Items[0],
          LineNumber: 2,
          QuotationItemNo: null,
          QuotationSubNo: null,
          Description: "Freight",
          UnitPriceMinor: 0,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
    });
    const summary = document.querySelector<HTMLElement>(
      '[data-testid="po-diff-summary"]'
    );
    expect(summary?.getAttribute("data-differences")).toBe("1");
    expect(summary?.getAttribute("data-missing")).toBe("1");
    expect(summary?.getAttribute("data-extra")).toBe("1");
  });
});
