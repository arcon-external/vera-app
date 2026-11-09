// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CurrencyInfo } from "../format";
import {
  buildFinalizeInput,
  evaluateReview,
  type PurchaseOrderReviewDTO,
} from "../poFinalize";
import {
  initialPurchaseOrderEditorState,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import {
  PurchaseOrderDifferenceReviewView,
} from "./PurchaseOrderFinalize";

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
  UUID: "uuid-receipt",
  State: "draft",
  QuoteNumber: "1502260042",
  CustomerName: "PT. Buyer",
  BillingAddress: "Jl. Billing",
  DeliveryAddress: "Jl. Delivery",
  ShipToCode: "",
  ContactEmail: "",
  ContactPhone: "",
  PONumber: "PO-RECEIPT",
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
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  document.body.innerHTML = "";
});

describe("final-save receipt from a recorded session (U2)", () => {
  it("renders session-resolved differences read-only with Change links, no second workbench", async () => {
    // Keep only line 1 in the working copy so source line 3 is missing.
    const full = initialPurchaseOrderEditorState(ORDER, USD);
    const state = { ...full, lines: full.lines.slice(0, 1) };
    const decisions = {
      "header:paymentTerms": "po" as const,
      "line:1:unitPrice": "po" as const,
    };
    const acknowledgedMissing = ["3"];
    const acknowledgedExtra: number[] = [];
    const onEditDifference = vi.fn();
    const onEditLine = vi.fn();

    await act(async () => {
      root.render(
        <PurchaseOrderDifferenceReviewView
          state={state}
          source={REVIEW.Source}
          currency={USD}
          quoteNumber="1502260042"
          documentState="none"
          decisions={decisions}
          acknowledgedMissing={acknowledgedMissing}
          acknowledgedExtra={acknowledgedExtra}
          onDecision={() => undefined}
          onToggleMissing={() => undefined}
          onToggleExtra={() => undefined}
          onMatchChange={() => undefined}
          onBack={() => undefined}
          onContinue={() => undefined}
          receiptResolvedKeys={["header:paymentTerms", "line:1:unitPrice"]}
          receiptAckedMissing={["3"]}
          receiptAckedExtra={[]}
          hasSession
          onEditDifference={onEditDifference}
          onEditLine={onEditLine}
        />
      );
    });

    // Resolved differences are receipts: recorded choice shown, no Use buttons.
    expect(document.body.textContent).toContain("PO entry kept in the reconciliation pane.");
    expect(
      document.querySelector('button[aria-label="Use PO entry for Payment terms"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Payment terms"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 unit price"]')
    ).toBeNull();
    // Change links return to the entry field.
    const changeTerms = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Change Payment terms"]'
    );
    expect(changeTerms).not.toBeNull();
    changeTerms!.click();
    expect(onEditDifference).toHaveBeenCalledWith("header:paymentTerms", "Payment terms");
    // The recorded omission is shown without a second checkbox.
    expect(document.body.textContent).toContain("omission acknowledged in the pane");
    expect(
      document.querySelector('input[aria-label="Acknowledge omission of quotation line 3"]')
    ).toBeNull();
    // Line matches are recorded evidence with a Change path, not a Select.
    expect(
      document.querySelector('button[aria-label="Change match for line 1"]')
    ).not.toBeNull();
    expect(
      document.querySelector('button[aria-label="Line 1 quotation match"]')
    ).toBeNull();
    // The misleading no-choice copy is gone in receipt mode.
    expect(document.body.textContent?.includes("No choice is selected")).toBe(false);
    expect(document.body.textContent).toContain("recorded in the reconciliation pane");
  });

  it("the receipt's recorded choices and acks produce the actual finalize payload and readiness", () => {
    const full = initialPurchaseOrderEditorState(ORDER, USD);
    const state = { ...full, lines: full.lines.slice(0, 1) };
    const decisions = {
      "header:paymentTerms": "po" as const,
      "line:1:unitPrice": "po" as const,
    };
    const evaluation = evaluateReview(
      state,
      REVIEW.Source,
      USD,
      decisions,
      ["3"],
      []
    );
    // Readiness: every difference decided, the omission acknowledged.
    expect(evaluation.blockers).toEqual([]);
    expect(evaluation.canContinue).toBe(true);
    const built = buildFinalizeInput(
      state,
      REVIEW.Source,
      USD,
      "none",
      decisions,
      ["3"],
      [],
      true
    );
    expect(built.input).not.toBeNull();
    expect(built.input?.Decisions).toEqual(
      expect.arrayContaining([
        { Key: "header:paymentTerms", Choice: "po" },
        { Key: "line:1:unitPrice", Choice: "po" },
      ])
    );
    expect(built.input?.AcknowledgedMissing).toEqual(["3"]);
    expect(built.input?.AcknowledgedExtra).toEqual([]);
  });

  it("preserves the legacy choice workbench when no session exists", async () => {    const full = initialPurchaseOrderEditorState(ORDER, USD);
    const state = { ...full, lines: full.lines.slice(0, 1) };
    await act(async () => {
      root.render(
        <PurchaseOrderDifferenceReviewView
          state={state}
          source={REVIEW.Source}
          currency={USD}
          quoteNumber="1502260042"
          documentState="none"
          decisions={{}}
          acknowledgedMissing={[]}
          acknowledgedExtra={[]}
          onDecision={() => undefined}
          onToggleMissing={() => undefined}
          onToggleExtra={() => undefined}
          onMatchChange={() => undefined}
          onBack={() => undefined}
          onContinue={() => undefined}
        />
      );
    });
    // No session: the choice UI remains with no invented receipt.
    expect(
      document.querySelector('button[aria-label="Use PO entry for Payment terms"]')
    ).not.toBeNull();
    expect(
      document.querySelector('input[aria-label="Acknowledge omission of quotation line 3"]')
    ).not.toBeNull();
    expect(
      document.querySelector('button[aria-label="Line 1 quotation match"]')
    ).not.toBeNull();
  });

  it("writes the whole receipt sentence with no legacy tail and receipt match intro", async () => {
    const full = initialPurchaseOrderEditorState(ORDER, USD);
    const state = { ...full, lines: full.lines.slice(0, 1) };
    await act(async () => {
      root.render(
        <PurchaseOrderDifferenceReviewView
          state={state}
          source={REVIEW.Source}
          currency={USD}
          quoteNumber="1502260042"
          documentState="none"
          decisions={{ "header:paymentTerms": "po" }}
          acknowledgedMissing={["3"]}
          acknowledgedExtra={[]}
          onDecision={() => undefined}
          onToggleMissing={() => undefined}
          onToggleExtra={() => undefined}
          onMatchChange={() => undefined}
          onBack={() => undefined}
          onContinue={() => undefined}
          receiptResolvedKeys={["header:paymentTerms"]}
          receiptAckedMissing={["3"]}
          receiptAckedExtra={[]}
          hasSession
          onEditDifference={() => undefined}
          onEditLine={() => undefined}
        />
      );
    });

    // One coherent receipt sentence; the legacy choice tail cannot append.
    expect(document.body.textContent).toContain(
      "Values that still need review appear below without choices; resolve them in the pane."
    );
    expect(document.body.textContent?.includes("value when you continue")).toBe(false);
    expect(document.body.textContent?.includes("Use PO entry keeps what you entered")).toBe(false);
    // The line-match intro matches the read-only receipt, not the editor.
    expect(document.body.textContent).toContain("Matches are recorded in the reconciliation pane");
    expect(document.body.textContent?.includes("Correct any match")).toBe(false);
  });

  it("renders an undecided difference read-only with a route back, never Use buttons", async () => {
    const full = initialPurchaseOrderEditorState(ORDER, USD);
    const state = { ...full, lines: full.lines.slice(0, 1) };
    await act(async () => {
      root.render(
        <PurchaseOrderDifferenceReviewView
          state={state}
          source={REVIEW.Source}
          currency={USD}
          quoteNumber="1502260042"
          documentState="none"
          decisions={{ "header:paymentTerms": "po" }}
          acknowledgedMissing={["3"]}
          acknowledgedExtra={[]}
          onDecision={() => undefined}
          onToggleMissing={() => undefined}
          onToggleExtra={() => undefined}
          onMatchChange={() => undefined}
          onBack={() => undefined}
          onContinue={() => undefined}
          receiptResolvedKeys={["header:paymentTerms"]}
          receiptAckedMissing={["3"]}
          receiptAckedExtra={[]}
          hasSession
          onEditDifference={() => undefined}
          onEditLine={() => undefined}
        />
      );
    });

    // The resolved header stays a receipt row.
    expect(
      document.querySelector('button[aria-label="Use PO entry for Payment terms"]')
    ).toBeNull();
    // The undecided line difference is read-only with a route back.
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 unit price"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 unit price"]')
    ).toBeNull();
    expect(document.body.textContent).toContain("Needs review in the reconciliation pane.");
    expect(
      document.querySelector('button[aria-label="Change Line 1 unit price"]')
    ).not.toBeNull();
  });

  it("a persisted session with no recorded choices still renders every difference read-only (N4/U2)", async () => {
    // N4: the session is authoritative (persisted, ID > 0) but carries no
    // recorded comparison choice or ack — e.g. only link-time tax
    // provenance. The receipt must not fall back to the choice workbench.
    const full = initialPurchaseOrderEditorState(ORDER, USD);
    const state = { ...full, lines: full.lines.slice(0, 1) };
    await act(async () => {
      root.render(
        <PurchaseOrderDifferenceReviewView
          state={state}
          source={REVIEW.Source}
          currency={USD}
          quoteNumber="1502260042"
          documentState="none"
          decisions={{}}
          acknowledgedMissing={[]}
          acknowledgedExtra={[]}
          onDecision={() => undefined}
          onToggleMissing={() => undefined}
          onToggleExtra={() => undefined}
          onMatchChange={() => undefined}
          onBack={() => undefined}
          onContinue={() => undefined}
          receiptResolvedKeys={[]}
          receiptAckedMissing={[]}
          receiptAckedExtra={[]}
          hasSession
          onEditDifference={() => undefined}
          onEditLine={() => undefined}
        />
      );
    });

    // No second workbench anywhere: no Use buttons, no match select, no
    // acknowledgement checkboxes.
    expect(
      document.querySelector('button[aria-label="Use PO entry for Line 1 unit price"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Use Quotation for Line 1 unit price"]')
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Line 1 quotation match"]')
    ).toBeNull();
    expect(
      document.querySelector('input[aria-label="Acknowledge omission of quotation line 3"]')
    ).toBeNull();
    // The unrecorded difference is named with a route back.
    expect(document.body.textContent).toContain("Needs review in the reconciliation pane.");
    expect(
      document.querySelector('button[aria-label="Change Line 1 unit price"]')
    ).not.toBeNull();
    // Receipt copy, not the legacy choice sentence.
    expect(document.body.textContent).toContain("recorded in the reconciliation pane");
    expect(document.body.textContent?.includes("Choose a side for each difference")).toBe(false);
  });
});
