import { describe, expect, it } from "vitest";
import type { poreport } from "@backend/models";
import type { CurrencyInfo } from "./format";
import {
  FULFILLMENT_DEFAULT_PAGE_SIZE,
  FULFILLMENT_STATUS_OPTIONS,
  FULFILLMENT_STATUS_ORDER,
  formatFulfillmentDate,
  formatFulfillmentLoss,
  formatFulfillmentMoney,
  fulfillmentPageBounds,
  fulfillmentQueryKey,
  fulfillmentStatusLabel,
  fulfillmentStatusTone,
  groupFulfillmentItems,
  normalizePurchaseOrderFulfillmentPage,
  type PurchaseOrderFulfillmentItem,
  type PurchaseOrderFulfillmentPage,
} from "./poFulfillment";

// Issue #657 phase B: the pure contract behind the Fulfillment card. The server
// owns status derivation and every figure; this module only normalizes the wire
// page, names the four statuses in one order, groups aggregate children, and
// formats money/quantity for the screen. It deliberately has no supplier unit
// cost field, so a wire field the server must not send is dropped rather than
// rendered.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

function wirePage(): poreport.FulfillmentPage {
  return {
    asOf: "2026-11-19",
    filter: {
      currency: "IDR",
      status: "all",
      search: "",
      page: 1,
      pageSize: 25,
    },
    quotations: [
      {
        quotationId: 7,
        quoteNumber: "1502260007",
        customerName: "PT. Infineon Technologies Batam",
        currency: "IDR",
        latestRevision: 1,
        issuedAt: "2026-10-10",
        validUntil: "2026-11-09",
        status: "closed",
        items: [
          {
            quotationItemNo: 1,
            label: "1",
            description: "Plain line",
            quotedQuantityScaled: 50000,
            boughtQuantityScaled: 30000,
            unsoldQuantityScaled: 0,
            overBoughtQuantityScaled: 0,
            realizedAmountMinor: 60000,
            realizedMarginMinor: 30000,
            unsoldAmountMinor: 0,
            unsoldMarginMinor: 0,
            quantityExtraIncomeAmountMinor: 0,
            quantityExtraIncomeMarginMinor: 0,
            priceExtraIncomeMinor: 0,
            priceLossMinor: 0,
            closedLossAmountMinor: 40000,
            closedLossMarginMinor: 20000,
            purchaseOrders: [
              {
                id: 11,
                poNumber: "PO-657-1",
                state: "saved",
                receivedDate: "2026-03-10",
                allocatedQuantityScaled: 30000,
              },
            ],
            // A field the server must never send. The normalizer is a
            // whitelist, so this must not survive into the screen's shape.
            costUnitMinor: 10000,
          },
          {
            quotationItemNo: 2,
            quotationSubNo: 2,
            label: "2.2",
            description: "Grouped child",
            groupTitle: "Delivered package",
            removed: false,
            quotedQuantityScaled: 20000,
            boughtQuantityScaled: 0,
            unsoldQuantityScaled: 20000,
            overBoughtQuantityScaled: 0,
            realizedAmountMinor: 0,
            realizedMarginMinor: 0,
            unsoldAmountMinor: 40000,
            unsoldMarginMinor: 20000,
            quantityExtraIncomeAmountMinor: 0,
            quantityExtraIncomeMarginMinor: 0,
            priceExtraIncomeMinor: 0,
            priceLossMinor: 0,
            closedLossAmountMinor: 0,
            closedLossMarginMinor: 0,
            purchaseOrders: [],
          },
        ],
        extraExpenses: [
          {
            purchaseOrderId: 11,
            poNumber: "PO-657-1",
            lineNumber: 2,
            description: "Freight surcharge",
            quantityScaled: 20000,
            uom: "Lot",
            amountMinor: 1000000,
            groupTitle: "Delivered package",
          },
        ],
      },
    ],
    page: 1,
    pageSize: 25,
    total: 1,
    totalPages: 1,
  } as unknown as poreport.FulfillmentPage;
}

