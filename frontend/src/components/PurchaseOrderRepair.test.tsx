import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import {
  evaluateReview,
  type PurchaseOrderReviewDTO,
  type PurchaseOrderSourceDTO,
} from "../poFinalize";
import {
  initialPurchaseOrderEditorState,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import {
  PurchaseOrderDifferenceReviewView,
  PurchaseOrderSavedView,
} from "./PurchaseOrderFinalize";
import {
  PurchaseOrderRepairConfirmationView,
  PurchaseOrderSourceChange,
} from "./PurchaseOrderRepair";

// Issue #475: the source-repair surface, rendered without a DOM. The pure suite
// owns the reason and payload rules; these tests prove the markup states what
// the ticket requires: the action exists only for an admin on a saved record,
// the review repeats the #471 choices against the new source, the reason is
// required before the save can run, and a customer move after assignment is
// shown as refused rather than silently attempted.
//
// What this cannot prove: typing, clicking, the actual binding call, focus and
// keyboard behaviour. The DOM suite covers the first three; the rest stays
// hand-driven.

// renderToString separates adjacent text expressions with comment nodes;
// asserting on a normalized copy reads the sentence React will paint.
function plain(html: string): string {
  return html.replaceAll("<!-- -->", "");
}

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
    UUID: "uuid-475",
    State: "saved",
    QuoteNumber: "1502260001",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing A",
    DeliveryAddress: "Jl. Billing A",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
    PONumber: "PO-475",
    ReceivedDate: "2026-10-02",
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-01T00:00:00Z",
    UpdatedAt: "2026-10-01T00:00:00Z",
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Priced widget",
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
    QuotationID: 1,
    SourceRevision: 0,
    CustomerID: 2,
    PaymentTerms: "45 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 20000,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    SavedAt: "2026-10-02T00:00:00Z",
    SavedBy: 1,
    PdfAbsentConfirmed: true,
    ConfirmationNumber: null,
    ConfirmationAssignedAt: null,
    ConfirmationWarn: false,
    Version: 4,
    ...overrides,
  };
}

function source(overrides: Partial<PurchaseOrderSourceDTO> = {}): PurchaseOrderSourceDTO {
  return {
    QuotationID: 9,
    RevisionNumber: 1,
    QuoteNumber: "1502260009",
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
        Description: "Priced widget",
        UoM: "Pc",
        QuantityScaled: 20000,
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
      },
    ],
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    ...overrides,
  };
}

function review(overrides: {
  order?: Partial<PurchaseOrderDTO>;
  source?: Partial<PurchaseOrderSourceDTO>;
} = {}): PurchaseOrderReviewDTO {
  return {
    Order: order(overrides.order),
    Source: source(overrides.source),
    DocumentState: "none",
  };
}

