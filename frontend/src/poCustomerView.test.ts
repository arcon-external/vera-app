import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import {
  calculatePurchaseOrder,
  initialPurchaseOrderEditorState,
  type PurchaseOrderAggregateGroupState,
  type PurchaseOrderEditorState,
  type PurchaseOrderLine,
} from "./purchaseOrder";
import { customerPOHeaderOf, projectCustomerRows } from "./poCustomerView";

// Issue #654: the customer-facing projection of a purchase order.
//
// The projection is what staff can show and print from the workspace. It walks
// the entered rows in document order, wraps each aggregate in one header row
// and, only when the aggregate's stored toggle says so, emits the child tree.
// Nothing internal — supplier cost, margin, variance, the quoted baseline or
// the remaining quantity — may exist on a projected row.
//
// The specs read line numbers the way the store does: array order is
// `line_number` order, and a split line is one parent heading with priced
// sub-items beneath it.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
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
    lineKind: "quotation",
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

/** One plain line: its own row, bullets attached, exact net amount. */
function plainRows() {
  return [
    line({
      key: 1,
      description: "Widget",
      quantity: "2",
      uom: "Pc",
      unitPrice: "$10.00",
      descriptionLines: ["First bullet", "Second bullet"],
    }),
  ];
}

/** One split line: a parent heading, two priced sub-items, sub-sub bullets. */
function splitLines() {
  return [
    line({
      key: 1,
      description: "Frame",
      quantity: "2",
      uom: "Pc",
      unitPrice: "$10.00",
      groupKey: "q:1",
      parentDescription: "Welded frame",
    }),
    line({
      key: 2,
      description: "Panel",
      quantity: "1",
      uom: "Pc",
      unitPrice: "$5.00",
      groupKey: "q:1",
      parentDescription: "Welded frame",
      descriptionLines: ["Mild steel", "Powder coated"],
    }),
  ];
}

describe("projectCustomerRows: ungrouped rows", () => {
  it("emits a plain line as one depth-0 row with its bullets and exact amount", () => {
    expect(projectCustomerRows(plainRows(), [], USD)).toEqual([
      {
        kind: "line",
        depth: 0,
        description: "Widget",
        descriptionLines: ["First bullet", "Second bullet"],
        quantity: "2",
        uom: "Pc",
        unitPriceMinor: 1000,
        amountMinor: 2000,
      },
    ]);
  });

  it("emits a split line as a parent heading, priced sub-items and depth-2 sub-sub bullets", () => {
    expect(projectCustomerRows(splitLines(), [], USD)).toEqual([
      {
        kind: "line",
        depth: 0,
        description: "Welded frame",
        descriptionLines: [],
        quantity: "",
        uom: "",
        unitPriceMinor: null,
        amountMinor: 2500,
      },
      {
        kind: "line",
        depth: 1,
        description: "Frame",
        descriptionLines: [],
        quantity: "2",
        uom: "Pc",
        unitPriceMinor: 1000,
        amountMinor: 2000,
      },
      {
        kind: "line",
        depth: 1,
        description: "Panel",
        descriptionLines: [],
        quantity: "1",
        uom: "Pc",
        unitPriceMinor: 500,
        amountMinor: 500,
      },
      {
        kind: "line",
        depth: 2,
        description: "Mild steel",
        descriptionLines: [],
        quantity: "",
        uom: "",
        unitPriceMinor: null,
        amountMinor: 0,
      },
      {
        kind: "line",
        depth: 2,
        description: "Powder coated",
        descriptionLines: [],
        quantity: "",
        uom: "",
        unitPriceMinor: null,
        amountMinor: 0,
      },
    ]);
  });

  it("keeps rows in line_number order across an ungrouped line set", () => {
    const rows = projectCustomerRows(
      [
        line({ key: 1, description: "First" }),
        line({ key: 2, description: "Second" }),
        line({ key: 3, description: "Third" }),
      ],
      [],
      USD
    );
    expect(rows.map((row) => (row.kind === "line" ? row.description : row.title))).toEqual([
      "First",
      "Second",
      "Third",
    ]);
  });
});

