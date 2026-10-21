import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import {
  createExtractionEditTracker,
  EXTRACTION_LINES_KEY,
  extractionEditorChanges,
  resolveExtractionConflicts,
  resolvedExtractionSelection,
  type PurchaseOrderExtractionResultDTO,
  type PurchaseOrderExtractionRowDTO,
} from "./poExtraction";
import type { PurchaseOrderHeader, PurchaseOrderLine } from "./purchaseOrder";

// Issue #479: reconciling a late extraction result with edits staff made while
// it was processing. These are the pure decisions; the panel and the editor
// only render and apply them. Every rule in the ticket's matrix is pinned
// here, including the two that are easy to get wrong: a field staff cleared is
// never repopulated without a choice, and a value nobody touched is still a
// conflict when it already held a different value.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function header(overrides: Partial<PurchaseOrderHeader> = {}): PurchaseOrderHeader {
  return {
    billingAddress: "",
    deliveryAddress: "",
    shipToCode: "",
    contactEmail: "",
    contactPhone: "",
    poNumber: "",
    poDate: "",
    receivedDate: "2026-10-02",
    requestedDeliveryDate: "",
    paymentTerms: "",
    incoterms: "",
    discountPercent: "0",
    adjustment: "",
    adjustmentReason: "",
    ...overrides,
  };
}

function line(overrides: Partial<PurchaseOrderLine> = {}): PurchaseOrderLine {
  return {
    key: 0,
    description: "Quotation line",
    quantity: "2",
    uom: "Pc",
    unitPrice: "$1,250.00",
    discountEnabled: false,
    discountPercent: "0",
    quotationItemNo: 1,
    quotationSubNo: null,
    ...overrides,
  };
}

function cell(value: string, status: "extracted" | "needs_review" | "absent" = "extracted") {
  return {
    Value: value,
    Raw: value,
    Status: status,
    Page: status === "absent" ? null : 1,
    BBox: status === "absent" ? null : { X: 86, Y: 82, Width: 56, Height: 9 },
    Confidence: null,
    Note: "",
  };
}

function row(overrides: Partial<PurchaseOrderExtractionRowDTO> = {}): PurchaseOrderExtractionRowDTO {
  return {
    LineNumber: cell("1"),
    Description: cell("Extracted line"),
    Quantity: cell("2"),
    UoM: cell("Pc"),
    UnitPrice: cell("1250.00"),
    Amount: cell("2500.00"),
    Status: "extracted",
    Page: 1,
    BBox: { X: 50, Y: 225, Width: 490, Height: 10 },
    Note: "",
    ...overrides,
  };
}

function result(overrides: Partial<PurchaseOrderExtractionResultDTO> = {}): PurchaseOrderExtractionResultDTO {
  return {
    Engine: "stub",
    EngineVersion: "1.0",
    PageCount: 1,
    PagesWithText: 1,
    PagesWithoutText: 0,
    PagesWithOCR: 0,
    DurationMs: 9,
    SchemaVersion: 2,
    Pages: [{ Number: 1, Width: 595, Height: 842 }],
    PageModes: [
      { Number: 1, Mode: "text", TextWords: 20, TextChars: 180, OCRWords: 0, DPI: 0, Confidence: null },
    ],
    Fields: [
      { Key: "poNumber", Cell: cell("4500123456") },
      { Key: "poDate", Cell: cell("2026-10-12") },
      { Key: "requestedDeliveryDate", Cell: cell("", "absent") },
      { Key: "currency", Cell: cell("USD") },
      { Key: "paymentTerms", Cell: cell("30 days after delivery", "needs_review") },
      { Key: "incoterms", Cell: cell("CIF Batam") },
    ],
    Rows: [row()],
    Abstentions: { Fields: 2, Rows: 0 },
    ...overrides,
  };
}

function comparison(
  plan: ReturnType<typeof resolveExtractionConflicts>,
  headerKey: string
) {
  return plan.fields.find((field) => field.key === `header:${headerKey}`);
}

