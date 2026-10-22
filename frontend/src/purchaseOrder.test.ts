import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import {
  blankPurchaseOrderDraft,
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  canAssignConfirmationNumber,
  confirmationNumberLabel,
  confirmationNumberOf,
  formatQuantityScaled,
  formatUnitPriceMinor,
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  MAX_PAYMENT_TERMS_LENGTH,
  parseAdjustmentMinor,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
  printedTotalsKnownOf,
  purchaseOrderEditorReducer,
  purchaseOrderPpnBps,
  sourceStateOf,
  MAX_ADJUSTMENT_REASON_LENGTH,
  MAX_EXACT_MINOR,
  validDateField,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorState,
} from "./purchaseOrder";

// Issue #468: the draft editor's pure decisions. The suite drives the exact
// values the database stores (4-decimal quantity, currency minor units), the
// dirty-cancel guard, and the replace-lines payload.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

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
    PODate: null,
    RequestedDeliveryDate: null,
    UUID: "uuid-1",
    State: "draft",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing",
    DeliveryAddress: "Jl. Delivery",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "+62 21 555",
    PONumber: "PO-1",
    ReceivedDate: "2026-10-20",
    Version: 1,
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "NonFTZ",
    TaxCategory: "SupplyInstall",
    CreatedAt: "2026-10-20T10:00:00Z",
    UpdatedAt: "2026-10-20T10:00:00Z",
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Widget",
        QuantityScaled: 25000,
        UoM: "Pc",
        UnitPriceMinor: 125050,
        DiscountEnabled: true,
        DiscountPercent: 10,
        GrossMinor: 312625,
        DiscountMinor: 31262,
        NetMinor: 281363,
      },
      {
        QuotationItemNo: 2,
        QuotationSubNo: 1,
        LineNumber: 2,
        Description: "Mounting",
        QuantityScaled: 10000,
        UoM: "Unit",
        UnitPriceMinor: 42000,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 42000,
        DiscountMinor: 0,
        NetMinor: 42000,
      },
    ],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTerms: "45 days",
    DiscountPercent: 5,
    PpnBps: 1100,
    SubtotalMinor: 323363,
    DiscountMinor: 16168,
    PpnMinor: 33791,
    AdjustmentMinor: -250,
    AdjustmentReason: "Printed total rounding",
    GrandTotalMinor: 340736,
    PrintedSubtotalMinor: 323363,
    PrintedDiscountMinor: 16168,
    PrintedPpnMinor: 33791,
    PrintedGrandTotalMinor: 340736,
    ...overrides,
  };
}

function initial(overrides: Partial<PurchaseOrderDTO> = {}) {
  return initialPurchaseOrderEditorState(order(overrides), USD);
}

// The shared contract fixtures. backend/store/po_contract_test.go and
// backend/api/purchase_orders_test.go read the same files, so an order accepted
// by the API cannot be silently refused here, and the whitespace set cannot
// drift between the editor, the store and the schema.
interface SharedCalculationCase {
  name: string;
  why: string;
  lines: Array<{
    quantityScaled: number;
    unitPriceMinor: number;
    discountEnabled: boolean;
    discountPercent: number;
  }>;
  discountPercent: number;
  ppnBps: number;
  adjustmentMinor: number;
  adjustmentReason: string;
  want: Record<string, number>;
}

const calculationFixture = JSON.parse(
  readFileSync(
    fileURLToPath(
      new URL(
        "../../backend/testdata/po_calculation_cases.json",
        import.meta.url
      )
    ),
    "utf8"
  )
) as { cases: SharedCalculationCase[] };

interface SharedCodePoint {
  codePoint: number;
  name: string;
  why?: string;
}

const whitespaceFixture = JSON.parse(
  readFileSync(
    fileURLToPath(
      new URL(
        "../../backend/testdata/po_adjustment_whitespace.json",
        import.meta.url
      )
    ),
    "utf8"
  )
) as { blank: SharedCodePoint[]; content: SharedCodePoint[] };

describe("PO quantity parsing (issue #468)", () => {
  it("parses up to four decimal places into the exact 10^4 scale", () => {
    expect(parseQuantityScaled("1")).toBe(10000);
    expect(parseQuantityScaled("2.5")).toBe(25000);
    expect(parseQuantityScaled("2,5")).toBe(25000);
    expect(parseQuantityScaled("0.0001")).toBe(1);
    expect(parseQuantityScaled("12.3456")).toBe(123456);
  });

  it("refuses zero, negatives, more than four places and junk", () => {
    for (const raw of [
      "0",
      "0.0000",
      "-1",
      "1.23456",
      "abc",
      "",
      "1.2.3",
      "1,2,3",
    ]) {
      expect(parseQuantityScaled(raw), raw).toBeNull();
    }
  });

  it("treats redundant decimal formatting as the same number", () => {
    // Comparison and storage are numeric: "1.0" and "1.00" are one unit, not
    // two different quantities.
    expect(parseQuantityScaled("1.0")).toBe(parseQuantityScaled("1.00"));
    expect(parseQuantityScaled("2.500")).toBe(parseQuantityScaled("2.5"));
  });

  it("refuses a scaled quantity beyond the exact transport range", () => {
    // 2^53-1 is the same ceiling the Go store and the schema enforce; beyond
    // it a number cannot name every minor unit.
    expect(MAX_EXACT_MINOR).toBe(Number.MAX_SAFE_INTEGER);
    expect(parseQuantityScaled("900719925474.0991")).toBe(MAX_EXACT_MINOR);
    expect(parseQuantityScaled("900719925474.0992")).toBeNull();
    expect(parseQuantityScaled("900719925475")).toBeNull();
  });

  it("formats the stored scale back to the shortest decimal", () => {
    expect(formatQuantityScaled(10000)).toBe("1");
    expect(formatQuantityScaled(25000)).toBe("2.5");
    expect(formatQuantityScaled(123456)).toBe("12.3456");
    expect(formatQuantityScaled(1)).toBe("0.0001");
  });
});

