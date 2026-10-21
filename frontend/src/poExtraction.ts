// Issue #477: the pure decisions behind "Extract and compare".
//
// The panel renders these values, and the editor applies exactly what the
// selection says. Keeping the rules here means the markup cannot disagree with
// the payload: an absent value is never applicable, an edited field is never
// preselected, currency is evidence only, and no path applies a proposal the
// staff member did not choose.
//
// Extraction is assistance, not a fact. Nothing here treats a proposal as
// correct; a `needs_review` value is offered with its evidence and an explicit
// checkbox.

import type { CurrencyInfo } from "./format";
import {
  formatQuantityScaled,
  formatUnitPriceMinor,
  MAX_EXACT_MINOR,
  parseQuantityScaled,
  type PurchaseOrderHeader,
  type PurchaseOrderLine,
} from "./purchaseOrder";

export interface PurchaseOrderExtractionBBoxDTO {
  X: number;
  Y: number;
  Width: number;
  Height: number;
}

export interface PurchaseOrderExtractionPageDTO {
  Number: number;
  Width: number;
  Height: number;
}

/**
 * One page's processing evidence (#478). Older stored results (schema 1) have
 * no page modes; the panel then says nothing about OCR rather than pretending
 * the page was read one way or the other.
 */
export interface PurchaseOrderExtractionPageModeDTO {
  Number: number;
  Mode: string;
  TextWords: number;
  TextChars: number;
  OCRWords: number;
  DPI?: number;
  Confidence?: number | null;
  Reason?: string;
}

export interface PurchaseOrderExtractionCellDTO {
  Value: string;
  Raw: string;
  Status: string;
  Page?: number | null;
  BBox?: PurchaseOrderExtractionBBoxDTO | null;
  Confidence?: number | null;
  Note?: string;
}

export interface PurchaseOrderExtractionFieldDTO {
  Key: string;
  Cell: PurchaseOrderExtractionCellDTO;
}

export interface PurchaseOrderExtractionRowDTO {
  LineNumber: PurchaseOrderExtractionCellDTO;
  Description: PurchaseOrderExtractionCellDTO;
  Quantity: PurchaseOrderExtractionCellDTO;
  UoM: PurchaseOrderExtractionCellDTO;
  UnitPrice: PurchaseOrderExtractionCellDTO;
  Amount: PurchaseOrderExtractionCellDTO;
  Status: string;
  Page: number;
  BBox?: PurchaseOrderExtractionBBoxDTO | null;
  Note?: string;
}

export interface PurchaseOrderExtractionResultDTO {
  Engine: string;
  EngineVersion: string;
  /** Present only when at least one page was OCRed (schema 2, #478). */
  OCREngine?: string;
  OCREngineVersion?: string;
  PageCount: number;
  PagesWithText: number;
  PagesWithoutText: number;
  PagesWithOCR?: number;
  DurationMs: number;
  SchemaVersion: number;
  Pages: PurchaseOrderExtractionPageDTO[];
  PageModes?: PurchaseOrderExtractionPageModeDTO[];
  Fields: PurchaseOrderExtractionFieldDTO[];
  Rows: PurchaseOrderExtractionRowDTO[];
  Abstentions: { Fields: number; Rows: number };
}

export interface PurchaseOrderExtractionDTO {
  UUID: string;
  Status: string;
  Engine: string;
  EngineVersion: string;
  DocumentSHA256: string;
  ErrorCode?: string | null;
  ErrorMessage?: string | null;
  Result?: PurchaseOrderExtractionResultDTO | null;
  ID: number;
  PurchaseOrderID: number;
  DocumentID: number;
  DocumentVersion: number;
  PageCount: number;
  RequestedBy: number;
  CreatedAt: string;
  StartedAt?: string | null;
  FinishedAt?: string | null;
  DurationMS?: number | null;
}

export interface PurchaseOrderExtractionDocumentDTO {
  ID: number;
  Version: number;
  SHA256: string;
}

export interface PurchaseOrderExtractionStateDTO {
  Run?: PurchaseOrderExtractionDTO | null;
  Document?: PurchaseOrderExtractionDocumentDTO | null;
}

export type ExtractionStatus = "extracted" | "needs_review" | "absent";

/** The six header keys, in the order the review panel shows them. */
export const EXTRACTION_FIELD_ORDER = [
  "poNumber",
  "poDate",
  "requestedDeliveryDate",
  "currency",
  "paymentTerms",
  "incoterms",
] as const;

