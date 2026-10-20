import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import {
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  formatQuantityScaled,
  formatUnitPriceMinor,
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  MAX_PAYMENT_TERMS_LENGTH,
  parseAdjustmentMinor,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
  purchaseOrderEditorReducer,
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
      error: "Line 2: quantity must be positive with up to 4 decimal places.",
    });

    const badPrice = purchaseOrderEditorReducer(initial(), {
      type: "lineChanged",
      key: 0,
      patch: { unitPrice: "not money" },
    });
    expect(buildPurchaseOrderUpdate(badPrice, USD).error).toBe(
      "Line 1: unit price must be a nonnegative USD amount.",
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
    expect(calculation.totals.ppnMinor).toBe(110);
    expect(calculation.totals.adjustmentMinor).toBe(-10);
    expect(calculation.totals.grandTotalMinor).toBe(1100);
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
    expect(positive.totals.grandTotalMinor).toBe(1135);
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

  it("leaves an unparseable line out of the totals and says which one", () => {
    const calculation = calculatePurchaseOrder(
      stateWith([
        { quantity: "2", unitPrice: "1.00" },
        { quantity: "1.23456", unitPrice: "1.00" },
      ]),
      USD,
    );
    expect(calculation.lines[1]).toBeNull();
    expect(calculation.totals.subtotalMinor).toBe(200);
    expect(calculation.error).toBe(
      "Line 2: quantity must be positive with up to 4 decimal places.",
    );
  });

  it("refuses amounts that would overflow the supported money range", () => {
    const calculation = calculatePurchaseOrder(
      stateWith([{ quantity: "900000000000", unitPrice: "900000000000" }]),
      USD,
    );
    expect(calculation.error).toContain("range");
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
