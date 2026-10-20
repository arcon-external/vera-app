// Issue #471: the final-save review, as pure decisions.
//
// The editor holds the entered values; the server holds the exact source
// revision. Everything between them -- matching by reference, naming every
// differing value, applying an explicit "Use Quotation" choice, showing the
// resulting totals and deciding whether the save may proceed -- is pure and
// tested here.
//
// None of this recalculates money differently from #470: applying a quotation
// value writes the source's own value into the editor, and the resulting
// totals come from the same calculatePurchaseOrder the entry screen uses. The
// saved total is the purchase order's own calculation -- the quotation's
// printed grand total is reference evidence, never a target that any value must
// be reversed to reach.

import { formatMoney, type CurrencyInfo } from "./format";
import {
  finalSaveNeedsConfirmation,
  finalSaveSendsAbsenceConfirmation,
} from "./purchaseOrderDocument";
import type {
  PurchaseOrderDTO,
  PurchaseOrderEditorState,
  PurchaseOrderItemInput,
  PurchaseOrderLine,
} from "./purchaseOrder";
import {
  buildPurchaseOrderUpdate,
  calculatePurchaseOrder,
  formatQuantityScaled,
  formatUnitPriceMinor,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
} from "./purchaseOrder";

/** One customer-facing line of the frozen source revision. */
export interface PurchaseOrderSourceLineDTO {
  QuotationItemNo: number;
  QuotationSubNo?: number | null;
  LineLabel: string;
  Description: string;
  UoM: string;
  QuantityScaled: number;
  UnitPriceMinor: number;
  DiscountEnabled: boolean;
  DiscountPercent: number;
}

/** The exact revision a final save is compared against. */
export interface PurchaseOrderSourceDTO {
  QuotationID: number;
  RevisionNumber: number;
  QuoteNumber: string;
  CustomerID: number;
  CustomerName: string;
  Currency: string;
  PaymentTerms: string;
  Incoterms: string;
  DiscountPercent: number;
  Lines: PurchaseOrderSourceLineDTO[];
  PrintedSubtotalMinor: number;
  PrintedDiscountMinor: number;
  PrintedPpnMinor: number;
  PrintedGrandTotalMinor: number;
}

/** The server's review response: the record, its source, and the document branch. */
export interface PurchaseOrderReviewDTO {
  Order: PurchaseOrderDTO;
  Source: PurchaseOrderSourceDTO;
  DocumentState: string;
}

export type POReviewChoice = "po" | "quotation";

/** Difference keys are stable: the server validates decisions by them. */
export type POReviewDecisions = Record<string, POReviewChoice | undefined>;

export interface POMatchedLine {
  line: PurchaseOrderLine;
  lineNumber: number;
  source: PurchaseOrderSourceLineDTO;
}

export interface PODifference {
  key: string;
  label: string;
  poValue: string;
  sourceValue: string;
}

export interface POMatchSuggestion {
  lineKey: number;
  lineNumber: number;
  source: PurchaseOrderSourceLineDTO;
  score: number;
}

export interface POComparison {
  matched: POMatchedLine[];
  missing: PurchaseOrderSourceLineDTO[];
  extraLineNumbers: number[];
  duplicateLineNumbers: number[];
  differences: PODifference[];
  suggestions: POMatchSuggestion[];
  /** Inputs that cannot be compared yet, named for the field. */
  blocking: string[];
}

const NONE_DISCOUNT = "None";

function trimmed(value: string): string {
  return value.trim();
}

function labelOf(line: PurchaseOrderLine): string | null {
  if (line.quotationItemNo === null) return null;
  return line.quotationSubNo === null
    ? String(line.quotationItemNo)
    : `${line.quotationItemNo}.${line.quotationSubNo}`;
}

function sourceLabel(line: PurchaseOrderSourceLineDTO): string {
  return line.LineLabel !== ""
    ? line.LineLabel
    : line.QuotationSubNo === null || line.QuotationSubNo === undefined
      ? String(line.QuotationItemNo)
      : `${line.QuotationItemNo}.${line.QuotationSubNo}`;
}

