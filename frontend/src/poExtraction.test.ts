import { describe, expect, it } from "vitest";
import {
  applyExtractionSelection,
  defaultExtractionSelection,
  extractionAbstentionSummary,
  extractionErrorMessage,
  extractionEvidenceLabel,
  extractionIsStale,
  extractionProgressText,
  extractionRegion,
  headerProposalApplicable,
  isTerminalExtractionStatus,
  parseExtractedDecimal,
  proposalCellKey,
  rowProposalApplicable,
  type PurchaseOrderExtractionResultDTO,
  type PurchaseOrderExtractionRowDTO,
} from "./poExtraction";
import type { PurchaseOrderHeader, PurchaseOrderLine } from "./purchaseOrder";

// Issue #477: the pure decisions behind "Extract and compare".
//
// The panel renders these, and the apply action is built from them, so a
// checkbox on screen and the editor values that change cannot disagree. The
// load-bearing rules are here rather than in markup: an absent value is never
// applicable, an edited field is never silently overwritten, currency is
// evidence only, and an unselected proposal never reaches the editor.

const USD = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before" as const,
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};
const IDR = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before" as const,
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: true,
};

function cell(
  value: string,
  status: "extracted" | "needs_review" | "absent" = "extracted",
  extra: Partial<PurchaseOrderExtractionResultDTO["Fields"][number]["Cell"]> = {}
) {
  return {
    Value: value,
    Raw: value,
    Status: status,
    Page: status === "absent" ? null : 1,
    BBox: status === "absent" ? null : { X: 86, Y: 82, Width: 56, Height: 9 },
    Confidence: null,
    Note: "",
    ...extra,
  };
}

function row(
  line: string,
  description: string,
  quantity: string,
  uom: string,
  unitPrice: string,
  amount: string,
  status: "extracted" | "needs_review" = "extracted"
): PurchaseOrderExtractionRowDTO {
  return {
    LineNumber: cell(line),
    Description: cell(description),
    Quantity: cell(quantity),
    UoM: cell(uom),
    UnitPrice: cell(unitPrice),
    Amount: cell(amount, amount === "" ? "absent" : "extracted"),
    Status: status,
    Page: 1,
    BBox: { X: 50, Y: 225, Width: 490, Height: 10 },
    Note: "",
  };
}

function result(): PurchaseOrderExtractionResultDTO {
  return {
    Engine: "stub",
    EngineVersion: "1.0",
    PageCount: 1,
    PagesWithText: 1,
    PagesWithoutText: 0,
    DurationMs: 9,
    SchemaVersion: 1,
    Pages: [{ Number: 1, Width: 595, Height: 842 }],
    Fields: [
      { Key: "poNumber", Cell: cell("4500123456") },
      { Key: "poDate", Cell: cell("2026-10-12") },
      { Key: "requestedDeliveryDate", Cell: cell("", "absent") },
      { Key: "currency", Cell: cell("USD") },
      { Key: "paymentTerms", Cell: cell("30 days after delivery", "needs_review") },
      { Key: "incoterms", Cell: cell("CIF Batam") },
    ],
    Rows: [
      row("1", "Electrical panel", "2", "Pc", "1250.00", "2500.00"),
      row("2", "Cable termination", "10.5", "M", "42.00", "441.00"),
    ],
    Abstentions: { Fields: 2, Rows: 0 },
  };
}

function emptyHeader(): PurchaseOrderHeader {
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
  };
}

function lines(): PurchaseOrderLine[] {
  return [
    {
      key: 0,
      description: "Quotation line",
      quantity: "1",
      uom: "Lot",
      unitPrice: "100.00",
      discountEnabled: false,
      discountPercent: "0",
      quotationItemNo: 1,
      quotationSubNo: null,
    },
    {
      key: 1,
      description: "Second quotation line",
      quantity: "2",
      uom: "Pc",
      unitPrice: "10.00",
      discountEnabled: false,
      discountPercent: "0",
      quotationItemNo: 2,
      quotationSubNo: null,
    },
  ];
}

