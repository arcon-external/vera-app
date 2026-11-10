// Issue #573: the bridge between the flat PO line rows and the quotation
// editor's line-item component.
//
// The PO stores one row per priced line: reconciliation, totals and the final
// gate all key off that shape. The quotation editor presents a split item as
// one parent row with sub-items beneath it. This module groups the flat rows
// for display and maps every editor edit back onto the rows that own it.
//
// A group has no parent row. Its parent is a presentation of the rows sharing
// `groupKey`; the parent description is copied onto each row so a read never
// needs a second query. A plain line is a group of one with no group key.

import { formatMoney, type CurrencyInfo } from "./format";
import type { EditorItem, EditorSubItem } from "./components/ItemEditor";
import { parseUnitPriceMinor, type PurchaseOrderLine } from "./purchaseOrder";

/** One display group: a plain line, or a split line and its sub-items. */
export interface PurchaseOrderLineGroup {
  /** The first row's key; stable while the group exists. */
  key: number;
  /** Empty for a plain line; shared by every row of a split line. */
  groupKey: string;
  /** The split line's title; empty for a plain line. */
  parentDescription: string;
  lines: PurchaseOrderLine[];
}

/**
 * The grouping key a row belongs to. Pre-migration rows have no stored key;
 * a row with a quotation sub-number is still a priced part of its item, so it
 * groups under the source item number. The migration backfills the same key,
 * which makes the fallback a compatibility path rather than a second rule.
 */
export function effectiveGroupKey(line: PurchaseOrderLine): string {
  if (line.groupKey !== "") return line.groupKey;
  if (line.quotationSubNo !== null && line.quotationItemNo !== null) {
    return `q:${line.quotationItemNo}`;
  }
  return "";
}

/** The title shown above a split line, with an honest fallback. */
export function effectiveParentDescription(group: PurchaseOrderLineGroup): string {
  const stored = group.parentDescription.trim();
  if (stored !== "") return stored;
  const first = group.lines[0];
  if (first?.quotationItemNo != null) {
    return `Quotation item ${first.quotationItemNo}`;
  }
  return "Split line";
}

/**
 * Groups rows in document order. The group's position is the position of its
 * first row, so a reorder inside a group never moves the group itself.
 */
export function groupPurchaseOrderLines(
  lines: PurchaseOrderLine[]
): PurchaseOrderLineGroup[] {
  const groups: PurchaseOrderLineGroup[] = [];
  const byKey = new Map<string, PurchaseOrderLineGroup>();
  for (const line of lines) {
    const groupKey = effectiveGroupKey(line);
    if (groupKey === "") {
      groups.push({
        key: line.key,
        groupKey: "",
        parentDescription: "",
        lines: [line],
      });
      continue;
    }
    const existing = byKey.get(groupKey);
    if (existing !== undefined) {
      existing.lines.push(line);
      continue;
    }
    const group: PurchaseOrderLineGroup = {
      key: line.key,
      groupKey,
      parentDescription: line.parentDescription,
      lines: [line],
    };
    byKey.set(groupKey, group);
    groups.push(group);
  }
  return groups;
}

/** ItemEditor holds money as a raw minor-unit string; a PO line holds the
 *  formatted display string. Convert at the boundary, never inside either. */
function rawMinor(currency: CurrencyInfo, display: string): string {
  if (display.trim() === "") return "";
  const minor = parseUnitPriceMinor(currency, display);
  return minor === null ? "" : String(minor);
}

function displayMinor(currency: CurrencyInfo, raw: string): string {
  if (raw.trim() === "") return "";
  const minor = Number(raw);
  return Number.isFinite(minor) ? formatMoney(currency, minor) : "";
}

function subItemFromLine(
  line: PurchaseOrderLine,
  currency: CurrencyInfo
): EditorSubItem {
  return {
    id: line.key,
    description: line.description,
    qty: line.quantity,
    uom: line.uom,
    unitCost: rawMinor(currency, line.unitPrice),
    marginEnabled: false,
    marginPercent: "0",
    discountEnabled: line.discountEnabled,
    discountPercent: line.discountPercent,
    bullets: line.descriptionLines,
  };
}

/**
 * The editor view model. A split group's parent row is read-only in the PO:
 * the ItemEditor renders its quantity, unit price, margin and discount as
 * em dashes and its "Sells for" as the sub-item rollup, exactly as it does for
 * a quotation split line.
 */
export function editorItemsFromGroups(
  groups: PurchaseOrderLineGroup[],
  currency: CurrencyInfo
): EditorItem[] {
  return groups.map((group) => {
    if (group.groupKey === "") {
      const line = group.lines[0];
      return {
        key: group.key,
        description: line.description,
        bullets: line.descriptionLines,
        qty: line.quantity,
        uom: line.uom,
        unitPrice: rawMinor(currency, line.unitPrice),
        marginEnabled: false,
        marginPercent: "0",
        discountEnabled: line.discountEnabled,
        discountPercent: line.discountPercent,
        subItems: [],
        showSubPrices: true,
      };
    }
    const first = group.lines[0];
    return {
      key: group.key,
      description: effectiveParentDescription(group),
      bullets: [],
      qty: "1",
      uom: first?.uom ?? "",
      unitPrice: "",
      marginEnabled: false,
      marginPercent: "0",
      discountEnabled: false,
      discountPercent: "0",
      subItems: group.lines.map((line) => subItemFromLine(line, currency)),
      showSubPrices: true,
    };
  });
}

