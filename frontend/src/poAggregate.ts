// Issue #654: the PO-only aggregate line, as pure domain rules.
//
// One customer-facing aggregate line may group one or more existing PO display
// groups (a plain line, or a split line and its sub-items). The aggregate is
// presentation only: its header has a title, a display quantity, a display
// unit, a "show children to the customer" toggle and a *derived* total. It
// never has a price, a cost, a variance or a stored total of its own, and the
// child rows stay the priced truth with their own allocations.
//
// This module owns every grouping rule and the exact arithmetic the editor,
// the customer projection and the save payload share. It has no React or DOM
// imports. It imports only types from purchaseOrder.ts, so the runtime
// dependency stays one-way: purchaseOrder.ts imports the runtime functions
// here, never the other way round. That is why the exact input parsing lives
// in this lower layer and purchaseOrder.ts re-exports it.

import { MAX_DISCOUNT_PERCENT, tryParseMoneyInput, type CurrencyInfo } from "./format";
import type {
  PurchaseOrderAggregateGroupInput,
  PurchaseOrderAggregateGroupState,
  PurchaseOrderEditorState,
  PurchaseOrderLine,
} from "./purchaseOrder";

// The discount ceiling is the same 100% cap the quotation pricing uses; keeping
// one definition in format.ts stops the two layers drifting.
export { MAX_DISCOUNT_PERCENT };

/** The fixed four-decimal quantity scale shared with the Go store. */
export const QUANTITY_SCALE = 10000;

/**
 * The supported money range: 2^53-1, JavaScript's Number.MAX_SAFE_INTEGER.
 *
 * Every amount crosses JSON as a `number`, which can only name every integer up
 * to this bound. Go's int64 is wider, but 27_021_597_764_222_973 reads back one
 * minor unit lower, so the store, the schema CHECK and this module all refuse
 * anything above it rather than display an inexact total. The Go constant is
 * MaxExactMinor in backend/store/po_calc.go.
 */
export const MAX_EXACT_MINOR = Number.MAX_SAFE_INTEGER;

const MAX_EXACT_MINOR_BIGINT = BigInt(MAX_EXACT_MINOR);

/**
 * Sanitizes one quantity keystroke for display: digits and at most one decimal
 * separator, with four places. A comma is accepted as the decimal separator and
 * normalized to a dot, because the office types both and `parseQuantityScaled`
 * reads both; dropping the comma would silently turn "2,5" into 25.
 *
 * This is the one implementation the quotation editor's line fields and the
 * aggregate header's display quantity share, so the two fields cannot drift.
 * It never parses, rounds or validates — saving does that.
 */
export function sanitizeQuantityInput(raw: string): string {
  const cleaned = raw.replace(/[^\d.,]/g, "").replace(/,/g, ".");
  const dot = cleaned.indexOf(".");
  if (dot === -1) return cleaned;
  return `${cleaned.slice(0, dot)}.${cleaned
    .slice(dot + 1)
    .replace(/\./g, "")
    .slice(0, 4)}`;
}

/**
 * Parses a quantity typed as a decimal string into the exact 10^4-scaled
 * integer. Up to four decimal places; a comma is accepted as the decimal
 * separator because the office types both. Positive only: a zero-quantity
 * line is not an order line, and the database refuses it too. Null means the
 * field cannot be saved.
 */
export function parseQuantityScaled(raw: string): number | null {  const text = raw.trim();
  if (text === "") return null;
  const match = /^(\d+)(?:([.,])(\d{0,4}))?$/.exec(text);
  if (!match) return null;
  const whole = Number(match[1]);
  const fraction = Number((match[3] ?? "").padEnd(4, "0"));
  if (!Number.isSafeInteger(whole) || whole > MAX_EXACT_MINOR / QUANTITY_SCALE) {
    return null;
  }
  const scaled = whole * QUANTITY_SCALE + fraction;
  if (!Number.isSafeInteger(scaled) || scaled <= 0 || scaled > MAX_EXACT_MINOR) {
    return null;
  }
  return scaled;
}