describe("normalizePurchaseOrderFulfillmentPage", () => {
  it("copies every server figure and drops any supplier cost the wire carries", () => {
    const page = normalizePurchaseOrderFulfillmentPage(wirePage());
    expect(page.asOf).toBe("2026-11-19");
    expect(page.filter).toEqual({
      currency: "IDR",
      status: "all",
      search: "",
      page: 1,
      pageSize: 25,
    });
    expect(page.total).toBe(1);
    expect(page.totalPages).toBe(1);

    const quotation = page.quotations[0];
    expect(quotation.quoteNumber).toBe("1502260007");
    expect(quotation.status).toBe("closed");

    const [plain, grouped] = quotation.items;
    expect(plain.quotationSubNo).toBeNull();
    expect(grouped.quotationSubNo).toBe(2);
    expect(grouped.groupTitle).toBe("Delivered package");
    expect(grouped.removed).toBe(false);
    expect(plain.quotedQuantityScaled).toBe(50000);
    expect(plain.boughtQuantityScaled).toBe(30000);
    expect(plain.realizedAmountMinor).toBe(60000);
    expect(plain.realizedMarginMinor).toBe(30000);
    expect(plain.closedLossAmountMinor).toBe(40000);
    expect(plain.purchaseOrders[0].poNumber).toBe("PO-657-1");
    expect(quotation.extraExpenses[0].amountMinor).toBe(1000000);
    expect(quotation.extraExpenses[0].groupTitle).toBe("Delivered package");

    // Story 42, at the pure seam: the normalized item is a whitelist. It has
    // no key that names a cost or a supplier.
    for (const key of Object.keys(plain)) {
      expect(key.toLowerCase()).not.toContain("cost");
      expect(key.toLowerCase()).not.toContain("supplier");
    }
    expect("costUnitMinor" in plain).toBe(false);
    expect(JSON.stringify(page).toLowerCase()).not.toContain("costunit");
  });

  it("defaults absent optional child fields instead of inventing a group or a removal", () => {
    const page = normalizePurchaseOrderFulfillmentPage(wirePage());
    const grouped = page.quotations[0].items[1];
    expect(grouped.groupTitle).toBe("Delivered package");
    const plain = page.quotations[0].items[0];
    expect(plain.groupTitle).toBe("");
    expect(plain.removed).toBe(false);
  });
});

describe("status vocabulary", () => {
  it("names the four derived statuses in one stable order, with All first", () => {
    expect(FULFILLMENT_STATUS_ORDER).toEqual([
      "open",
      "expired_unclosed",
      "all_bought",
      "closed",
    ]);
    expect(FULFILLMENT_STATUS_OPTIONS.map((option) => option.value)).toEqual([
      "all",
      ...FULFILLMENT_STATUS_ORDER,
    ]);
    expect(FULFILLMENT_STATUS_OPTIONS.map((option) => option.label)).toEqual([
      "All statuses",
      "Open",
      "Expired (unclosed)",
      "All bought",
      "Closed",
    ]);
  });

  it("labels a known status and echoes an unknown one rather than inventing a name", () => {
    expect(fulfillmentStatusLabel("expired_unclosed")).toBe("Expired (unclosed)");
    expect(fulfillmentStatusLabel("mystery")).toBe("mystery");
    expect(fulfillmentStatusTone("open")).toBe("success");
    expect(fulfillmentStatusTone("expired_unclosed")).toBe("accent");
    expect(fulfillmentStatusTone("closed")).toBe("danger");
  });
});

