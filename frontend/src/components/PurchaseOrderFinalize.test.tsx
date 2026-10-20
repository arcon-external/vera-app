import type { ReactElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import { evaluateReview, type PurchaseOrderSourceDTO } from "../poFinalize";
import {
  initialPurchaseOrderEditorState,
  buildPurchaseOrderUpdate,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import {
  PurchaseOrderDifferenceReviewView,
  PurchaseOrderFinalConfirmationView,
  PurchaseOrderSavedView,
} from "./PurchaseOrderFinalize";

// Issue #471: the assembled review, confirmation and saved record, rendered
// without a DOM. The pure suite owns matching, choices and totals; this suite
// proves the markup states what the rules require: two choices with none
// preselected, the explicit missing/extra review, the currency statement, the
// return to entry, the no-PDF confirmation, and a saved record with no editable
// control.
//
// What it cannot prove: typing, clicking and the actual recalculation after a
// choice. Those are covered by the DOM suite; focus and keyboard behaviour stay
// hand-driven.

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
    UUID: "uuid-471",
    State: "draft",
    QuoteNumber: "1502260042",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing",
    DeliveryAddress: "Jl. Delivery",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
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
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 20000,
        DiscountMinor: 0,
        NetMinor: 20000,
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
    PaymentTerms: "45 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 24500,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 24500,
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
      DiscountEnabled: true,
      DiscountPercent: 10,
    },
  ],
  PrintedSubtotalMinor: 24500,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 24500,
};

function reviewState(overrides: Partial<PurchaseOrderDTO> = {}) {
  return initialPurchaseOrderEditorState(order(overrides), USD);
}