function lineIndex(lines: PurchaseOrderLine[], key: number): number {
  return lines.findIndex((line) => line.key === key);
}

/**
 * Replaces one line's fields. The editor works in display fields; this maps
 * them onto the stored ones and ignores the fields the PO does not carry
 * (margin, sub-item price display).
 */
export function patchLine(
  lines: PurchaseOrderLine[],
  key: number,
  field: keyof EditorItem,
  value: string | string[] | boolean,
  currency: CurrencyInfo
): PurchaseOrderLine[] {
  const at = lineIndex(lines, key);
  if (at === -1) return lines;
  const line = lines[at];
  const next = { ...line };
  switch (field) {
    case "description":
      if (typeof value === "string") next.description = value;
      break;
    case "bullets":
      if (Array.isArray(value)) next.descriptionLines = value;
      break;
    case "qty":
      if (typeof value === "string") next.quantity = value;
      break;
    case "uom":
      if (typeof value === "string") next.uom = value;
      break;
    case "unitPrice":
      if (typeof value === "string") {
        next.unitPrice = displayMinor(currency, value);
      }
      break;
    case "discountEnabled":
      if (typeof value === "boolean") next.discountEnabled = value;
      break;
    case "discountPercent":
      if (typeof value === "string") next.discountPercent = value;
      break;
    default:
      return lines;
  }
  const out = [...lines];
  out[at] = next;
  return out;
}

/** Patches the title on every row of a group. */
export function patchGroupParentDescription(
  lines: PurchaseOrderLine[],
  groupKey: string,
  description: string
): PurchaseOrderLine[] {
  return lines.map((line) =>
    effectiveGroupKey(line) === groupKey
      ? { ...line, groupKey, parentDescription: description }
      : line
  );
}

/** Removes a whole display group. */
export function removeGroup(
  lines: PurchaseOrderLine[],
  groupKey: string,
  key: number
): PurchaseOrderLine[] {
  if (groupKey === "") return lines.filter((line) => line.key !== key);
  return lines.filter((line) => effectiveGroupKey(line) !== groupKey);
}

/** Patches one sub-item row. */
export function patchSubLine(
  lines: PurchaseOrderLine[],
  subId: number,
  patch: Partial<EditorSubItem>,
  currency: CurrencyInfo
): PurchaseOrderLine[] {
  const at = lineIndex(lines, subId);
  if (at === -1) return lines;
  const line = lines[at];
  const next = { ...line };
  if (patch.description !== undefined) next.description = patch.description;
  if (patch.qty !== undefined) next.quantity = patch.qty;
  if (patch.uom !== undefined) next.uom = patch.uom;
  if (patch.unitCost !== undefined) {
    next.unitPrice = displayMinor(currency, patch.unitCost);
  }
  if (patch.discountEnabled !== undefined) next.discountEnabled = patch.discountEnabled;
  if (patch.discountPercent !== undefined) next.discountPercent = patch.discountPercent;
  if (patch.bullets !== undefined) next.descriptionLines = patch.bullets;
  const out = [...lines];
  out[at] = next;
  return out;
}

/** Moves one sub-item to the adjacent position inside its group. */
export function moveSubLine(
  lines: PurchaseOrderLine[],
  groupKey: string,
  subId: number,
  direction: -1 | 1
): PurchaseOrderLine[] {
  const groupPositions: number[] = [];
  lines.forEach((line, index) => {
    if (effectiveGroupKey(line) === groupKey) groupPositions.push(index);
  });
  const position = groupPositions.findIndex((index) => lines[index].key === subId);
  if (position === -1) return lines;
  const target = position + direction;
  if (target < 0 || target >= groupPositions.length) return lines;
  const out = [...lines];
  const from = groupPositions[position];
  const to = groupPositions[target];
  [out[from], out[to]] = [out[to], out[from]];
  return out;
}

/**
 * Splits a plain line into a group. The original row becomes the first
 * sub-item, preserving its match, price and discount; one empty sub-item is
 * added beside it. The quotation discards the line's price on split, but a PO
 * line is already matched to a customer line, so discarding it would silently
 * lose a priced row.
 */
export function splitPlainLine(
  lines: PurchaseOrderLine[],
  key: number,
  groupKey: string,
  newLine: PurchaseOrderLine
): PurchaseOrderLine[] {
  const at = lineIndex(lines, key);
  if (at === -1) return lines;
  const parent = lines[at];
  const title = parent.description;
  const grouped: PurchaseOrderLine = {
    ...parent,
    groupKey,
    parentDescription: title,
  };
  const sibling: PurchaseOrderLine = {
    ...newLine,
    groupKey,
    parentDescription: title,
  };
  const out = [...lines];
  out[at] = grouped;
  out.splice(at + 1, 0, sibling);
  return out;
}

/** Adds an extra sub-item to an existing group. */
export function addSubLine(
  lines: PurchaseOrderLine[],
  groupKey: string,
  newLine: PurchaseOrderLine
): PurchaseOrderLine[] {
  const last = lines.reduce(
    (found, line, index) => (effectiveGroupKey(line) === groupKey ? index : found),
    -1
  );
  if (last === -1) return lines;
  const parentDescription = lines[last].parentDescription;
  const out = [...lines];
  out.splice(last + 1, 0, {
    ...newLine,
    groupKey,
    parentDescription,
  });
  return out;
}
