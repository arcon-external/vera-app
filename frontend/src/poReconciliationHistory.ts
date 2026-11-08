import { formatHistoryTime, type PurchaseOrderHistoryEntry } from "./poCorrection";
import {
  normalizeSession,
  type PaneFilter,
  type ReconciliationItem,
  type ReconciliationSession,
} from "./poReconciliation";
import type { PurchaseOrderDTO } from "./purchaseOrder";

// #518: the read side of a saved purchase order's reconciliation history.
//
// One call returns every generation for one PO in one snapshot: the outer
// POVersion is the *current* parent version, while each nested Session.POVersion
// is the version stored on that generation (an audit value that a later
// correction does not rewrite). Current selection therefore uses the stored
// Active flag, never the nested version.
//
// This module is pure: it owns the wire normalisation, the current/history
// split, the saved-only filter counts and the exact labels the saved view
// shows. The component mounts it; the transport lives in
// poReconciliationAdapter.

/** One stored generation, exactly as the B1 envelope names it. */
export interface ReconciliationHistoryRecord {
  Generation: number;
  SourceAvailable: boolean;
  /** Null for create/link sessions; set on a session installed by a repair. */
  RepairCorrectionID: number | null;
  Session: ReconciliationSession;
}

export interface ReconciliationHistoryRead {
  PurchaseOrderID: number;
  /** Current parent version at the read snapshot. */
  POVersion: number;
  Sessions: ReconciliationHistoryRecord[];
}

/**
 * The loose shape the binding may hand back: generated classes type absent
 * values as undefined and a nullable collection as undefined, so everything
 * here is optional until normalised.
 */
export interface ReconciliationHistoryInput {
  PurchaseOrderID?: number;
  POVersion?: number;
  Sessions?: Array<{
    Generation?: number;
    SourceAvailable?: boolean;
    RepairCorrectionID?: number | null;
    Session?: ReconciliationSession;
  }> | null;
}

const EMPTY_SESSION: ReconciliationSession = {
  ID: 0,
  PurchaseOrderID: 0,
  SourceQuotationID: 0,
  SourceRevision: 0,
  POVersion: 0,
  Active: false,
  ReadOnly: true,
  CreatedBy: 0,
  CreatedAt: "",
  UpdatedAt: "",
  LineIdentities: [],
  Items: [],
};

/**
 * Normalises one read: every record keeps its stored generation, source
 * availability and repair link; the nested session is normalised by the same
 * function the editor uses. Order is generation descending, then session id
 * descending, so the caller never depends on the server's array order.
 */
export function normalizeReconciliationHistory(
  input: ReconciliationHistoryInput
): ReconciliationHistoryRead {
  const sessions = (input.Sessions ?? [])
    .map((record) => ({
      Generation: record.Generation ?? 0,
      SourceAvailable: record.SourceAvailable ?? true,
      RepairCorrectionID:
        record.RepairCorrectionID === undefined ? null : record.RepairCorrectionID,
      Session: normalizeSession(record.Session ?? EMPTY_SESSION),
    }))
    .sort(
      (a, b) =>
        b.Generation - a.Generation || b.Session.ID - a.Session.ID
    );
  return {
    PurchaseOrderID: input.PurchaseOrderID ?? 0,
    POVersion: input.POVersion ?? 0,
    Sessions: sessions,
  };
}

export type SavedReconciliationView =
  | { kind: "none" }
  | { kind: "stale" }
  | {
      kind: "ready";
      current: ReconciliationHistoryRecord;
      previous: ReconciliationHistoryRecord[];
    };

/**
 * The saved-state decision, in one place so the rail, the sheet and the
 * trigger cannot disagree: no sessions is a legacy record (not an error); a
 * read whose outer version differs from the displayed record is stale and must
 * not be combined with the values on screen; otherwise the active generation
 * (or, defensively, the newest one) is current and the rest are history.
 */
export function savedReconciliationView(
  read: ReconciliationHistoryRead,
  orderVersion: number
): SavedReconciliationView {
  if (read.Sessions.length === 0) return { kind: "none" };
  if (read.POVersion !== orderVersion) return { kind: "stale" };
  const current =
    read.Sessions.find((record) => record.Session.Active) ?? read.Sessions[0];
  if (current === undefined) return { kind: "none" };
  return {
    kind: "ready",
    current,
    previous: read.Sessions.filter((record) => record !== current),
  };
}

export interface SavedReconciliationCounts {
  all: number;
  resolved: number;
  unresolved: number;
}

export function savedReconciliationCounts(
  record: ReconciliationHistoryRecord
): SavedReconciliationCounts {
  const items: ReconciliationItem[] = record.Session.Items;
  return {
    all: items.length,
    resolved: items.filter((item) => item.Status === "resolved").length,
    unresolved: items.filter((item) => item.Status === "unresolved").length,
  };
}

