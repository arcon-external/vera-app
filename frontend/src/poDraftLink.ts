// Issue #497: the Staff draft link's pure decisions, separated from the panel.
//
// A link starts from the draft's entered values and repeats the #471
// difference review against the candidate revision. Everything here is pure:
// the tax choice the server will require, the currency remedy wording, the
// exact payload the binding sends, and the stale-version sentence. The
// comparison, matching and "Use Quotation" application are the existing
// finalize implementation, not a second one.

import { conflictActorName, formatHistoryTime, type PurchaseOrderConflictDTO } from "./poCorrection";
import type { CurrencyInfo } from "./format";
import {
  evaluateReview,
  sourceLabel,
  type POReviewChoice,
  type POReviewDecisions,
  type PurchaseOrderSourceDTO,
} from "./poFinalize";
import {
  buildPurchaseOrderUpdate,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorState,
  type PurchaseOrderSourceState,
  type PurchaseOrderUpdateInput,
} from "./purchaseOrder";

/** The explicit tax-basis choice a link may need. */
export type PurchaseOrderDraftTaxChoice = "" | "keep" | "source";

/** One side of the link's tax decision, as the server reports it. */
export interface PurchaseOrderDraftTaxBasisDTO {
  TaxZone: string;
  TaxType: string;
  ServiceCategory: string;
  TaxCategory: string;
  PpnBps: number;
}

/**
 * The Staff draft-link review: the same record/source/document shape a final
 * save reads, plus the two tax bases and whether a keep/source choice is
 * required.
 */
export interface PurchaseOrderDraftLinkReviewDTO {
  Order: PurchaseOrderDTO;
  Source: PurchaseOrderSourceDTO;
  DocumentState: string;
  DraftTax: PurchaseOrderDraftTaxBasisDTO;
  SourceTax: PurchaseOrderDraftTaxBasisDTO;
  TaxChoiceRequired: boolean;
}

/** The link payload the desktop binding sends, as the Go input names it. */
export interface PurchaseOrderDraftLinkInput {
  ID: number;
  ExpectedVersion: number;
  TargetQuotationID: number;
  TargetRevisionNumber: number;
  Decisions: Array<{ Key: string; Choice: POReviewChoice }>;
  AcknowledgedMissing: string[];
  AcknowledgedExtra: number[];
  TaxChoice: string;
}

/** "11%" for 1100 bps, "1.75%" for 175 bps. */
export function formatPpnBps(bps: number): string {
  const percent = bps / 100;
  return `${Number.isInteger(percent) ? percent : percent.toFixed(2)}%`;
}

/** One readable tax basis: "NonFTZ / Goods · 11% PPN". */
export function draftTaxBasisLabel(basis: PurchaseOrderDraftTaxBasisDTO): string {
  const parts = [basis.TaxZone, basis.TaxType, basis.ServiceCategory]
    .map((part) => part.trim())
    .filter((part) => part !== "");
  const ppn = basis.PpnBps > 0 ? `${formatPpnBps(basis.PpnBps)} PPN` : "no PPN";
  return parts.length === 0 ? ppn : `${parts.join(" / ")} · ${ppn}`;
}

/**
 * The tax rule the server enforces: a choice is required exactly when the two
 * bases differ, and an unknown choice is never sent.
 */
export function draftLinkTaxChoiceError(
  review: PurchaseOrderDraftLinkReviewDTO,
  choice: PurchaseOrderDraftTaxChoice
): string | null {
  if (choice !== "" && choice !== "keep" && choice !== "source") {
    return "Choose whether to keep the draft's tax basis or use the quotation's.";
  }
  if (review.TaxChoiceRequired && choice === "") {
    return "Choose whether to keep this purchase order's tax basis or use the quotation revision's.";
  }
  return null;
}

/**
 * What staff do about a currency mismatch. A never-linked draft can be edited;
 * a formerly-linked draft's basis is frozen (#496), so the remedy is choosing a
 * revision in its own currency. Neither path converts an amount.
 */