describe("extraction edit tracking", () => {
  it("records a header change, a clear, and a retained value as different kinds of edit", () => {
    const baselineHeader = header({ poNumber: "PO-1", paymentTerms: "45 days" });
    const tracker = createExtractionEditTracker(1, baselineHeader, [line()]);
    const changes = extractionEditorChanges(
      tracker,
      header({ poNumber: "PO-EDITED", paymentTerms: "" }),
      [line()]
    );
    expect(changes.headerChanged).toContain("poNumber");
    expect(changes.headerChanged).toContain("paymentTerms");
    expect(changes.headerCleared).toContain("paymentTerms");
    expect(changes.headerCleared).not.toContain("poNumber");
    expect(changes.anyLineChange).toBe(false);
  });

  it("records a line-cell edit and added or removed lines", () => {
    const tracker = createExtractionEditTracker(1, header(), [line(), line({ key: 1 })]);
    const edited = extractionEditorChanges(
      tracker,
      header(),
      [line({ description: "Typed by hand" }), line({ key: 1 })]
    );
    expect(edited.lineCellsChanged).toEqual([{ index: 0, field: "description" }]);
    expect(edited.anyLineChange).toBe(true);

    const added = extractionEditorChanges(tracker, header(), [line(), line({ key: 1 }), line({ key: 2 })]);
    expect(added.linesAdded).toEqual([2]);
    expect(added.anyLineChange).toBe(true);

    const removed = extractionEditorChanges(tracker, header(), [line()]);
    expect(removed.linesRemoved).toEqual([1]);
    expect(removed.anyLineChange).toBe(true);
  });

  it("treats a semantically equal numeric edit as unchanged", () => {
    const tracker = createExtractionEditTracker(1, header(), [line({ quantity: "2" })]);
    const changes = extractionEditorChanges(tracker, header(), [line({ quantity: "2.00" })]);
    expect(changes.lineCellsChanged).toEqual([]);
    expect(changes.anyLineChange).toBe(false);
  });
});

describe("extraction conflict matrix", () => {
  it("keeps a value whose normalized text is equal, with no prompt", () => {
    const baseline = header({ poNumber: "PO 1", incoterms: "CIF Batam" });
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({
        Rows: [],
        Fields: [
          { Key: "poNumber", Cell: cell("PO   1") },
          { Key: "incoterms", Cell: cell("CIF Batam") },
        ],
      }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "poNumber")?.outcome).toBe("keep");
    expect(comparison(plan, "incoterms")?.outcome).toBe("keep");
    expect(plan.conflicts).toEqual([]);
  });

  it("keeps a manual nonempty value when extraction is empty, with no prompt", () => {
    const baseline = header({ paymentTerms: "50% advance" });
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "paymentTerms", Cell: cell("", "absent") }] }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "paymentTerms")?.outcome).toBe("keep");
    expect(plan.conflicts).toEqual([]);
    expect(plan.selection["header:paymentTerms"]).not.toBe(true);
  });

  it("populates an unedited empty field automatically", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "poDate", Cell: cell("2026-10-12") }] }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "poDate")?.outcome).toBe("auto");
    expect(comparison(plan, "poDate")?.touched).toBe(false);
    expect(plan.selection["header:poDate"]).toBe(true);
    expect(plan.conflicts).toEqual([]);
  });

  it("asks about a field staff deliberately cleared", () => {
    const baseline = header({ poNumber: "PO-1" });
    const tracker = createExtractionEditTracker(1, baseline, []);
    const cleared = header({ poNumber: "" });
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "poNumber", Cell: cell("4500123456") }] }),
      cleared,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "poNumber")?.outcome).toBe("conflict");
    expect(comparison(plan, "poNumber")?.touched).toBe(true);
    expect(plan.conflicts).toContain("header:poNumber");
    expect(plan.selection["header:poNumber"]).not.toBe(true);
  });

  it("asks about an untouched nonempty field that differs, and never overwrites it", () => {
    const baseline = header({ poNumber: "PO-1", paymentTerms: "45 days" });
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(result({ Rows: [] }), baseline, [], tracker, USD);
    expect(comparison(plan, "poNumber")?.outcome).toBe("conflict");
    expect(comparison(plan, "paymentTerms")?.outcome).toBe("conflict");
    expect(plan.selection["header:poNumber"]).not.toBe(true);
    expect(plan.selection["header:paymentTerms"]).not.toBe(true);
  });

  it("creates no prompt when both sides are empty", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "requestedDeliveryDate", Cell: cell("", "absent") }] }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "requestedDeliveryDate")?.outcome).toBe("none");
    expect(plan.conflicts).toEqual([]);
  });

  it("keeps meaningful wording different instead of hiding it as whitespace", () => {
    const baseline = header({ incoterms: "CIF Batam" });
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "incoterms", Cell: cell("CIF Jakarta") }] }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "incoterms")?.outcome).toBe("conflict");
  });

  it("never auto-applies a needs_review value into an empty field", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "paymentTerms", Cell: cell("30 days", "needs_review") }] }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "paymentTerms")?.outcome).toBe("review");
    expect(plan.selection["header:paymentTerms"]).not.toBe(true);
  });

  it("treats currency as evidence only and never a conflict", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, []);
    const plan = resolveExtractionConflicts(
      result({ Rows: [], Fields: [{ Key: "currency", Cell: cell("EUR") }] }),
      baseline,
      [],
      tracker,
      USD
    );
    expect(comparison(plan, "currency")?.outcome).toBe("none");
    expect(plan.conflicts).toEqual([]);
  });
});