describe("PO unit-price parsing (issue #468)", () => {
  it("reads money in the PO currency's own convention", () => {
    expect(parseUnitPriceMinor(USD, "$1,250.50")).toBe(125050);
    expect(parseUnitPriceMinor(USD, "1250.50")).toBe(125050);
    expect(parseUnitPriceMinor(IDR, "Rp125.000")).toBe(125000);
    expect(parseUnitPriceMinor(IDR, "0")).toBe(0);
  });

  it("refuses text that cannot be money, without turning it into zero", () => {
    expect(parseUnitPriceMinor(USD, "abc")).toBeNull();
    expect(parseUnitPriceMinor(USD, "")).toBeNull();
    expect(parseUnitPriceMinor(USD, "-5.00")).toBeNull();
    expect(parseUnitPriceMinor(USD, "1.234")).toBeNull();
  });

  it("refuses a unit price or adjustment beyond the exact transport range", () => {
    expect(parseUnitPriceMinor(USD, "90,071,992,547,409.91")).toBe(
      MAX_EXACT_MINOR
    );
    expect(parseUnitPriceMinor(USD, "90,071,992,547,409.92")).toBeNull();
    expect(parseAdjustmentMinor(USD, "-90,071,992,547,409.91")).toBe(
      -MAX_EXACT_MINOR
    );
    expect(parseAdjustmentMinor(USD, "-90,071,992,547,409.92")).toBeNull();
    expect(parseAdjustmentMinor(USD, "90,071,992,547,409.92")).toBeNull();
  });

  it("treats redundant decimal formatting as the same amount", () => {
    expect(parseUnitPriceMinor(USD, "1.0")).toBe(parseUnitPriceMinor(USD, "1.00"));
    expect(parseAdjustmentMinor(USD, "-1.0")).toBe(
      parseAdjustmentMinor(USD, "-1.00")
    );
  });

  it("formats a stored amount back into its money input", () => {
    expect(formatUnitPriceMinor(USD, 125050)).toBe("$1,250.50");
    expect(formatUnitPriceMinor(IDR, 125000)).toBe("Rp125.000");
  });
});

describe("PO editor dirty-cancel guard (issue #468)", () => {
  it("starts clean and stays clean when a change is reverted", () => {
    const state = initial();
    expect(isPurchaseOrderDirty(state)).toBe(false);

    const changed = purchaseOrderEditorReducer(state, {
      type: "headerChanged",
      patch: { billingAddress: "Jl. Edited" },
    });
    expect(isPurchaseOrderDirty(changed)).toBe(true);

    const reverted = purchaseOrderEditorReducer(changed, {
      type: "headerChanged",
      patch: { billingAddress: "Jl. Billing" },
    });
    expect(isPurchaseOrderDirty(reverted)).toBe(false);
  });

  it("treats a line edit, an added line and a removed line as dirty", () => {
    const state = initial();
    const lineEdit = purchaseOrderEditorReducer(state, {
      type: "lineChanged",
      key: 0,
      patch: { quantity: "3" },
    });
    expect(isPurchaseOrderDirty(lineEdit)).toBe(true);

    const added = purchaseOrderEditorReducer(state, {
      type: "lineAdded",
      uom: "Lot",
    });
    expect(isPurchaseOrderDirty(added)).toBe(true);
    expect(added.lines[2].quotationItemNo).toBeNull();
    expect(added.lines[2].unitPrice).toBe("0");

    const removed = purchaseOrderEditorReducer(state, {
      type: "lineRemoved",
      key: 1,
    });
    expect(isPurchaseOrderDirty(removed)).toBe(true);
  });

  it("reports dirtiness for the shared leave guard", () => {
    const clean = initial();
    expect(isPurchaseOrderDirty(clean)).toBe(false);

    const dirty = purchaseOrderEditorReducer(clean, {
      type: "lineChanged",
      key: 0,
      patch: { description: "Edited" },
    });
    expect(isPurchaseOrderDirty(dirty)).toBe(true);
  });

  it("adopts a saved-record transition's version and applied review values", () => {
    const state = initial();
    const adopted = purchaseOrderEditorReducer(state, {
      type: "versionAdopted",
      version: 9,
    });
    expect(adopted.version).toBe(9);
    // The version is not part of the working copy, so it alone is not dirty.
    expect(isPurchaseOrderDirty(adopted)).toBe(false);

    const replaced = purchaseOrderEditorReducer(adopted, {
      type: "valuesReplaced",
      header: { ...state.header, paymentTerms: "From the quotation" },
      lines: state.lines,
    });
    expect(replaced.version).toBe(9);
    expect(replaced.header.paymentTerms).toBe("From the quotation");
    expect(isPurchaseOrderDirty(replaced)).toBe(true);
  });

  it("resets the baseline after a successful save", () => {
    const dirty = purchaseOrderEditorReducer(initial(), {
      type: "lineChanged",
      key: 0,
      patch: { description: "Saved description" },
    });
    const saved = purchaseOrderEditorReducer(dirty, {
      type: "saveSucceeded",
      order: order({
        Items: [
          {
            QuotationItemNo: 1,
            QuotationSubNo: null,
            LineNumber: 1,
            Description: "Saved description",
            QuantityScaled: 25000,
            UoM: "Pc",
            UnitPriceMinor: 125050,
            DiscountEnabled: false,
            DiscountPercent: 0,
            GrossMinor: 312625,
            DiscountMinor: 0,
            NetMinor: 312625,
          },
        ],
      }),
      currency: USD,
    });
    expect(saved.saving).toBe(false);
    expect(isPurchaseOrderDirty(saved)).toBe(false);
    expect(saved.lines).toHaveLength(1);
    expect(saved.lines[0].description).toBe("Saved description");
  });
});