function reviewElement(
  state = reviewState({ PaymentTerms: "30 days" }),
  decisions: Record<string, "po" | "quotation"> = {},
  documentState: "none" | "attached" = "none"
) {
  return (
    <PurchaseOrderDifferenceReviewView
      state={state}
      source={SOURCE}
      currency={USD}
      quoteNumber="1502260042"
      documentState={documentState}
      decisions={decisions}
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
}

function markup(element: ReactElement): string {
  return renderToString(element).replace(/<!-- -->/g, "");
}

describe("PurchaseOrderDifferenceReviewView (issue #471)", () => {
  it("shows both choices for every difference with none preselected", () => {
    const html = markup(reviewElement());
    expect(html).toContain("Use PO entry for Payment terms");
    expect(html).toContain("Use Quotation for Payment terms");
    expect(html).not.toContain('aria-pressed="true"');
    expect(html).toContain("No choice is selected for you");
  });

  it("marks the chosen side and enables Continue once every value is decided", () => {
    const html = markup(reviewElement(undefined, { "header:paymentTerms": "quotation" }));
    const quotationButton = html.match(
      /<button[^>]*aria-label="Use Quotation for Payment terms"[^>]*>/
    );
    expect(quotationButton?.[0]).toContain('aria-pressed="true"');
    expect(html).toContain('aria-label="Continue final save"');
    expect(html).toContain('aria-label="Back to entry"');
    // The applied choice resolves the only difference, so Continue is enabled.
    expect(html).not.toMatch(/aria-label="Continue final save"[^>]*disabled/);
  });

  it("states the fixed currency basis and that no conversion happens", () => {
    const html = markup(reviewElement());
    expect(html).toContain("kept in USD");
    expect(html).toContain("does not convert between currencies");
  });

  it("shows missing and extra lines as explicit review items", () => {
    const base = order();
    const reversed = {
      ...base,
      Items: [
        { ...base.Items[0], QuotationItemNo: null, QuotationSubNo: null },
        base.Items[1],
        {
          ...base.Items[1],
          LineNumber: 3,
          QuotationItemNo: null,
          QuotationSubNo: null,
          Description: "Freight",
          UnitPriceMinor: 0,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
    };
    const html = markup(reviewElement(reviewState(reversed)));
    expect(html).toContain("Acknowledge omission of quotation line 1");
    expect(html).toContain("Acknowledge line 1 as extra");
    expect(html).toContain("Acknowledge line 3 as extra");
    expect(html).toContain("Line 1 quotation match");
    expect(html).toContain("Line 2 quotation match");
    expect(html).toContain("Line 3 quotation match");
    expect(html).toContain("Currently quotation line 3");
    expect(html).toContain("Quotation match");
  });

  it("keeps Continue disabled while a difference is undecided", () => {
    const html = markup(reviewElement());
    expect(html).toMatch(/aria-label="Continue final save"[^>]*disabled/);
    expect(html).toContain("still differs and needs a choice");
  });

  it("says the no-PDF confirmation follows this review", () => {
    const html = markup(reviewElement());
    expect(html).toContain("No customer PO PDF is attached");
    expect(html).toContain("not a business approval");
  });

  it("shows the attached branch without asking for the no-PDF confirmation", () => {
    const html = markup(reviewElement(undefined, {}, "attached"));
    expect(html).toContain("customer PO PDF is attached and stays with this record");
    expect(html).not.toContain("No customer PO PDF is attached");
  });
});

describe("PurchaseOrderFinalConfirmationView (issue #471)", () => {
  function confirmationElement(
    documentConfirmed: boolean,
    documentState = "none",
    state = reviewState(),
    stored: PurchaseOrderDTO = order()
  ) {
    const evaluation = evaluateReview(state, SOURCE, USD, {}, [], []);
    const payload = buildPurchaseOrderUpdate(evaluation.applied, USD).input!;
    return (
      <PurchaseOrderFinalConfirmationView
        review={{ Order: stored, Source: SOURCE, DocumentState: documentState }}
        evaluation={evaluation}
        payload={payload}
        currency={USD}
        documentConfirmed={documentConfirmed}
        onToggleDocumentConfirmed={() => undefined}
        onBack={() => undefined}
        onFinalSave={() => undefined}
        saving={false}
        error={null}
      />
    );
  }

  it("requires the no-PDF confirmation before Final save is enabled", () => {
    const html = markup(confirmationElement(false));
    expect(html).toContain("Confirm saving without the customer PDF");
    expect(html).toMatch(/aria-label="Final save"[^>]*disabled/);
    expect(html).not.toMatch(/aria-label="Confirm saving without the customer PDF"[^>]*checked/);

    const confirmed = markup(confirmationElement(true));
    expect(confirmed).toMatch(/aria-label="Confirm saving without the customer PDF"[^>]*checked/);
    expect(confirmed).not.toMatch(/aria-label="Final save"[^>]*disabled/);
  });

  it("does not ask for the confirmation in the attached-PDF branch", () => {
    const html = markup(confirmationElement(false, "attached"));
    expect(html).not.toContain("Confirm saving without the customer PDF");
    expect(html).toContain("attached customer PO PDF");
    expect(html).not.toMatch(/aria-label="Final save"[^>]*disabled/);
  });

  it("distinguishes finalized data readiness from approval", () => {
    const html = markup(confirmationElement(true));
    expect(html).toContain("It is not a business approval");
    expect(html).toContain("PO-471");
    expect(html).toContain("PT. Buyer");
  });

  it("confirms the applied editor values, not the stored record", () => {
    const stored = order({ PONumber: "PO-STORED", PaymentTerms: "45 days" });
    const html = markup(
      confirmationElement(
        true,
        "none",
        reviewState({ PONumber: "PO-EDITED", PaymentTerms: "30 days" }),
        stored
      )
    );
    expect(html).toContain("PO-EDITED");
    expect(html).not.toContain("PO-STORED");
    expect(html).toContain("30 days");
    expect(html).toContain("The details below are the values this save will store.");
  });
});

describe("PurchaseOrderSavedView (issue #471)", () => {
  it("shows the saved purchase order with no editable control", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order({ State: "saved", PdfAbsentConfirmed: true })}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
      />
    );
    expect(html).toContain("Saved");
    expect(html).toContain("not a business approval");
    expect(html).toContain("read-only in this release");
    expect(html).toContain("PO-471");
    expect(html).toContain("Widget");
    expect(html).toContain("Quotation 1");
    expect(html).toContain("No customer PO PDF attached");
    // #472 attaches the one customer PO PDF from a saved record too, so the
    // hidden file input is the only input. No PO field became editable.
    expect(html).toContain('aria-label="Customer PO PDF file"');
    expect(html).not.toContain("<textarea");
    expect(html).not.toContain('type="text"');
    expect(html).not.toContain('type="date"');
    expect(html).not.toContain("Save draft");
    expect(html).not.toContain("Final Save");
    expect(html).not.toContain("Add line");
  });

  it("still shows the printed source totals beside the stored calculation", () => {
    const html = markup(
      <PurchaseOrderSavedView order={order({ State: "saved" })} currency={USD} onBack={() => undefined} />
    );
    expect(html).toContain("Printed on quotation revision 1");
    expect(html).toContain("Printed grand total");
  });
});