export type ExtractionFieldKey = (typeof EXTRACTION_FIELD_ORDER)[number];

export const EXTRACTION_FIELD_LABELS: Record<ExtractionFieldKey, string> = {
  poNumber: "Customer PO number",
  poDate: "Customer PO date",
  requestedDeliveryDate: "Requested delivery date",
  currency: "Currency",
  paymentTerms: "Payment terms",
  incoterms: "Incoterms",
};

/**
 * Currency is deliberately not applicable: the PO's currency is copied from
 * the Quotation and is protected. The proposal is shown so a difference is
 * visible, never offered as an edit.
 */
export const APPLICABLE_HEADER_KEYS: ExtractionFieldKey[] = [
  "poNumber",
  "poDate",
  "requestedDeliveryDate",
  "paymentTerms",
  "incoterms",
];

export const EXTRACTION_STATUS_LABELS: Record<ExtractionStatus, string> = {
  extracted: "Found",
  needs_review: "Needs review",
  absent: "Not found",
};

export function isTerminalExtractionStatus(status: string): boolean {
  return status === "succeeded" || status === "failed" || status === "stale";
}

/** Honest progress: queued/running only, never a fabricated percentage. */
export function extractionProgressText(status: string): string {
  if (status === "queued") return "Queued for extraction…";
  if (status === "running") return "Reading the PDF pages (text layers and scans)…";
  return "";
}

/**
 * A result is stale when its status says so or when the run read a different
 * document generation than the one now active. The client checks both so a
 * missed replacement event cannot make old proposals look current.
 */
export function extractionIsStale(
  run: PurchaseOrderExtractionDTO | null | undefined,
  document: PurchaseOrderExtractionDocumentDTO | null | undefined
): boolean {
  if (run == null) return false;
  if (run.Status === "stale") return true;
  if (document == null) return true;
  // A replacement is a new generation even when the bytes happen to be
  // identical (same SHA): the document row and version are the identity.
  if (run.DocumentID !== document.ID || run.DocumentVersion !== document.Version) return true;
  return run.DocumentSHA256 !== document.SHA256;
}

/**
 * Whether a run may supply proposals to the editor for the active document.
 * A succeeded result is not enough: the document generation must still match,
 * otherwise a refresh that has not returned yet could leave old proposals
 * actionable after the PDF was replaced.
 */
export function extractionResultActionable(
  run: PurchaseOrderExtractionDTO | null | undefined,
  document: PurchaseOrderExtractionDocumentDTO | null | undefined
): boolean {
  if (run == null || run.Status !== "succeeded" || run.Result == null) return false;
  return !extractionIsStale(run, document);
}

/** The reason staff read when extraction did not succeed. */
export function extractionErrorMessage(run: {
  Status: string;
  ErrorCode?: string | null;
  ErrorMessage?: string | null;
}): string {
  if (run.Status === "stale") {
    return "The PDF changed after this extraction, so its proposals were discarded. Your entries were not changed.";
  }
  if (run.Status !== "failed") return "";
  switch (run.ErrorCode ?? "") {
    case "invalid_output":
      return "The extracted pages could not be read into usable values. Enter the values manually.";
    case "page_limit":
      return "This PDF has more pages than this release extracts. Read it and enter the order manually.";
    case "ocr_page_limit":
      return "This PDF has more scanned pages than this release reads. Read it and enter the order manually.";
    case "page_too_large":
      return "A page in this PDF is too large to read automatically. Enter the values manually.";
    case "ocr_unavailable":
      return "This server could not read the scanned pages. Enter the values manually.";
    case "timeout":
      return "Extraction took too long. Try again, or enter the values manually.";
    case "output_too_large":
      return "The extractor returned more data than this release accepts. Enter the values manually.";
    case "extractor_unavailable":
      return "Automatic extraction is not available on this server. Enter the values manually.";
    case "document_too_large":
      return "The PDF is larger than the extraction limit. Enter the values manually.";
    case "document_missing":
      return "The attached PDF could not be read. Attach it again or enter the values manually.";
    case "document_changed":
      return "The PDF changed before extraction read it. Run Extract again, or enter the values manually.";
    case "interrupted":
      return "The server restarted before extraction finished. Run Extract again, or enter the values manually.";
    case "cancelled":
      return "Extraction was stopped before it finished. Run Extract again.";
    default:
      return run.ErrorMessage?.trim()
        ? `${run.ErrorMessage.trim()} You can still enter the values manually.`
        : "The text extractor failed. Enter the values manually.";
  }
}