describe("PO draft update payload (issue #468)", () => {
  it("builds the full replace-lines payload with exact values and references", () => {
    const state = initial();
    const result = buildPurchaseOrderUpdate(state, USD);
    expect(result.error).toBeNull();
    const input = result.input!;
    expect(input.ID).toBe(7);
    expect(input.BillingAddress).toBe("Jl. Billing");
    expect(input.PONumber).toBe("PO-1");
    expect(input.ReceivedDate).toBe("2026-10-20");
    expect(input.PaymentTerms).toBe("45 days");
    expect(input.Items).toHaveLength(2);
    expect(input.Items[0]).toEqual({
      QuotationItemNo: 1,
      QuotationSubNo: null,
      Description: "Widget",
      QuantityScaled: 25000,
      UoM: "Pc",
      UnitPriceMinor: 125050,
      DiscountEnabled: true,
      DiscountPercent: 10,
    });
    expect(input.Items[1].QuotationSubNo).toBe(1);
  });

  it("names the failing line instead of storing an unparseable value", () => {
    const badQuantity = purchaseOrderEditorReducer(initial(), {
      type: "lineChanged",
      key: 1,
      patch: { quantity: "1.23456" },
    });
    expect(buildPurchaseOrderUpdate(badQuantity, USD)).toEqual({
      input: null,
      error:
        "Line 2: quantity must be positive with up to 4 decimal places and within the supported range.",
    });

    const badPrice = purchaseOrderEditorReducer(initial(), {
      type: "lineChanged",
      key: 0,
      patch: { unitPrice: "not money" },
    });
    expect(buildPurchaseOrderUpdate(badPrice, USD).error).toBe(
      "Line 1: unit price must be a nonnegative USD amount within the supported range.",
    );
  });

  it("requires a real received date and validates the optional dates", () => {
    const missing = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { receivedDate: "" },
    });
    expect(buildPurchaseOrderUpdate(missing, USD).error).toContain(
      "Received date",
    );

    const badPO = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { poDate: "2026-02-31" },
    });
    expect(buildPurchaseOrderUpdate(badPO, USD).error).toContain("PO date");

    const badRequested = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { requestedDeliveryDate: "next week" },
    });
    expect(buildPurchaseOrderUpdate(badRequested, USD).error).toContain(
      "Requested delivery date",
    );
  });

  it("carries commercial payment wording, not a day count", () => {
    const wording = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { paymentTerms: "50% advance" },
    });
    expect(buildPurchaseOrderUpdate(wording, USD).input?.PaymentTerms).toBe(
      "50% advance",
    );

    const second = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { paymentTerms: "30 days after delivery" },
    });
    expect(buildPurchaseOrderUpdate(second, USD).input?.PaymentTerms).toBe(
      "30 days after delivery",
    );

    // A draft may leave the terms unagreed; only the length is bounded.
    const cleared = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { paymentTerms: "   " },
    });
    expect(buildPurchaseOrderUpdate(cleared, USD).input?.PaymentTerms).toBe("");

    const overlong = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { paymentTerms: "x".repeat(MAX_PAYMENT_TERMS_LENGTH + 1) },
    });
    expect(buildPurchaseOrderUpdate(overlong, USD)).toEqual({
      input: null,
      error: `Payment terms must be ${MAX_PAYMENT_TERMS_LENGTH} characters or fewer.`,
    });
  });

  it("bounds payment terms by Unicode characters, not UTF-16 units", () => {
    const withTerms = (terms: string) =>
      buildPurchaseOrderUpdate(
        purchaseOrderEditorReducer(initial(), {
          type: "headerChanged",
          patch: { paymentTerms: terms },
        }),
        USD,
      );
    const overLimit = {
      input: null,
      error: `Payment terms must be ${MAX_PAYMENT_TERMS_LENGTH} characters or fewer.`,
    };

    // BMP multi-byte text: 500 characters, 1500 UTF-8 bytes. The schema's
    // char_length counts these as 500 and the store must agree.
    const ideographs = "界".repeat(MAX_PAYMENT_TERMS_LENGTH);
    expect(Array.from(ideographs)).toHaveLength(MAX_PAYMENT_TERMS_LENGTH);
    expect(withTerms(ideographs).input?.PaymentTerms).toBe(ideographs);
    expect(withTerms(ideographs + "界")).toEqual(overLimit);

    // Astral text: each emoji is one character but two UTF-16 units, so the
    // old `.length` guard refused 500 of them (1000 UTF-16 units) as over.
    const emoji = "😀".repeat(MAX_PAYMENT_TERMS_LENGTH);
    expect(Array.from(emoji)).toHaveLength(MAX_PAYMENT_TERMS_LENGTH);
    expect(withTerms(emoji).input?.PaymentTerms).toBe(emoji);
    expect(withTerms(emoji + "😀")).toEqual(overLimit);

    // Mixed text at exactly 500 characters.
    const mixed = "PO 界 #".repeat(83) + "AB";
    expect(Array.from(mixed)).toHaveLength(MAX_PAYMENT_TERMS_LENGTH);
    expect(withTerms(mixed).input?.PaymentTerms).toBe(mixed);
    expect(withTerms(mixed + "!")).toEqual(overLimit);
  });

  it("validates calendar dates", () => {
    expect(validDateField("2026-02-28")).toBe(true);
    expect(validDateField("2026-02-29")).toBe(false);
    expect(validDateField("")).toBe(true);
  });
});

// Issue #470: the exact calculation, mirrored from the Go pipeline. Expectations
// are worked literals, never a second call of the same formula.