function discountLabel(enabled: boolean, percent: number): string {
  if (!enabled) return NONE_DISCOUNT;
  return `${percent}%`;
}

function normalizedText(value: string): string {
  return value.trim().toLocaleLowerCase();
}

/**
 * Suggests a source line for one unmatched PO line. The suggestion is a hint,
 * never a decision: an exact description, or the same price with the same unit
 * and quantity, scores high enough to offer; a shared price alone does not.
 */
export function suggestSourceLine(
  line: PurchaseOrderLine,
  candidates: PurchaseOrderSourceLineDTO[],
  currency: CurrencyInfo
): { source: PurchaseOrderSourceLineDTO; score: number } | null {
  const price = parseUnitPriceMinor(currency, line.unitPrice);
  const quantity = parseQuantityScaled(line.quantity);
  const description = normalizedText(line.description);
  const uom = normalizedText(line.uom);
  let best: { source: PurchaseOrderSourceLineDTO; score: number } | null = null;
  for (const candidate of candidates) {
    let score = 0;
    const candidateDescription = normalizedText(candidate.Description);
    if (description !== "" && candidateDescription !== "") {
      if (description === candidateDescription) {
        score += 5;
      } else if (
        description.includes(candidateDescription) ||
        candidateDescription.includes(description)
      ) {
        score += 3;
      }
    }
    const samePrice = price !== null && price === candidate.UnitPriceMinor;
    const sameQuantity =
      quantity !== null && quantity === candidate.QuantityScaled;
    const sameUom = uom !== "" && uom === normalizedText(candidate.UoM);
    if (samePrice && sameQuantity && sameUom) {
      score += 4;
    } else if (samePrice) {
      score += 1;
      if (sameQuantity) score += 1;
      if (sameUom) score += 1;
    }
    if (score < 3) continue;
    if (best === null || score > best.score) {
      best = { source: candidate, score };
    }
  }
  return best;
}

/**
 * Matches PO lines to the source by the stored item/sub reference, never by
 * position. A line with no reference, or one whose reference the revision does
 * not carry, is extra; a source line no line claims is missing; two lines
 * claiming one source line are a duplicate match.
 */