describe("line reconciliation", () => {
  it("leaves untouched differing rows as explicit proposals, not a conflict prompt", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, [line()]);
    const plan = resolveExtractionConflicts(result(), baseline, [line()], tracker, USD);
    expect(plan.lines?.outcome).toBe("review");
    expect(plan.conflicts).not.toContain(EXTRACTION_LINES_KEY);
    expect(plan.selection["row:0"]).not.toBe(true);
  });

  it("asks before replacing lines staff edited while extraction ran", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, [line()]);
    const edited = [line({ description: "Typed by hand" })];
    const plan = resolveExtractionConflicts(result(), baseline, edited, tracker, USD);
    expect(plan.lines?.outcome).toBe("conflict");
    expect(plan.lines?.touched).toBe(true);
    expect(plan.conflicts).toContain(EXTRACTION_LINES_KEY);
  });

  it("asks before replacing lines staff added or removed", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, [line()]);
    const added = [line(), line({ key: 1 })];
    const addedPlan = resolveExtractionConflicts(result(), baseline, added, tracker, USD);
    expect(addedPlan.lines?.outcome).toBe("conflict");

    const removed = [line({ key: 1 })];
    const removedPlan = resolveExtractionConflicts(result(), baseline, removed, tracker, USD);
    expect(removedPlan.lines?.outcome).toBe("conflict");
  });

  it("prompts for nothing when the rows already equal the lines semantically", () => {
    const baseline = header();
    const matching = line({ description: "Extracted line", quantity: "2.0" });
    const tracker = createExtractionEditTracker(1, baseline, [matching]);
    const plan = resolveExtractionConflicts(result(), baseline, [matching], tracker, USD);
    expect(plan.lines?.outcome).toBe("keep");
    expect(plan.conflicts).toEqual([]);
  });

  it("keeps the manual lines when extraction proposed no rows", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, [line()]);
    const plan = resolveExtractionConflicts(result({ Rows: [] }), baseline, [line()], tracker, USD);
    expect(plan.lines?.outcome).toBe("keep");
    expect(plan.conflicts).toEqual([]);
  });
});

describe("resolving conflict choices into an apply selection", () => {
  function conflictPlan() {
    const baseline = header({ poNumber: "PO-1" });
    const tracker = createExtractionEditTracker(1, baseline, [line()]);
    return resolveExtractionConflicts(
      result({ Rows: [] }),
      baseline,
      [line()],
      tracker,
      USD
    );
  }

  it("applies an auto field and keeps a conflict field unless extraction is chosen", () => {
    const plan = conflictPlan();
    const kept = resolvedExtractionSelection(plan, {}, {});
    expect(kept["header:poDate"]).toBe(true);
    expect(kept["header:poNumber"]).not.toBe(true);

    const chosen = resolvedExtractionSelection(plan, { "header:poNumber": "extracted" }, {});
    expect(chosen["header:poNumber"]).toBe(true);

    const manual = resolvedExtractionSelection(plan, { "header:poNumber": "manual" }, {});
    expect(manual["header:poNumber"]).not.toBe(true);
  });

  it("preserves a non-conflict tick and clears all rows while the line choice is Keep", () => {
    const baseline = header();
    const tracker = createExtractionEditTracker(1, baseline, [line()]);
    const plan = resolveExtractionConflicts(result(), baseline, [line({ description: "Typed" })], tracker, USD);
    const kept = resolvedExtractionSelection(
      plan,
      { [EXTRACTION_LINES_KEY]: "manual" },
      { "row:0": true, "header:poNumber": true }
    );
    expect(kept["row:0"]).toBe(false);
    expect(kept["header:poNumber"]).toBe(true);

    const used = resolvedExtractionSelection(
      plan,
      { [EXTRACTION_LINES_KEY]: "extracted" },
      { "row:0": true }
    );
    expect(used["row:0"]).toBe(true);
  });
});