describe("projectCustomerRows: aggregates", () => {
  it("emits one aggregate row and no child description at all when the toggle is off", () => {
    const rows = projectCustomerRows(
      [
        line({ key: 1, description: "Widget", quantity: "2", unitPrice: "$10.00", aggregateKey: 0 }),
        line({ key: 2, description: "Spare", quantity: "1", unitPrice: "$5.00", aggregateKey: 0 }),
      ],
      [group({ key: 0, title: "Combined items", orderedQuantity: "3", orderedUoM: "Pc" })],
      USD
    );
    expect(rows).toHaveLength(1);
    expect(rows[0]).toEqual({
      kind: "aggregate",
      title: "Combined items",
      quantity: "3",
      uom: "Pc",
      unitPriceMinor: 833,
      totalMinor: 2500,
      children: null,
    });
    // Title-only is absolute: neither child description may appear anywhere,
    // including inside a hidden child list.
    const serialized = JSON.stringify(rows);
    expect(serialized).not.toContain("Widget");
    expect(serialized).not.toContain("Spare");
  });

  it("emits the child tree at depth 0/1/2 when the toggle is on", () => {
    const rows = projectCustomerRows(
      [
        line({
          key: 1,
          description: "Widget",
          quantity: "2",
          unitPrice: "$10.00",
          aggregateKey: 0,
          descriptionLines: ["Item scope"],
        }),
        ...splitLines().map((entry) => ({ ...entry, key: entry.key + 10, aggregateKey: 0 })),
      ],
      [group({ key: 0, title: "Combined items", orderedQuantity: "2", orderedUoM: "Lot", showChildrenToCustomer: true })],
      USD
    );
    expect(rows).toHaveLength(1);
    const aggregate = rows[0];
    if (aggregate.kind !== "aggregate") throw new Error("expected an aggregate row");
    expect(aggregate.totalMinor).toBe(4500);
    expect(aggregate.children).toEqual([
      {
        kind: "line",
        depth: 0,
        description: "Widget",
        descriptionLines: ["Item scope"],
        quantity: "2",
        uom: "Pc",
        unitPriceMinor: 1000,
        amountMinor: 2000,
      },
      {
        kind: "line",
        depth: 0,
        description: "Welded frame",
        descriptionLines: [],
        quantity: "",
        uom: "",
        unitPriceMinor: null,
        amountMinor: 2500,
      },
      {
        kind: "line",
        depth: 1,
        description: "Frame",
        descriptionLines: [],
        quantity: "2",
        uom: "Pc",
        unitPriceMinor: 1000,
        amountMinor: 2000,
      },
      {
        kind: "line",
        depth: 1,
        description: "Panel",
        descriptionLines: [],
        quantity: "1",
        uom: "Pc",
        unitPriceMinor: 500,
        amountMinor: 500,
      },
      {
        kind: "line",
        depth: 2,
        description: "Mild steel",
        descriptionLines: [],
        quantity: "",
        uom: "",
        unitPriceMinor: null,
        amountMinor: 0,
      },
      {
        kind: "line",
        depth: 2,
        description: "Powder coated",
        descriptionLines: [],
        quantity: "",
        uom: "",
        unitPriceMinor: null,
        amountMinor: 0,
      },
    ]);
    const depths = aggregate.children?.map((child) => child.depth) ?? [];
    expect(new Set(depths)).toEqual(new Set([0, 1, 2]));
  });

  it("places each aggregate at its first child and leaves interleaved lines in order", () => {
    const rows = projectCustomerRows(
      [
        line({ key: 1, description: "Before" }),
        line({ key: 2, description: "Grouped A", quantity: "1", unitPrice: "$10.00", aggregateKey: 0 }),
        line({ key: 3, description: "Grouped B", quantity: "1", unitPrice: "$5.00", aggregateKey: 0 }),
        line({ key: 4, description: "Between" }),
        line({ key: 5, description: "Second group child", quantity: "1", unitPrice: "$7.00", aggregateKey: 1 }),
        line({ key: 6, description: "After" }),
      ],
      [
        group({ key: 0, title: "First aggregate", orderedQuantity: "2" }),
        group({ key: 1, title: "Second aggregate", orderedQuantity: "1" }),
      ],
      USD
    );
    expect(
      rows.map((row) => (row.kind === "line" ? row.description : row.title))
    ).toEqual([
      "Before",
      "First aggregate",
      "Between",
      "Second aggregate",
      "After",
    ]);
  });

  it("emits one aggregate row for one group even when its children are not contiguous", () => {
    const rows = projectCustomerRows(
      [
        line({ key: 1, description: "Grouped A", aggregateKey: 0 }),
        line({ key: 2, description: "Interleaved" }),
        line({ key: 3, description: "Grouped B", aggregateKey: 0 }),
      ],
      [group({ key: 0, title: "One aggregate" })],
      USD
    );
    expect(rows.filter((row) => row.kind === "aggregate")).toHaveLength(1);
  });

  it("never invents a header for a group with no lines", () => {
    const rows = projectCustomerRows([line({ key: 1, description: "Plain" })], [
      group({ key: 5, title: "No children" }),
    ], USD);
    expect(rows).toHaveLength(1);
    expect(rows[0].kind).toBe("line");
  });

  it("treats a membership that names no known group as an ordinary line", () => {
    const rows = projectCustomerRows(
      [line({ key: 1, description: "Dangling", aggregateKey: 9 })],
      [],
      USD
    );
    expect(rows).toEqual([
      {
        kind: "line",
        depth: 0,
        description: "Dangling",
        descriptionLines: [],
        quantity: "1",
        uom: "Pc",
        unitPriceMinor: 100,
        amountMinor: 100,
      },
    ]);
  });

  it("renders every entered line exactly once when the child trees are shown", () => {
    const lines = [
      line({ key: 1, description: "Standalone" }),
      line({ key: 2, description: "Grouped plain", aggregateKey: 0 }),
      line({ key: 3, description: "Package part A", groupKey: "q:1", parentDescription: "Package", aggregateKey: 1 }),
      line({ key: 4, description: "Package part B", groupKey: "q:1", parentDescription: "Package", aggregateKey: 1 }),
      line({ key: 5, description: "Grouped package", groupKey: "q:2", parentDescription: "Second package", aggregateKey: 1 }),
    ];
    const rows = projectCustomerRows(
      lines,
      [
        group({ key: 0, showChildrenToCustomer: true }),
        group({ key: 1, showChildrenToCustomer: true }),
      ],
      USD
    );
    const seen: string[] = [];
    const walk = (row: (typeof rows)[number]) => {
      if (row.kind === "aggregate") {
        for (const child of row.children ?? []) seen.push(child.description);
        return;
      }
      seen.push(row.description);
    };
    rows.forEach(walk);
    for (const entry of lines) {
      expect(seen.filter((value) => value === entry.description)).toHaveLength(1);
    }
  });
});

