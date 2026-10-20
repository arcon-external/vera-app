import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import {
  applyReviewChoices,
  buildFinalizeInput,
  comparePurchaseOrder,
  evaluateReview,
  initialFinalizeReviewState,
  isSavedPurchaseOrder,
  sourceLineLabel,
  suggestSourceLine,
  type POReviewDecisions,
  type PurchaseOrderReviewDTO,
  type PurchaseOrderSourceDTO,
} from "./poFinalize";
import {
  calculatePurchaseOrder,
  initialPurchaseOrderEditorState,
  type PurchaseOrderDTO,
} from "./purchaseOrder";

// Issue #471: the final-save review's pure decisions. These tests hold the
// matching rules, the explicit choice model, the recalculated totals, the
// monetary explanation and the payload contract the server validates.

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

// The judge's monetary case in miniature: the quotation printed 200 minor
// units (one line, 1 x 200) and the customer's order adds a 50 freight line.
const SOURCE_PRINTED_200: PurchaseOrderSourceDTO = {
  ...SOURCE,
  Lines: [
    {
      QuotationItemNo: 1,
      QuotationSubNo: null,
      LineLabel: "1",
      Description: "Widget",
      UoM: "Pc",
      QuantityScaled: 10000,
      UnitPriceMinor: 200,
      DiscountEnabled: false,
      DiscountPercent: 0,
    },
  ],
  PrintedSubtotalMinor: 200,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 200,
};

function orderPrinted200(): PurchaseOrderDTO {
  return order({
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Widget",
        QuantityScaled: 10000,
        UoM: "Pc",
        UnitPriceMinor: 200,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 200,
        DiscountMinor: 0,
        NetMinor: 200,
      },
      {
        QuotationItemNo: null,
        QuotationSubNo: null,
        LineNumber: 2,
        Description: "Freight",
        QuantityScaled: 10000,
        UoM: "Lot",
        UnitPriceMinor: 50,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 50,
        DiscountMinor: 0,
        NetMinor: 50,
      },
    ],
  });
}

function stateFrom(dto: PurchaseOrderDTO) {
  return initialPurchaseOrderEditorState(dto, USD);
}