describe("PO discount and adjustment parsing (issue #470)", () => {
  it("accepts whole percentages from 0 to 100 and nothing else", () => {
    expect(parseDiscountPercent("0")).toBe(0);
    expect(parseDiscountPercent("10")).toBe(10);
    expect(parseDiscountPercent("100")).toBe(100);
    expect(parseDiscountPercent("")).toBe(0);
    expect(parseDiscountPercent(" 25 ")).toBe(25);
  });

  it("refuses decimals, signs and out-of-range percentages", () => {
    for (const raw of ["10.5", "-5", "101", "abc", "1e2", "1,5"]) {
      expect(parseDiscountPercent(raw), raw).toBeNull();
    }
  });

  it("reads the signed adjustment in the PO currency and keeps empty as none", () => {
    expect(parseAdjustmentMinor(USD, "")).toBe(0);
    expect(parseAdjustmentMinor(USD, "0.00")).toBe(0);
    expect(parseAdjustmentMinor(USD, "-2.50")).toBe(-250);
    expect(parseAdjustmentMinor(USD, "$-2.50")).toBe(-250);
    expect(parseAdjustmentMinor(USD, "2.50")).toBe(250);
    expect(parseAdjustmentMinor(IDR, "-1.500")).toBe(-1500);
    expect(parseAdjustmentMinor(USD, "abc")).toBeNull();
  });
});

describe("PO explained calculation (issue #470)", () => {
  function stateWith(
    lines: Array<{
      quantity: string;
      unitPrice: string;
      discountEnabled?: boolean;
      discountPercent?: string;
    }>,
    header: Partial<PurchaseOrderEditorState["header"]> = {},
    ppnBps = 0,
  ): PurchaseOrderEditorState {
    const base = initial();
    return {
      ...base,
      ppnBps,
      header: {
        ...base.header,
        discountPercent: "0",
        adjustment: "",
        adjustmentReason: "",
        ...header,
      },
      lines: lines.map((line, index) => ({
        key: index,
        description: `Line ${index + 1}`,
        quantity: line.quantity,
        uom: "Pc",
        unitPrice: line.unitPrice,
        discountEnabled: line.discountEnabled ?? false,
        discountPercent: line.discountPercent ?? "0",
        quotationItemNo: null,
        quotationSubNo: null,
      })),
    };
  }

  it("rounds a fractional line half away from zero and truncates discounts", () => {
    const calculation = calculatePurchaseOrder(
      stateWith([
        { quantity: "0.5", unitPrice: "0.01" },
        { quantity: "1666.9999", unitPrice: "0" },
      ]),
      USD,
    );
    expect(calculation.lines[0]).toEqual({
      grossMinor: 1,
      discountMinor: 0,
      netMinor: 1,
    });
    expect(calculation.lines[1]).toEqual({
      grossMinor: 0,
      discountMinor: 0,
      netMinor: 0,
    });

    // 2.5 x 650 = 1625; 10% is 162.5 and truncates to 162.
    const discounted = calculatePurchaseOrder(
      stateWith(
        [
          {
            quantity: "2.5",
            unitPrice: "6.50",
            discountEnabled: true,
            discountPercent: "10",
          },
          { quantity: "2", unitPrice: "4.20" },
          {
            quantity: "1",
            unitPrice: "4.50",
            discountEnabled: true,
            discountPercent: "50",
          },
        ],
        { discountPercent: "10" },
      ),
      USD,
    );
    expect(discounted.lines).toEqual([
      { grossMinor: 1625, discountMinor: 162, netMinor: 1463 },
      { grossMinor: 840, discountMinor: 0, netMinor: 840 },
      { grossMinor: 450, discountMinor: 225, netMinor: 225 },
    ]);
    // Subtotal 2528; 10% whole discount is 252.8 -> 252; no PPN.
    expect(discounted.totals).toMatchObject({
      grossMinor: 2915,
      rowDiscountMinor: 387,
      subtotalMinor: 2528,
      discountMinor: 252,
      afterDiscountMinor: 2276,
      ppnMinor: 0,
      adjustmentMinor: 0,
      grandTotalMinor: 2276,
    });
    expect(discounted.error).toBeNull();
  });

  it("takes tax on the discounted subtotal and adds it separately", () => {
    const calculation = calculatePurchaseOrder(
      stateWith(
        [{ quantity: "1", unitPrice: "10.00" }],
        { discountPercent: "10" },
        1100,
      ),
      USD,
    );
    expect(calculation.totals).toMatchObject({
      subtotalMinor: 1000,
      discountMinor: 100,
      afterDiscountMinor: 900,
      ppnMinor: 99,
      grandTotalMinor: 999,
    });
  });

  it("adds the signed adjustment last, without touching rows or tax", () => {
    const calculation = calculatePurchaseOrder(
      stateWith(
        [{ quantity: "1", unitPrice: "10.00" }],
        {
          adjustment: "-0.10",
          adjustmentReason: "Printed total",
        },
        1100,
      ),
      USD,
    );
    expect(calculation.totals).not.toBeNull();
    expect(calculation.totals!.ppnMinor).toBe(110);
    expect(calculation.totals!.adjustmentMinor).toBe(-10);
    expect(calculation.totals!.grandTotalMinor).toBe(1100);
    expect(calculation.lines[0]).toEqual({
      grossMinor: 1000,
      discountMinor: 0,
      netMinor: 1000,
    });

    const positive = calculatePurchaseOrder(
      stateWith(
        [{ quantity: "1", unitPrice: "10.00" }],
        {
          adjustment: "0.25",
          adjustmentReason: "Freight",
        },
        1100,
      ),
      USD,
    );
    expect(positive.totals).not.toBeNull();
    expect(positive.totals!.grandTotalMinor).toBe(1135);
  });

  it("marks an unexplained adjustment and a negative total as errors", () => {
    const unexplained = calculatePurchaseOrder(
      stateWith([{ quantity: "1", unitPrice: "10.00" }], { adjustment: "-0.10" }),
      USD,
    );
    expect(unexplained.error).toContain("reason");

    const negative = calculatePurchaseOrder(
      stateWith(
        [{ quantity: "1", unitPrice: "1.00" }],
        {
          adjustment: "-1.12",
          adjustmentReason: "Too far",
        },
        1100,
      ),
      USD,
    );
    expect(negative.error).toContain("negative");
  });

  it("publishes no totals while a line cannot be parsed", () => {
    const calculation = calculatePurchaseOrder(
      stateWith([
        { quantity: "2", unitPrice: "1.00" },
        { quantity: "1.23456", unitPrice: "1.00" },
      ]),
      USD,
    );
    expect(calculation.lines[0]).toEqual({
      grossMinor: 200,
      discountMinor: 0,
      netMinor: 200,
    });
    expect(calculation.lines[1]).toBeNull();
    // A subtotal of only the parsable lines would understate the order, so no
    // totals are published at all until the bad line is fixed.
    expect(calculation.totals).toBeNull();
    expect(calculation.error).toBe(
      "Line 2: quantity must be positive with up to 4 decimal places and within the supported range.",
    );
  });

  it("refuses the judge's counterexample instead of publishing an inexact total", () => {
    // 3 x 9_007_199_254_740_991 is 27_021_597_764_222_973 in Go; a JS number
    // reads the same figure back one minor unit lower, so it must be refused.
    const calculation = calculatePurchaseOrder(
      stateWith([{ quantity: "3", unitPrice: "90,071,992,547,409.91" }]),
      USD,
    );
    expect(calculation.lines[0]).toBeNull();
    expect(calculation.totals).toBeNull();
    expect(calculation.error).toBe(
      "Line 1: the line total exceeds the supported amount range.",
    );
  });

  it("accepts an amount at the exact bound and refuses one above it", () => {
    const atBound = calculatePurchaseOrder(
      stateWith([{ quantity: "1", unitPrice: "90,071,992,547,409.91" }]),
      USD,
    );
    expect(atBound.error).toBeNull();
    expect(atBound.totals?.grandTotalMinor).toBe(MAX_EXACT_MINOR);

    const above = calculatePurchaseOrder(
      stateWith([{ quantity: "1", unitPrice: "90,071,992,547,409.92" }]),
      USD,
    );
    expect(above.error).toContain("range");
    expect(above.totals).toBeNull();
  });

  it("refuses amounts that would overflow the supported money range", () => {
    const calculation = calculatePurchaseOrder(
      stateWith([{ quantity: "900000000000", unitPrice: "900000000000" }]),
      USD,
    );
    expect(calculation.error).toContain("range");
    expect(calculation.totals).toBeNull();
  });

  it("publishes no totals while a header field cannot be parsed", () => {
    const badAdjustment = calculatePurchaseOrder(
      stateWith([{ quantity: "1", unitPrice: "10.00" }], {
        adjustment: "not money",
      }),
      USD,
    );
    expect(badAdjustment.totals).toBeNull();
    expect(badAdjustment.error).toBe("Adjustment must be a signed USD amount.");

    const badDiscount = calculatePurchaseOrder(
      stateWith([{ quantity: "1", unitPrice: "10.00" }], {
        discountPercent: "10.5",
      }),
      USD,
    );
    expect(badDiscount.totals).toBeNull();
    expect(badDiscount.error).toBe("Whole-PO discount must be between 0 and 100.");
  });
});

