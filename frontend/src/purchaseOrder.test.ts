import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import {
  buildPurchaseOrderUpdate,
  formatQuantityScaled,
  formatUnitPriceMinor,
  initialPurchaseOrderEditorState,
  isPurchaseOrderDirty,
  parsePaymentTermsDays,
  parseQuantityScaled,
  parseUnitPriceMinor,
  purchaseOrderEditorReducer,
  validDateField,
  type PurchaseOrderDTO,
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
      },
      {
        QuotationItemNo: 2,
        QuotationSubNo: 1,
        LineNumber: 2,
        Description: "Mounting",
        QuantityScaled: 10000,
        UoM: "Unit",
        UnitPriceMinor: 42000,
      },
    ],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTermsDays: 45,
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
    for (const raw of ["0", "0.0000", "-1", "1.23456", "abc", "", "1.2.3", "1,2,3"]) {
      expect(parseQuantityScaled(raw), raw).toBeNull();
    }
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

  it("asks before discarding a dirty draft and closes silently when clean", () => {
    const clean = initial();
    expect(
      purchaseOrderEditorReducer(clean, { type: "cancelRequested" }).confirmDiscard
    ).toBe(false);

    const dirty = purchaseOrderEditorReducer(clean, {
      type: "lineChanged",
      key: 0,
      patch: { description: "Edited" },
    });
    const asked = purchaseOrderEditorReducer(dirty, { type: "cancelRequested" });
    expect(asked.confirmDiscard).toBe(true);
    expect(
      purchaseOrderEditorReducer(asked, { type: "cancelDismissed" }).confirmDiscard
    ).toBe(false);
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
    expect(input.PaymentTermsDays).toBe(45);
    expect(input.Items).toHaveLength(2);
    expect(input.Items[0]).toEqual({
      QuotationItemNo: 1,
      QuotationSubNo: null,
      Description: "Widget",
      QuantityScaled: 25000,
      UoM: "Pc",
      UnitPriceMinor: 125050,
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
      "Line 1: unit price must be a nonnegative USD amount."
    );
  });

  it("requires a real received date and validates the optional dates", () => {
    const missing = purchaseOrderEditorReducer(initial(), {
      type: "headerChanged",
      patch: { receivedDate: "" },
    });
    expect(buildPurchaseOrderUpdate(missing, USD).error).toContain("Received date");

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
      "Requested delivery date"
    );
  });

  it("validates payment terms and calendar dates", () => {
    expect(parsePaymentTermsDays("0")).toBe(0);
    expect(parsePaymentTermsDays("45")).toBe(45);
    expect(parsePaymentTermsDays("")).toBe(0);
    expect(parsePaymentTermsDays("3651")).toBeNull();
    expect(parsePaymentTermsDays("4.5")).toBeNull();
    expect(validDateField("2026-02-28")).toBe(true);
    expect(validDateField("2026-02-29")).toBe(false);
    expect(validDateField("")).toBe(true);
  });
});