/**
 * Parses a unit-price field in the PO's own currency. Null means not a
 * number; zero is a real, valid price. `tryParseMoneyInput` is the strict
 * variant of parseMoneyInput, so unparseable text cannot silently become 0.
 */
export function parseUnitPriceMinor(
  currency: CurrencyInfo,
  raw: string
): number | null {
  const parsed = tryParseMoneyInput(currency, raw);
  if (
    parsed === null ||
    parsed < 0 ||
    !Number.isSafeInteger(parsed) ||
    parsed > MAX_EXACT_MINOR
  ) {
    return null;
  }
  return parsed;
}

/**
 * Parses a discount field as a whole percentage from 0 to 100. Empty means no
 * discount. A decimal ("10.5") is refused rather than rounded, because the
 * stored column is an integer percentage and a silent round would print a
 * different discount from the one entered.
 */
export function parseDiscountPercent(raw: string): number | null {
  const text = raw.trim();
  if (text === "") return 0;
  if (!/^\d+$/.test(text)) return null;
  const percent = Number(text);
  if (!Number.isSafeInteger(percent)) return null;
  if (percent < 0 || percent > MAX_DISCOUNT_PERCENT) return null;
  return percent;
}

/** The one range check every exact minor-unit result passes through. */
export function toMoneyMinor(value: bigint): number | null {
  if (value < 0n || value > MAX_EXACT_MINOR_BIGINT) return null;
  return Number(value);
}

/**
 * Exact `quantity × price ÷ 10^4` with the half-away-from-zero rounding the Go
 * calculator uses. A big-int intermediate is what makes an overflowing line
 * detectable instead of silently wrapped.
 */
export function lineGrossMinor(
  quantityScaled: number,
  unitPriceMinor: number
): number | null {
  const product = BigInt(quantityScaled) * BigInt(unitPriceMinor);
  const divisor = BigInt(QUANTITY_SCALE);
  const quotient = product / divisor;
  const remainder = product % divisor;
  const rounded = remainder * 2n >= divisor ? quotient + 1n : quotient;
  return toMoneyMinor(rounded);
}

/** `amount × percent ÷ 100`, truncated, exactly as the quotation does it. */
export function rowDiscountMinor(
  amountMinor: number,
  percent: number
): number | null {
  return toMoneyMinor((BigInt(amountMinor) * BigInt(percent)) / 100n);
}

/**
 * One line's exact net: gross minus its legacy row discount. Null when any
 * field cannot be parsed or an intermediate exceeds the supported range, so a
 * caller never totals an order from a partial figure.
 */
export function lineNetMinor(
  line: PurchaseOrderLine,
  currency: CurrencyInfo
): number | null {
  const quantity = parseQuantityScaled(line.quantity);
  if (quantity === null) return null;
  const price = parseUnitPriceMinor(currency, line.unitPrice);
  if (price === null) return null;
  const percent = parseDiscountPercent(line.discountPercent);
  if (percent === null) return null;
  const gross = lineGrossMinor(quantity, price);
  if (gross === null) return null;
  const discount = line.discountEnabled ? rowDiscountMinor(gross, percent) : 0;
  if (discount === null) return null;
  return gross - discount;
}

/**
 * The aggregate membership of one line. A single accessor, so callers never
 * reach into the field and an absent key can never be confused with group 0.
 */
export function aggregateKeyOf(line: PurchaseOrderLine): number | null {
  const key = line.aggregateKey;
  return typeof key === "number" && Number.isSafeInteger(key) ? key : null;
}

/**
 * The derived header total: the sum of its child lines' nets. Null when any
 * child cannot be totalled (never a partial sum) or the exact sum exceeds the
 * supported range.
 */
export function aggregateTotalMinor(
  lines: PurchaseOrderLine[],
  aggregateKey: number,
  currency: CurrencyInfo
): number | null {
  let total = 0n;
  for (const line of lines) {
    if (aggregateKeyOf(line) !== aggregateKey) continue;
    const net = lineNetMinor(line, currency);
    if (net === null) return null;
    total += BigInt(net);
  }
  return toMoneyMinor(total);
}