describe("projectCustomerRows: exact money", () => {
  it("amounts are the row net: gross minus the row discount", () => {
    const rows = projectCustomerRows(
      [
        line({
          key: 1,
          quantity: "2",
          unitPrice: "$10.00",
          discountEnabled: true,
          discountPercent: "10",
          aggregateKey: 0,
        }),
      ],
      [group({ key: 0, title: "Discounted", orderedQuantity: "2" })],
      USD
    );
    expect(rows[0]).toMatchObject({
      kind: "aggregate",
      totalMinor: 1800,
      unitPriceMinor: 900,
    });
  });

  it("derives the display unit price with round-half-away, and keeps the exact total authoritative", () => {
    // $0.01 over a display quantity of 2 is half a minor unit: rounds away.
    const half = projectCustomerRows(
      [line({ key: 1, quantity: "1", unitPrice: "$0.01", aggregateKey: 0 })],
      [group({ key: 0, title: "Half", orderedQuantity: "2" })],
      USD
    );
    expect(half[0]).toMatchObject({ totalMinor: 1, unitPriceMinor: 1 });

    // $1.00 over 3 is 33.33…: the total is 100 and the unit price is 33, so
    // 33 × 3 = 99 must never be mistaken for the order's value.
    const third = projectCustomerRows(
      [line({ key: 1, quantity: "1", unitPrice: "$1.00", aggregateKey: 0 })],
      [group({ key: 0, title: "Third", orderedQuantity: "3" })],
      USD
    );
    expect(third[0]).toMatchObject({ totalMinor: 100, unitPriceMinor: 33 });
  });

  it("reports an out-of-range derived unit price as null rather than an inexact number", () => {
    // $2 quadrillion over a display quantity of 0.0001 needs 2×10^19 minor
    // units per unit: wider than the exact range, so the derivation refuses
    // rather than printing an inexact figure. The total itself is in range.
    const rows = projectCustomerRows(
      [
        line({
          key: 1,
          quantity: "1",
          unitPrice: "$20,000,000,000,000.00",
          aggregateKey: 0,
        }),
      ],
      [group({ key: 0, title: "Huge", orderedQuantity: "0.0001" })],
      USD
    );
    expect(rows[0]).toMatchObject({
      kind: "aggregate",
      totalMinor: 2_000_000_000_000_000,
      unitPriceMinor: null,
    });
  });

  it("reports an unparseable display quantity as a null unit price and still totals the children", () => {
    const rows = projectCustomerRows(
      [
        line({ key: 1, quantity: "1", unitPrice: "$4.00", aggregateKey: 0 }),
        line({ key: 2, quantity: "1", unitPrice: "$6.00", aggregateKey: 0 }),
      ],
      [group({ key: 0, title: "No quantity", orderedQuantity: "" })],
      USD
    );
    expect(rows[0]).toMatchObject({
      totalMinor: 1000,
      unitPriceMinor: null,
      quantity: "",
    });
  });

  it("carries a null amount, never a fabricated zero, for a line that cannot be totalled", () => {
    // Only reachable from a draft the editor refuses to save: the view gates
    // the projection on a complete calculation, so this row never prints. A
    // zero here would still be a lie, so the projection keeps it unknown.
    const rows = projectCustomerRows(
      [line({ key: 1, description: "Pending", unitPrice: "", aggregateKey: null })],
      [],
      USD
    );
    expect(rows[0]).toMatchObject({ unitPriceMinor: null, amountMinor: null });
  });

  it("keeps the group total unknown when one child cannot be totalled, never a partial sum", () => {
    const rows = projectCustomerRows(
      [
        line({ key: 1, quantity: "1", unitPrice: "$4.00", aggregateKey: 0 }),
        line({ key: 2, quantity: "abc", unitPrice: "$6.00", aggregateKey: 0 }),
      ],
      [group({ key: 0, title: "Partly unknown", orderedQuantity: "2" })],
      USD
    );
    expect(rows[0]).toMatchObject({
      kind: "aggregate",
      totalMinor: null,
      unitPriceMinor: null,
    });
  });

  it("keeps a split package heading unknown when one of its sub-items cannot be totalled", () => {
    const rows = projectCustomerRows(
      [
        line({
          key: 1,
          description: "Frame",
          quantity: "1",
          unitPrice: "$10.00",
          groupKey: "q:1",
          parentDescription: "Welded frame",
        }),
        line({
          key: 2,
          description: "Panel",
          quantity: "1",
          unitPrice: "",
          groupKey: "q:1",
          parentDescription: "Welded frame",
        }),
      ],
      [],
      USD
    );
    expect(rows[0]).toMatchObject({ depth: 0, amountMinor: null });
    expect(rows[1]).toMatchObject({ depth: 1, amountMinor: 1000 });
    expect(rows[2]).toMatchObject({ depth: 1, amountMinor: null });
  });
});