export function comparePurchaseOrder(
  state: PurchaseOrderEditorState,
  source: PurchaseOrderSourceDTO,
  currency: CurrencyInfo
): POComparison {
  const byLabel = new Map<string, PurchaseOrderSourceLineDTO>();
  for (const line of source.Lines) {
    byLabel.set(sourceLabel(line), line);
  }
  const claimed = new Map<string, number>();
  const matched: POMatchedLine[] = [];
  const extraLineNumbers: number[] = [];
  const duplicateLineNumbers: number[] = [];
  const differences: PODifference[] = [];
  const blocking: string[] = [];

  const addText = (key: string, label: string, poValue: string, sourceValue: string) => {
    if (trimmed(poValue) === trimmed(sourceValue)) return;
    differences.push({ key, label, poValue: trimmed(poValue), sourceValue: trimmed(sourceValue) });
  };
  const addValue = (key: string, label: string, poValue: string, sourceValue: string) => {
    if (poValue === sourceValue) return;
    differences.push({ key, label, poValue, sourceValue });
  };

  addText("header:paymentTerms", "Payment terms", state.header.paymentTerms, source.PaymentTerms);
  addText("header:incoterms", "Incoterms", state.header.incoterms, source.Incoterms);
  const wholeDiscount = parseDiscountPercent(state.header.discountPercent);
  if (wholeDiscount === null) {
    blocking.push("Whole-PO discount must be between 0 and 100.");
  } else {
    addValue(
      "header:discountPercent",
      "Whole-PO discount",
      `${wholeDiscount}%`,
      `${source.DiscountPercent}%`
    );
  }

  state.lines.forEach((line, index) => {
    const lineNumber = index + 1;
    const label = labelOf(line);
    if (label === null) {
      extraLineNumbers.push(lineNumber);
      return;
    }
    const sourceLine = byLabel.get(label);
    if (sourceLine === undefined) {
      extraLineNumbers.push(lineNumber);
      return;
    }
    if (claimed.has(label)) {
      duplicateLineNumbers.push(lineNumber);
      return;
    }
    claimed.set(label, lineNumber);
    matched.push({ line, lineNumber, source: sourceLine });

    const prefix = `line:${lineNumber}`;
    const name = `Line ${lineNumber}`;
    addText(`${prefix}:description`, `${name} description`, line.description, sourceLine.Description);
    addText(`${prefix}:uom`, `${name} unit of measure`, line.uom, sourceLine.UoM);

    const quantity = parseQuantityScaled(line.quantity);
    if (quantity === null) {
      blocking.push(
        `Line ${lineNumber}: quantity must be positive with up to 4 decimal places.`
      );
    } else {
      addValue(
        `${prefix}:quantity`,
        `${name} quantity`,
        formatQuantityScaled(quantity),
        formatQuantityScaled(sourceLine.QuantityScaled)
      );
    }
    const price = parseUnitPriceMinor(currency, line.unitPrice);
    if (price === null) {
      blocking.push(`Line ${lineNumber}: unit price must be a nonnegative amount.`);
    } else {
      addValue(
        `${prefix}:unitPrice`,
        `${name} unit price`,
        formatMoney(currency, price),
        formatMoney(currency, sourceLine.UnitPriceMinor)
      );
    }
    const percent = parseDiscountPercent(line.discountPercent);
    if (percent === null) {
      blocking.push(`Line ${lineNumber}: discount must be between 0 and 100.`);
    } else {
      addValue(
        `${prefix}:discount`,
        `${name} discount`,
        discountLabel(line.discountEnabled, line.discountEnabled ? percent : 0),
        discountLabel(sourceLine.DiscountEnabled, sourceLine.DiscountPercent)
      );
    }
  });

  const missing = source.Lines.filter((line) => !claimed.has(sourceLabel(line)));

  const unclaimed = source.Lines.filter((line) => !claimed.has(sourceLabel(line)));
  const suggestions: POMatchSuggestion[] = [];
  for (let index = 0; index < state.lines.length; index++) {
    if (!extraLineNumbers.includes(index + 1)) continue;
    const suggestion = suggestSourceLine(state.lines[index], unclaimed, currency);
    if (suggestion !== null) {
      suggestions.push({
        lineKey: state.lines[index].key,
        lineNumber: index + 1,
        source: suggestion.source,
        score: suggestion.score,
      });
    }
  }

  return {
    matched,
    missing,
    extraLineNumbers,
    duplicateLineNumbers,
    differences,
    suggestions,
    blocking,
  };
}

/** The initial review: no difference has a choice yet. */
export function initialReviewDecisions(): POReviewDecisions {
  return {};
}

/**
 * Applies every "Use Quotation" choice to the entered values. A "Use PO entry"
 * choice changes nothing: the entered value is what the PO will store.
 */
export function applyReviewChoices(
  state: PurchaseOrderEditorState,
  source: PurchaseOrderSourceDTO,
  comparison: POComparison,
  decisions: POReviewDecisions,
  currency: CurrencyInfo
): PurchaseOrderEditorState {
  const sourceByLineNumber = new Map<number, PurchaseOrderSourceLineDTO>();
  for (const pair of comparison.matched) {
    sourceByLineNumber.set(pair.lineNumber, pair.source);
  }
  const lines = state.lines.map((line) => ({ ...line }));
  const header = { ...state.header };

  for (const difference of comparison.differences) {
    if (decisions[difference.key] !== "quotation") continue;
    if (difference.key === "header:paymentTerms") {
      header.paymentTerms = source.PaymentTerms;
      continue;
    }
    if (difference.key === "header:incoterms") {
      header.incoterms = source.Incoterms;
      continue;
    }
    if (difference.key === "header:discountPercent") {
      header.discountPercent = String(source.DiscountPercent);
      continue;
    }
    const match = /^line:(\d+):(description|quantity|uom|unitPrice|discount)$/.exec(
      difference.key
    );
    if (match === null) continue;
    const lineNumber = Number(match[1]);
    const field = match[2];
    const sourceLine = sourceByLineNumber.get(lineNumber);
    const line = lines[lineNumber - 1];
    if (sourceLine === undefined || line === undefined) continue;
    switch (field) {
      case "description":
        line.description = sourceLine.Description;
        break;
      case "quantity":
        line.quantity = formatQuantityScaled(sourceLine.QuantityScaled);
        break;
      case "uom":
        line.uom = sourceLine.UoM;
        break;
      case "unitPrice":
        line.unitPrice = formatUnitPriceMinor(currency, sourceLine.UnitPriceMinor);
        break;
      case "discount":
        line.discountEnabled = sourceLine.DiscountEnabled;
        line.discountPercent = String(sourceLine.DiscountPercent);
        break;
    }
  }
  return { ...state, header, lines };
}