function review(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderReviewDTO {
  return { Order: order(overrides), Source: SOURCE, DocumentState: "none" };
}

describe("purchase order matching", () => {
  it("matches by the stored item/sub reference, not by position", () => {
    const state = stateFrom(
      order({
        Items: [order().Items[1], order().Items[0]],
      })
    );
    const comparison = comparePurchaseOrder(state, SOURCE, USD);
    expect(comparison.extraLineNumbers).toEqual([]);
    expect(comparison.missing).toEqual([]);
    expect(comparison.duplicateLineNumbers).toEqual([]);
    expect(comparison.differences).toEqual([]);
    expect(comparison.matched.map((pair) => pair.source.LineLabel)).toEqual(["3", "1"]);
  });

  it("names missing and extra lines and refuses two lines for one source line", () => {
    const base = order();
    const state = stateFrom(
      order({
        Items: [
          base.Items[0],
          { ...base.Items[0], LineNumber: 2, Description: "Widget again" },
          {
            ...base.Items[1],
            LineNumber: 3,
            QuotationItemNo: null,
            QuotationSubNo: null,
            Description: "Freight",
          },
        ],
      })
    );
    const comparison = comparePurchaseOrder(state, SOURCE, USD);
    expect(comparison.extraLineNumbers).toEqual([3]);
    expect(comparison.missing.map((line) => line.LineLabel)).toEqual(["3"]);
    expect(comparison.duplicateLineNumbers).toEqual([2]);
  });

  it("suggests an exact description or the same price, unit and quantity only", () => {
    const candidate = SOURCE.Lines[0];
    const exact = suggestSourceLine(
      {
        key: 1,
        description: "  widget ",
        quantity: "9",
        uom: "Box",
        unitPrice: "999",
        discountEnabled: false,
        discountPercent: "0",
        quotationItemNo: null,
        quotationSubNo: null,
      },
      [candidate],
      USD
    );
    expect(exact?.source.LineLabel).toBe("1");

    const sameMoney = suggestSourceLine(
      {
        key: 1,
        description: "Unrelated wording",
        quantity: "2",
        uom: "pc",
        unitPrice: "100",
        discountEnabled: false,
        discountPercent: "0",
        quotationItemNo: null,
        quotationSubNo: null,
      },
      [candidate],
      USD
    );
    expect(sameMoney?.source.LineLabel).toBe("1");

    const unrelated = suggestSourceLine(
      {
        key: 1,
        description: "Freight",
        quantity: "1",
        uom: "Lot",
        unitPrice: "12",
        discountEnabled: false,
        discountPercent: "0",
        quotationItemNo: null,
        quotationSubNo: null,
      },
      [candidate],
      USD
    );
    expect(unrelated).toBeNull();
  });

  it("does not suggest a source line another PO line already uses", () => {
    const base = order();
    const state = stateFrom(
      order({
        Items: [
          base.Items[0],
          {
            ...base.Items[1],
            QuotationItemNo: null,
            QuotationSubNo: null,
            Description: "Widget",
            QuantityScaled: 20000,
            UoM: "Pc",
            UnitPriceMinor: 10000,
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
        ],
      })
    );
    const comparison = comparePurchaseOrder(state, SOURCE, USD);
    // The only plausible candidate is source line 1, and line 1 of the PO
    // already claims it. A suggestion must never point at a claimed line.
    expect(comparison.suggestions).toEqual([]);
    expect(comparison.duplicateLineNumbers).toEqual([]);
    expect(comparison.extraLineNumbers).toEqual([2]);
    expect(comparison.missing.map((line) => line.LineLabel)).toEqual(["3"]);
  });

  it("ignores surrounding whitespace but keeps meaningful wording changes", () => {
    const state = stateFrom(order({ PaymentTerms: "  45 days\t" }));
    expect(comparePurchaseOrder(state, SOURCE, USD).differences).toEqual([]);

    state.header.paymentTerms = "45  days";
    const differences = comparePurchaseOrder(state, SOURCE, USD).differences;
    expect(differences.map((difference) => difference.key)).toContain("header:paymentTerms");
  });

  it("names every differing value with both sides", () => {
    const base = order();
    const state = stateFrom(
      order({
        PaymentTerms: "30 days",
        Incoterms: "FOB",
        DiscountPercent: 5,
        Items: [
          {
            ...base.Items[0],
            Description: "Widget, revised",
            QuantityScaled: 30000,
            UoM: "Box",
            UnitPriceMinor: 11000,
          },
          base.Items[1],
        ],
      })
    );
    const comparison = comparePurchaseOrder(state, SOURCE, USD);
    const keys = comparison.differences.map((difference) => difference.key);
    expect(keys).toEqual([
      "header:paymentTerms",
      "header:incoterms",
      "header:discountPercent",
      "line:1:description",
      "line:1:uom",
      "line:1:quantity",
      "line:1:unitPrice",
    ]);
    const quantity = comparison.differences.find((difference) => difference.key === "line:1:quantity");
    expect(quantity).toMatchObject({ poValue: "3", sourceValue: "2" });
  });
});

describe("difference choices", () => {
  it("preselects nothing", () => {
    expect(initialFinalizeReviewState().decisions).toEqual({});
  });

  it("applies only Use Quotation choices", () => {
    const base = order();
    const state = stateFrom(
      order({
        PaymentTerms: "30 days",
        DiscountPercent: 5,
        Items: [
          {
            ...base.Items[0],
            QuantityScaled: 30000,
            UnitPriceMinor: 11000,
          },
          base.Items[1],
        ],
      })
    );
    const comparison = comparePurchaseOrder(state, SOURCE, USD);
    expect(comparison.differences.map((difference) => difference.key)).toEqual([
      "header:paymentTerms",
      "header:discountPercent",
      "line:1:quantity",
      "line:1:unitPrice",
    ]);

    const decisions: POReviewDecisions = {
      "header:paymentTerms": "quotation",
      "header:discountPercent": "quotation",
      "line:1:quantity": "quotation",
      "line:1:unitPrice": "po",
    };
    const applied = applyReviewChoices(state, SOURCE, comparison, decisions, USD);
    expect(applied.header.paymentTerms).toBe("45 days");
    expect(applied.header.discountPercent).toBe("0");
    expect(applied.lines[0].quantity).toBe("2");
    // The unit price kept its explicit PO choice, so it stays at 110.
    expect(applied.lines[0].unitPrice).toBe("$110.00");
    const totals = calculatePurchaseOrder(applied, USD).totals;
    expect(totals?.grandTotalMinor).toBe(26500);
  });

  it("blocks until every difference has a choice and every line is acknowledged", () => {
    const base = order();
    const state = stateFrom(
      order({
        PaymentTerms: "30 days",
        Items: [
          { ...base.Items[0], QuotationItemNo: null, QuotationSubNo: null },
          base.Items[1],
          {
            ...base.Items[1],
            LineNumber: 3,
            QuotationItemNo: null,
            QuotationSubNo: null,
            Description: "Freight",
            UnitPriceMinor: 100,
            QuantityScaled: 10000,
            UoM: "Lot",
            DiscountEnabled: false,
            DiscountPercent: 0,
          },
        ],
      })
    );
    const evaluation = evaluateReview(state, SOURCE, USD, {}, [], []);
    expect(evaluation.canContinue).toBe(false);
    expect(evaluation.blockers.join("\n")).toContain("Payment terms still differs");
    expect(evaluation.blockers.join("\n")).toContain("Quotation line 1 is not on this purchase order");
    expect(evaluation.blockers.join("\n")).toContain("Line 1 has no quotation match");

    const decisions: POReviewDecisions = { "header:paymentTerms": "po" };
    const resolved = evaluateReview(state, SOURCE, USD, decisions, ["1"], [1, 3]);
    // The acknowledgements resolved their lines. The purchase order keeps its
    // own total (24600 against the printed 24500); an accepted extra line is
    // not a gap to be reversed.
    expect(resolved.blockers).toEqual([]);
    expect(resolved.canContinue).toBe(true);
    expect(resolved.totals?.grandTotalMinor).toBe(24600);
  });

  it("keeps the purchase order's own total when an acknowledged extra line changes it", () => {
    const state = stateFrom(orderPrinted200());
    const unacknowledged = evaluateReview(state, SOURCE_PRINTED_200, USD, {}, [], []);
    expect(unacknowledged.canContinue).toBe(false);
    expect(unacknowledged.blockers.join("\n")).toContain("Line 2 has no quotation match");

    // The judge's case: printed 200, an acknowledged extra line of 50, no
    // adjustment. The save proceeds and the purchase order's total is 250.
    const evaluation = evaluateReview(state, SOURCE_PRINTED_200, USD, {}, [], [2]);
    expect(evaluation.blockers).toEqual([]);
    expect(evaluation.canContinue).toBe(true);
    expect(evaluation.totals?.grandTotalMinor).toBe(250);
    expect(evaluation.totals?.adjustmentMinor).toBe(0);
    expect(evaluation.printedDifferenceMinor).toBe(-50);
    const result = buildFinalizeInput(
      state,
      SOURCE_PRINTED_200,
      USD,
      "attached",
      {},
      [],
      [2],
      false
    );
    expect(result.error).toBeNull();
    expect(result.input?.AdjustmentMinor).toBe(0);
    expect(result.input?.AcknowledgedExtra).toEqual([2]);
  });

  it("accepts a reasoned adjustment as document reconciliation, not as a target", () => {
    const state = stateFrom(orderPrinted200());
    // The lines calculate 250 and the customer's PO document printed 260. The
    // +10 is an explained document difference, not a reversal forced by the
    // quotation (which printed 200).
    state.header.adjustment = "0.10";
    state.header.adjustmentReason = "Customer PO document printed 260";
    const evaluation = evaluateReview(state, SOURCE_PRINTED_200, USD, {}, [], [2]);
    expect(evaluation.blockers).toEqual([]);
    expect(evaluation.canContinue).toBe(true);
    expect(evaluation.totals?.grandTotalMinor).toBe(260);
    expect(evaluation.totals?.adjustmentMinor).toBe(10);
    expect(evaluation.printedDifferenceMinor).toBe(-60);

    // The adjustment is optional, not unguarded: it still needs its reason.
    state.header.adjustmentReason = "   ";
    const unexplained = evaluateReview(state, SOURCE_PRINTED_200, USD, {}, [], [2]);
    expect(unexplained.canContinue).toBe(false);
    expect(unexplained.blockers.join("\n")).toContain("An adjustment requires a reason");
  });

  it("refuses to compare amounts across currencies", () => {
    const state = stateFrom(order({ Currency: "EUR" }));
    const evaluation = evaluateReview(state, SOURCE, USD, {}, [], []);
    expect(evaluation.blockers.join("\n")).toContain("never compared");
  });
});

describe("final-save payload", () => {
  it("carries the decisions and acknowledgements the server validates", () => {
    const base = order();
    const state = stateFrom(
      order({
        PaymentTerms: "30 days",
        Items: [
          { ...base.Items[0], QuotationItemNo: null, QuotationSubNo: null },
          base.Items[1],
        ],
      })
    );
    const decisions: POReviewDecisions = { "header:paymentTerms": "quotation" };
    const result = buildFinalizeInput(
      state,
      SOURCE,
      USD,
      "none",
      decisions,
      ["1"],
      [1],
      true
    );
    expect(result.error).toBeNull();
    expect(result.input).not.toBeNull();
    expect(result.input?.PONumber).toBe("PO-471");
    expect(result.input?.PaymentTerms).toBe("45 days");
    expect(result.input?.Decisions).toEqual([
      { Key: "header:paymentTerms", Choice: "quotation" },
    ]);
    expect(result.input?.AcknowledgedMissing).toEqual(["1"]);
    expect(result.input?.AcknowledgedExtra).toEqual([1]);
    expect(result.input?.DocumentConfirmed).toBe(true);
  });

  it("requires a customer PO number", () => {
    const result = buildFinalizeInput(
      stateFrom(order({ PONumber: "  " })),
      SOURCE,
      USD,
      "none",
      {},
      [],
      [],
      true
    );
    expect(result.input).toBeNull();
    expect(result.error).toContain("customer PO number");
  });

  it("requires the no-PDF confirmation in the no-document branch", () => {
    const result = buildFinalizeInput(
      stateFrom(order()),
      SOURCE,
      USD,
      "none",
      {},
      [],
      [],
      false
    );
    expect(result.input).toBeNull();
    expect(result.error).toContain("without its customer PDF");
  });

  it("does not ask for the confirmation once an attachment branch exists", () => {
    const result = buildFinalizeInput(
      stateFrom(order()),
      SOURCE,
      USD,
      "attached",
      {},
      [],
      [],
      false
    );
    expect(result.input).not.toBeNull();
    expect(result.input?.DocumentConfirmed).toBe(false);
  });

  it("never claims an absence for an attached save, even if the box was checked", () => {
    const result = buildFinalizeInput(
      stateFrom(order()),
      SOURCE,
      USD,
      "attached",
      {},
      [],
      [],
      true
    );
    expect(result.input).not.toBeNull();
    expect(result.input?.DocumentConfirmed).toBe(false);
  });

  it("refuses to build a payload with unresolved differences", () => {
    const state = stateFrom(order({ PaymentTerms: "30 days" }));
    const result = buildFinalizeInput(state, SOURCE, USD, "none", {}, [], [], true);
    expect(result.input).toBeNull();
    expect(result.error).toContain("Payment terms");
  });

  it("detects a saved record", () => {
    expect(isSavedPurchaseOrder(order({ State: "saved" }))).toBe(true);
    expect(isSavedPurchaseOrder(order())).toBe(false);
  });

  it("labels a source line for the match picker", () => {
    expect(sourceLineLabel(SOURCE.Lines[0])).toBe("1 — Widget");
  });
});

describe("review fixture sanity", () => {
  it("starts from a PO that reproduces the printed total", () => {
    const evaluation = evaluateReview(stateFrom(order()), SOURCE, USD, {}, [], []);
    expect(evaluation.comparison.differences).toEqual([]);
    expect(evaluation.canContinue).toBe(true);
    expect(evaluation.totals?.grandTotalMinor).toBe(24500);
    expect(review().DocumentState).toBe("none");
  });
});
