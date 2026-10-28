import { orderPaneItems, type ReconciliationItem, type ReconciliationSession } from "./poReconciliation";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
import {
  formatQuantityScaled,
  formatUnitPriceMinor,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
} from "./purchaseOrder";
import { formatMoney } from "./format";

// Ticket 05, slice A: line identity, canonical values and the commit core.
// Appended (not interleaved) so each vertical slice stays reviewable.

// ---------------------------------------------------------------------------
// Stable line identity
// ---------------------------------------------------------------------------

/**
 * #517: the session key for one editor line — first-seen wins, mirroring the
 * server seeding (alias where the row carries one, `id:<row>` otherwise) and
 * the additive draft-save bindings. A stored identity pinpoints the row by
 * its bound id or by its key, so an existing `client:` decision is never
 * re-keyed to `id:<row>` when the row later gains an id, and a pre-042 row
 * without alias or identity stays `id:<row>` (the writer accepts it naming
 * the live row) instead of a fresh alias the writer would refuse with
 * save-draft-first guidance. The displayed line number and array position
 * never appear in a key, so a reorder cannot re-key a decision.
 */
export interface SessionLineInput {
  persistedId?: number | null | undefined;
  /** The row-carried alias from the DTO (migration 042), if any. */
  dtoAlias?: string | null | undefined;
  /** The editor alias, always submitted and adopted onto the row on save. */
  clientKey: string;
}

export function sessionLineKey(
  identities: { Key: string; ID?: number | null }[],
  line: SessionLineInput
): string {
  for (const identity of identities) {
    if (
      line.persistedId !== null &&
      line.persistedId !== undefined &&
      line.persistedId > 0 &&
      identity.ID === line.persistedId
    ) {
      return identity.Key;
    }
  }
  const aliases = [line.dtoAlias, line.clientKey].filter(
    (alias): alias is string => typeof alias === "string" && alias !== ""
  );
  for (const identity of identities) {
    if (aliases.includes(identity.Key)) return identity.Key;
  }
  if (
    line.persistedId !== null &&
    line.persistedId !== undefined &&
    line.persistedId > 0 &&
    (line.dtoAlias === null || line.dtoAlias === undefined || line.dtoAlias === "")
  ) {
    return `id:${line.persistedId}`;
  }
  return aliases[0] ?? line.clientKey;
}

/** A collision-resistant client key for a new editor line. */
export function newClientKey(): string {
  const random = (globalThis as { crypto?: { randomUUID?: () => string } })
    .crypto?.randomUUID;
  if (typeof random === "function") return `client:${random()}`;
  return `client:local-${Date.now().toString(36)}-${Math.floor(Math.random() * 0xffffffff).toString(36)}`;
}

export type LineField = "description" | "uom" | "quantity" | "unitPrice" | "discount";

export const LINE_FIELD_SUFFIX: Record<LineField, string> = {
  description: "description",
  uom: "uom",
  quantity: "quantity",
  unitPrice: "unitPrice",
  discount: "discount",
};

const LINE_FIELD_LABELS: Record<LineField, string> = {
  description: "description",
  uom: "unit of measure",
  quantity: "quantity",
  unitPrice: "unit price",
  discount: "discount",
};

/** The catalogue field id for one line column, e.g. `line:quantity`. */
export function lineFieldId(field: LineField): string {
  return `line:${LINE_FIELD_SUFFIX[field]}`;
}

/** The pane label for one line column, e.g. `Line 2 unit of measure`. */
export function lineFieldLabel(displayNumber: number, field: LineField): string {
  return `Line ${displayNumber} ${LINE_FIELD_LABELS[field]}`;
}

// ---------------------------------------------------------------------------
// Canonical line values
// ---------------------------------------------------------------------------

export type CanonicalLineValue =
  | { ok: true; value: string | number | { Enabled: boolean; Percent: number } }
  | { ok: false; error: string };

/**
 * Canonicalises one raw line input for comparison. Quantity and money
 * compare in stored integer units (so `2` and `2.0000` match); the Discount
 * object keeps Enabled so a disabled 0% never equals an enabled 0%.
 */
export function canonicalLineFieldValue(
  field: LineField,
  raw: string | { enabled: boolean; percent: string | number },
  currency: CurrencyInfo
): CanonicalLineValue {
  if (field === "description" || field === "uom") {
    return { ok: true, value: (raw as string).trim() };
  }
  if (field === "quantity") {
    const scaled = parseQuantityScaled(raw as string);
    if (scaled === null) {
      return { ok: false, error: "Quantity must be greater than zero." };
    }
    return { ok: true, value: scaled };
  }
  if (field === "unitPrice") {
    const minor = parseUnitPriceMinor(currency, raw as string);
    if (minor === null) {
      return { ok: false, error: "Unit price must be a valid amount." };
    }
    return { ok: true, value: minor };
  }
  const entered = raw as { enabled: boolean; percent: string | number };
  const percent =
    typeof entered.percent === "number"
      ? entered.percent
      : parseDiscountPercent(entered.percent);
  if (
    percent === null ||
    !Number.isInteger(percent) ||
    percent < 0 ||
    percent > 100
  ) {
    return { ok: false, error: "Discount must be between 0 and 100." };
  }
  return { ok: true, value: { Enabled: entered.enabled, Percent: percent } };
}