describe("extraction status and progress", () => {
  it("recognizes terminal statuses", () => {
    expect(isTerminalExtractionStatus("queued")).toBe(false);
    expect(isTerminalExtractionStatus("running")).toBe(false);
    expect(isTerminalExtractionStatus("succeeded")).toBe(true);
    expect(isTerminalExtractionStatus("failed")).toBe(true);
    expect(isTerminalExtractionStatus("stale")).toBe(true);
  });

  it("words progress honestly without a fake percentage", () => {
    expect(extractionProgressText("queued")).toContain("Queued");
    expect(extractionProgressText("running")).toContain("Reading");
    expect(extractionProgressText("succeeded")).toBe("");
    expect(extractionProgressText("failed")).toBe("");
    expect(extractionProgressText("stale")).toBe("");
  });

  it("treats a run from another document generation as stale", () => {
    const run = {
      UUID: "run",
      Status: "succeeded",
      Engine: "stub",
      EngineVersion: "1.0",
      DocumentSHA256: "aaaa",
      ErrorCode: null,
      ErrorMessage: null,
      Result: null,
      ID: 1,
      PurchaseOrderID: 2,
      DocumentID: 3,
      DocumentVersion: 1,
      PageCount: 1,
      RequestedBy: 4,
      CreatedAt: "",
      StartedAt: null,
      FinishedAt: null,
      DurationMS: null,
    };
    expect(extractionIsStale(run, { ID: 3, Version: 1, SHA256: "aaaa" })).toBe(false);
    expect(extractionIsStale(run, { ID: 3, Version: 2, SHA256: "bbbb" })).toBe(true);
    // Same bytes, new document generation: still a different source.
    expect(extractionIsStale(run, { ID: 3, Version: 2, SHA256: "aaaa" })).toBe(true);
    expect(extractionIsStale({ ...run, Status: "stale" }, { ID: 3, Version: 1, SHA256: "aaaa" })).toBe(true);
    expect(extractionIsStale(run, null)).toBe(true);
  });

  it("maps typed failures to a manual-path sentence", () => {
    expect(extractionErrorMessage({ Status: "failed", ErrorCode: "page_limit", ErrorMessage: "" })).toContain(
      "more pages"
    );
    expect(extractionErrorMessage({ Status: "failed", ErrorCode: "timeout", ErrorMessage: "" })).toContain(
      "too long"
    );
    expect(extractionErrorMessage({ Status: "failed", ErrorCode: "invalid_output", ErrorMessage: "" })).toContain(
      "scanned"
    );
    expect(extractionErrorMessage({ Status: "failed", ErrorCode: "interrupted", ErrorMessage: "" })).toContain(
      "restarted"
    );
    expect(
      extractionErrorMessage({ Status: "failed", ErrorCode: "engine_failed", ErrorMessage: "Poppler said no." })
    ).toContain("Poppler said no.");
    expect(extractionErrorMessage({ Status: "stale", ErrorCode: "document_changed", ErrorMessage: "" })).toContain(
      "PDF changed"
    );
  });
});

describe("exact decimal parsing", () => {
  it("parses plain printed decimals into minor units without floats", () => {
    expect(parseExtractedDecimal("1250.00", 2)).toBe(125000);
    expect(parseExtractedDecimal("10.5", 4)).toBe(105000);
    expect(parseExtractedDecimal("2", 0)).toBe(2);
    expect(parseExtractedDecimal("1250.00", 0)).toBe(1250);
    expect(parseExtractedDecimal("0.01", 2)).toBe(1);
  });

  it("refuses grouped or over-precise values rather than rounding them", () => {
    expect(parseExtractedDecimal("1,250.00", 2)).toBeNull();
    expect(parseExtractedDecimal("1250.005", 2)).toBeNull();
    expect(parseExtractedDecimal("1250.5", 0)).toBeNull();
    expect(parseExtractedDecimal("", 2)).toBeNull();
    expect(parseExtractedDecimal("9007199254740992", 0)).toBeNull();
  });
});