/**
 * Parses a plain printed decimal (a dot, no grouping) into an exact minor-unit
 * count. Grouped text and values with more precision than the currency holds
 * are refused rather than rounded: a wrong amount is worse than no proposal.
 */
export function parseExtractedDecimal(raw: string, exponent: number): number | null {
  if (!Number.isInteger(exponent) || exponent < 0 || exponent > 6) return null;
  const text = raw.trim();
  const match = /^(\d+)(?:\.(\d+))?$/.exec(text);
  if (!match) return null;
  const whole = match[1] ?? "";
  const fraction = match[2] ?? "";
  if (fraction.length > exponent) {
    if (fraction.slice(exponent).replace(/0+$/, "") !== "") return null;
  }
  const padded = fraction.padEnd(exponent, "0").slice(0, exponent);
  const digits = `${whole}${padded}`.replace(/^0+(?=\d)/, "");
  const value = Number(digits);
  if (!Number.isSafeInteger(value) || value < 0 || value > MAX_EXACT_MINOR) return null;
  return value;
}

function currentHeaderValue(header: PurchaseOrderHeader, key: ExtractionFieldKey): string {
  switch (key) {
    case "poNumber":
      return header.poNumber;
    case "poDate":
      return header.poDate;
    case "requestedDeliveryDate":
      return header.requestedDeliveryDate;
    case "paymentTerms":
      return header.paymentTerms;
    case "incoterms":
      return header.incoterms;
    default:
      return "";
  }
}

/** The editor-side value a proposal would replace. */
export function extractionCurrentValue(
  header: PurchaseOrderHeader,
  key: ExtractionFieldKey
): string {
  return currentHeaderValue(header, key).trim();
}

const DATE_VALUE = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Whether the editor can hold this header proposal exactly. Currency is never
 * applicable; dates must already be normalised; absent and empty values are
 * disabled. `needs_review` is applicable only after the staff member ticks it.
 */
export function headerProposalApplicable(
  field: PurchaseOrderExtractionFieldDTO,
  _currency: CurrencyInfo
): boolean {
  const key = field.Key as ExtractionFieldKey;
  if (!APPLICABLE_HEADER_KEYS.includes(key)) return false;
  if (field.Cell.Status !== "extracted" && field.Cell.Status !== "needs_review") return false;
  const value = field.Cell.Value.trim();
  if (value === "") return false;
  if (key === "poDate" || key === "requestedDeliveryDate") return DATE_VALUE.test(value);
  return true;
}

/** Whether one extracted row can become an editor line exactly. */
export function rowProposalApplicable(
  row: PurchaseOrderExtractionRowDTO,
  currency: CurrencyInfo
): boolean {
  const description = row.Description.Value.trim();
  const uom = row.UoM.Value.trim();
  if (description === "" || uom === "") return false;
  const quantity = parseQuantityScaled(row.Quantity.Value);
  if (quantity === null) return false;
  const unitPrice = parseExtractedDecimal(row.UnitPrice.Value, currency.exponent);
  if (unitPrice === null) return false;
  return true;
}

export function proposalCellKey(kind: "header" | "row", id: string): string {
  return `${kind}:${id}`;
}

/**
 * The default selection is deliberately conservative. Only an `extracted`
 * header proposal into an empty editor field is preselected. A field the staff
 * member already filled (whether equal or different) stays untouched; a
 * `needs_review` value waits for a tick; every row waits for a tick because
 * applying rows replaces the current line set.
 */
export function defaultExtractionSelection(
  result: PurchaseOrderExtractionResultDTO,
  header: PurchaseOrderHeader,
  _lines: PurchaseOrderLine[],
  currency: CurrencyInfo
): Record<string, boolean> {
  const selection: Record<string, boolean> = {};
  for (const field of result.Fields) {
    const key = field.Key as ExtractionFieldKey;
    if (!APPLICABLE_HEADER_KEYS.includes(key)) continue;
    if (!headerProposalApplicable(field, currency)) {
      selection[proposalCellKey("header", field.Key)] = false;
      continue;
    }
    if (field.Cell.Status !== "extracted") {
      selection[proposalCellKey("header", field.Key)] = false;
      continue;
    }
    const current = extractionCurrentValue(header, key);
    selection[proposalCellKey("header", field.Key)] = current === "";
  }
  for (let index = 0; index < result.Rows.length; index += 1) {
    selection[proposalCellKey("row", String(index))] = false;
  }
  return selection;
}

