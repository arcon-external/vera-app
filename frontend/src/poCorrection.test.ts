import { describe, expect, it } from "vitest";
import {
  attachmentReferenceLabel,
  canCorrectPurchaseOrder,
  conflictActorName,
  conflictNeedsCorrection,
  conflictWarning,
  correctionKindLabel,
  correctionReasonError,
  formatHistoryTime,
  historyLineRows,
  trimCorrectionReason,
  writeOutcome,
  type PurchaseOrderConflictDTO,
  type PurchaseOrderHistoryItem,
} from "./poCorrection";
import type { PurchaseOrderDTO } from "./purchaseOrder";

const CONFLICT: PurchaseOrderConflictDTO = {
  CurrentState: "draft",
  CurrentVersion: 5,
  UpdatedBy: 11,
  UpdatedByName: "Sari",
  UpdatedAt: "2026-10-21T10:15:00+07:00",
};

function order(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    UUID: "uuid-473",
    State: "saved",
    QuoteNumber: "1502260012",
    CustomerName: "PT. Buyer",
    BillingAddress: "",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "PO-473",
    ReceivedDate: "2026-10-02",
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-02T10:00:00Z",
    UpdatedAt: "2026-10-02T10:00:00Z",
    Items: [],
    ID: 7,
    QuotationID: 12,
    SourceRevision: 1,
    CustomerID: 2,
    PaymentTerms: "30 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
    Version: 4,
    ...overrides,
  };
}

describe("correction reason", () => {
  it("requires more than whitespace, using the shared trim set", () => {
    expect(correctionReasonError("")).toBe("A correction needs a reason.");
    expect(correctionReasonError("   \t\n")).toBe("A correction needs a reason.");
    // U+0085 is whitespace to Go and Postgres; U+FEFF is content.
    expect(correctionReasonError("\u0085")).toBe("A correction needs a reason.");
    expect(correctionReasonError("\uFEFF")).toBeNull();
  });

  it("trims the same characters on both sides and keeps internal spacing", () => {
    expect(trimCorrectionReason("  customer moved  ")).toBe("customer moved");
    expect(trimCorrectionReason("customer  moved")).toBe("customer  moved");
  });

  it("bounds the reason to 500 code points, not UTF-16 units", () => {
    expect(correctionReasonError("a".repeat(500))).toBeNull();
    expect(correctionReasonError("a".repeat(501))).toContain("500");
    // An astral character is one code point for Go and Postgres, two for
    // JavaScript's String.length; 300 of them are within the bound.
    expect(correctionReasonError("😀".repeat(300))).toBeNull();
    expect(correctionReasonError("😀".repeat(501))).toContain("500");
  });
});

describe("write outcome", () => {
  it("prefers the saved record", () => {
    const outcome = writeOutcome({ Order: order(), Conflict: null });
    expect(outcome.kind).toBe("saved");
  });

  it("treats a conflict as an answer, not a failure", () => {
    const outcome = writeOutcome({ Order: null, Conflict: CONFLICT });
    expect(outcome).toEqual({ kind: "conflict", conflict: CONFLICT });
  });

  it("reports an empty result as such rather than inventing a save", () => {
    expect(writeOutcome({ Order: null, Conflict: null }).kind).toBe("empty");
    expect(writeOutcome({}).kind).toBe("empty");
  });

  it("treats a saved-record refusal as the correction-flow answer", () => {
    expect(
      writeOutcome({ Order: null, Conflict: null, SavedRecord: true })
    ).toEqual({ kind: "savedRecord" });
  });
});

describe("conflict wording", () => {
  it("names who saved, when and which version is current", () => {
    const warning = conflictWarning(CONFLICT);
    expect(warning).toContain("Sari");
    expect(warning).toContain("version 5");
    expect(warning).toContain("2026-10-21 10:15 +07:00");
    expect(warning).toContain("keeps the previous version in the history");
  });

  it("knows when a draft write must offer a correction instead of an overwrite", () => {
    expect(conflictNeedsCorrection(CONFLICT)).toBe(false);
    expect(conflictNeedsCorrection({ ...CONFLICT, CurrentState: "saved" })).toBe(true);
  });

  it("never promises an overwrite that the server would refuse", () => {
    const warning = conflictWarning({ ...CONFLICT, CurrentState: "saved" }, true);
    expect(warning).toContain("cannot be overwritten as a draft");
    expect(warning).toContain("requires a reason");
    expect(warning).not.toContain("Overwriting applies");
  });

  it("never claims a name it was not given", () => {
    expect(conflictActorName({ ...CONFLICT, UpdatedByName: "   " })).toBe("Someone else");
  });

  it("formats the server time deterministically without the machine locale", () => {
    expect(formatHistoryTime("2026-10-21T10:15:00+07:00")).toBe("2026-10-21 10:15 +07:00");
    expect(formatHistoryTime("2026-10-21T03:15:00.123Z")).toBe("2026-10-21 03:15 UTC");
    expect(formatHistoryTime("")).toBe("");
    expect(formatHistoryTime("not-a-date")).toBe("not-a-date");
  });
});

