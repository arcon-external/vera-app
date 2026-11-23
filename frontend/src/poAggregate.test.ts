import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type {
  PurchaseOrderAggregateGroupState,
  PurchaseOrderEditorState,
  PurchaseOrderLine,
} from "./purchaseOrder";
import {
  aggregateDisplayUnitPriceMinor,
  aggregateKeyOf,
  aggregateRuns,
  aggregateTotalMinor,
  buildAggregatePayload,
  groupIntoAggregate,
  lineNetMinor,
  pruneEmptyAggregates,
  ungroupAggregate,
  validateAggregateLayout,
} from "./poAggregate";

// Issue #654: the PO-only aggregate line is presentation over the flat priced
// rows. This suite holds the arithmetic and the grouping rules the editor, the
// customer projection and the save payload all share, at the exact scale the
// store uses (4-decimal quantity, currency minor units).

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

function line(overrides: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  return {
    key: 1,
    clientKey: "client:test-1",
    description: "Widget",
    quantity: "1",
    uom: "Pc",
    unitPrice: "$1.00",
    discountEnabled: false,
    discountPercent: "0",
    quotationItemNo: null,
    quotationSubNo: null,
    groupKey: "",
    parentDescription: "",
    descriptionLines: [],
    aggregateKey: null,
    ...overrides,
  };
}

function group(
  overrides: Partial<PurchaseOrderAggregateGroupState> = {}
): PurchaseOrderAggregateGroupState {
  return {
    key: 0,
    title: "Combined items",
    orderedQuantity: "1",
    orderedUoM: "Lot",
    showChildrenToCustomer: false,
    ...overrides,
  };
}

function editorState(
  overrides: Partial<PurchaseOrderEditorState> = {}
): PurchaseOrderEditorState {
  return {
    orderId: 1,
    quoteNumber: "1502260001",
    customerName: "PT. Buyer",
    sourceRevision: 1,
    sourceState: "live",
    printedTotalsKnown: true,
    currency: "USD",
    taxZone: "FTZ",
    taxType: "Goods",
    serviceCategory: "",
    taxCategory: "",
    header: {
      billingAddress: "",
      deliveryAddress: "",
      shipToCode: "",
      contactEmail: "",
      contactPhone: "",
      poNumber: "PO-1",
      poDate: "",
      receivedDate: "2026-11-23",
      requestedDeliveryDate: "",
      paymentTerms: "",
      incoterms: "",
      businessCategory: "",
      confirmationRequested: false,
    },
    lines: [],
    ppnBps: 0,
    printedTotals: {
      subtotalMinor: 0,
      discountMinor: 0,
      ppnMinor: 0,
      grandTotalMinor: 0,
    },
    version: 1,
    baseline: "",
    nextKey: 10,
    saving: false,
    error: null,
    aggregateGroups: [],
    nextAggregateKey: 0,
    ...overrides,
  };
}

describe("aggregate line net arithmetic (#654)", () => {
  it("computes the exact quantity x price net with half-away rounding", () => {
    // 2.5 x $10.00 = $25.00 exactly.
    expect(lineNetMinor(line({ quantity: "2.5", unitPrice: "$10.00" }), USD)).toBe(2500);
    // 0.5 x $0.01 = 0.5 minor units, rounded half away from zero to 1.
    expect(lineNetMinor(line({ quantity: "0.5", unitPrice: "$0.01" }), USD)).toBe(1);
    // 0.4999 x $0.01 is below a half unit and rounds to 0.
    expect(lineNetMinor(line({ quantity: "0.4999", unitPrice: "$0.01" }), USD)).toBe(0);
  });

  it("subtracts the row discount with the store's truncation, and nothing else", () => {
    // 1.53 x $1.00 = $1.53; 10% truncates to 15 minor units; net 138.
    expect(
      lineNetMinor(
        line({ quantity: "1.53", unitPrice: "$1.00", discountEnabled: true, discountPercent: "10" }),
        USD
      )
    ).toBe(138);
    // A disabled discount never applies even when a percentage is present.
    expect(
      lineNetMinor(
        line({ quantity: "1.53", unitPrice: "$1.00", discountEnabled: false, discountPercent: "90" }),
        USD
      )
    ).toBe(153);
  });

  it("refuses an unparseable or out-of-range child instead of returning a partial figure", () => {
    for (const bad of [
      line({ quantity: "abc" }),
      line({ quantity: "0" }),
      line({ unitPrice: "" }),
      line({ unitPrice: "-5.00" }),
      line({ discountPercent: "10.5" }),
      line({ discountPercent: "101" }),
    ]) {
      expect(lineNetMinor(bad, USD)).toBeNull();
    }
  });

  it("reads money in the PO's own currency convention", () => {
    expect(lineNetMinor(line({ quantity: "2", unitPrice: "Rp1.250" }), IDR)).toBe(2500);
  });
});