export interface POReviewEvaluation {
  comparison: POComparison;
  applied: PurchaseOrderEditorState;
  decisions: POReviewDecisions;
  acknowledgedMissing: string[];
  acknowledgedExtra: number[];
  totals: ReturnType<typeof calculatePurchaseOrder>["totals"];
  calculationError: string | null;
  /**
   * Printed grand total minus the applied calculated total. This is reference
   * information only: the saved grand total belongs to the purchase order, so
   * any value here is allowed and never blocks the save.
   */
  printedDifferenceMinor: number | null;
  blockers: string[];
  canContinue: boolean;
}

export function evaluateReview(
  state: PurchaseOrderEditorState,
  source: PurchaseOrderSourceDTO,
  currency: CurrencyInfo,
  decisions: POReviewDecisions,
  acknowledgedMissing: string[],
  acknowledgedExtra: number[]
): POReviewEvaluation {
  const comparison = comparePurchaseOrder(state, source, currency);
  const blockers = [...comparison.blocking];

  if (source.Currency !== state.currency) {
    blockers.push(
      `The purchase order is in ${state.currency} but its quotation revision is in ${source.Currency}. Amounts in different currencies are never compared.`
    );
  }

  for (const difference of comparison.differences) {
    if (decisions[difference.key] === undefined) {
      blockers.push(`${difference.label} still differs and needs a choice.`);
    }
  }
  for (const lineNumber of comparison.duplicateLineNumbers) {
    blockers.push(`Line ${lineNumber} matches a quotation line another line already uses.`);
  }
  for (const line of comparison.missing) {
    if (!acknowledgedMissing.includes(sourceLabel(line))) {
      blockers.push(`Quotation line ${sourceLabel(line)} is not on this purchase order. Acknowledge the omission.`);
    }
  }
  for (const lineNumber of comparison.extraLineNumbers) {
    if (!acknowledgedExtra.includes(lineNumber)) {
      blockers.push(`Line ${lineNumber} has no quotation match. Acknowledge it as an extra line.`);
    }
  }

  const applied = applyReviewChoices(state, source, comparison, decisions, currency);
  const appliedComparison = comparePurchaseOrder(applied, source, currency);
  for (const difference of appliedComparison.differences) {
    // An undecided difference already has its blocker. This second pass only
    // catches a "Use Quotation" choice that failed to apply, which would
    // otherwise send the server a value the client claimed to have replaced.
    if (decisions[difference.key] === "quotation") {
      blockers.push(`${difference.label} chose the quotation but still differs.`);
    }
  }

  const calculation = calculatePurchaseOrder(applied, currency);
  let printedDifferenceMinor: number | null = null;
  if (calculation.totals !== null) {
    printedDifferenceMinor =
      source.PrintedGrandTotalMinor - calculation.totals.grandTotalMinor;
  }
  if (calculation.error !== null) {
    // A calculation refusal always blocks: an unparseable field, an adjustment
    // without its reason, or a negative total. The last one still has totals to
    // show beside its error, so this is checked separately from `totals`.
    blockers.push(calculation.error);
  }

  return {
    comparison,
    applied,
    decisions,
    acknowledgedMissing,
    acknowledgedExtra,
    totals: calculation.totals,
    calculationError: calculation.error,
    printedDifferenceMinor,
    blockers,
    canContinue: blockers.length === 0,
  };
}