/** The saved surface offers only All and Resolved: history is never reopened. */
export const SAVED_PANE_FILTERS: { id: PaneFilter; label: string }[] = [
  { id: "all", label: "All" },
  { id: "resolved", label: "Resolved" },
];

/** The exact context line above the pane. */
export function savedReconciliationContext(
  record: ReconciliationHistoryRecord,
  isCurrent: boolean
): string {
  const prefix = isCurrent ? "Saved session" : "Previous saved session";
  return `${prefix} · revision ${record.Session.SourceRevision} · generation ${record.Generation}`;
}

/**
 * The source line for one generation. The current quotation number is only
 * truthful for the generation that still points at the live link; an older
 * generation falls back to its stored numeric source, never the current label.
 */
export function savedSourceLabel(
  record: ReconciliationHistoryRecord,
  order: Pick<PurchaseOrderDTO, "QuotationID" | "QuoteNumber" | "SourceRevision">
): string {
  const matchesCurrentLink =
    record.Session.SourceQuotationID === order.QuotationID &&
    record.Session.SourceRevision === order.SourceRevision;
  const revision = record.Session.SourceRevision;
  if (matchesCurrentLink && order.QuoteNumber.trim() !== "") {
    return `QT-${order.QuoteNumber} · revision ${revision}`;
  }
  return `Quotation ${record.Session.SourceQuotationID} · revision ${revision}`;
}

/** The "Source deleted" notice for a generation whose source row is gone. */
export function savedSourceDeletedLabel(
  record: ReconciliationHistoryRecord
): string {
  return `Source deleted · Quotation ${record.Session.SourceQuotationID} · revision ${record.Session.SourceRevision}`;
}

/**
 * The repair-history row that matches this generation's link, when the parent
 * already loaded the correction history. Null when the link is absent, or when
 * the entry is not a source repair.
 */
export function savedRepairEntry(
  record: ReconciliationHistoryRecord,
  entries: PurchaseOrderHistoryEntry[]
): PurchaseOrderHistoryEntry | null {
  const id = record.RepairCorrectionID;
  if (id === null) return null;
  return (
    entries.find((entry) => entry.ID === id && entry.Kind === "source_repair") ??
    null
  );
}

export interface SavedHistoryRow {
  /** `Revision 2 · 3 resolved · 2026-10-30 10:04 UTC` */
  primary: string;
  /** Source, generation and (when known) the repair transition. */
  secondary: string;
}

/**
 * One history row. The correction-history panel stays the authority for the
 * reason, actor and choices; this row only names the generation and, when the
 * link resolves, which repair produced it.
 */
export function savedHistoryRow(
  record: ReconciliationHistoryRecord,
  entries: PurchaseOrderHistoryEntry[]
): SavedHistoryRow {
  const counts = savedReconciliationCounts(record);
  const revision = record.Session.SourceRevision;
  const at =
    formatHistoryTime(record.Session.CreatedAt) ||
    formatHistoryTime(record.Session.UpdatedAt);
  const primary = `Revision ${revision} · ${counts.resolved} resolved${
    at === "" ? "" : ` · ${at}`
  }`;
  const repair = savedRepairEntry(record, entries);
  const generation = record.Generation;
  let secondary: string;
  if (repair !== null && repair.Repair !== null && repair.Repair !== undefined) {
    const { PreviousQuoteNumber, PreviousSourceRevision, NewQuoteNumber, NewSourceRevision } =
      repair.Repair;
    secondary = `Source repaired · Quotation ${PreviousQuoteNumber} rev ${PreviousSourceRevision} → Quotation ${NewQuoteNumber} rev ${NewSourceRevision} · generation ${generation}`;
  } else if (!record.SourceAvailable) {
    secondary = `Source deleted · Quotation ${record.Session.SourceQuotationID} · revision ${revision} · generation ${generation}`;
  } else if (record.RepairCorrectionID !== null) {
    secondary = `Source repaired · details in correction history · generation ${generation}`;
  } else {
    secondary = `Quotation ${record.Session.SourceQuotationID} · generation ${generation}`;
  }
  return { primary, secondary };
}

/** The trigger's state words, per the saved state matrix. */
export interface SavedTriggerWords {
  /** Visible label; the Read-only pill carries the mode separately. */
  text: string;
  /** Whether the trigger shows the Read-only pill. */
  pill: boolean;
  ariaLabel: string;
}

export function savedTriggerState(
  status: "loading" | "error" | "ready" | "none" | "stale"
): SavedTriggerWords {
  if (status === "loading") {
    return {
      text: "Reconciliation · loading",
      pill: false,
      ariaLabel: "Reconciliation, loading. Open panel.",
    };
  }
  if (status === "error" || status === "stale") {
    return {
      text: "Reconciliation · unavailable",
      pill: false,
      ariaLabel: "Reconciliation, failed to load. Open panel.",
    };
  }
  return {
    text: "Reconciliation",
    pill: true,
    ariaLabel: "Reconciliation, read-only. Open panel.",
  };
}