describe("aggregate derived display unit price (#654)", () => {
  it("divides the total by the display quantity exactly, half away from zero", () => {
    // 288 minor units over 2 Lot = 144 each.
    expect(aggregateDisplayUnitPriceMinor(288, 20000)).toBe(144);
    // 1 over 2 = 0.5, rounded half away from zero to 1.
    expect(aggregateDisplayUnitPriceMinor(1, 20000)).toBe(1);
    // 1 over 3 = 0.333..., below a half and rounded to 0.
    expect(aggregateDisplayUnitPriceMinor(1, 30000)).toBe(0);
    // 2 over 3 = 0.666..., above a half and rounded to 1.
    expect(aggregateDisplayUnitPriceMinor(2, 30000)).toBe(1);
  });

  it("is null when the exact value cannot be named in the safe range", () => {
    const max = Number.MAX_SAFE_INTEGER;
    // The total itself fits...
    expect(aggregateDisplayUnitPriceMinor(max, 10000)).toBe(max);
    // ...but a display quantity below one unit doubles it past the bound.
    expect(aggregateDisplayUnitPriceMinor(max, 5000)).toBeNull();
  });

  it("is null for a missing, zero, negative or fractional display quantity", () => {
    expect(aggregateDisplayUnitPriceMinor(100, 0)).toBeNull();
    expect(aggregateDisplayUnitPriceMinor(100, -10000)).toBeNull();
    expect(aggregateDisplayUnitPriceMinor(100, 1.5)).toBeNull();
    expect(aggregateDisplayUnitPriceMinor(-1, 10000)).toBeNull();
  });
});

describe("aggregate total (#654)", () => {
  it("sums only the named group's children", () => {
    const lines = [
      line({ key: 1, quantity: "1", unitPrice: "$1.00", aggregateKey: 4 }),
      line({ key: 2, quantity: "2", unitPrice: "$1.00", aggregateKey: 4 }),
      line({ key: 3, quantity: "9", unitPrice: "$1.00", aggregateKey: null }),
      line({ key: 4, quantity: "9", unitPrice: "$1.00", aggregateKey: 5 }),
    ];
    expect(aggregateTotalMinor(lines, 4, USD)).toBe(300);
    expect(aggregateTotalMinor(lines, 5, USD)).toBe(900);
    expect(aggregateTotalMinor(lines, 6, USD)).toBe(0);
  });

  it("is null when any child cannot be totalled, never a partial sum", () => {
    const lines = [
      line({ key: 1, quantity: "1", unitPrice: "$1.00", aggregateKey: 4 }),
      line({ key: 2, quantity: "abc", unitPrice: "$1.00", aggregateKey: 4 }),
    ];
    expect(aggregateTotalMinor(lines, 4, USD)).toBeNull();
  });

  it("reads membership only through aggregateKeyOf", () => {
    expect(aggregateKeyOf(line({ aggregateKey: 3 }))).toBe(3);
    expect(aggregateKeyOf(line({ aggregateKey: null }))).toBeNull();
  });
});

