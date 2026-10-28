import type { backend } from "@backend/models";
import type { CurrencyInfo } from "./format";
import type { POReviewDecisions, PurchaseOrderSourceLineDTO } from "./poFinalize";
import {
  formatQuantityScaled,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
} from "./purchaseOrder";

// Ticket 04: the live header reconciliation domain for the PO workbench.
//
// Pure state machine and pane view-model for the three comparable header
// fields (contract section 2 catalogue). The commit handler runs on blur only,
// never per keystroke: an empty required field shows a notice and creates no
// item (R-01 keeps every header field non-required, so the empty branch only
// fires when validation requires the field); a matching commit shows the match
// notice only when the field has no item; a differing commit creates or
// reopens one unresolved item with both values. Lines, tax basis and derived
// totals are ticket 05: this module compares, filters and gates header items,
// and renders any other session kinds read-only.
//
// The session/item shapes below are the production DTOs: plain data interfaces
// mirroring the generated Wails bindings field-for-field (house pattern, like
// purchaseOrder.ts). The contract test binds its former test-only declarations
// to these, and the bind assertions in poReconciliation.test.ts fail the build
// if the generated side drifts.

export interface ReconciliationHistoryEvent {
  Kind: string;
  SourceLineRef?: string | null;
  POValue: unknown;
  SourceValue: unknown;
  Status: string;
  Resolution?: string | null;
  ActorID: number;
  At: string;
}

export interface ReconciliationLineIdentity {
  Key: string;
  ID?: number | null;
  Removed: boolean;
}

export interface ReconciliationItem {
  Key: string;
  Field: string;
  Kind: string;
  LineKey?: string | null;
  SourceLineRef?: string | null;
  Label: string;
  POValue: unknown;
  SourceValue: unknown;
  Status: string;
  Resolution?: string | null;
  ActorID: number;
  UpdatedAt: string;
  History: ReconciliationHistoryEvent[];
}

export interface ReconciliationSession {
  ID: number;
  PurchaseOrderID: number;
  SourceQuotationID: number;
  SourceRevision: number;
  POVersion: number;
  Active: boolean;
  ReadOnly: boolean;
  CreatedBy: number;
  CreatedAt: string;
  UpdatedAt: string;
  LineIdentities: ReconciliationLineIdentity[];
  Items: ReconciliationItem[];
}

// The generated bindings stay the transport shape. The pane consumes the
// load direction, so these assertions prove a binding response still fits the
// production interfaces (a renamed key or a narrowed value type fails here).
// The save direction crosses through the adapter with one explicit cast,
// because class methods do not travel over JSON.
const _sessionFromWails: ReconciliationSession =
  null as unknown as backend.POReconciliationSession;
const _itemFromWails: ReconciliationItem =
  null as unknown as backend.POReconciliationItem;
const _eventFromWails: ReconciliationHistoryEvent =
  null as unknown as backend.POReconciliationHistoryEvent;
const _identityFromWails: ReconciliationLineIdentity =
  null as unknown as backend.POReconciliationLineIdentity;
void [_sessionFromWails, _itemFromWails, _eventFromWails, _identityFromWails];

/**
 * Normalizes a transport response to contract-exact nulls: generated classes
 * type absent values as undefined, but the wire and every comparison here use
 * explicit JSON null.
 */
export function normalizeSession(
  session: ReconciliationSession
): ReconciliationSession {
  const nullish = <T>(value: T | null | undefined): T | null =>
    value === undefined ? null : value;
  return {
    ...session,
    LineIdentities: (session.LineIdentities ?? []).map((identity) => ({
      ...identity,
      ID: nullish(identity.ID),
    })),
    Items: (session.Items ?? []).map((item) => ({
      ...item,
      LineKey: nullish(item.LineKey),
      SourceLineRef: nullish(item.SourceLineRef),
      Resolution: nullish(item.Resolution),
      History: (item.History ?? []).map((event) => ({
        ...event,
        SourceLineRef: nullish(event.SourceLineRef),
        Resolution: nullish(event.Resolution),
      })),
    })),
  };
}