describe("projectCustomerRows: nothing internal crosses the boundary", () => {
  const INTERNAL_WORDS = [
    "cost",
    "margin",
    "variance",
    "pph",
    "quoted",
    "remaining",
    "profit",
  ];

  function projected() {
    return projectCustomerRows(
      [
        line({
          key: 1,
          description: "Widget",
          quantity: "2",
          unitPrice: "$10.00",
          aggregateKey: 0,
          quotedUnitPriceMinor: 900,
          remainingQuantityScaled: 5000,
        }),
        ...splitLines().map((entry) => ({
          ...entry,
          key: entry.key + 10,
          aggregateKey: 0,
          quotedUnitPriceMinor: 950,
          remainingQuantityScaled: 10000,
        })),
      ],
      [group({ key: 0, showChildrenToCustomer: true })],
      USD
    );
  }

  it("exposes no cost, margin, variance, quoted baseline or remaining quantity field", () => {
    const serialized = JSON.stringify(projected()).toLowerCase();
    for (const word of INTERNAL_WORDS) expect(serialized).not.toContain(word);
    for (const row of projected()) {
      for (const field of Object.keys(row)) {
        for (const word of INTERNAL_WORDS) expect(field.toLowerCase()).not.toContain(word);
      }
      if (row.kind !== "aggregate" || row.children === null) continue;
      for (const child of row.children) {
        for (const field of Object.keys(child)) {
          for (const word of INTERNAL_WORDS) {
            expect(field.toLowerCase()).not.toContain(word);
          }
        }
      }
    }
  });

  it("projects the saved record and the pre-save draft to the same rows and totals", () => {
    const saved = initialPurchaseOrderEditorState(
      {
        PODate: "2026-11-20",
        RequestedDeliveryDate: null,
        UUID: "uuid-parity",
        State: "saved",
        QuoteNumber: "1502260100",
        CustomerName: "PT. Parity",
        BillingAddress: "",
        DeliveryAddress: "",
        ShipToCode: "",
        ContactEmail: "",
        ContactPhone: "",
        PONumber: "PO-PARITY",
        ReceivedDate: "2026-11-21",
        Incoterms: "CIF",
        Currency: "USD",
        TaxZone: "FTZ",
        TaxCategory: "",
        AdjustmentReason: "",
        CreatedAt: "2026-11-21T08:00:00Z",
        UpdatedAt: "2026-11-21T08:00:00Z",
        Items: [
          {
            ID: 11,
            LineNumber: 1,
            Description: "Widget",
            QuantityScaled: 20000,
            UoM: "Pc",
            UnitPriceMinor: 1000,
            DiscountEnabled: false,
            DiscountPercent: 0,
            GrossMinor: 2000,
            DiscountMinor: 0,
            NetMinor: 2000,
            AggregateGroupID: 7,
          },
          {
            ID: 12,
            LineNumber: 2,
            Description: "Frame",
            QuantityScaled: 20000,
            UoM: "Pc",
            UnitPriceMinor: 1000,
            DiscountEnabled: false,
            DiscountPercent: 0,
            GrossMinor: 2000,
            DiscountMinor: 0,
            NetMinor: 2000,
            GroupKey: "q:1",
            ParentDescription: "Welded frame",
            AggregateGroupID: 7,
          },
          {
            ID: 13,
            LineNumber: 3,
            Description: "Panel",
            QuantityScaled: 10000,
            UoM: "Pc",
            UnitPriceMinor: 500,
            DiscountEnabled: true,
            DiscountPercent: 10,
            GrossMinor: 500,
            DiscountMinor: 50,
            NetMinor: 450,
            GroupKey: "q:1",
            ParentDescription: "Welded frame",
            DescriptionLines: ["Mild steel"],
            AggregateGroupID: 7,
          },
        ],
        ID: 100,
        QuotationID: 44,
        SourceRevision: 3,
        CustomerID: 9,
        PaymentTerms: "30 days",
        DiscountPercent: 0,
        PpnBps: 1100,
        SubtotalMinor: 4450,
        DiscountMinor: 0,
        PpnMinor: 489,
        AdjustmentMinor: 0,
        GrandTotalMinor: 4939,
        PrintedSubtotalMinor: 4450,
        PrintedDiscountMinor: 0,
        PrintedPpnMinor: 489,
        PrintedGrandTotalMinor: 4939,
        Version: 2,
        AggregateGroups: [
          {
            ID: 7,
            DisplayOrder: 1,
            Title: "Combined items",
            OrderedQuantityScaled: 20000,
            OrderedUoM: "Lot",
            ShowChildrenToCustomer: true,
            TotalMinor: 4450,
            DisplayUnitPriceMinor: 2225,
          },
        ],
      },
      USD
    );

    // The confirmation path: the applied editor state, built from the values
    // the review will submit. Same numbers, no persisted ids.
    const draft: PurchaseOrderEditorState = {
      ...saved,
      lines: saved.lines.map((entry) => ({ ...entry })),
      aggregateGroups: saved.aggregateGroups.map((entry) => ({ ...entry })),
    };

    expect(projectCustomerRows(draft.lines, draft.aggregateGroups, USD)).toEqual(
      projectCustomerRows(saved.lines, saved.aggregateGroups, USD)
    );
    expect(calculatePurchaseOrder(draft, USD).totals).toEqual({
      grossMinor: 4500,
      rowDiscountMinor: 50,
      subtotalMinor: 4450,
      discountMinor: 0,
      afterDiscountMinor: 4450,
      ppnMinor: 489,
      grandTotalMinor: 4939,
    });
  });
});