export function draftLinkCurrencyRemedy(
  sourceState: PurchaseOrderSourceState,
  draftCurrency: string,
  sourceCurrency: string
): string {
  if (draftCurrency === sourceCurrency) return "";
  if (sourceState === "never") {
    return `This draft is in ${draftCurrency}; the quotation is in ${sourceCurrency}. Choose Back without linking, change the draft currency to ${sourceCurrency} in the editor's Source basis card, then link again. Amounts are never converted.`;
  }
  return `This draft is fixed in ${draftCurrency}; the quotation is in ${sourceCurrency}. Choose a quotation in ${draftCurrency}. Amounts are never converted.`;
}

/** The differences the review decided, in the payload's shape. */
function draftLinkDecisions(
  differences: Array<{ key: string }>,
  decisions: POReviewDecisions
): Array<{ Key: string; Choice: POReviewChoice }> {
  return differences.map((difference) => ({
    Key: difference.key,
    Choice: decisions[difference.key]!,
  }));
}

/**
 * Builds both writes a link commits: the ordinary draft save that stores the
 * applied review (Use Quotation choices and corrected line matches), and the
 * link itself. It refuses what the server refuses -- an unresolved difference,
 * an unacknowledged missing/extra line, a required tax choice, a currency
 * mismatch -- so the button cannot promise a save the server will reject.
 */
export interface DraftLinkPlan {
  /** The draft save that persists the applied review values. */
  update: PurchaseOrderUpdateInput | null;
  /** The link request that becomes authoritative after that save. */
  input: PurchaseOrderDraftLinkInput | null;
  error: string | null;
}

export function planDraftLink(
  state: PurchaseOrderEditorState,
  review: PurchaseOrderDraftLinkReviewDTO,
  currency: CurrencyInfo,
  decisions: POReviewDecisions,
  acknowledgedMissing: string[],
  acknowledgedExtra: number[],
  taxChoice: PurchaseOrderDraftTaxChoice
): DraftLinkPlan {
  const evaluation = evaluateReview(
    state,
    review.Source,
    currency,
    decisions,
    acknowledgedMissing,
    acknowledgedExtra
  );
  if (!evaluation.canContinue) {
    return {
      update: null,
      input: null,
      error: evaluation.blockers[0] ?? "Resolve every difference before linking the source.",
    };
  }
  const taxError = draftLinkTaxChoiceError(review, taxChoice);
  if (taxError !== null) {
    return { update: null, input: null, error: taxError };
  }
  const update = buildPurchaseOrderUpdate(evaluation.applied, currency);
  if (update.input === null) {
    return { update: null, input: null, error: update.error ?? "The purchase order is incomplete." };
  }
  return {
    update: update.input,
    input: {
      ID: evaluation.applied.orderId,
      ExpectedVersion: evaluation.applied.version,
      TargetQuotationID: review.Source.QuotationID,
      TargetRevisionNumber: review.Source.RevisionNumber,
      Decisions: draftLinkDecisions(evaluation.comparison.differences, decisions),
      AcknowledgedMissing: evaluation.comparison.missing.map(sourceLabel),
      AcknowledgedExtra: [...evaluation.comparison.extraLineNumbers],
      TaxChoice: review.TaxChoiceRequired ? taxChoice : "keep",
    },
    error: null,
  };
}

/**
 * The link's stale-version sentence. Like a repair, there is no overwrite here:
 * a moved record means reviewing the draft again, because the link is what
 * makes identity authoritative.
 */
export function draftLinkConflictWarning(conflict: PurchaseOrderConflictDTO): string {
  const time = formatHistoryTime(conflict.UpdatedAt);
  return `${conflictActorName(conflict)} saved version ${conflict.CurrentVersion}${
    time === "" ? "" : ` at ${time}`
  }. The link was not applied; review the draft again.`;
}