describe("groupFulfillmentItems", () => {
  const item = (label: string, groupTitle: string): PurchaseOrderFulfillmentItem =>
    ({
      quotationItemNo: Number(label),
      quotationSubNo: null,
      label,
      description: `Line ${label}`,
      groupTitle,
      removed: false,
      quotedQuantityScaled: 10000,
      boughtQuantityScaled: 0,
      unsoldQuantityScaled: 10000,
      overBoughtQuantityScaled: 0,
      realizedAmountMinor: 0,
      realizedMarginMinor: 0,
      unsoldAmountMinor: 10000,
      unsoldMarginMinor: 5000,
      quantityExtraIncomeAmountMinor: 0,
      quantityExtraIncomeMarginMinor: 0,
      priceExtraIncomeMinor: 0,
      priceLossMinor: 0,
      closedLossAmountMinor: 0,
      closedLossMarginMinor: 0,
      purchaseOrders: [],
    }) as PurchaseOrderFulfillmentItem;

  it("keeps contiguous children under one title and never merges a later repeat", () => {
    const groups = groupFulfillmentItems([
      item("1", ""),
      item("2", "Package A"),
      item("3", "Package A"),
      item("4", ""),
      item("5", "Package A"),
    ]);
    expect(groups.map((group) => group.title)).toEqual([
      "",
      "Package A",
      "",
      "Package A",
    ]);
    expect(groups.map((group) => group.items.map((entry) => entry.label))).toEqual([
      ["1"],
      ["2", "3"],
      ["4"],
      ["5"],
    ]);
  });
});

describe("pagination and filters", () => {
  it("cuts 26 quotations into 25 and 1 with honest bounds", () => {
    const first = fulfillmentPageBounds(26, FULFILLMENT_DEFAULT_PAGE_SIZE, 1);
    expect(first).toEqual({
      page: 1,
      totalPages: 2,
      hasPrevious: false,
      hasNext: true,
    });
    const second = fulfillmentPageBounds(26, 25, 2);
    expect(second).toEqual({
      page: 2,
      totalPages: 2,
      hasPrevious: true,
      hasNext: false,
    });
  });

  it("reports one empty page, not page 1 of 0", () => {
    expect(fulfillmentPageBounds(0, 25, 1)).toEqual({
      page: 1,
      totalPages: 1,
      hasPrevious: false,
      hasNext: false,
    });
  });

  it("clamps a page past the end for display while keeping Previous available", () => {
    expect(fulfillmentPageBounds(26, 25, 9)).toEqual({
      page: 2,
      totalPages: 2,
      hasPrevious: true,
      hasNext: false,
    });
  });

  it("changes the query key for every field that changes the server read", () => {
    const base = {
      currency: "IDR",
      status: "all",
      search: "",
      page: 1,
      pageSize: 25,
    };
    const keys = new Set([
      fulfillmentQueryKey(base),
      fulfillmentQueryKey({ ...base, currency: "USD" }),
      fulfillmentQueryKey({ ...base, status: "closed" }),
      fulfillmentQueryKey({ ...base, search: "infineon" }),
      fulfillmentQueryKey({ ...base, page: 2 }),
      fulfillmentQueryKey({ ...base, pageSize: 50 }),
    ]);
    expect(keys.size).toBe(6);
    expect(fulfillmentQueryKey({ ...base })).toBe(fulfillmentQueryKey(base));
  });
});

describe("formatting", () => {
  it("formats minor units in the page's currency and never substitutes one", () => {
    expect(formatFulfillmentMoney(IDR, 40000)).toBe("Rp40.000");
    expect(formatFulfillmentMoney(null, 40000)).toBe("-");
    expect(formatFulfillmentLoss(IDR, 0)).toBe("Rp0");
    expect(formatFulfillmentLoss(IDR, 40000)).toBe("−Rp40.000");
    expect(formatFulfillmentLoss(null, 40000)).toBe("-");
  });

  it("renders a scaled quantity as the shortest decimal", () => {
    const page: PurchaseOrderFulfillmentPage = normalizePurchaseOrderFulfillmentPage(wirePage());
    expect(page.quotations[0].items[0].boughtQuantityScaled).toBe(30000);
  });

  it("formats a saved-PO date compactly and echoes anything that is not a date", () => {
    expect(formatFulfillmentDate("2026-10-05")).toBe("05 Oct 2026");
    expect(formatFulfillmentDate(" 2026-01-31 ")).toBe("31 Jan 2026");
    expect(formatFulfillmentDate("2026-13-01")).toBe("2026-13-01");
    expect(formatFulfillmentDate("not-a-date")).toBe("not-a-date");
    expect(formatFulfillmentDate("")).toBe("");
  });
});