export interface AppliedExtraction {
  headerPatch: Partial<PurchaseOrderHeader>;
  /** Undefined keeps the existing lines; an array replaces them. */
  lines?: PurchaseOrderLine[];
  appliedFields: number;
  appliedRows: number;
}

/**
 * Applies exactly the ticked proposals. An unselected field is never written,
 * an inapplicable selection is skipped, and choosing any row replaces the
 * whole line set with the chosen rows -- mixing extracted rows into the
 * quotation-seeded lines would silently duplicate quantities.
 */
export function applyExtractionSelection(
  result: PurchaseOrderExtractionResultDTO,
  selection: Record<string, boolean>,
  header: PurchaseOrderHeader,
  _lines: PurchaseOrderLine[],
  nextKey: number,
  currency: CurrencyInfo
): AppliedExtraction {
  const headerPatch: Partial<PurchaseOrderHeader> = {};
  let appliedFields = 0;
  for (const field of result.Fields) {
    if (!selection[proposalCellKey("header", field.Key)]) continue;
    if (!headerProposalApplicable(field, currency)) continue;
    const key = field.Key as ExtractionFieldKey;
    if (key === "currency") continue;
    const value = field.Cell.Value.trim();
    switch (key) {
      case "poNumber":
        headerPatch.poNumber = value;
        break;
      case "poDate":
        headerPatch.poDate = value;
        break;
      case "requestedDeliveryDate":
        headerPatch.requestedDeliveryDate = value;
        break;
      case "paymentTerms":
        headerPatch.paymentTerms = value;
        break;
      case "incoterms":
        headerPatch.incoterms = value;
        break;
    }
    appliedFields += 1;
  }

  const built: PurchaseOrderLine[] = [];
  for (let index = 0; index < result.Rows.length; index += 1) {
    if (!selection[proposalCellKey("row", String(index))]) continue;
    const row = result.Rows[index]!;
    if (!rowProposalApplicable(row, currency)) continue;
    const quantity = parseQuantityScaled(row.Quantity.Value);
    const unitPrice = parseExtractedDecimal(row.UnitPrice.Value, currency.exponent);
    if (quantity === null || unitPrice === null) continue;
    built.push({
      key: nextKey + built.length,
      description: row.Description.Value.trim(),
      quantity: formatQuantityScaled(quantity),
      uom: row.UoM.Value.trim(),
      unitPrice: formatUnitPriceMinor(currency, unitPrice),
      discountEnabled: false,
      discountPercent: "0",
      quotationItemNo: null,
      quotationSubNo: null,
    });
  }

  return {
    headerPatch,
    lines: built.length > 0 ? built : undefined,
    appliedFields,
    appliedRows: built.length,
  };
}

