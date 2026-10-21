// Issue #475: the source repair's pure decisions, separated from the panel.
//
// A repair starts from the saved record's current values and repeats the #471
// difference review against the candidate revision. Everything here is pure:
// whether the admin may see the action at all, the reason rule the server also
// enforces, the exact payload the binding sends, and the wording the history
// panel uses for the old and new link.

import {
  evaluateReview,
  type POReviewChoice,
  type POReviewDecisions,
  type PurchaseOrderReviewDTO,
} from "./poFinalize";
import {
  buildPurchaseOrderUpdate,
  trimAdjustmentReason,
  type PurchaseOrderDTO,
  type PurchaseOrderEditorState,
  type PurchaseOrderItemInput,
} from "./purchaseOrder";
import type { CurrencyInfo } from "./format";
import { conflictActorName, formatHistoryTime, type PurchaseOrderConflictDTO } from "./poCorrection";

/** The schema/store bound on a repair reason, in code points. */
export const MAX_REPAIR_REASON_LENGTH = 500;

/** The reason rule, shared by the panel and the payload so they cannot drift. */
export function repairReasonError(value: string): string | null {
  const reason = trimAdjustmentReason(value);
  if (reason === "") {
    return "A source repair needs a reason.";
  }
  if (Array.from(reason).length > MAX_REPAIR_REASON_LENGTH) {
    return `A source repair reason must be ${MAX_REPAIR_REASON_LENGTH} characters or fewer.`;
  }
  return null;
}

/** Trims exactly what Go's strings.TrimSpace trims. */
export function trimRepairReason(value: string): string {
  return trimAdjustmentReason(value);
}

/**
 * The admin action exists only on a saved record: a draft has no confirmation
 * number or retained history to protect, and it is replaced by ordinary draft
 * saves until it is finalized. The server re-checks the role; this decides
 * whether the control is rendered at all.
 */
export function canRepairSource(order: PurchaseOrderDTO, isAdmin: boolean): boolean {
  return isAdmin && order.State === "saved";
}

/**
 * The saved view's action name (#481). A record whose Quotation was deleted is
 * not being corrected against a known source; it is being linked to one for the
 * first time, which is what the staff-facing wording should say. The operation
 * and its rules are the same #475 repair either way.
 */
export function sourceRepairActionLabel(order: PurchaseOrderDTO): string {
  return order.SourceMissing === true ? "Link Quotation" : "Repair source link";
}

/**
 * The exact source link a saved record shows. A source-missing record carries
 * a former-source reference rather than a live link, so the label says so
 * instead of presenting a deleted quotation as current.
 */
export function sourceReferenceLabel(order: PurchaseOrderDTO): string {
  const link = `quotation ${order.QuoteNumber} revision ${order.SourceRevision}`;
  return order.SourceMissing === true ? `former ${link}` : link;
}

/** The repair payload the desktop binding sends, as the Go input names it. */
export interface PurchaseOrderSourceRepairInput {
  ID: number;
  TargetQuotationID: number;
  TargetRevisionNumber: number;
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
  Reason: string;
  Items: PurchaseOrderItemInput[];
  Decisions: Array<{ Key: string; Choice: POReviewChoice }>;
  AcknowledgedMissing: string[];
  AcknowledgedExtra: number[];
  DiscountPercent: number;
  AdjustmentMinor: number;
  ExpectedVersion: number;
}

function sourceLabelOf(line: { LineLabel: string; QuotationItemNo: number; QuotationSubNo?: number | null }): string {
  if (line.LineLabel !== "") return line.LineLabel;
  return line.QuotationSubNo === null || line.QuotationSubNo === undefined
    ? String(line.QuotationItemNo)
    : `${line.QuotationItemNo}.${line.QuotationSubNo}`;
}

/**
 * Builds the repair payload from the editor state the review actually shows,
 * after the admin corrected matches and chose values. It refuses exactly what
 * the server refuses: a missing reason, an unresolved difference, an
 * unacknowledged missing/extra line, a currency mismatch, or an incomplete
 * record. The applied review state becomes the editable values, so a "Use
 * Quotation" choice is stored rather than merely claimed, and the version the
 * panel loaded is stated for the CAS.
 */
export function buildSourceRepairInput(
  state: PurchaseOrderEditorState,
  review: PurchaseOrderReviewDTO,
  currency: CurrencyInfo,
  decisions: POReviewDecisions,
  acknowledgedMissing: string[],
  acknowledgedExtra: number[],
  reason: string
): { input: PurchaseOrderSourceRepairInput | null; error: string | null } {
  const reasonError = repairReasonError(reason);
  if (reasonError !== null) {
    return { input: null, error: reasonError };
  }
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
      input: null,
      error: evaluation.blockers[0] ?? "Resolve every difference before repairing the source.",
    };
  }
  const update = buildPurchaseOrderUpdate(evaluation.applied, currency);
  if (update.input === null) {
    return { input: null, error: update.error ?? "The purchase order is incomplete." };
  }
  return {
    input: {
      ...update.input,
      ID: state.orderId,
      TargetQuotationID: review.Source.QuotationID,
      TargetRevisionNumber: review.Source.RevisionNumber,
      Reason: trimRepairReason(reason),
      Decisions: evaluation.comparison.differences.map((difference) => ({
        Key: difference.key,
        Choice: decisions[difference.key]!,
      })),
      AcknowledgedMissing: evaluation.comparison.missing.map(sourceLabelOf),
      AcknowledgedExtra: [...evaluation.comparison.extraLineNumbers],
      ExpectedVersion: state.version,
    },
    error: null,
  };
}

/**
 * The repair's stale-version sentence. Unlike a draft save there is no
 * overwrite offer here: the repair is the admin operation, so a moved record
 * means reload and review again rather than reapply stale choices.
 */
export function repairConflictWarning(conflict: PurchaseOrderConflictDTO): string {
  const time = formatHistoryTime(conflict.UpdatedAt);
  return `${conflictActorName(conflict)} saved version ${conflict.CurrentVersion}${
    time === "" ? "" : ` at ${time}`
  }. The repair was not applied; reload the purchase order and review the new source again.`;
}

/** The old and new link one history entry recorded, as the panel renders it. */
export interface SourceRepairSummaryInput {
  PreviousQuoteNumber: string;
  PreviousSourceRevision: number;
  PreviousCustomerName: string;
  NewQuoteNumber: string;
  NewSourceRevision: number;
  NewCustomerName: string;
}

/**
 * "Quotation A · revision 2 → Quotation B · revision 0", with the customer
 * change named only when the repair moved the record to another customer.
 */
export function sourceRepairSummary(repair: SourceRepairSummaryInput): string {
  const link = `Quotation ${repair.PreviousQuoteNumber} · revision ${repair.PreviousSourceRevision} → Quotation ${repair.NewQuoteNumber} · revision ${repair.NewSourceRevision}`;
  if (repair.PreviousCustomerName === repair.NewCustomerName) {
    return link;
  }
  return `${link} · customer ${repair.PreviousCustomerName} → ${repair.NewCustomerName}`;
}

/** What one recorded choice means, in the history panel's words. */
export function sourceRepairChoiceLabel(choice: string): string {
  return choice === "quotation" ? "Used Quotation" : "Kept PO entry";
}