/**
 * The derived display unit price: `round_half_away(total × 10^4 ÷ quantity)`.
 * It is a display figure only; the total is authoritative, quantities are
 * never summed across units (AGENTS.md §10), and an exact value outside the
 * safe range is null rather than an inexact number.
 */
export function aggregateDisplayUnitPriceMinor(
  totalMinor: number,
  quantityScaled: number
): number | null {
  if (!Number.isSafeInteger(totalMinor) || totalMinor < 0) return null;
  if (
    !Number.isSafeInteger(quantityScaled) ||
    quantityScaled <= 0
  ) {
    return null;
  }
  const product = BigInt(totalMinor) * BigInt(QUANTITY_SCALE);
  const divisor = BigInt(quantityScaled);
  const quotient = product / divisor;
  const remainder = product % divisor;
  const rounded = remainder * 2n >= divisor ? quotient + 1n : quotient;
  return toMoneyMinor(rounded);
}

/**
 * The #573 display-group key of one row, mirrored from the quotation editor's
 * grouping: a stored group key wins; a pre-migration row with a quotation
 * sub-number still belongs to its item, under the key migration 048 backfills;
 * a plain line has no display key.
 *
 * It lives here because a whole display group is the unit an aggregate moves,
 * and one implementation is what keeps a split package from being torn apart.
 */
export function displayGroupKeyOf(line: PurchaseOrderLine): string {
  if (line.groupKey !== "") return line.groupKey;
  if (line.quotationItemNo !== null && line.quotationSubNo !== null) {
    return `q:${line.quotationItemNo}`;
  }
  return "";
}

interface DisplayGroup {
  key: string;
  lines: PurchaseOrderLine[];
}

/**
 * Rows grouped into display groups at the position of their first row, exactly
 * as the quotation editor presents a split line: one package, never a row-by-
 * row walk. A plain line is a group of one with an empty key.
 */
function displayGroupsOf(lines: PurchaseOrderLine[]): DisplayGroup[] {
  const groups: DisplayGroup[] = [];
  const byKey = new Map<string, DisplayGroup>();
  for (const line of lines) {
    const key = displayGroupKeyOf(line);
    if (key === "") {
      groups.push({ key: "", lines: [line] });
      continue;
    }
    const existing = byKey.get(key);
    if (existing !== undefined) {
      existing.lines.push(line);
      continue;
    }
    const group: DisplayGroup = { key, lines: [line] };
    byKey.set(key, group);
    groups.push(group);
  }
  return groups;
}

/**
 * Moves the selected display groups into one aggregate run.
 *
 * - A selected ghost is materialized first, in the order the caller supplies.
 * - Selecting any row of a split package selects the whole package.
 * - The chosen groups move together to the earliest selected display group's
 *   position, preserving their relative order.
 * - A child already inside another aggregate is reparented, never nested.
 * - When only ghosts were selected there is no position to anchor to, so they
 *   join the end of the order.
 * - When the target key already owns rows, the selection joins that run's end
 *   so the aggregate stays contiguous.
 *
 * The caller owns the header (title, quantity, unit, toggle): this function
 * only decides which rows belong and where they sit. A source aggregate left
 * with no children is not removed here (there is no header list to edit); the
 * caller dispatches `aggregatePruned` after the move.
 */
