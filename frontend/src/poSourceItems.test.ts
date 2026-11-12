import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
import type { PurchaseOrderLine } from "./purchaseOrder";
import {
  groupSourceItems,
  linesFromSourceItem,
  sourceItemApplied,
} from "./poSourceItems";

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

function line(
  overrides: Partial<PurchaseOrderSourceLineDTO> = {}
): PurchaseOrderSourceLineDTO {
  return {
    QuotationItemNo: 1,
    QuotationSubNo: null,
    LineLabel: "1",
    Description: "Widget",
    UoM: "Pc",
    QuantityScaled: 20000,
    UnitPriceMinor: 10000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    ...overrides,
  };
}

function poLine(overrides: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  return {
    key: 1,
    clientKey: "client:1",
    description: "Widget",
    quantity: "2",
    uom: "Pc",
    unitPrice: "Rp10.000",
    discountEnabled: false,
    discountPercent: "0",
    quotationItemNo: 1,
    quotationSubNo: null,
    groupKey: "",
    parentDescription: "",
    descriptionLines: [],
    ...overrides,
  };
}

describe("groupSourceItems", () => {
  it("groups priced parts into one quotation item and sums their net totals", () => {
    const items = groupSourceItems([
      line({
        QuotationItemNo: 2,
        QuotationSubNo: 1,
        LineLabel: "2.1",
        ParentDescription: "Mounting package",
        Description: "Mounting and cable termination",
        QuantityScaled: 10000,
        UnitPriceMinor: 42000,
        DescriptionLines: ["Cable tray"],
      }),
      line({
        QuotationItemNo: 2,
        QuotationSubNo: 2,
        LineLabel: "2.2",
        ParentDescription: "Mounting package",
        Description: "Cable termination and labelling",
        QuantityScaled: 10000,
        UnitPriceMinor: 30000,
        DiscountEnabled: true,
        DiscountPercent: 5,
        DescriptionLines: ["Two technicians"],
      }),
    ]);

    expect(items).toHaveLength(1);
    const item = items[0];
    expect(item.description).toBe("Mounting package");
    expect(item.subItems.map((sub) => sub.label)).toEqual(["2.1", "2.2"]);
    expect(item.subItems[1].descriptionLines).toEqual(["Two technicians"]);
    // 42,000 + (30,000 less 5%) = 70,500.
    expect(item.unitPriceMinor).toBe(70500);
  });

  it("keeps a plain item as a single item with its description lines", () => {
    const items = groupSourceItems([
      line({ DescriptionLines: ["One-year consumables"] }),
    ]);
    expect(items).toHaveLength(1);
    expect(items[0].subItems).toEqual([]);
    expect(items[0].descriptionLines).toEqual(["One-year consumables"]);
  });
});

describe("sourceItemApplied", () => {
  it("accepts the whole split item only when every part matches", () => {
    const item = groupSourceItems([
      line({ QuotationItemNo: 2, QuotationSubNo: 1, LineLabel: "2.1", Description: "A", UnitPriceMinor: 100 }),
      line({ QuotationItemNo: 2, QuotationSubNo: 2, LineLabel: "2.2", Description: "B", UnitPriceMinor: 200 }),
    ])[0];
    const lines = [
      poLine({ key: 2, clientKey: "client:2", quotationItemNo: 2, quotationSubNo: 1, description: "A", unitPrice: "Rp100", groupKey: "q:2" }),
      poLine({ key: 3, clientKey: "client:3", quotationItemNo: 2, quotationSubNo: 2, description: "B", unitPrice: "Rp200", groupKey: "q:2" }),
    ];
    expect(sourceItemApplied(item, lines, IDR)).toBe(true);

    const changed = [lines[0], { ...lines[1], unitPrice: "Rp201" }];
    expect(sourceItemApplied(item, changed, IDR)).toBe(false);
  });

  it("compares a plain item's values and description lines", () => {
    const item = groupSourceItems([line({ DescriptionLines: ["Note"] })])[0];
    expect(sourceItemApplied(item, [poLine({ descriptionLines: ["Note"] })], IDR)).toBe(true);
    expect(sourceItemApplied(item, [poLine({ descriptionLines: [] })], IDR)).toBe(false);
  });
});

describe("linesFromSourceItem", () => {
  it("emits one flat row per priced part with the shared group and parent title", () => {
    const item = groupSourceItems([
      line({ QuotationItemNo: 2, QuotationSubNo: 1, LineLabel: "2.1", ParentDescription: "Mounting package", Description: "A", DescriptionLines: ["Cable tray"] }),
      line({ QuotationItemNo: 2, QuotationSubNo: 2, LineLabel: "2.2", ParentDescription: "Mounting package", Description: "B" }),
    ])[0];
    const rows = linesFromSourceItem(item, IDR, 10);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toMatchObject({
      key: 10,
      quotationItemNo: 2,
      quotationSubNo: 1,
      groupKey: "q:2",
      parentDescription: "Mounting package",
      descriptionLines: ["Cable tray"],
    });
    expect(rows[1]).toMatchObject({ key: 11, quotationSubNo: 2 });
    expect(rows[1].clientKey).not.toBe(rows[0].clientKey);
  });
});