describe("PO calculation agreement with the backend (issue #470 wave 3)", () => {
  // The fixture's `want` values are the ones backend/api's shared test asserts
  // against the real GET response. Handing them to the editor as a DTO is the
  // reopen path the Judge used: if the API accepts an order, the editor must
  // calculate it and build the save payload for it.
  function fixtureOrder(testCase: SharedCalculationCase): PurchaseOrderDTO {
    return order({
      DiscountPercent: testCase.discountPercent,
      PpnBps: testCase.ppnBps,
      AdjustmentMinor: testCase.adjustmentMinor,
      AdjustmentReason: testCase.adjustmentReason,
      Items: testCase.lines.map((line, index) => ({
        QuotationItemNo: null,
        QuotationSubNo: null,
        LineNumber: index + 1,
        Description: `Line ${index + 1}`,
        QuantityScaled: line.quantityScaled,
        UoM: "Pc",
        UnitPriceMinor: line.unitPriceMinor,
        DiscountEnabled: line.discountEnabled,
        DiscountPercent: line.discountPercent,
        GrossMinor: testCase.want.lineGrossMinor,
        DiscountMinor: testCase.want.lineDiscountMinor,
        NetMinor: testCase.want.lineNetMinor,
      })),
      SubtotalMinor: testCase.want.subtotalMinor,
      DiscountMinor: testCase.want.discountMinor,
      PpnMinor: testCase.want.ppnMinor,
      GrandTotalMinor: testCase.want.grandTotalMinor,
    });
  }

  function fixtureState(testCase: SharedCalculationCase): PurchaseOrderEditorState {
    return initialPurchaseOrderEditorState(fixtureOrder(testCase), USD);
  }

  it("publishes totals and builds the save payload for an order the API accepts", () => {
    expect(calculationFixture.cases.length).toBeGreaterThan(0);
    for (const testCase of calculationFixture.cases) {
      const state = fixtureState(testCase);
      const calculation = calculatePurchaseOrder(state, USD);
      expect(calculation.error, testCase.why).toBeNull();
      expect(calculation.lines, testCase.why).toHaveLength(testCase.lines.length);
      for (const line of calculation.lines) {
        expect(line, testCase.why).not.toBeNull();
        expect(line!.grossMinor).toBe(testCase.want.lineGrossMinor);
        expect(line!.discountMinor).toBe(testCase.want.lineDiscountMinor);
        expect(line!.netMinor).toBe(testCase.want.lineNetMinor);
      }
      expect(calculation.totals, testCase.why).not.toBeNull();
      expect(calculation.totals!.subtotalMinor).toBe(testCase.want.subtotalMinor);
      expect(calculation.totals!.discountMinor).toBe(testCase.want.discountMinor);
      expect(calculation.totals!.afterDiscountMinor).toBe(
        testCase.want.afterDiscountMinor
      );
      expect(calculation.totals!.ppnMinor).toBe(testCase.want.ppnMinor);
      expect(calculation.totals!.adjustmentMinor).toBe(
        testCase.want.adjustmentMinor
      );
      expect(calculation.totals!.grandTotalMinor).toBe(
        testCase.want.grandTotalMinor
      );
      // The aggregate gross is display-only: it is never stored or sent, and
      // here it is twice the exact bound, so it must say "unavailable" rather
      // than refuse the order or print an inexact figure.
      expect(calculation.totals!.grossMinor).toBeNull();
      expect(calculation.totals!.rowDiscountMinor).toBeNull();

      const payload = buildPurchaseOrderUpdate(state, USD);
      expect(payload.error, testCase.why).toBeNull();
      expect(payload.input, testCase.why).not.toBeNull();
      expect(payload.input!.Items).toHaveLength(testCase.lines.length);
      for (let index = 0; index < testCase.lines.length; index++) {
        const input = payload.input!.Items[index];
        const line = testCase.lines[index];
        expect(input.QuantityScaled).toBe(line.quantityScaled);
        expect(input.UnitPriceMinor).toBe(line.unitPriceMinor);
        expect(input.DiscountEnabled).toBe(line.discountEnabled);
        expect(input.DiscountPercent).toBe(line.discountPercent);
      }
      expect(payload.input!.DiscountPercent).toBe(testCase.discountPercent);
      expect(payload.input!.AdjustmentMinor).toBe(testCase.adjustmentMinor);
      expect(payload.input!.AdjustmentReason).toBe(testCase.adjustmentReason);
    }
  });
});