export function groupIntoAggregate(
  lines: PurchaseOrderLine[],
  selectedKeys: number[],
  freshLines: PurchaseOrderLine[],
  aggregateKey: number
): PurchaseOrderLine[] {
  const selected = new Set(selectedKeys);
  const chosen: PurchaseOrderLine[][] = [];
  let earliestPosition = -1;
  for (const group of displayGroupsOf(lines)) {
    if (!group.lines.some((line) => selected.has(line.key))) continue;
    chosen.push(group.lines);
    for (const line of group.lines) {
      const position = lines.indexOf(line);
      if (position !== -1 && (earliestPosition === -1 || position < earliestPosition)) {
        earliestPosition = position;
      }
    }
  }
  if (chosen.length === 0 && freshLines.length === 0) return lines;

  const chosenKeys = new Set(chosen.flatMap((group) => group.map((line) => line.key)));
  const retained = lines.filter((line) => !chosenKeys.has(line.key));
  const movedRows = [
    ...freshLines.map((line) => ({ ...line, aggregateKey })),
    ...chosen.flatMap((group) => group.map((line) => ({ ...line, aggregateKey }))),
  ];

  let insertAt: number;
  if (retained.some((line) => aggregateKeyOf(line) === aggregateKey)) {
    // Merge into the aggregate that already owns rows: append at its end.
    let lastTarget = -1;
    retained.forEach((line, index) => {
      if (aggregateKeyOf(line) === aggregateKey) lastTarget = index;
    });
    insertAt = lastTarget + 1;
  } else if (earliestPosition >= 0) {
    // Count the retained rows that sat before the earliest selected row.
    insertAt = lines
      .slice(0, earliestPosition)
      .filter((line) => !chosenKeys.has(line.key)).length;
  } else {
    insertAt = retained.length;
  }

  const out = [...retained];
  out.splice(insertAt, 0, ...movedRows);
  return out;
}

/**
 * Clears one aggregate's membership. The rows keep their order, their prices,
 * their allocations and their display groups; only the presentation wrapper
 * goes away.
 */
export function ungroupAggregate(
  lines: PurchaseOrderLine[],
  aggregateKey: number
): PurchaseOrderLine[] {
  return lines.map((line) =>
    aggregateKeyOf(line) === aggregateKey ? { ...line, aggregateKey: null } : line
  );
}

/**
 * Removes headers left with no child rows after a removal, and clears any
 * membership that would dangle. `nextAggregateKey` is kept: keys are session
 * identity and are never reused, so a later group cannot collide with a stale
 * reference.
 */
export function pruneEmptyAggregates(
  state: PurchaseOrderEditorState
): PurchaseOrderEditorState {
  const used = new Set<number>();
  for (const line of state.lines) {
    const key = aggregateKeyOf(line);
    if (key !== null) used.add(key);
  }
  const kept = state.aggregateGroups.filter((group) => used.has(group.key));
  const keptKeys = new Set(kept.map((group) => group.key));
  const linesChanged = state.lines.some((line) => {
    const key = aggregateKeyOf(line);
    return key !== null && !keptKeys.has(key);
  });
  if (kept.length === state.aggregateGroups.length && !linesChanged) return state;
  return {
    ...state,
    aggregateGroups: kept,
    lines: linesChanged
      ? state.lines.map((line) => {
          const key = aggregateKeyOf(line);
          return key !== null && !keptKeys.has(key) ? { ...line, aggregateKey: null } : line;
        })
      : state.lines,
  };
}

/** The store's title bound for an aggregate header, in code points. */
export const MAX_AGGREGATE_TITLE_LENGTH = 500;
/** The store's unit-label bound, in code points. Label only; no conversion. */
export const MAX_AGGREGATE_UOM_LENGTH = 80;

function codePointCount(value: string): number {
  return Array.from(value).length;
}

/** The header fields and item indices a save submits (#654). */
export interface AggregatePayload {
  groups: PurchaseOrderAggregateGroupInput[];
  /** Editor group key → its 1-based position in `groups`. */
  indexByGroupKey: Map<number, number>;
  error: string | null;
}

/**
 * Builds the submitted headers and the 1-based position each line's group
 * occupies. Positions mirror the store's placement rules: a group is a
 * contiguous run of rows, a split display group never straddles two
 * aggregates, every header has at least one child, and no row names a header
 * that does not exist. The first violation is named with its group or line.
 */