function repairReviewMarkup(reviewRow: PurchaseOrderReviewDTO): string {
  const state = initialPurchaseOrderEditorState(reviewRow.Order, USD);
  return renderToString(
    <PurchaseOrderDifferenceReviewView
      state={state}
      source={reviewRow.Source}
      currency={USD}
      quoteNumber={reviewRow.Source.QuoteNumber}
      mode="repair"
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
}

function confirmationMarkup(
  reviewRow: PurchaseOrderReviewDTO,
  reason: string,
  decisions: Record<string, "po" | "quotation"> = {}
): string {
  const state = initialPurchaseOrderEditorState(reviewRow.Order, USD);
  const evaluation = evaluateReview(
    state,
    reviewRow.Source,
    USD,
    decisions,
    [],
    []
  );
  return renderToString(
    <PurchaseOrderRepairConfirmationView
      review={reviewRow}
      evaluation={evaluation}
      currency={USD}
      reason={reason}
      onReasonChange={() => undefined}
      onBack={() => undefined}
      onSave={() => undefined}
      saving={false}
      error={null}
    />
  );
}

describe("PurchaseOrderSavedView source repair action (issue #475)", () => {
  it("shows the admin action on a saved record", () => {
    const html = renderToString(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onCorrect={() => undefined}
        canRepairSource
        onRepairSource={() => undefined}
        onBack={() => undefined}
      />
    );
    expect(html).toContain("Repair source link");
  });

  it("renders no action for a staff member", () => {
    const html = renderToString(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onCorrect={() => undefined}
        canRepairSource={false}
        onRepairSource={() => undefined}
        onBack={() => undefined}
      />
    );
    expect(html).not.toContain("Repair source link");
  });

  it("renders no action without a handler, even when told the user is an admin", () => {
    const html = renderToString(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        canRepairSource
        onBack={() => undefined}
      />
    );
    expect(html).not.toContain("Repair source link");
  });
});

describe("PurchaseOrderSourceChange (issue #475)", () => {
  it("names the old and new link and a retained confirmation number", () => {
    const html = renderToString(
      <PurchaseOrderSourceChange
        review={review({ order: { ConfirmationNumber: "2302260001" } })}
        confirmationNumber="2302260001"
      />
    );
    expect(plain(html)).toContain("Quotation 1502260001 · revision 0");
    expect(plain(html)).toContain("Quotation 1502260009 · revision 1");
    expect(html).toContain("2302260001 — kept unchanged by a same-customer repair.");
  });

  it("warns when the record would move to another customer", () => {
    const html = renderToString(
      <PurchaseOrderSourceChange
        review={review({
          order: { CustomerID: 2, ConfirmationNumber: null },
          source: { CustomerID: 4, CustomerName: "PT. Other" },
        })}
        confirmationNumber={null}
      />
    );
    expect(html).toContain("Customer change");
    expect(html).toContain("PT. Buyer");
    expect(html).toContain("PT. Other");
  });

  it("states the refusal when a number is already assigned", () => {
    const html = renderToString(
      <PurchaseOrderSourceChange
        review={review({
          order: { CustomerID: 2, ConfirmationNumber: "2302260001" },
          source: { CustomerID: 4, CustomerName: "PT. Other" },
        })}
        confirmationNumber="2302260001"
      />
    );
    expect(html).toContain("Moving it to another customer is refused");
  });
});

describe("PurchaseOrderDifferenceReviewView in repair mode (issue #475)", () => {
  it("repeats the difference choices against the new source", () => {
    const html = repairReviewMarkup(review({ source: { PaymentTerms: "30 days" } }));
    expect(html).toContain("Review differences against the new source");
    expect(html).toContain("Source repair");
    expect(html).toContain("Payment terms");
    expect(html).toContain("Use PO entry for Payment terms");
    expect(html).toContain("Use Quotation for Payment terms");
    // No automatic choice is pressed for the admin.
    expect(html).not.toContain('aria-pressed="true"');
    expect(html).toContain("Continue to repair");
    expect(html).toContain("Back without repairing");
    // The no-PDF finalization confirmation belongs to a final save, not here.
    expect(html).not.toContain("The final confirmation after this review");
  });
});

describe("PurchaseOrderRepairConfirmationView (issue #475)", () => {
  it("requires a reason before the repair can run", () => {
    const html = confirmationMarkup(
      review({ source: { PaymentTerms: "30 days" } }),
      "",
      { "header:paymentTerms": "po" }
    );
    expect(html).toContain("Reason for repairing the source link");
    expect(html).toContain("A source repair needs a reason.");
    expect(html).toContain("Repair source link");
    // The Save button is disabled: React prints the attribute.
    const saveButton = html
      .split("<button")
      .find((fragment) => fragment.includes("Repair source link"));
    expect(saveButton).toContain('disabled=""');
  });

  it("enables the save once the reason is typed", () => {
    const html = confirmationMarkup(
      review({ source: { PaymentTerms: "30 days" } }),
      "the order followed the wrong revision",
      { "header:paymentTerms": "po" }
    );
    const saveButton = html
      .split("<button")
      .find((fragment) => fragment.includes("Repair source link"));
    expect(saveButton).not.toContain('disabled=""');
    expect(html).toContain("Values this repair stores");
    expect(html).toContain("Kept PO entry");
  });

  it("disables the save when the customer would change after assignment", () => {
    const html = confirmationMarkup(
      review({
        order: { CustomerID: 2, ConfirmationNumber: "2302260001" },
        source: { CustomerID: 4, CustomerName: "PT. Other" },
      }),
      "move to the other customer"
    );
    const saveButton = html
      .split("<button")
      .find((fragment) => fragment.includes("Repair source link"));
    expect(saveButton).toContain('disabled=""');
    expect(html).toContain("Moving it to another customer is refused");
  });
});
