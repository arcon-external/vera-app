// Issue #473: corrections, version conflicts and the history panel's pure
// decisions, separated from the editor's markup.
//
// Everything here is pure: the reason rule the server also enforces, the
// outcome of a versioned write, the warning wording, and the labels the
// history panel prints. The editor owns the state machine that calls these; no
// function here reads a clock, a locale or a binding.

import { trimAdjustmentReason, type PurchaseOrderDTO } from "./purchaseOrder";

/** The version conflict the desktop binding returns instead of throwing. */
export interface PurchaseOrderConflictDTO {
  CurrentState: string;
  UpdatedByName: string;
  UpdatedAt: string;
  CurrentVersion: number;
  UpdatedBy: number;
}

/** The typed answer to a versioned write (#473). */
export interface PurchaseOrderWriteResultDTO {
  Order?: PurchaseOrderDTO | null;
  Conflict?: PurchaseOrderConflictDTO | null;
  /**
   * True when the record is already saved, so the write cannot be a draft
   * overwrite. The result is an answer, not a failure: the editor keeps its
   * working copy and opens the correction flow (wave 2).
   */
  SavedRecord?: boolean | null;
}

/** One line of a retained snapshot, as the history panel renders it. */
export interface PurchaseOrderHistoryItem {
  LineNumber: number;
  Description: string;
  QuantityScaled: number;
  UoM: string;
  UnitPriceMinor: number;
  DiscountMinor: number;
  NetMinor: number;
}

/**
 * The subset of a retained snapshot the history panel needs. The binding
 * returns a larger generated class; structural typing keeps this module free
 * of the generated model import.
 */
export interface PurchaseOrderHistoryEntry {
  UUID: string;
  Kind: string;
  State: string;
  Reason: string;
  ActorName: string;
  CreatedAt: string;
  QuoteNumber: string;
  CustomerName: string;
  SourceRevision: number;
  Currency: string;
  TaxZone: string;
  TaxCategory: string;
  BillingAddress: string;
  DeliveryAddress: string;
  ShipToCode: string;
  ContactEmail: string;
  ContactPhone: string;
  PONumber: string;
  PODate?: string | null;
  ReceivedDate: string;
  RequestedDeliveryDate?: string | null;
  PaymentTerms: string;
  Incoterms: string;
  AdjustmentReason: string;
  PreviousVersion: number;
  ResultingVersion: number;
  DiscountPercent: number;
  SubtotalMinor: number;
  DiscountMinor: number;
  PpnBps: number;
  PpnMinor: number;
  AdjustmentMinor: number;
  GrandTotalMinor: number;
  Attachment?: { Version: number; SHA256: string; Filename: string } | null;
  /**
   * Present only for kind "source_repair" (#475): the old and new link and the
   * admin's difference choices. Nil for every other kind.
   */
  Repair?: {
    PreviousQuoteNumber: string;
    PreviousSourceRevision: number;
    PreviousCustomerName: string;
    NewQuoteNumber: string;
    NewSourceRevision: number;
    NewCustomerName: string;
    Decisions?: Array<{ Key: string; Choice: string }>;
  } | null;
  Items: PurchaseOrderHistoryItem[];
}

/** What a write result means to the editor. */
export type PurchaseOrderWriteOutcome =
  | { kind: "saved"; order: PurchaseOrderDTO }
  | { kind: "conflict"; conflict: PurchaseOrderConflictDTO }
  | { kind: "savedRecord" }
  | { kind: "empty" };

/**
 * Classifies a binding result. A conflict is a real answer, not a failure: the
 * editor shows the warning and keeps the working copy; only a result with
 * neither side is a failed call.
 */
export function writeOutcome(result: PurchaseOrderWriteResultDTO): PurchaseOrderWriteOutcome {
  if (result.Order) return { kind: "saved", order: result.Order };
  if (result.Conflict) return { kind: "conflict", conflict: result.Conflict };
  if (result.SavedRecord === true) return { kind: "savedRecord" };
  return { kind: "empty" };
}