describe("aggregate display runs (#654)", () => {
  it("joins only consecutive rows that share a group", () => {
    const lines = [
      line({ key: 1, aggregateKey: 4 }),
      line({ key: 2, aggregateKey: 4 }),
      line({ key: 3, aggregateKey: null }),
      line({ key: 4, aggregateKey: 5 }),
      line({ key: 5, aggregateKey: 5 }),
      line({ key: 6, aggregateKey: null }),
    ];
    const runs = aggregateRuns(lines);
    expect(runs.map((run) => [run.aggregateKey, run.lines.map((row) => row.key)])).toEqual([
      [4, [1, 2]],
      [null, [3]],
      [5, [4, 5]],
      [null, [6]],
    ]);
    expect(new Set(runs.map((run) => run.key)).size).toBe(runs.length);
  });

  it("never splits one split package across runs", () => {
    // The stored rows of q:2 are interrupted by a plain line, exactly the
    // shape #573 already regroups for display. The package moves as one
    // display group, so its aggregate cannot be torn in half.
    const lines = [
      line({ key: 1, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 1, aggregateKey: 7 }),
      line({ key: 2, aggregateKey: null }),
      line({ key: 3, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 2, aggregateKey: 7 }),
    ];
    const runs = aggregateRuns(lines);
    expect(runs.map((run) => [run.aggregateKey, run.lines.map((row) => row.key)])).toEqual([
      [7, [1, 3]],
      [null, [2]],
    ]);
  });
});

describe("grouping and ungrouping (#654)", () => {
  it("moves selected display groups together at the earliest selected position", () => {
    const lines = [line({ key: 1 }), line({ key: 2 }), line({ key: 3 })];
    const grouped = groupIntoAggregate(lines, [3, 1], [], 9);
    expect(grouped.map((row) => [row.key, row.aggregateKey])).toEqual([
      [1, 9],
      [3, 9],
      [2, null],
    ]);
  });

  it("moves a whole split package when any of its rows is selected", () => {
    const lines = [
      line({ key: 1, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 1 }),
      line({ key: 2, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 2 }),
      line({ key: 3 }),
    ];
    const grouped = groupIntoAggregate(lines, [2], [], 9);
    expect(grouped.map((row) => row.key)).toEqual([1, 2, 3]);
    expect(grouped[0].aggregateKey).toBe(9);
    expect(grouped[1].aggregateKey).toBe(9);
    expect(grouped[2].aggregateKey).toBeNull();
  });

  it("reparents a child already in another aggregate, never nesting it", () => {
    const lines = [
      line({ key: 1, aggregateKey: 3 }),
      line({ key: 2, aggregateKey: 3 }),
      line({ key: 3 }),
    ];
    const grouped = groupIntoAggregate(lines, [1], [], 8);
    expect(grouped.map((row) => [row.key, row.aggregateKey])).toEqual([
      [1, 8],
      [2, 3],
      [3, null],
    ]);
  });

  it("materializes selected ghosts at the earliest selected position", () => {
    const lines = [line({ key: 1 }), line({ key: 2 })];
    const fresh = [line({ key: 10, description: "Freight" })];
    const grouped = groupIntoAggregate(lines, [1], fresh, 9);
    expect(grouped.map((row) => [row.key, row.aggregateKey])).toEqual([
      [10, 9],
      [1, 9],
      [2, null],
    ]);
  });

  it("appends ghosts when no existing line was selected", () => {
    const lines = [line({ key: 1 })];
    const fresh = [line({ key: 10 })];
    const grouped = groupIntoAggregate(lines, [], fresh, 9);
    expect(grouped.map((row) => [row.key, row.aggregateKey])).toEqual([
      [1, null],
      [10, 9],
    ]);
  });

  it("keeps an existing aggregate contiguous when another group joins it", () => {
    const lines = [
      line({ key: 1, aggregateKey: 9 }),
      line({ key: 2 }),
      line({ key: 3 }),
    ];
    const grouped = groupIntoAggregate(lines, [3], [], 9);
    expect(grouped.map((row) => [row.key, row.aggregateKey])).toEqual([
      [1, 9],
      [3, 9],
      [2, null],
    ]);
  });

  it("clears membership on ungroup without deleting or reordering", () => {
    const lines = [
      line({ key: 1, aggregateKey: 4 }),
      line({ key: 2 }),
      line({ key: 3, aggregateKey: 4 }),
    ];
    const ungrouped = ungroupAggregate(lines, 4);
    expect(ungrouped.map((row) => [row.key, row.aggregateKey])).toEqual([
      [1, null],
      [2, null],
      [3, null],
    ]);
  });
});