function formatPoint(value: number): string {
  if (!Number.isFinite(value)) return "?";
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

/** "Page 2 · 86–142 × 82–91 pt", or the honest missing-evidence wording. */
export function extractionEvidenceLabel(cell: PurchaseOrderExtractionCellDTO): string {
  if (cell.Status === "absent") return "Not printed";
  if (cell.Page == null) return "Page unknown";
  if (cell.BBox == null) return `Page ${cell.Page}`;
  const right = cell.BBox.X + cell.BBox.Width;
  const bottom = cell.BBox.Y + cell.BBox.Height;
  return `Page ${cell.Page} · ${formatPoint(cell.BBox.X)}–${formatPoint(right)} × ${formatPoint(cell.BBox.Y)}–${formatPoint(bottom)} pt`;
}

export interface ExtractionRegion {
  left: number;
  top: number;
  width: number;
  height: number;
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(100, Math.max(0, value));
}

/** A relative rectangle for the small page map in the review panel. */
export function extractionRegion(
  cell: PurchaseOrderExtractionCellDTO,
  page: PurchaseOrderExtractionPageDTO
): ExtractionRegion | null {
  if (cell.Status === "absent" || cell.BBox == null) return null;
  if (!Number.isFinite(page.Width) || !Number.isFinite(page.Height) || page.Width <= 0 || page.Height <= 0) {
    return null;
  }
  const left = clampPercent((cell.BBox.X / page.Width) * 100);
  const top = clampPercent((cell.BBox.Y / page.Height) * 100);
  const width = clampPercent((cell.BBox.Width / page.Width) * 100);
  const height = clampPercent((cell.BBox.Height / page.Height) * 100);
  return {
    left,
    top,
    width: Math.min(width, 100 - left),
    height: Math.min(height, 100 - top),
  };
}

/**
 * What the extractor did not assert, stated as counts. This is not an accuracy
 * score; it is the honest amount of review left.
 */
export function extractionAbstentionSummary(result: PurchaseOrderExtractionResultDTO): string {
  const fields = result.Fields.filter((field) => field.Cell.Status !== "extracted").length;
  const rows = result.Rows.filter((row) => row.Status !== "extracted").length;
  return `${fields} of ${result.Fields.length} header fields need review or were not found · ${rows} of ${result.Rows.length} lines need review.`;
}

/**
 * The confidence floor, matching DefaultMinConfidence in the extractor. It is a
 * release policy, not a measured optimum: #476 §2.5 has not been validated.
 */
export const EXTRACTION_MIN_CONFIDENCE = 70;

/** Whether any page was read by OCR, which changes how "no text" reads. */
export function extractionUsesOCR(result: PurchaseOrderExtractionResultDTO): boolean {
  if ((result.PagesWithOCR ?? 0) > 0) return true;
  return (result.PageModes ?? []).some((page) => page.Mode === "ocr");
}

/** Which engines produced this result, and how many pages OCR read. */
export function extractionEngineSummary(result: PurchaseOrderExtractionResultDTO): string {
  const text = `Text layer: ${result.Engine || "unknown"} ${result.EngineVersion || ""}`.trim();
  const hasEvidence = (result.PageModes?.length ?? 0) > 0 || result.PagesWithOCR != null;
  if (!hasEvidence) {
    // A stored schema-1 result predates OCR and has no page evidence. Saying
    // "no page needed OCR" would be a claim the result cannot support.
    return `${text} · OCR evidence not recorded`;
  }
  if (!extractionUsesOCR(result)) return `${text} · no page needed OCR`;
  const ocr = `OCR: ${result.OCREngine || "unknown"} ${result.OCREngineVersion || ""}`.trim();
  const pages = (result.PageModes ?? []).filter((page) => page.Mode === "ocr").length ||
    result.PagesWithOCR || 0;
  return `${text} · ${ocr} (${pages} of ${result.PageCount} pages)`;
}

/** One page's processing mode, with the evidence that decided it. */
export function extractionPageModeSummary(mode: PurchaseOrderExtractionPageModeDTO): string {
  const page = `Page ${mode.Number}`;
  if (mode.Mode === "ocr") {
    const dpi = mode.DPI ? ` at ${mode.DPI} DPI` : "";
    const words = `${mode.OCRWords} ${mode.OCRWords === 1 ? "word" : "words"}`;
    const confidence =
      mode.Confidence == null
        ? "confidence not reported"
        : `${Math.round(mode.Confidence)}% mean confidence`;
    return `${page}: OCR${dpi} (${words}, ${confidence})`;
  }
  return `${page}: text layer (${mode.TextChars} letters/digits)`;
}

/**
 * The confidence label for one cell. A text-layer value has no confidence, and
 * this returns nothing rather than inventing a score for it.
 */
export function extractionCellConfidenceLabel(cell: PurchaseOrderExtractionCellDTO): string {
  if (cell.Confidence == null) return "";
  const value = Math.round(cell.Confidence);
  if (cell.Confidence < EXTRACTION_MIN_CONFIDENCE) {
    return `OCR confidence ${value}% (below the ${EXTRACTION_MIN_CONFIDENCE}% floor)`;
  }
  return `OCR confidence ${value}%`;
}

/** Which cells of a row are below the floor, e.g. "quantity 55%, amount 61%". */
export function extractionLowConfidenceCellLabels(row: PurchaseOrderExtractionRowDTO): string {
  const cells: Array<[string, PurchaseOrderExtractionCellDTO]> = [
    ["line number", row.LineNumber],
    ["description", row.Description],
    ["quantity", row.Quantity],
    ["unit", row.UoM],
    ["unit price", row.UnitPrice],
    ["amount", row.Amount],
  ];
  return cells
    .filter(([, cell]) => cell.Confidence != null && cell.Confidence < EXTRACTION_MIN_CONFIDENCE)
    .map(([label, cell]) => `${label} ${Math.round(cell.Confidence ?? 0)}%`)
    .join(", ");
}

/** True when the text layer produced nothing and no page was OCRed. */
export function extractionFoundNoText(result: PurchaseOrderExtractionResultDTO): boolean {
  if (result.Rows.length > 0) return false;
  if (extractionUsesOCR(result)) return false;
  return result.Fields.every((field) => field.Cell.Status === "absent");
}