/** The schema and Go store's bound on a correction reason, in code points. */
export const MAX_CORRECTION_REASON_LENGTH = 500;

/** Trims exactly what Go's strings.TrimSpace trims, via the shared helper. */
export function trimCorrectionReason(value: string): string {
  return trimAdjustmentReason(value);
}

/**
 * The reason rule, shared by the panel and the payload so they cannot diverge:
 * a correction needs a non-blank reason, bounded to 500 code points. A null
 * answer means the reason is acceptable.
 */
export function correctionReasonError(value: string): string | null {
  const reason = trimCorrectionReason(value);
  if (reason === "") {
    return "A correction needs a reason.";
  }
  if (Array.from(reason).length > MAX_CORRECTION_REASON_LENGTH) {
    return `A correction reason must be ${MAX_CORRECTION_REASON_LENGTH} characters or fewer.`;
  }
  return null;
}

/** The actor's name, or an honest fallback when the server did not carry one. */
export function conflictActorName(conflict: PurchaseOrderConflictDTO): string {
  const name = conflict.UpdatedByName.trim();
  return name === "" ? "Someone else" : name;
}

/** "YYYY-MM-DD HH:mm UTC+07:00" from the server's RFC3339 value. */
export function formatHistoryTime(iso: string): string {
  const text = iso.trim();
  if (text === "") return "";
  const match =
    /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})(?::\d{2}(?:\.\d+)?)?(Z|[+-]\d{2}:\d{2})$/.exec(text);
  if (match === null) return text;
  return `${match[1]} ${match[2]} ${match[3] === "Z" ? "UTC" : match[3]}`;
}

/**
 * True when the record in the warning is no longer a draft. A draft write
 * cannot overwrite it: the only safe offer is the correction flow, which keeps
 * the working copy and requires a reason. A correction already has its reason,
 * so it treats a saved-state conflict as an ordinary version conflict.
 */
export function conflictNeedsCorrection(conflict: PurchaseOrderConflictDTO): boolean {
  return conflict.CurrentState !== "draft";
}

/**
 * The warning's one sentence. It names who saved, when and which version is
 * current, because the requirement is that a save since load warns with real
 * values regardless of how much time elapsed. When the record is saved
 * (`savedTransition`), it never promises a draft overwrite: that write would be
 * refused by design.
 */
export function conflictWarning(
  conflict: PurchaseOrderConflictDTO,
  savedTransition = false
): string {
  const time = formatHistoryTime(conflict.UpdatedAt);
  const saved = `${conflictActorName(conflict)} saved version ${conflict.CurrentVersion}${
    time === "" ? "" : ` at ${time}`
  }.`;
  if (savedTransition) {
    return `${saved} A saved record cannot be overwritten as a draft. Correcting it keeps everything you have typed and requires a reason.`;
  }
  return `${saved} Overwriting applies your changes and keeps the previous version in the history.`;
}

/** "Correction", "Draft overwritten" or "Source repaired", as the panel labels a kind. */
export function correctionKindLabel(kind: string): string {
  if (kind === "draft_overwrite") return "Draft overwritten";
  if (kind === "source_repair") return "Source repaired";
  return "Correction";
}

/** The customer PO PDF version a snapshot referenced, as one readable line. */
export function attachmentReferenceLabel(
  attachment: { Version: number; SHA256: string; Filename: string } | null | undefined
): string {
  if (attachment === null || attachment === undefined) {
    return "No customer PO PDF was attached at that moment.";
  }
  const filename = attachment.Filename.trim() === "" ? "customer-po.pdf" : attachment.Filename;
  const digest = attachment.SHA256.length > 12 ? `${attachment.SHA256.slice(0, 12)}…` : attachment.SHA256;
  return `Customer PO PDF v${attachment.Version} · ${filename} · sha256 ${digest}`;
}

/**
 * A saved record can be corrected; a draft is replaced by a normal save. The
 * editor uses this to decide whether the Correct action exists at all.
 */
export function canCorrectPurchaseOrder(order: PurchaseOrderDTO): boolean {
  return order.State === "saved";
}