describe("customerPOHeaderOf", () => {
  const source = {
    PONumber: "PO-1",
    CustomerName: "PT. Buyer",
    QuoteNumber: "1502260100",
    SourceRevision: 2,
    PODate: "2026-11-20",
    ReceivedDate: "2026-11-21",
    RequestedDeliveryDate: null,
    PaymentTerms: "  30 days  ",
    Incoterms: "",
  };

  it("reads an absent date as absent and trims the free-text terms", () => {
    expect(customerPOHeaderOf(source)).toEqual({
      poNumber: "PO-1",
      customerName: "PT. Buyer",
      poDate: "2026-11-20",
      receivedDate: "2026-11-21",
      requestedDeliveryDate: null,
      quoteNumber: "1502260100",
      sourceRevision: 2,
      paymentTerms: "30 days",
      incoterms: "",
    });
  });

  it("names a missing PO number rather than printing an empty document title", () => {
    expect(customerPOHeaderOf({ ...source, PONumber: "   " }).poNumber).toBe(
      "No PO number"
    );
  });

  it("normalizes the same order identically from a record and an applied payload", () => {
    const payload = {
      PONumber: source.PONumber,
      PODate: source.PODate,
      ReceivedDate: source.ReceivedDate,
      RequestedDeliveryDate: source.RequestedDeliveryDate,
      PaymentTerms: source.PaymentTerms,
      Incoterms: source.Incoterms,
    };
    expect(customerPOHeaderOf({ ...source, ...payload })).toEqual(
      customerPOHeaderOf(source)
    );
  });
});