export type HeaderFieldKey =
  | "header:paymentTerms"
  | "header:incoterms"
  | "header:discountPercent";

export interface HeaderFieldDef {
  key: HeaderFieldKey;
  label: string;
  /**
   * R-01: only fields the existing validation already requires may show an
   * empty notice. None of the three header fields is required today, so the
   * empty branch below is dormant for them but covered by tests.
   */
  required: boolean;
  kind: "text" | "percent";
}

export const HEADER_FIELDS: Record<HeaderFieldKey, HeaderFieldDef> = {
  "header:paymentTerms": {
    key: "header:paymentTerms",
    label: "Payment terms",
    required: false,
    kind: "text",
  },
  "header:incoterms": {
    key: "header:incoterms",
    label: "Incoterms",
    required: false,
    kind: "text",
  },
  "header:discountPercent": {
    key: "header:discountPercent",
    label: "Whole-PO discount",
    required: false,
    kind: "percent",
  },
};

export type CommitNotice = "empty" | "matched" | null;

export interface CommitInput {
  field: HeaderFieldDef;
  /** The raw editor input, untrimmed. */
  poValue: string;
  /**
   * The source scalar in canonical display form: trimmed text, or an integer
   * percent for the discount. The caller canonicalises; the handler never
   * formats.
   */
  sourceValue: string | number;
  existingItem: ReconciliationItem | null;
  actorId: number;
  /** UTC RFC3339, from the caller so tests pin exact timestamps. */
  now: string;
}

export interface CommitResult {
  notice: CommitNotice;
  /** The created or updated item, or null when no item is owed. */
  item: ReconciliationItem | null;
  /** True when the session must be saved (an item was created or changed). */
  itemChanged: boolean;
  /** An invalid non-empty number: a validation error, never an item. */
  validationError: string | null;
}

function canonicalText(raw: string): string {
  return raw.trim();
}

function canonicalPercent(raw: string): number | null {
  const text = raw.trim();
  if (text === "") return 0;
  if (!/^\d+$/.test(text)) return null;
  const percent = Number(text);
  if (!Number.isSafeInteger(percent) || percent < 0 || percent > 100) return null;
  return percent;
}

function scalarOf(item: ReconciliationItem): string | number {
  return item.POValue as string | number;
}

function historyEventOf(
  item: ReconciliationItem,
  status: "unresolved" | "resolved",
  resolution: string | null,
  actorId: number,
  now: string
): ReconciliationHistoryEvent {
  return {
    Kind: item.Kind,
    SourceLineRef: item.SourceLineRef,
    POValue: item.POValue,
    SourceValue: item.SourceValue,
    Status: status,
    Resolution: resolution,
    ActorID: actorId,
    At: now,
  };
}

/**
 * One commit of one header field. Idempotent: re-committing the same value
 * returns the same item unchanged; editing back to the source value resolves
 * it as values-now-match while retaining its history.
 */
export function commitHeaderField(input: CommitInput): CommitResult {
  const { field, existingItem, actorId, now } = input;

  if (field.kind === "percent") {
    const percent = canonicalPercent(input.poValue);
    if (percent === null) {
      return {
        notice: null,
        item: null,
        itemChanged: false,
        validationError: `${field.label} must be between 0 and 100.`,
      };
    }
    const sourcePercent =
      typeof input.sourceValue === "number"
        ? input.sourceValue
        : canonicalPercent(String(input.sourceValue));
    if (sourcePercent === null) {
      return {
        notice: null,
        item: null,
        itemChanged: false,
        validationError: `${field.label} must be between 0 and 100.`,
      };
    }
    return commitCanonical(field, percent, sourcePercent, existingItem, actorId, now);
  }

  const poText = canonicalText(input.poValue);
  const sourceText = canonicalText(String(input.sourceValue ?? ""));
  if (poText === "" && field.required) {
    return { notice: "empty", item: null, itemChanged: false, validationError: null };
  }
  return commitCanonical(field, poText, sourceText, existingItem, actorId, now);
}