describe("pruning and layout validation (#654)", () => {
  it("drops a group that lost its last child and clears dangling keys", () => {
    const state = editorState({
      lines: [line({ key: 1, aggregateKey: 0 }), line({ key: 2, aggregateKey: 9 })],
      aggregateGroups: [group({ key: 0 }), group({ key: 1 })],
      nextAggregateKey: 2,
    });
    const pruned = pruneEmptyAggregates(state);
    expect(pruned.aggregateGroups.map((entry) => entry.key)).toEqual([0]);
    expect(pruned.lines.map((row) => row.aggregateKey)).toEqual([0, null]);
    expect(pruned.nextAggregateKey).toBe(2);
  });

  it("returns the same state when nothing needs pruning", () => {
    const state = editorState({
      lines: [line({ key: 1, aggregateKey: 0 })],
      aggregateGroups: [group({ key: 0 })],
    });
    expect(pruneEmptyAggregates(state)).toBe(state);
  });

  it("accepts a valid contiguous layout", () => {
    const state = editorState({
      lines: [
        line({ key: 1, aggregateKey: 0 }),
        line({ key: 2, aggregateKey: 0 }),
        line({ key: 3 }),
      ],
      aggregateGroups: [group({ key: 0 })],
    });
    expect(validateAggregateLayout(state, USD)).toBeNull();
  });

  it("names a missing title, quantity and unit label", () => {
    expect(
      validateAggregateLayout(
        editorState({
          lines: [line({ key: 1, aggregateKey: 0 })],
          aggregateGroups: [group({ key: 0, title: "  " })],
        }),
        USD
      )
    ).toBe("Aggregate 1: title is required.");
    expect(
      validateAggregateLayout(
        editorState({
          lines: [line({ key: 1, aggregateKey: 0 })],
          aggregateGroups: [group({ key: 0, orderedQuantity: "0" })],
        }),
        USD
      )
    ).toBe(
      "Aggregate 1: display quantity must be positive with up to 4 decimal places and within the supported range."
    );
    expect(
      validateAggregateLayout(
        editorState({
          lines: [line({ key: 1, aggregateKey: 0 })],
          aggregateGroups: [group({ key: 0, orderedUoM: " " })],
        }),
        USD
      )
    ).toBe("Aggregate 1: unit label is required.");
  });

  it("bounds the title and the unit label in code points", () => {
    const longTitle = editorState({
      lines: [line({ key: 1, aggregateKey: 0 })],
      aggregateGroups: [group({ key: 0, title: "T".repeat(501) })],
    });
    expect(validateAggregateLayout(longTitle, USD)).toBe(
      "Aggregate 1: the title must be 500 characters or fewer."
    );
    const longUoM = editorState({
      lines: [line({ key: 1, aggregateKey: 0 })],
      aggregateGroups: [group({ key: 0, orderedUoM: "U".repeat(81) })],
    });
    expect(validateAggregateLayout(longUoM, USD)).toBe(
      "Aggregate 1: the unit label must be 80 characters or fewer."
    );
  });

  it("refuses a group with no child lines", () => {
    expect(
      validateAggregateLayout(
        editorState({ lines: [line({ key: 1 })], aggregateGroups: [group({ key: 0 })] }),
        USD
      )
    ).toBe("Aggregate 1: add at least one line to the aggregate.");
  });

  it("refuses a dangling membership and non-contiguous children", () => {
    expect(
      validateAggregateLayout(
        editorState({
          lines: [line({ key: 1 }), line({ key: 2, aggregateKey: 9 })],
          aggregateGroups: [group({ key: 0 })],
        }),
        USD
      )
    ).toBe("Line 2: belongs to an aggregate that no longer exists.");
    expect(
      validateAggregateLayout(
        editorState({
          lines: [
            line({ key: 1, aggregateKey: 0 }),
            line({ key: 2 }),
            line({ key: 3, aggregateKey: 0 }),
          ],
          aggregateGroups: [group({ key: 0 })],
        }),
        USD
      )
    ).toBe("Aggregate 1: its lines must stay together; line 3 does not follow line 1.");
  });

  it("refuses half a split package inside one aggregate", () => {
    const state = editorState({
      lines: [
        line({ key: 1, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 1, aggregateKey: 0 }),
        line({ key: 2, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 2 }),
      ],
      aggregateGroups: [group({ key: 0 })],
    });
    expect(validateAggregateLayout(state, USD)).toBe(
      'Lines 1 and 2: split line "q:2" must stay in one aggregate.'
    );
  });

  it("names the child that cannot be totalled", () => {
    const state = editorState({
      lines: [line({ key: 1, aggregateKey: 0 }), line({ key: 2, quantity: "abc", aggregateKey: 0 })],
      aggregateGroups: [group({ key: 0 })],
    });
    expect(validateAggregateLayout(state, USD)).toBe(
      "Aggregate 1: line 2 cannot be totalled; check its quantity and price."
    );
  });
});