describe("PO adjustment reason agreement (issue #470 wave 2)", () => {
  function reasonState(reason: string, adjustment = "-0.10") {
    const base = initial();
    return purchaseOrderEditorReducer(base, {
      type: "headerChanged",
      patch: { adjustment, adjustmentReason: reason },
    });
  }

  it("bounds the reason by Unicode code points, not UTF-16 units", () => {
    expect(MAX_ADJUSTMENT_REASON_LENGTH).toBe(200);

    const ideographs = "界".repeat(MAX_ADJUSTMENT_REASON_LENGTH);
    const accepted = buildPurchaseOrderUpdate(reasonState(ideographs), USD);
    expect(accepted.error).toBeNull();
    expect(accepted.input?.AdjustmentReason).toBe(ideographs);

    const over = buildPurchaseOrderUpdate(
      reasonState(ideographs + "界"),
      USD,
    );
    expect(over.input).toBeNull();
    expect(over.error).toBe("Adjustment reason must be 200 characters or fewer.");

    // Astral text: each emoji is two UTF-16 units but one code point, so 200
    // must fit where `.length` would count 400.
    const emoji = "😀".repeat(MAX_ADJUSTMENT_REASON_LENGTH);
    expect(Array.from(emoji)).toHaveLength(MAX_ADJUSTMENT_REASON_LENGTH);
    expect(buildPurchaseOrderUpdate(reasonState(emoji), USD).input).not.toBeNull();
    expect(
      buildPurchaseOrderUpdate(reasonState(emoji + "😀"), USD).error,
    ).toBe("Adjustment reason must be 200 characters or fewer.");
  });

  it("requires a reason that is not whitespace-only", () => {
    // The set is the shared fixture's: the same code points the store's
    // strings.TrimSpace removes and the schema's CHECK refuses. String.trim()
    // is deliberately not used -- it strips a BOM that Go and the database keep,
    // and misses NEL (U+0085), which both of them remove.
    expect(whitespaceFixture.blank.length).toBeGreaterThan(0);
    for (const point of whitespaceFixture.blank) {
      const reason = String.fromCodePoint(point.codePoint);
      const result = buildPurchaseOrderUpdate(reasonState(reason), USD);
      expect(result.input, `${point.name} (U+${point.codePoint.toString(16)})`).toBeNull();
      expect(result.error, point.name).toBe("An adjustment requires a reason.");
    }
    const joined = whitespaceFixture.blank
      .map((point) => String.fromCodePoint(point.codePoint))
      .join("");
    expect(buildPurchaseOrderUpdate(reasonState(joined), USD).error).toBe(
      "An adjustment requires a reason."
    );

    // Content the editor must keep verbatim, even when it is invisible.
    for (const point of whitespaceFixture.content) {
      const reason = String.fromCodePoint(point.codePoint);
      const result = buildPurchaseOrderUpdate(reasonState(reason), USD);
      expect(result.error, `${point.name} is content`).toBeNull();
      expect(result.input?.AdjustmentReason, point.name).toBe(reason);
    }
  });

  it("trims the stored reason and drops it when the adjustment is zero", () => {
    const trimmed = buildPurchaseOrderUpdate(
      reasonState("  Match the printed total  "),
      USD,
    );
    expect(trimmed.input?.AdjustmentReason).toBe("Match the printed total");

    const noAmount = buildPurchaseOrderUpdate(
      reasonState("typed before the amount", ""),
      USD,
    );
    expect(noAmount.error).toBeNull();
    expect(noAmount.input?.AdjustmentReason).toBe("");

    // A too-long reason is refused even with no amount, exactly as the Go
    // store checks length before it drops an empty adjustment's reason.
    const noAmountTooLong = buildPurchaseOrderUpdate(
      reasonState("x".repeat(MAX_ADJUSTMENT_REASON_LENGTH + 1), ""),
      USD,
    );
    expect(noAmountTooLong.error).toBe(
      "Adjustment reason must be 200 characters or fewer.",
    );
  });
});