function commitCanonical(
  field: HeaderFieldDef,
  poValue: string | number,
  sourceValue: string | number,
  existingItem: ReconciliationItem | null,
  actorId: number,
  now: string
): CommitResult {
  const idle = { validationError: null };
  if (poValue === sourceValue) {
    if (existingItem === null) {
      return { notice: "matched", item: null, itemChanged: false, ...idle };
    }
    if (scalarOf(existingItem) === poValue && existingItem.Status === "resolved") {
      // Re-committing the kept value: retain the decision and its history.
      return { notice: null, item: existingItem, itemChanged: false, ...idle };
    }
    // Edited back to the source: the same item resolves as values-now-match.
    const resolved: ReconciliationItem = {
      ...existingItem,
      POValue: poValue,
      SourceValue: sourceValue,
      Status: "resolved",
      Resolution: "values_now_match",
      ActorID: actorId,
      UpdatedAt: now,
      History: [
        ...existingItem.History,
        historyEventOf(
          { ...existingItem, POValue: poValue, SourceValue: sourceValue },
          "resolved",
          "values_now_match",
          actorId,
          now
        ),
      ],
    };
    return { notice: null, item: resolved, itemChanged: true, ...idle };
  }

  if (existingItem === null) {
    const created: ReconciliationItem = {
      Key: field.key,
      Field: field.key,
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: field.label,
      POValue: poValue,
      SourceValue: sourceValue,
      Status: "unresolved",
      Resolution: null,
      ActorID: actorId,
      UpdatedAt: now,
      History: [],
    };
    return { notice: null, item: created, itemChanged: true, ...idle };
  }

  if (existingItem.Status === "unresolved" && scalarOf(existingItem) === poValue) {
    return { notice: null, item: existingItem, itemChanged: false, ...idle };
  }
  // A new differing value reopens the same item; earlier decisions stay in
  // its history rather than duplicating rows.
  const reopened: ReconciliationItem = {
    ...existingItem,
    POValue: poValue,
    SourceValue: sourceValue,
    Status: "unresolved",
    Resolution: null,
    ActorID: actorId,
    UpdatedAt: now,
  };
  return { notice: null, item: reopened, itemChanged: true, ...idle };
}

/** Keep PO confirms the kept value: the item resolves, the field is untouched. */
export function applyKeepResolution(
  item: ReconciliationItem,
  actorId: number,
  now: string
): ReconciliationItem {
  return {
    ...item,
    Status: "resolved",
    Resolution: "kept_po",
    ActorID: actorId,
    UpdatedAt: now,
    History: [
      ...item.History,
      historyEventOf(item, "resolved", "kept_po", actorId, now),
    ],
  };
}

/**
 * Use Quotation resolves the item and carries the Quotation value the caller
 * must write into the field before the save can succeed.
 */
export function applyUseResolution(
  item: ReconciliationItem,
  actorId: number,
  now: string
): { item: ReconciliationItem; value: string | number } {
  return {
    item: {
      ...item,
      Status: "resolved",
      Resolution: "used_quotation",
      ActorID: actorId,
      UpdatedAt: now,
      History: [
        ...item.History,
        historyEventOf(item, "resolved", "used_quotation", actorId, now),
      ],
    },
    value: item.SourceValue as string | number,
  };
}

/** Header items gate Final Save: lines, tax and totals are ticket 05. */
export function isHeaderItem(item: ReconciliationItem): boolean {
  return (
    item.Field === "header:paymentTerms" ||
    item.Field === "header:incoterms" ||
    item.Field === "header:discountPercent"
  );
}

export function countUnresolvedHeaderItems(items: ReconciliationItem[]): number {
  return items.filter((item) => isHeaderItem(item) && item.Status === "unresolved")
    .length;
}

export type PaneFilter = "all" | "unresolved" | "resolved";

/** The latest commit event echoed under the pane filters. */
export interface LatestNotice {
  /** Header key or `line:{clientKey}:{field}` for line commits. */
  field: string;
  label: string;
  kind: "empty" | "matched";
}