export function buildAggregatePayload(
  groups: PurchaseOrderAggregateGroupState[],
  lines: PurchaseOrderLine[]
): AggregatePayload {
  const inputs: PurchaseOrderAggregateGroupInput[] = [];
  const indexByGroupKey = new Map<number, number>();

  for (let index = 0; index < groups.length; index++) {
    const position = index + 1;
    const group = groups[index];
    if (indexByGroupKey.has(group.key)) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: its editor key is duplicated.`,
      };
    }
    indexByGroupKey.set(group.key, position);
    const title = group.title.trim();
    if (title === "") {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: title is required.`,
      };
    }
    if (codePointCount(title) > MAX_AGGREGATE_TITLE_LENGTH) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: the title must be ${MAX_AGGREGATE_TITLE_LENGTH} characters or fewer.`,
      };
    }
    const quantity = parseQuantityScaled(group.orderedQuantity);
    if (quantity === null) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: display quantity must be positive with up to 4 decimal places and within the supported range.`,
      };
    }
    const uom = group.orderedUoM.trim();
    if (uom === "") {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: unit label is required.`,
      };
    }
    if (codePointCount(uom) > MAX_AGGREGATE_UOM_LENGTH) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: the unit label must be ${MAX_AGGREGATE_UOM_LENGTH} characters or fewer.`,
      };
    }
    inputs.push({
      Title: title,
      OrderedQuantityScaled: quantity,
      OrderedUoM: uom,
      ShowChildrenToCustomer: group.showChildrenToCustomer,
    });
  }

  const firstLineOf = new Map<number, number>();
  const lastLineOf = new Map<number, number>();
  const displayGroupOwner = new Map<string, number | null>();
  const displayGroupFirstLine = new Map<string, number>();
  for (let index = 0; index < lines.length; index++) {
    const lineNumber = index + 1;
    const line = lines[index];
    const key = aggregateKeyOf(line);
    const displayKey = displayGroupKeyOf(line);
    if (displayKey !== "") {
      if (displayGroupOwner.has(displayKey)) {
        const owner = displayGroupOwner.get(displayKey) ?? null;
        if (owner !== key) {
          return {
            groups: [],
            indexByGroupKey: new Map(),
            error: `Lines ${displayGroupFirstLine.get(displayKey)} and ${lineNumber}: split line "${displayKey}" must stay in one aggregate.`,
          };
        }
      } else {
        displayGroupOwner.set(displayKey, key);
        displayGroupFirstLine.set(displayKey, lineNumber);
      }
    }
    if (key === null) continue;
    const position = indexByGroupKey.get(key);
    if (position === undefined) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Line ${lineNumber}: belongs to an aggregate that no longer exists.`,
      };
    }
    const first = firstLineOf.get(key);
    if (first === undefined) {
      firstLineOf.set(key, lineNumber);
      lastLineOf.set(key, lineNumber);
      continue;
    }
    const last = lastLineOf.get(key) as number;
    if (lineNumber !== last + 1) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${position}: its lines must stay together; line ${lineNumber} does not follow line ${last}.`,
      };
    }
    lastLineOf.set(key, lineNumber);
  }
  for (let index = 0; index < groups.length; index++) {
    if (!firstLineOf.has(groups[index].key)) {
      return {
        groups: [],
        indexByGroupKey: new Map(),
        error: `Aggregate ${index + 1}: add at least one line to the aggregate.`,
      };
    }
  }
  return { groups: inputs, indexByGroupKey, error: null };
}

/**
 * The editor's full layout check before a save: the store's placement rules
 * (through `buildAggregatePayload`) plus the one rule the editor owns — every
 * child must be totallable in the PO's currency, because the header total and
 * display unit price are derived from the children and a partial sum would
 * lie. Null means the layout may be saved.
 */
export function validateAggregateLayout(
  state: PurchaseOrderEditorState,
  currency: CurrencyInfo
): string | null {
  const built = buildAggregatePayload(state.aggregateGroups, state.lines);
  if (built.error !== null) return built.error;
  for (let index = 0; index < state.aggregateGroups.length; index++) {
    const key = state.aggregateGroups[index].key;
    for (let lineIndex = 0; lineIndex < state.lines.length; lineIndex++) {
      const line = state.lines[lineIndex];
      if (aggregateKeyOf(line) !== key) continue;
      if (lineNetMinor(line, currency) === null) {
        return `Aggregate ${index + 1}: line ${lineIndex + 1} cannot be totalled; check its quantity and price.`;
      }
    }
  }
  return null;
}