describe("PO calculation in the saved payload (issue #470)", () => {
  it("sends the row and whole-order discounts and the signed adjustment", () => {
    const state = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: {
        discountPercent: "10",
        adjustment: "-0.26",
        adjustmentReason: "Match the printed total",
      },
    });
    const withRow = purchaseOrderEditorReducer(state, {
      type: "lineChanged",
      key: 0,
      patch: { discountEnabled: true, discountPercent: "10" },
    });
    const result = buildPurchaseOrderUpdate(withRow, USD);
    expect(result.error).toBeNull();
    const input = result.input!;
    expect(input.DiscountPercent).toBe(10);
    expect(input.AdjustmentMinor).toBe(-26);
    expect(input.AdjustmentReason).toBe("Match the printed total");
    expect(input.Items[0].DiscountEnabled).toBe(true);
    expect(input.Items[0].DiscountPercent).toBe(10);
    expect(input.Items[1].DiscountEnabled).toBe(false);
  });

  it("names the bad percentage, the missing reason and the negative total", () => {
    const badRow = purchaseOrderEditorReducer(initial(), {
      type: "lineChanged",
      key: 1,
      patch: { discountEnabled: true, discountPercent: "101" },
    });
    expect(buildPurchaseOrderUpdate(badRow, USD)).toEqual({
      input: null,
      error: "Line 2: discount must be between 0 and 100.",
    });

    const missingReason = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { adjustment: "-0.10", adjustmentReason: "" },
    });
    expect(buildPurchaseOrderUpdate(missingReason, USD).error).toBe(
      "An adjustment requires a reason.",
    );

    const negative = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: {
        discountPercent: "100",
        adjustment: "-0.01",
        adjustmentReason: "Too far",
      },
    });
    expect(buildPurchaseOrderUpdate(negative, USD).error).toContain("negative");
  });

  it("tracks the calculation fields as unsaved changes", () => {
    const base = initial();
    expect(isPurchaseOrderDirty(base)).toBe(false);

    const discount = purchaseOrderEditorReducer(base, {
      type: "headerChanged",
      patch: { discountPercent: "6" },
    });
    expect(isPurchaseOrderDirty(discount)).toBe(true);

    const row = purchaseOrderEditorReducer(base, {
      type: "lineChanged",
      key: 0,
      patch: { discountPercent: "15" },
    });
    expect(isPurchaseOrderDirty(row)).toBe(true);

    const adjustment = purchaseOrderEditorReducer(base, {
      type: "headerChanged",
      patch: { adjustmentReason: "Changed" },
    });
    expect(isPurchaseOrderDirty(adjustment)).toBe(true);
  });

  it("restores the stored discounts and the tax rate from a saved order", () => {
    const state = initial();
    expect(state.ppnBps).toBe(1100);
    expect(state.header.discountPercent).toBe("5");
    expect(state.lines[0].discountEnabled).toBe(true);
    expect(state.lines[0].discountPercent).toBe("10");
    expect(state.lines[1].discountEnabled).toBe(false);
    expect(state.printedTotals.grandTotalMinor).toBe(340736);
  });
});

// Issue #474: the confirmation number is optional. Absence is null and is
// displayed as "Not assigned"; zero never stands in for an issued identifier.
describe("confirmation number decisions (issue #474)", () => {
  it("renders absence as Not assigned, never as zero or an empty field", () => {
    expect(confirmationNumberOf(order())).toBe("");
    expect(confirmationNumberLabel(order())).toBe("Not assigned");
    expect(confirmationNumberLabel(order({ ConfirmationNumber: null }))).toBe(
      "Not assigned"
    );
    expect(confirmationNumberLabel(order({ ConfirmationNumber: "  " }))).toBe(
      "Not assigned"
    );
    expect(confirmationNumberLabel(order({ ConfirmationNumber: "2302260001" }))).toBe(
      "2302260001"
    );
  });

  it("offers a later assignment only on a saved record with no number", () => {
    expect(canAssignConfirmationNumber(order({ State: "draft" }))).toBe(false);
    expect(canAssignConfirmationNumber(order({ State: "saved" }))).toBe(true);
    expect(
      canAssignConfirmationNumber(
        order({ State: "saved", ConfirmationNumber: "2302260001" })
      )
    ).toBe(false);
    // An empty string from a hand-built payload is absence, not an issued
    // identifier, so the action is still offered.
    expect(
      canAssignConfirmationNumber(order({ State: "saved", ConfirmationNumber: "" }))
    ).toBe(true);
  });
});