describe("history labels", () => {
  it("distinguishes a correction, a recovered draft and a source repair", () => {
    expect(correctionKindLabel("correction")).toBe("Correction");
    expect(correctionKindLabel("draft_overwrite")).toBe("Draft overwritten");
    expect(correctionKindLabel("source_repair")).toBe("Source repaired");
    expect(correctionKindLabel("something-new")).toBe("Correction");
  });

  it("renders the attachment reference with its version and short sha", () => {
    const label = attachmentReferenceLabel({
      Version: 2,
      Filename: "Customer PO 4500098123.pdf",
      SHA256: "d2f4a6c8e0b1".padEnd(64, "0"),
    });
    expect(label).toContain("v2");
    expect(label).toContain("Customer PO 4500098123.pdf");
    expect(label).toContain("sha256 d2f4a6c8e0b1…");
    expect(attachmentReferenceLabel(null)).toContain("No customer PO PDF");
  });

  it("offers correction only on a saved record", () => {
    expect(canCorrectPurchaseOrder(order())).toBe(true);
    expect(canCorrectPurchaseOrder(order({ State: "draft" }))).toBe(false);
  });
});

function historyItem(
  lineNumber: number,
  description: string,
  aggregateGroupOrder = 0
): PurchaseOrderHistoryItem {
  return {
    LineNumber: lineNumber,
    Description: description,
    QuantityScaled: 10000,
    UoM: "Pc",
    UnitPriceMinor: 10000,
    DiscountMinor: 0,
    NetMinor: 10000,
    AggregateGroupOrder: aggregateGroupOrder,
  };
}

function rowLabels(rows: ReturnType<typeof historyLineRows>): string[] {
  return rows.map((row) =>
    row.kind === "group"
      ? `group ${row.group.DisplayOrder}:${row.group.Title}`
      : `line ${row.item.LineNumber}:${row.item.Description}`
  );
}

describe("history line rows (issue #654)", () => {
  it("leaves an ungrouped snapshot as plain rows in the order the snapshot stored", () => {
    const items = [
      historyItem(3, "Third"),
      historyItem(1, "First"),
      historyItem(2, "Second"),
    ];
    expect(rowLabels(historyLineRows({ Items: items }))).toEqual([
      "line 3:Third",
      "line 1:First",
      "line 2:Second",
    ]);
    // The binding sends an empty array, not undefined, for a snapshot with no
    // aggregate groups.
    expect(rowLabels(historyLineRows({ Items: items, Groups: [] }))).toEqual([
      "line 3:Third",
      "line 1:First",
      "line 2:Second",
    ]);
  });

  it("puts each group title immediately above its children, matched by display order", () => {
    // The group array is deliberately out of order: the snapshot stores the
    // order on the header and on each child, so the array's order is not
    // meaningful. The leading ungrouped line must stay where the snapshot put
    // it, not move below the groups.
    const rows = historyLineRows({
      Groups: [
        {
          DisplayOrder: 2,
          Title: "Second package",
          OrderedQuantityScaled: 30000,
          OrderedUoM: "Set",
          ShowChildrenToCustomer: true,
        },
        {
          DisplayOrder: 1,
          Title: "First package",
          OrderedQuantityScaled: 20000,
          OrderedUoM: "Pallet",
          ShowChildrenToCustomer: false,
        },
      ],
      Items: [
        historyItem(1, "Free line"),
        historyItem(2, "Child alpha", 1),
        historyItem(3, "Child beta", 1),
        historyItem(4, "Child gamma", 2),
      ],
    });
    expect(rowLabels(rows)).toEqual([
      "line 1:Free line",
      "group 1:First package",
      "line 2:Child alpha",
      "line 3:Child beta",
      "group 2:Second package",
      "line 4:Child gamma",
    ]);
  });

  it("never invents a group title the snapshot did not store", () => {
    const rows = historyLineRows({
      Groups: [
        {
          DisplayOrder: 1,
          Title: "Known package",
          OrderedQuantityScaled: 20000,
          OrderedUoM: "Pallet",
          ShowChildrenToCustomer: false,
        },
        {
          DisplayOrder: 2,
          Title: "Empty package",
          OrderedQuantityScaled: 10000,
          OrderedUoM: "Set",
          ShowChildrenToCustomer: true,
        },
      ],
      Items: [historyItem(1, "Orphan", 7), historyItem(2, "Known child", 1)],
    });
    // The orphan names a group the snapshot does not carry; the group with no
    // child lines has no heading to sit above. Neither invents a title.
    expect(rowLabels(rows)).toEqual([
      "line 1:Orphan",
      "group 1:Known package",
      "line 2:Known child",
    ]);
  });
});