export interface PurchaseOrderFinalizeInput {
  ID: number;
  BillingAddress: string;
  DeliveryAddress: string;
  ShipToCode: string;
  ContactEmail: string;
  ContactPhone: string;
  PONumber: string;
  PODate: string;
  ReceivedDate: string;
  RequestedDeliveryDate: string;
  Incoterms: string;
  PaymentTerms: string;
  AdjustmentReason: string;
  Items: PurchaseOrderItemInput[];
  Decisions: Array<{ Key: string; Choice: POReviewChoice }>;
  AcknowledgedMissing: string[];
  AcknowledgedExtra: number[];
  DiscountPercent: number;
  AdjustmentMinor: number;
  DocumentConfirmed: boolean;
}

/**
 * Builds the final-save payload. It refuses exactly what the server refuses:
 * the minimum identity, the review's explicit decisions, the line
 * acknowledgements and the no-PDF confirmation while that branch applies. The
 * saved total is the purchase order's own calculation, so no printed-total
 * target appears here.
 */
export function buildFinalizeInput(
  state: PurchaseOrderEditorState,
  source: PurchaseOrderSourceDTO,
  currency: CurrencyInfo,
  documentState: string,
  decisions: POReviewDecisions,
  acknowledgedMissing: string[],
  acknowledgedExtra: number[],
  documentConfirmed: boolean
): { input: PurchaseOrderFinalizeInput | null; error: string | null } {
  if (state.header.poNumber.trim() === "") {
    return { input: null, error: "A customer PO number is required before the PO can be saved." };
  }
  // An unknown branch is treated as "none": asking for the confirmation again
  // is safe, while silently assuming a PDF exists is not.
  const branch = documentState === "attached" ? "attached" : "none";
  if (finalSaveNeedsConfirmation(branch, documentConfirmed)) {
    return {
      input: null,
      error: "Confirm that this purchase order is saved without its customer PDF.",
    };
  }
  const evaluation = evaluateReview(
    state,
    source,
    currency,
    decisions,
    acknowledgedMissing,
    acknowledgedExtra
  );
  if (!evaluation.canContinue) {
    return {
      input: null,
      error: evaluation.blockers[0] ?? "Resolve every difference before the final save.",
    };
  }
  const update = buildPurchaseOrderUpdate(evaluation.applied, currency);
  if (update.input === null) {
    return { input: null, error: update.error ?? "The purchase order is incomplete." };
  }
  return {
    input: {
      ...update.input,
      Decisions: evaluation.comparison.differences.map((difference) => ({
        Key: difference.key,
        Choice: decisions[difference.key]!,
      })),
      AcknowledgedMissing: evaluation.comparison.missing.map((line) => sourceLabel(line)),
      AcknowledgedExtra: [...evaluation.comparison.extraLineNumbers],
      DocumentConfirmed: finalSaveSendsAbsenceConfirmation(branch, documentConfirmed),
    },
    error: null,
  };
}

/** A compact label for one source line, used by the reviewer's match picker. */
export function sourceLineLabel(line: PurchaseOrderSourceLineDTO): string {
  return `${sourceLabel(line)} — ${line.Description || "(no description)"}`;
}

/** Builds the blank review state for a freshly opened record. */
export function initialFinalizeReviewState(): {
  decisions: POReviewDecisions;
  acknowledgedMissing: string[];
  acknowledgedExtra: number[];
  documentConfirmed: boolean;
} {
  return {
    decisions: initialReviewDecisions(),
    acknowledgedMissing: [],
    acknowledgedExtra: [],
    documentConfirmed: false,
  };
}

/** The editor state a saved record opens in (#471 read-only view). */
export function isSavedPurchaseOrder(order: PurchaseOrderDTO): boolean {
  return order.State === "saved";
}