describe("proposal applicability and default selection", () => {
  it("enables only values the editor can hold exactly", () => {
    const extraction = result();
    expect(headerProposalApplicable(extraction.Fields[0], USD)).toBe(true);
    expect(headerProposalApplicable(extraction.Fields[1], USD)).toBe(true);
    expect(headerProposalApplicable(extraction.Fields[2], USD)).toBe(false);
    expect(headerProposalApplicable(extraction.Fields[3], USD)).toBe(false);
    expect(headerProposalApplicable(extraction.Fields[4], USD)).toBe(true);
    expect(headerProposalApplicable(extraction.Fields[5], USD)).toBe(true);
    expect(rowProposalApplicable(extraction.Rows[0], USD)).toBe(true);
    expect(rowProposalApplicable(extraction.Rows[1], USD)).toBe(true);
    expect(rowProposalApplicable({ ...extraction.Rows[0], Quantity: cell("abc", "needs_review") }, USD)).toBe(false);
    expect(rowProposalApplicable({ ...extraction.Rows[0], UnitPrice: cell("", "absent") }, USD)).toBe(false);
  });

  it("defaults to applying only extracted fields into empty editor fields", () => {
    const selection = defaultExtractionSelection(result(), emptyHeader(), lines(), USD);
    expect(selection[proposalCellKey("header", "poNumber")]).toBe(true);
    expect(selection[proposalCellKey("header", "poDate")]).toBe(true);
    expect(selection[proposalCellKey("header", "paymentTerms")]).toBe(false);
    expect(selection[proposalCellKey("header", "incoterms")]).toBe(true);
    expect(selection[proposalCellKey("header", "currency")]).toBeUndefined();
    expect(selection[proposalCellKey("row", "0")]).toBe(false);
    expect(selection[proposalCellKey("row", "1")]).toBe(false);
  });

  it("protects a field staff already edited and never preselects needs_review", () => {
    const header = { ...emptyHeader(), poNumber: "MANUAL-1" };
    const selection = defaultExtractionSelection(result(), header, lines(), USD);
    expect(selection[proposalCellKey("header", "poNumber")]).toBe(false);
    expect(selection[proposalCellKey("header", "paymentTerms")]).toBe(false);
  });
});

describe("applying chosen proposals", () => {
  it("applies nothing without an explicit selection", () => {
    const applied = applyExtractionSelection(result(), {}, emptyHeader(), lines(), 10, USD);
    expect(applied.headerPatch).toEqual({});
    expect(applied.lines).toBeUndefined();
    expect(applied.appliedFields).toBe(0);
  });

  it("applies only the selected header fields and never the protected currency", () => {
    const selection = {
      [proposalCellKey("header", "poNumber")]: true,
      [proposalCellKey("header", "currency")]: true,
      [proposalCellKey("header", "incoterms")]: true,
    };
    const applied = applyExtractionSelection(result(), selection, emptyHeader(), lines(), 10, USD);
    expect(applied.headerPatch.poNumber).toBe("4500123456");
    expect(applied.headerPatch.incoterms).toBe("CIF Batam");
    expect(applied.headerPatch).not.toHaveProperty("currency");
    expect(applied.lines).toBeUndefined();
    expect(applied.appliedFields).toBe(2);
  });

  it("replaces the line set with only the selected rows when any row is chosen", () => {
    const selection = { [proposalCellKey("row", "1")]: true };
    const applied = applyExtractionSelection(result(), selection, emptyHeader(), lines(), 10, USD);
    expect(applied.lines).toHaveLength(1);
    const line = applied.lines![0]!;
    expect(line.key).toBe(10);
    expect(line.description).toBe("Cable termination");
    expect(line.quantity).toBe("10.5");
    expect(line.uom).toBe("M");
    expect(line.unitPrice).toBe("$42.00");
    expect(line.quotationItemNo).toBeNull();
    expect(line.discountEnabled).toBe(false);
    expect(applied.appliedRows).toBe(1);
  });

  it("keeps the existing lines when no row is selected", () => {
    const selection = { [proposalCellKey("header", "incoterms")]: true };
    const applied = applyExtractionSelection(result(), selection, emptyHeader(), lines(), 10, USD);
    expect(applied.lines).toBeUndefined();
    expect(applied.appliedRows).toBe(0);
  });

  it("skips a selected proposal that cannot be represented exactly", () => {
    const extraction = result();
    extraction.Rows[0] = { ...extraction.Rows[0], UnitPrice: cell("1,250.00") };
    const selection = { [proposalCellKey("row", "0")]: true };
    const applied = applyExtractionSelection(extraction, selection, emptyHeader(), lines(), 10, USD);
    expect(applied.lines).toBeUndefined();
    expect(applied.appliedRows).toBe(0);
  });
});

describe("evidence and abstention wording", () => {
  it("names the page and draws a clamped relative region", () => {
    expect(extractionEvidenceLabel(result().Fields[0]!.Cell)).toBe("Page 1 · 86–142 × 82–91 pt");
    const region = extractionRegion(result().Fields[0]!.Cell, result().Pages[0]!);
    expect(region).not.toBeNull();
    expect(region!.left).toBeGreaterThan(10);
    expect(region!.top).toBeGreaterThan(5);
    expect(region!.width).toBeGreaterThan(5);
    expect(extractionRegion(cell("", "absent"), result().Pages[0]!)).toBeNull();
  });

  it("summarizes abstentions instead of implying accuracy", () => {
    expect(extractionAbstentionSummary(result())).toContain("2 of 6 header fields");
    expect(extractionAbstentionSummary(result())).toContain("0 of 2 lines");
  });
});