/** Canonicalises one source line column for comparison. */
export function canonicalSourceLineValue(
  field: LineField,
  source: PurchaseOrderSourceLineDTO
): string | number | { Enabled: boolean; Percent: number } {
  switch (field) {
    case "description":
      return source.Description.trim();
    case "uom":
      return source.UoM.trim();
    case "quantity":
      return source.QuantityScaled;
    case "unitPrice":
      return source.UnitPriceMinor;
    case "discount":
      return { Enabled: source.DiscountEnabled, Percent: source.DiscountPercent };
  }
}

function lineHistoryEventOf(
  item: ReconciliationItem,
  status: "unresolved" | "resolved",
  resolution: string | null,
  actorId: number,
  now: string
): ReconciliationItem["History"][number] {
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

function lineValuesEqual(a: unknown, b: unknown): boolean {
  if (typeof a !== typeof b) return false;
  if (a !== null && b !== null && typeof a === "object") {
    const left = a as Record<string, unknown>;
    const right = b as Record<string, unknown>;
    const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
    for (const key of keys) {
      if (left[key] !== right[key]) return false;
    }
    return true;
  }
  return a === b;
}

// ---------------------------------------------------------------------------
// Line commit state machine
// ---------------------------------------------------------------------------

export interface LineCommitInput {
  /** Stable line identity, `id:<row>` or `client:<UUID>` — never the position. */
  lineKey: string;
  /** Displayed line number, label only. */
  displayNumber: number;
  field: LineField;
  /** Raw editor value: text, or {enabled, percent} for the discount. */
  poValue: string | { enabled: boolean; percent: string | number };
  /** Null for an unmatched line: nothing to compare, never an item. */
  source: PurchaseOrderSourceLineDTO | null;
  existingItem: ReconciliationItem | null;
  actorId: number;
  now: string;
  currency: CurrencyInfo;
}

export interface LineCommitResult {
  notice: "empty" | "matched" | null;
  item: ReconciliationItem | null;
  itemChanged: boolean;
  validationError: string | null;
}

function scalarOf(item: ReconciliationItem): unknown {
  return item.POValue;
}

/**
 * One commit of one line field. The same notice/review behaviour as the
 * header handler: an unmatched line never compares, an invalid number is a
 * validation error instead of an item, a match with no item shows the
 * matched notice, and a difference creates or reopens one unresolved item
 * keyed by the stable line identity.
 */
export function commitLineField(input: LineCommitInput): LineCommitResult {
  const { lineKey, displayNumber, field, existingItem, actorId, now } = input;
  const canonical = canonicalLineFieldValue(field, input.poValue, input.currency);
  if (!canonical.ok) {
    return {
      notice: null,
      item: null,
      itemChanged: false,
      validationError: (canonical as { error: string }).error,
    };
  }
  if (input.source === null) {
    return { notice: null, item: null, itemChanged: false, validationError: null };
  }
  const idle = { validationError: null as string | null };
  const poValue = (canonical as { value: unknown }).value as
    | string
    | number
    | { Enabled: boolean; Percent: number };
  const sourceValue = canonicalSourceLineValue(field, input.source);
  const sourceRef = sourceLineRef(
    input.source.QuotationItemNo,
    input.source.QuotationSubNo
  );
  const key = `line:${lineKey}:${LINE_FIELD_SUFFIX[field]}`;
  const label = lineFieldLabel(displayNumber, field);
  if (lineValuesEqual(poValue, sourceValue)) {
    if (existingItem === null) {
      return { notice: "matched", item: null, itemChanged: false, ...idle };
    }
    if (
      existingItem.Status === "resolved" &&
      lineValuesEqual(scalarOf(existingItem), poValue)
    ) {
      return { notice: null, item: existingItem, itemChanged: false, ...idle };
    }
    const resolved: ReconciliationItem = {
      ...existingItem,
      Key: key,
      Field: `line:${LINE_FIELD_SUFFIX[field]}`,
      POValue: poValue,
      SourceValue: sourceValue,
      SourceLineRef: sourceRef,
      Label: label,
      Status: "resolved",
      Resolution: "values_now_match",
      ActorID: actorId,
      UpdatedAt: now,
      History: [
        ...existingItem.History,
        lineHistoryEventOf(
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
      Key: key,
      Field: `line:${LINE_FIELD_SUFFIX[field]}`,
      Kind: "difference",
      LineKey: lineKey,
      SourceLineRef: sourceRef,
      Label: label,
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

  if (
    existingItem.Status === "unresolved" &&
    lineValuesEqual(scalarOf(existingItem), poValue)
  ) {
    return { notice: null, item: existingItem, itemChanged: false, ...idle };
  }
  if (
    existingItem.Status === "resolved" &&
    lineValuesEqual(scalarOf(existingItem), poValue)
  ) {
    // Re-committing the kept value: retain the decision and its history.
    return { notice: null, item: existingItem, itemChanged: false, ...idle };
  }
  const reopened: ReconciliationItem = {
    ...existingItem,
    Key: key,
    Field: `line:${LINE_FIELD_SUFFIX[field]}`,
    POValue: poValue,
    SourceValue: sourceValue,
    SourceLineRef: sourceRef,
    Label: label,
    Status: "unresolved",
    Resolution: null,
    ActorID: actorId,
    UpdatedAt: now,
  };
  return { notice: null, item: reopened, itemChanged: true, ...idle };
}

// ---------------------------------------------------------------------------
// Line-set derivation (matched / missing / extra / unknown / duplicate)
// ---------------------------------------------------------------------------

export interface LineMatchInput {
  /** The stable session key — never the display position. */
  sessionKey: string;
  lineNumber: number;
  quotationItemNo: number | null;
  quotationSubNo: number | null | undefined;
}

export interface MatchedLinePair {
  line: LineMatchInput;
  source: PurchaseOrderSourceLineDTO;
}

export interface UnknownLineMatch extends LineMatchInput {
  reference: string;
}

export interface DuplicateMatch {
  reference: string;
  lineKeys: string[];
}

export interface DerivedLineSet {
  matched: MatchedLinePair[];
  missing: PurchaseOrderSourceLineDTO[];
  extra: LineMatchInput[];
  unknown: UnknownLineMatch[];
  duplicates: DuplicateMatch[];
}

/** The stable source reference: "2" or "2.1". Never a position. */
export function sourceLineRef(
  itemNo: number,
  subNo: number | null | undefined
): string {
  return subNo === null || subNo === undefined ? `${itemNo}` : `${itemNo}.${subNo}`;
}

/**
 * The line set from the editor's match assignments, not from positions. A
 * null reference is an extra line; a non-null reference absent from the
 * source is unknown (R-02: acknowledgement alone cannot clear it); a
 * reference claimed twice is a blocking duplicate until one match is
 * corrected.
 */
export function deriveLineSet(
  lines: LineMatchInput[],
  sources: PurchaseOrderSourceLineDTO[]
): DerivedLineSet {
  const byRef = new Map<string, PurchaseOrderSourceLineDTO>();
  for (const source of sources) {
    byRef.set(sourceLineRef(source.QuotationItemNo, source.QuotationSubNo), source);
  }
  const matched: MatchedLinePair[] = [];
  const extra: LineMatchInput[] = [];
  const unknown: UnknownLineMatch[] = [];
  const claims = new Map<string, string[]>();
  for (const line of lines) {
    if (line.quotationItemNo === null) {
      extra.push(line);
      continue;
    }
    const reference = sourceLineRef(line.quotationItemNo, line.quotationSubNo);
    const source = byRef.get(reference);
    if (source === undefined) {
      unknown.push({ ...line, reference });
      continue;
    }
    matched.push({ line, source });
    const claimants = claims.get(reference) ?? [];
    claimants.push(line.sessionKey);
    claims.set(reference, claimants);
  }
  const claimedRefs = new Set(
    matched.map((entry) =>
      sourceLineRef(entry.source.QuotationItemNo, entry.source.QuotationSubNo)
    )
  );
  const missing = sources.filter(
    (source) =>
      !claimedRefs.has(sourceLineRef(source.QuotationItemNo, source.QuotationSubNo))
  );
  const duplicates: DuplicateMatch[] = [];
  for (const [reference, lineKeys] of claims) {
    if (lineKeys.length > 1) duplicates.push({ reference, lineKeys });
  }
  return { matched, missing, extra, unknown, duplicates };
}

/**
 * Resolving a removed line: every item keyed to it resolves as line_removed
 * with its history retained. The surviving line stays exactly as it was
 * (R-08). Returns the removed stable keys so the caller can tombstone them.
 */
export function resolveLineRemoved(
  items: ReconciliationItem[],
  sessionKey: string,
  actorId: number,
  now: string
): { items: ReconciliationItem[]; removed: string[] } {
  let touched = false;
  const next = items.map((item) => {
    if (item.LineKey !== sessionKey || item.Resolution === "line_removed") {
      return item;
    }
    touched = true;
    return {
      ...item,
      Status: "resolved",
      Resolution: "line_removed",
      ActorID: actorId,
      UpdatedAt: now,
      History: [
        ...item.History,
        lineHistoryEventOf(item, "resolved", "line_removed", actorId, now),
      ],
    };
  });
  return { items: next, removed: touched ? [sessionKey] : [] };
}

// ---------------------------------------------------------------------------
// Totals strip (derived only; never a decision item)
// ---------------------------------------------------------------------------

export interface TotalsExplainingDifference {
  key: string;
  label: string;
  poDisplay: string;
  sourceDisplay: string;
  differs: boolean;
}

/**
 * The first differing comparable input in document order, formatted for the
 * strip. Null is never returned: with no differing input the reason names
 * the unexplained printed total as evidence, never a target (R-09).
 */
export function explainTotalsDifference(
  differences: TotalsExplainingDifference[]
): string {
  const first = differences.find((entry) => entry.differs);
  if (first === undefined) {
    return "no input difference explains the printed total";
  }
  return `${first.label} ${first.poDisplay} vs ${first.sourceDisplay}`;
}

// ---------------------------------------------------------------------------
// Final Save blockers
// ---------------------------------------------------------------------------

export interface BlockerMissingLine {
  quotationItemNo: number;
  quotationSubNo: number | null | undefined;
  acknowledged: boolean;
}

export interface BlockerExtraLine {
  sessionKey: string;
  label: string;
  acknowledged: boolean;
}

export interface BlockerDuplicate {
  reference: string;
  lineKeys: string[];
}

export interface BlockerUnknownLine {
  sessionKey: string;
  reference: string;
  acknowledged: boolean;
}

export interface SessionBlockerInput {
  items: ReconciliationItem[];
  missing: BlockerMissingLine[];
  extra: BlockerExtraLine[];
  duplicates: BlockerDuplicate[];
  unknown: BlockerUnknownLine[];
}

export interface SessionBlocker {
  kind: "unresolved" | "missing" | "extra" | "duplicate" | "unknown";
  label: string;
}

/**
 * Everything that blocks Final Save, named. Unresolved items of any kind,
 * unacknowledged missing lines, unacknowledged plain extras, every duplicate
 * group and every unknown reference. An acknowledged unknown still blocks
 * (R-02): the acknowledgement records the decision but cannot clear a
 * reference the saved-reference validation will refuse.
 */
export function sessionBlockers(input: SessionBlockerInput): SessionBlocker[] {
  const blockers: SessionBlocker[] = [];
  const unresolvedByKey = new Set(
    input.items.filter((item) => item.Status === "unresolved").map((item) => item.Key)
  );
  for (const item of orderPaneItems(input.items, "all")) {
    if (item.Status !== "unresolved") continue;
    blockers.push({ kind: "unresolved", label: item.Label });
  }
  // A set entry already represented by an unresolved match item is covered:
  // the item above names the blocker, so the entry adds nothing.
  for (const missing of input.missing) {
    if (missing.acknowledged) continue;
    const reference = sourceLineRef(missing.quotationItemNo, missing.quotationSubNo);
    if (unresolvedByKey.has(`source:${reference}:match`)) continue;
    blockers.push({
      kind: "missing",
      label: `Quotation line ${reference} is not on this purchase order`,
    });
  }
  for (const extra of input.extra) {
    if (extra.acknowledged) continue;
    if (unresolvedByKey.has(`line:${extra.sessionKey}:match`)) continue;
    blockers.push({ kind: "extra", label: extra.label });
  }
  for (const duplicate of input.duplicates) {
    if (unresolvedByKey.has(`source:${duplicate.reference}:duplicate`)) continue;
    blockers.push({
      kind: "duplicate",
      label: `Duplicate match · Quotation ${duplicate.reference}`,
    });
  }
  for (const unknown of input.unknown) {
    if (unresolvedByKey.has(`line:${unknown.sessionKey}:match`)) continue;
    blockers.push({
      kind: "unknown",
      label: `Unknown reference · ${unknown.reference}`,
    });
  }
  return blockers;
}

export function countBlockingItems(blockers: SessionBlocker[]): number {
  return blockers.length;
}

/**
 * Whether the item carries a recorded acknowledgement of the given kind.
 * Unknown references keep their unresolved status either way (R-02); this
 * only decides whether the card shows the recorded-acknowledgement note.
 */
export function hasAckRecording(
  item: ReconciliationItem,
  resolution: "acknowledged_missing" | "acknowledged_extra"
): boolean {
  return item.History.some((event) => event.Resolution === resolution);
}

// ---------------------------------------------------------------------------
// Finalize prefill for lines
// ---------------------------------------------------------------------------

export interface LineValuesInput {
  description: string;
  uom: string;
  quantity: string;
  unitPrice: string;
  discountEnabled: boolean;
  discountPercent: string;
}

/**
 * Section 5.5 for lines: a kept line decision with unchanged values
 * pre-fills the keep choice under the review's positional key. Changed
 * values, used or matched resolutions and unknown keys pre-fill nothing.
 * The caller passes raw editor values; canonicalisation happens here so the
 * comparison uses stored units, never display text.
 */
export function prefillLineDecisions(
  items: ReconciliationItem[],
  lines: { sessionKey: string; displayNumber: number; values: LineValuesInput }[],
  sources: PurchaseOrderSourceLineDTO[],
  currency: CurrencyInfo
): { [key: string]: "po" } {
  const decisions: { [key: string]: "po" } = {};
  const bySessionKey = new Map(lines.map((line) => [line.sessionKey, line]));
  const byRef = new Map(
    sources.map((source) => [
      sourceLineRef(source.QuotationItemNo, source.QuotationSubNo),
      source,
    ])
  );
  for (const item of items) {
    if (item.Status !== "resolved" || item.Resolution !== "kept_po") continue;
    if (item.LineKey === null || item.LineKey === undefined) continue;
    const line = bySessionKey.get(item.LineKey);
    if (line === undefined) continue;
    const field = item.Field;
    const suffix =
      field === "line:description"
        ? ("description" as const)
        : field === "line:uom"
          ? ("uom" as const)
          : field === "line:quantity"
            ? ("quantity" as const)
            : field === "line:unitPrice"
              ? ("unitPrice" as const)
              : field === "line:discount"
                ? ("discount" as const)
                : null;
    if (suffix === null) continue;
    const raw =
      suffix === "discount"
        ? {
            enabled: line.values.discountEnabled,
            percent: line.values.discountPercent,
          }
        : line.values[suffix];
    const canonical = canonicalLineFieldValue(suffix, raw, currency);
    if (!canonical.ok) continue;
    if (!lineValuesEqual(canonical.value, item.POValue)) continue;
    // The recorded source value must still match the linked revision;
    // otherwise the decision belongs to a different comparison.
    if (typeof item.SourceLineRef === "string") {
      const source = byRef.get(item.SourceLineRef);
      if (source === undefined) continue;
      const currentSource = canonicalSourceLineValue(suffix, source);
      if (!lineValuesEqual(item.SourceValue, currentSource)) continue;
    }
    decisions[`line:${line.displayNumber}:${suffix}`] = "po";
  }
  return decisions;
}

// ---------------------------------------------------------------------------
// Declarative line-set sync (the rematch/remove effect target)
// ---------------------------------------------------------------------------

export interface ReconcileLineInput {
  /** The stable session key, resolved by the caller via sessionLineKey. */
  sessionKey: string;
  persistedId?: number | null;
  displayNumber: number;
  quotationItemNo: number | null;
  quotationSubNo: number | null | undefined;
}

export interface ReconcileLineSessionInput {
  session: ReconciliationSession;
  lines: ReconcileLineInput[];
  sources: PurchaseOrderSourceLineDTO[];
  actorId: number;
  now: string;
}

function matchItemOf(
  sessionKey: string,
  line: ReconcileLineInput,
  reference: string | null,
  actorId: number,
  now: string
): ReconciliationItem {
  const unknown = reference !== null;
  return {
    Key: `line:${sessionKey}:match`,
    Field: "line:match",
    Kind: "extra_line",
    LineKey: sessionKey,
    SourceLineRef: reference,
    Label: unknown ? `Unknown reference · ${reference}` : `Extra line · Line ${line.displayNumber}`,
    POValue: unknown ? `Line ${line.displayNumber} names quotation line ${reference}, which does not exist` : `Line ${line.displayNumber} is not on the quotation`,
    SourceValue: null,
    Status: "unresolved",
    Resolution: null,
    ActorID: actorId,
    UpdatedAt: now,
    History: [],
  };
}

function missingItemOf(
  reference: string,
  source: PurchaseOrderSourceLineDTO,
  actorId: number,
  now: string
): ReconciliationItem {
  return {
    Key: `source:${reference}:match`,
    Field: "source:match",
    Kind: "missing_line",
    LineKey: null,
    SourceLineRef: reference,
    Label: `Missing line · Quotation ${reference}`,
    POValue: null,
    SourceValue: source.Description,
    Status: "unresolved",
    Resolution: null,
    ActorID: actorId,
    UpdatedAt: now,
    History: [],
  };
}

function duplicateItemOf(
  reference: string,
  actorId: number,
  now: string
): ReconciliationItem {
  return {
    Key: `source:${reference}:duplicate`,
    Field: "source:match",
    Kind: "duplicate_match",
    LineKey: null,
    SourceLineRef: reference,
    Label: `Duplicate match · Quotation ${reference}`,
    POValue: null,
    SourceValue: null,
    Status: "resolved",
    Resolution: null,
    ActorID: actorId,
    UpdatedAt: now,
    History: [],
  };
}

function resolveAs(
  item: ReconciliationItem,
  status: "resolved",
  resolution: string,
  actorId: number,
  now: string
): ReconciliationItem {
  return {
    ...item,
    Status: status,
    Resolution: resolution,
    ActorID: actorId,
    UpdatedAt: now,
    History: [
      ...item.History,
      lineHistoryEventOf(item, status, resolution, actorId, now),
    ],
  };
}

/**
 * Declaratively reconciles the session's line set with the editor: removals
 * resolve as line_removed with history retained (R-08); missing, extra,
 * unknown and duplicate matches gain their items; a corrected match resolves
 * as match_corrected. Existing resolutions are never rewritten — only an
 * unresolved item, or a resolved ack whose condition cleared, transitions.
 * Returns the same session reference when nothing changed.
 */
export function reconcileLineSession(
  input: ReconcileLineSessionInput
): { session: ReconciliationSession; changed: boolean } {
  const { session, lines, sources, actorId, now } = input;
  let changed = false;
  let items = session.Items;
  let identities = session.LineIdentities;

  // 1. Identities: current lines stay live, vanished ones mark removed.
  const currentKeys = new Set(lines.map((line) => line.sessionKey));
  const nextIdentities: ReconciliationSession["LineIdentities"] = [];
  for (const line of lines) {
    nextIdentities.push({
      Key: line.sessionKey,
      ID: line.persistedId ?? null,
      Removed: false,
    });
  }
  for (const identity of identities) {
    if (!currentKeys.has(identity.Key) && !identity.Removed) {
      nextIdentities.push({ ...identity, Removed: true });
    } else if (!currentKeys.has(identity.Key) && identity.Removed) {
      nextIdentities.push(identity);
    }
  }
  if (JSON.stringify(nextIdentities) !== JSON.stringify(identities)) {
    identities = nextIdentities;
    changed = true;
  }

  // 2. Removed lines resolve their items as line_removed (history retained).
  const removedKeys = nextIdentities
    .filter((identity) => identity.Removed)
    .map((identity) => identity.Key);
  const liveBeforeCounters = items;
  for (const key of removedKeys) {
    const stillLive = items.some(
      (item) => item.LineKey === key && item.Resolution !== "line_removed"
    );
    if (!stillLive) continue;
    const result = resolveLineRemoved(items, key, actorId, now);
    if (result.removed.length > 0) {
      items = result.items;
      changed = true;
    }
  }
  void liveBeforeCounters;

  // 3. Derive the set once, then sync match items.
  const set = deriveLineSet(
    lines.map((line) => ({
      sessionKey: line.sessionKey,
      lineNumber: line.displayNumber,
      quotationItemNo: line.quotationItemNo,
      quotationSubNo: line.quotationSubNo,
    })),
    sources
  );
  const missingRefs = new Set(
    set.missing.map((source) => sourceLineRef(source.QuotationItemNo, source.QuotationSubNo))
  );
  void missingRefs;
  const byKey = new Map(items.map((item) => [item.Key, item]));
  let next = [...items];
  const upsert = (item: ReconciliationItem, replace: boolean) => {
    const at = next.findIndex((entry) => entry.Key === item.Key);
    if (at === -1) {
      next.push(item);
      changed = true;
    } else if (replace) {
      next[at] = item;
      changed = true;
    }
  };

  // Missing: ensure unresolved items; resolve as match_corrected once claimed.
  const claimedRefs = new Set(
    set.matched.map((entry) =>
      sourceLineRef(entry.source.QuotationItemNo, entry.source.QuotationSubNo)
    )
  );
  for (const source of set.missing) {
    const reference = sourceLineRef(source.QuotationItemNo, source.QuotationSubNo);
    const existing = byKey.get(`source:${reference}:match`);
    if (existing === undefined) {
      upsert(missingItemOf(reference, source, actorId, now), false);
    }
  }
  for (const item of items) {
    if (
      item.Kind === "missing_line" &&
      typeof item.SourceLineRef === "string" &&
      claimedRefs.has(item.SourceLineRef) &&
      item.Resolution !== "match_corrected"
    ) {
      upsert(resolveAs(item, "resolved", "match_corrected", actorId, now), true);
    }
  }

  // Extra / unknown: one match item per line, keyed by stable identity.
  for (const extra of set.extra) {
    const line = lines.find((entry) => entry.sessionKey === extra.sessionKey);
    const key = `line:${extra.sessionKey}:match`;
    const existing = byKey.get(key);
    if (existing === undefined) {
      upsert(matchItemOf(extra.sessionKey, line ?? { ...extra, displayNumber: extra.lineNumber }, null, actorId, now), false);
    } else if (
      existing.SourceLineRef !== null &&
      existing.Status === "resolved" &&
      existing.Resolution !== "match_corrected"
    ) {
      // A formerly unknown line is now a plain extra: re-explain it.
      upsert(
        {
          ...existing,
          SourceLineRef: null,
          Label: `Extra line · Line ${extra.lineNumber}`,
          POValue: `Line ${extra.lineNumber} is not on the quotation`,
          Status: "unresolved",
          Resolution: null,
          ActorID: actorId,
          UpdatedAt: now,
        },
        true
      );
    } else if (
      existing.SourceLineRef === null &&
      existing.Status === "resolved" &&
      existing.Resolution !== "acknowledged_extra" &&
      existing.Resolution !== "match_corrected"
    ) {
      void 0;
    }
  }
  for (const unknown of set.unknown) {
    const key = `line:${unknown.sessionKey}:match`;
    const existing = byKey.get(key);
    if (existing === undefined) {
      const line = lines.find((entry) => entry.sessionKey === unknown.sessionKey);
      upsert(
        matchItemOf(unknown.sessionKey, line ?? { sessionKey: "", persistedId: null, displayNumber: unknown.lineNumber, quotationItemNo: null, quotationSubNo: null }, unknown.reference, actorId, now),
        false
      );
    } else if (existing.SourceLineRef !== unknown.reference) {
      // The claim moved to another unknown reference: reopen on the new one.
      upsert(
        {
          ...existing,
          SourceLineRef: unknown.reference,
          Label: `Unknown reference · ${unknown.reference}`,
          POValue: `Line ${unknown.lineNumber} names quotation line ${unknown.reference}, which does not exist`,
          Status: "unresolved",
          Resolution: null,
          ActorID: actorId,
          UpdatedAt: now,
          History: [
            ...existing.History,
            lineHistoryEventOf(
              { ...existing, SourceLineRef: unknown.reference },
              "unresolved",
              null,
              actorId,
              now
            ),
          ],
        },
        true
      );
    }
  }
  // Matched lines: a stale extra/unknown match item resolves as corrected.
  const matchedKeys = new Set(set.matched.map((entry) => entry.line.sessionKey));
  for (const pair of set.matched) {
    const key = `line:${pair.line.sessionKey}:match`;
    const existing = byKey.get(key);
    if (
      existing !== undefined &&
      existing.Status !== "resolved" &&
      existing.Kind === "extra_line"
    ) {
      upsert(resolveAs(existing, "resolved", "match_corrected", actorId, now), true);
    } else if (
      existing !== undefined &&
      existing.Status === "resolved" &&
      existing.Resolution !== "match_corrected" &&
      existing.Kind === "extra_line"
    ) {
      upsert(resolveAs(existing, "resolved", "match_corrected", actorId, now), true);
    }
  }
  void matchedKeys;

  // Duplicates: one blocking item per over-claimed reference.
  const dupRefs = new Set(set.duplicates.map((entry) => entry.reference));
  for (const duplicate of set.duplicates) {
    const key = `source:${duplicate.reference}:duplicate`;
    if (byKey.get(key) === undefined) {
      upsert(duplicateItemOf(duplicate.reference, actorId, now), false);
      const created = next.find((entry) => entry.Key === key);
      if (created !== undefined) {
        upsert({ ...created, Status: "unresolved", Resolution: null }, true);
      }
    } else {
      const existing = byKey.get(key);
      if (existing !== undefined && existing.Status === "resolved" && existing.Resolution !== "match_corrected") {
        upsert({ ...existing, Status: "unresolved", Resolution: null, ActorID: actorId, UpdatedAt: now }, true);
      }
    }
  }
  for (const item of items) {
    if (
      item.Kind === "duplicate_match" &&
      typeof item.SourceLineRef === "string" &&
      !dupRefs.has(item.SourceLineRef) &&
      item.Status === "unresolved"
    ) {
      upsert(resolveAs(item, "resolved", "match_corrected", actorId, now), true);
    }
  }
  void missingRefs;

  if (!changed) return { session, changed: false };
  return { session: { ...session, LineIdentities: identities, Items: next }, changed: true };
}

/**
 * Pane acknowledgement for a line-set card. Missing and plain extra cards
 * resolve; an unknown reference only records the acknowledgement and stays
 * unresolved (R-02: the saved-reference validation will still refuse it).
 */
export function acknowledgeLineMatch(
  session: ReconciliationSession,
  key: string,
  resolution: "acknowledged_missing" | "acknowledged_extra",
  actorId: number,
  now: string
): { session: ReconciliationSession; changed: boolean } {
  const target = session.Items.find((entry) => entry.Key === key);
  if (target === undefined || target.Status === "resolved") {
    return { session, changed: false };
  }
  const unknown =
    resolution === "acknowledged_extra" &&
    typeof target.SourceLineRef === "string" &&
    target.SourceLineRef !== "";
  if (unknown) {
    const noted: ReconciliationItem = {
      ...target,
      ActorID: actorId,
      UpdatedAt: now,
      History: [
        ...target.History,
        lineHistoryEventOf(target, "unresolved", resolution, actorId, now),
      ],
    };
    return {
      session: {
        ...session,
        Items: session.Items.map((entry) => (entry.Key === key ? noted : entry)),
      },
      changed: true,
    };
  }
  const resolved = resolveAs(target, "resolved", resolution, actorId, now);
  return {
    session: {
      ...session,
      Items: session.Items.map((entry) => (entry.Key === key ? resolved : entry)),
    },
    changed: true,
  };
}

// ---------------------------------------------------------------------------
// Totals-strip reason inputs (document order: header discount, then lines)
// ---------------------------------------------------------------------------

function discountDisplay(enabled: boolean, percent: number): string {
  return enabled ? `${percent}%` : "No discount";
}

/**
 * The whole-PO discount as a comparable strip input. An unparseable editor
 * value never claims to explain the total: the strip reads pending instead.
 */
export function headerDiscountDifference(
  poRaw: string,
  sourcePercent: number
): TotalsExplainingDifference {
  const parsed = parseDiscountPercent(poRaw);
  const differs = parsed === null ? false : parsed !== sourcePercent;
  return {
    key: "header:discountPercent",
    label: "Whole-PO discount",
    poDisplay: poRaw.trim() === "" ? "(empty)" : `${poRaw.trim()}%`,
    sourceDisplay: `${sourcePercent}%`,
    differs,
  };
}

export interface StripLineInput {
  displayNumber: number;
  values: LineValuesInput;
  source: PurchaseOrderSourceLineDTO | null;
}

/**
 * One strip input per money-moving line field, in document order.
 * Description and unit of measure never move a total and are excluded.
 * Unmatched lines compare against nothing and are excluded too.
 */
export function lineMoneyDifferences(
  lines: StripLineInput[],
  currency: CurrencyInfo
): TotalsExplainingDifference[] {
  const out: TotalsExplainingDifference[] = [];
  const fields: LineField[] = ["quantity", "unitPrice", "discount"];
  for (const line of lines) {
    if (line.source === null) continue;
    for (const field of fields) {
      const raw =
        field === "discount"
          ? { enabled: line.values.discountEnabled, percent: line.values.discountPercent }
          : line.values[field];
      const canonical = canonicalLineFieldValue(field, raw, currency);
      const sourceValue = canonicalSourceLineValue(field, line.source);
      const label = lineFieldLabel(line.displayNumber, field);
      const key = `line:${line.displayNumber}:${LINE_FIELD_SUFFIX[field]}`;
      if (!canonical.ok) {
        out.push({ key, label, poDisplay: "(invalid)", sourceDisplay: sourceDisplayOf(field, line.source, currency), differs: false });
        continue;
      }
      const differs = !lineValuesEqual(canonical.value, sourceValue);
      out.push({
        key,
        label,
        poDisplay: poDisplayOf(field, raw, currency),
        sourceDisplay: sourceDisplayOf(field, line.source, currency),
        differs,
      });
    }
  }
  return out;
}

function poDisplayOf(
  field: LineField,
  raw: string | { enabled: boolean; percent: string | number },
  currency: CurrencyInfo
): string {
  if (field === "quantity") return (raw as string).trim() === "" ? "(empty)" : (raw as string).trim();
  if (field === "unitPrice") {
    const minor = parseUnitPriceMinor(currency, raw as string);
    if (minor === null) return (raw as string).trim() === "" ? "(empty)" : (raw as string).trim();
    return formatMoney(currency, minor);
  }
  const entered = raw as { enabled: boolean; percent: string | number };
  const percent = typeof entered.percent === "number" ? entered.percent : parseDiscountPercent(entered.percent);
  if (percent === null) return "(invalid)";
  return discountDisplay(entered.enabled, percent);
}

function sourceDisplayOf(
  field: LineField,
  source: PurchaseOrderSourceLineDTO,
  currency: CurrencyInfo
): string {
  switch (field) {
    case "quantity":
      return formatQuantityScaled(source.QuantityScaled);
    case "unitPrice":
      return formatMoney(currency, source.UnitPriceMinor);
    case "discount":
      return discountDisplay(source.DiscountEnabled, source.DiscountPercent);
    default:
      return String(canonicalSourceLineValue(field, source));
  }
}