describe("aggregate save payload (#654)", () => {
  it("keeps group order and reports each group's 1-based item index", () => {
    const groups = [group({ key: 7, title: "  First  " }), group({ key: 4, title: "Second" })];
    const lines = [
      line({ key: 1, aggregateKey: 7 }),
      line({ key: 2, aggregateKey: 7 }),
      line({ key: 3, aggregateKey: 4 }),
    ];
    const payload = buildAggregatePayload(groups, lines);
    expect(payload.error).toBeNull();
    expect(payload.groups).toEqual([
      { Title: "First", OrderedQuantityScaled: 10000, OrderedUoM: "Lot", ShowChildrenToCustomer: false },
      { Title: "Second", OrderedQuantityScaled: 10000, OrderedUoM: "Lot", ShowChildrenToCustomer: false },
    ]);
    expect(payload.indexByGroupKey.get(7)).toBe(1);
    expect(payload.indexByGroupKey.get(4)).toBe(2);
  });

  it("returns an empty array for an order with no aggregates", () => {
    const payload = buildAggregatePayload([], [line({ key: 1 })]);
    expect(payload.error).toBeNull();
    expect(payload.groups).toEqual([]);
    expect(payload.indexByGroupKey.size).toBe(0);
  });

  it("keeps mixed-depth display groups in one aggregate position", () => {
    const groups = [group({ key: 0, showChildrenToCustomer: false })];
    const lines = [
      line({ key: 1, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 1, aggregateKey: 0 }),
      line({ key: 2, groupKey: "q:2", quotationItemNo: 2, quotationSubNo: 2, aggregateKey: 0 }),
      line({ key: 3, aggregateKey: 0 }),
    ];
    const payload = buildAggregatePayload(groups, lines);
    expect(payload.error).toBeNull();
    expect(payload.groups).toHaveLength(1);
    expect(payload.groups[0].ShowChildrenToCustomer).toBe(false);
    expect(lines.map((row) => payload.indexByGroupKey.get(row.aggregateKey!))).toEqual([1, 1, 1]);
  });

  it("refuses the same layout problems the editor gate names", () => {
    expect(buildAggregatePayload([group({ key: 0, title: "" })], [line({ key: 1, aggregateKey: 0 })]).error).toBe(
      "Aggregate 1: title is required."
    );
    expect(buildAggregatePayload([group({ key: 0 })], [line({ key: 1 })]).error).toBe(
      "Aggregate 1: add at least one line to the aggregate."
    );
  });
});