/** Source scalars for live comparison, from the loaded review source. */
export interface ReconciliationSourceValues {
  paymentTerms: string;
  incoterms: string;
  discountPercent: number;
}

function updatedAtOf(item: ReconciliationItem): string {
  return typeof item.UpdatedAt === "string" ? item.UpdatedAt : "";
}

/**
 * Unresolved newest first, then resolved newest first. Resolved rows are
 * retained, never deleted; filters only hide.
 */
export function orderPaneItems(
  items: ReconciliationItem[],
  filter: PaneFilter
): ReconciliationItem[] {
  const visible =
    filter === "all" ? items : items.filter((item) => item.Status === filter);
  const byRecency = (a: ReconciliationItem, b: ReconciliationItem) =>
    updatedAtOf(b).localeCompare(updatedAtOf(a));
  return [
    ...visible.filter((item) => item.Status === "unresolved").sort(byRecency),
    ...visible.filter((item) => item.Status === "resolved").sort(byRecency),
  ];
}

export function paneFilterCounts(items: ReconciliationItem[]): {
  all: number;
  unresolved: number;
  resolved: number;
} {
  return {
    all: items.length,
    unresolved: items.filter((item) => item.Status === "unresolved").length,
    resolved: items.filter((item) => item.Status === "resolved").length,
  };
}

export interface HeaderSourceValues {
  PaymentTerms: string;
  Incoterms: string;
  DiscountPercent: number;
}

export interface HeaderCurrentValues {
  paymentTerms: string;
  incoterms: string;
  discountPercent: string;
}

/**
 * Brief section 5.5: the finalize review reads the session, it does not
 * re-ask. A kept-PO resolution pre-fills the keep choice when the kept value
 * still differs exactly as recorded; anything else stays undecided. The
 * existing finalize input already carries Decisions keyed by the same contract
 * keys, so no Go signature change is needed.
 */
export function prefillDecisionsFromSession(
  items: ReconciliationItem[],
  current: HeaderCurrentValues,
  source: HeaderSourceValues
): POReviewDecisions {
  const decisions: POReviewDecisions = {};
  const currentByKey: Record<HeaderFieldKey, string | number> = {
    "header:paymentTerms": canonicalText(current.paymentTerms),
    "header:incoterms": canonicalText(current.incoterms),
    "header:discountPercent": canonicalPercent(current.discountPercent) ?? NaN,
  };
  const sourceByKey: Record<HeaderFieldKey, string | number> = {
    "header:paymentTerms": canonicalText(source.PaymentTerms),
    "header:incoterms": canonicalText(source.Incoterms),
    "header:discountPercent": source.DiscountPercent,
  };
  for (const item of items) {
    if (
      item.Field !== "header:paymentTerms" &&
      item.Field !== "header:incoterms" &&
      item.Field !== "header:discountPercent"
    ) {
      continue;
    }
    if (item.Status !== "resolved" || item.Resolution !== "kept_po") continue;
    const key = item.Field as HeaderFieldKey;
    if (scalarOf(item) !== currentByKey[key]) continue;
    if ((item.SourceValue as string | number) !== sourceByKey[key]) continue;
    if (currentByKey[key] === sourceByKey[key]) continue;
    decisions[key] = "po";
  }
  return decisions;
}

/** An empty local session shell for a linked draft with no stored session. */
export function emptyLocalSession(options: {
  purchaseOrderId: number;
  sourceQuotationId: number;
  sourceRevision: number;
  poVersion: number;
}): ReconciliationSession {
  return {
    ID: 0,
    PurchaseOrderID: options.purchaseOrderId,
    SourceQuotationID: options.sourceQuotationId,
    SourceRevision: options.sourceRevision,
    POVersion: options.poVersion,
    Active: true,
    ReadOnly: false,
    CreatedBy: 0,
    CreatedAt: "",
    UpdatedAt: "",
    LineIdentities: [],
    Items: [],
  } as ReconciliationSession;
}