// Issue #496: a PO-first draft has no source, no Customer and no printed
// totals. Presence is decided by the additive state, never by a zero id, so a
// real revision 00 stays a live link.
describe("PO-first draft states (issue #496)", () => {
  it("derives the source state from an older payload without inventing never", () => {
    // An old server could send only live or former. Revision 00 is live.
    expect(
      sourceStateOf(
        order({ SourceState: undefined, SourceMissing: false, QuotationID: 7 })
      )
    ).toBe("live");
    expect(
      sourceStateOf(
        order({ SourceState: undefined, SourceMissing: true, QuotationID: 7 })
      )
    ).toBe("former");
    // The new enum is authoritative even when the numeric fields are zero.
    expect(
      sourceStateOf(
        order({ SourceState: "never", SourceMissing: true, QuotationID: 0 })
      )
    ).toBe("never");
    expect(
      sourceStateOf(
        order({ SourceState: "live", SourceMissing: false, QuotationID: 9 })
      )
    ).toBe("live");
  });

  it("knows printed totals for live and former, never for a never-linked draft", () => {
    expect(printedTotalsKnownOf(order({ PrintedTotalsKnown: undefined }))).toBe(true);
    expect(
      printedTotalsKnownOf(
        order({ PrintedTotalsKnown: false, SourceState: "never" })
      )
    ).toBe(false);
    expect(
      printedTotalsKnownOf(
        order({ PrintedTotalsKnown: true, SourceState: "former" })
      )
    ).toBe(true);
  });

  it("builds a transient blank draft with the company currency and user tax defaults", () => {
    const blank = blankPurchaseOrderDraft({
      currency: "IDR",
      taxZone: "NonFTZ",
      taxType: "Goods",
      serviceCategory: "",
      receivedDate: "2026-10-22",
      paymentTerms: "50% advance",
      incoterms: "FOB",
    });
    expect(blank.ID).toBe(0);
    expect(blank.Version).toBe(0);
    expect(blank.SourceState).toBe("never");
    expect(blank.SourceMissing).toBe(true);
    expect(blank.PrintedTotalsKnown).toBe(false);
    expect(blank.QuotationID).toBe(0);
    expect(blank.CustomerID).toBe(0);
    expect(blank.SourceRevision).toBe(0);
    expect(blank.QuoteNumber).toBe("");
    expect(blank.CustomerName).toBe("");
    expect(blank.Currency).toBe("IDR");
    expect(blank.TaxType).toBe("Goods");
    expect(blank.PpnBps).toBe(1100);
    expect(blank.Items).toEqual([]);
    expect(blank.ReceivedDate).toBe("2026-10-22");
    expect(blank.PaymentTerms).toBe("50% advance");
    expect(blank.Incoterms).toBe("FOB");
  });

  it("mirrors the server's PPN resolution from the full triple", () => {
    expect(purchaseOrderPpnBps("NonFTZ", "Goods", "", "")).toBe(1100);
    expect(purchaseOrderPpnBps("FTZ", "Goods", "", "")).toBe(0);
    expect(purchaseOrderPpnBps("NonFTZ", "Services", "NonConstruction", "")).toBe(0);
    expect(purchaseOrderPpnBps("NonFTZ", "Services", "Construction", "")).toBe(0);
    // The legacy category maps forward when the type is absent.
    expect(purchaseOrderPpnBps("NonFTZ", "", "", "SupplyOnly")).toBe(1100);
    expect(purchaseOrderPpnBps("NonFTZ", "", "", "SupplyInstall")).toBe(0);
    // Invalid combinations are named, not silently treated as zero.
    expect(purchaseOrderPpnBps("NonFTZ", "Goods", "Construction", "")).toBeNull();
    expect(purchaseOrderPpnBps("NonFTZ", "Services", "", "")).toBeNull();
  });

  it("keeps the typed header and lines when the first create adopts an id", () => {
    const base = initialPurchaseOrderEditorState(
      blankPurchaseOrderDraft({
        currency: "IDR",
        taxZone: "FTZ",
        taxType: "Goods",
        serviceCategory: "",
        receivedDate: "2026-10-22",
      }),
      IDR
    );
    const typed = purchaseOrderEditorReducer(base, {
      type: "headerChanged",
      patch: { poNumber: "CUSTOMER-1" },
    });
    const created = purchaseOrderEditorReducer(typed, {
      type: "created",
      orderId: 41,
      version: 1,
    });
    expect(created.orderId).toBe(41);
    expect(created.version).toBe(1);
    expect(created.header.poNumber).toBe("CUSTOMER-1");
    expect(created.lines).toEqual(typed.lines);
  });

  it("sends the editable basis in the update payload", () => {
    const base = initialPurchaseOrderEditorState(
      blankPurchaseOrderDraft({
        currency: "IDR",
        taxZone: "FTZ",
        taxType: "Goods",
        serviceCategory: "",
        receivedDate: "2026-10-22",
      }),
      IDR
    );
    const nonFtz = purchaseOrderEditorReducer(base, {
      type: "sourceBasisChanged",
      patch: { taxZone: "NonFTZ" },
    });
    expect(nonFtz.ppnBps).toBe(1100);
    const services = purchaseOrderEditorReducer(nonFtz, {
      type: "sourceBasisChanged",
      patch: { taxType: "Services" },
    });
    expect(services.serviceCategory).toBe("NonConstruction");
    expect(services.ppnBps).toBe(0);
    const goods = purchaseOrderEditorReducer(services, {
      type: "sourceBasisChanged",
      patch: { taxType: "Goods" },
    });
    expect(goods.serviceCategory).toBe("");

    const built = buildPurchaseOrderUpdate(nonFtz, IDR);
    expect(built.input).not.toBeNull();
    expect(built.input?.Currency).toBe("IDR");
    expect(built.input?.TaxZone).toBe("NonFTZ");
    expect(built.input?.TaxType).toBe("Goods");
    expect(built.input?.ServiceCategory).toBe("");
  });

  it("marks a currency or basis edit dirty, so the leave guard sees it", () => {
    const base = initialPurchaseOrderEditorState(
      blankPurchaseOrderDraft({
        currency: "IDR",
        taxZone: "FTZ",
        taxType: "Goods",
        serviceCategory: "",
        receivedDate: "2026-10-22",
      }),
      IDR
    );
    expect(isPurchaseOrderDirty(base)).toBe(false);
    expect(
      isPurchaseOrderDirty(
        purchaseOrderEditorReducer(base, {
          type: "sourceBasisChanged",
          patch: { taxZone: "NonFTZ" },
        })
      )
    ).toBe(true);
    expect(
      isPurchaseOrderDirty(
        purchaseOrderEditorReducer(base, {
          type: "currencyChanged",
          code: "USD",
          from: IDR,
          to: USD,
        })
      )
    ).toBe(true);
  });
});
