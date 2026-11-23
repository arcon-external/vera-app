// Issue #654: the customer-facing projection of a purchase order.
//
// An aggregate is presentation over the flat priced rows, so the customer view
// has to re-flatten it the way the customer should read it: one header row per
// aggregate, then — only when the aggregate's stored toggle says so — the child
// tree at its original depth. This module is pure. It imports the same money
// kernel and the same display-group rule the editor uses, so the preview, the
// print and the saved record cannot disagree about a figure.
//
// Nothing internal crosses this boundary. A `CustomerPORow` has a description,
// a quantity, a unit, a unit price and an amount: no supplier cost, no margin,
// no variance, no quoted baseline and no remaining quantity. `poCustomerView`
// is the type a customer-facing renderer may consume, and the print module
// consumes nothing else.

import type { CurrencyInfo } from "./format";
import {
  aggregateDisplayUnitPriceMinor,
  aggregateKeyOf,
  aggregateTotalMinor,
  lineNetMinor,
  parseQuantityScaled,
  parseUnitPriceMinor,
} from "./poAggregate";
import type {
  PurchaseOrderAggregateGroupState,
  PurchaseOrderLine,
} from "./purchaseOrder";
import {
  effectiveParentDescription,
  groupPurchaseOrderLines,
  type PurchaseOrderLineGroup,
} from "./purchaseOrderLines";

/** One priced child row, or one heading or bullet line of the child tree. */
export interface CustomerPOLeaf {
  kind: "line";
  /** 0 plain/package parent, 1 sub-item, 2 sub-sub bullet. */
  depth: 0 | 1 | 2;
  description: string;
  descriptionLines: string[];
  /** Formatted as entered; empty on a heading or bullet row. */
  quantity: string;
  uom: string;
  /** Null when the field is not a price in this currency or is out of range. */
  unitPriceMinor: number | null;
  /**
   * The exact net (gross minus the row discount). A heading row carries its
   * display group's sum; a depth-2 bullet is text and carries zero.
   */
  amountMinor: number;
}

/** One aggregate as the customer reads it. */
export interface CustomerPOAggregate {
  kind: "aggregate";
  title: string;
  quantity: string;
  uom: string;
  /** Derived with the editor's exact rule; null when out of range or unnamed. */
  unitPriceMinor: number | null;
  /** The exact sum of the child nets: authoritative over the unit price. */
  totalMinor: number;
  /** Null when the group is title-only: no child description may leak. */
  children: CustomerPOLeaf[] | null;
}

export type CustomerPORow = CustomerPOLeaf | CustomerPOAggregate;

/** The order facts a customer document carries, normalized across mounts. */
export interface CustomerPOHeader {
  poNumber: string;
  customerName: string;
  poDate: string | null;
  receivedDate: string;
  requestedDeliveryDate: string | null;
  quoteNumber: string;
  sourceRevision: number;
  paymentTerms: string;
  incoterms: string;
}

/** The stored record fields the header adapter reads (PO value casing). */
export interface CustomerPOHeaderSource {
  PONumber: string;
  CustomerName: string;
  QuoteNumber: string;
  SourceRevision: number;
  PODate?: string | null;
  ReceivedDate: string;
  RequestedDeliveryDate?: string | null;
  PaymentTerms: string;
  Incoterms: string;
}

/** An empty field reads as absent, never as a blank line inside a document. */
function orNull(value: string | null | undefined): string | null {
  if (value === null || value === undefined) return null;
  const trimmed = value.trim();
  return trimmed === "" ? null : trimmed;
}

/**
 * Builds the normalized header from either a saved record or the applied save
 * payload. Both mounts route through this, so the customer view and the print
 * describe the same order.
 */
export function customerPOHeaderOf(
  source: CustomerPOHeaderSource
): CustomerPOHeader {
  return {
    poNumber: orNull(source.PONumber) ?? "No PO number",
    customerName: source.CustomerName,
    poDate: orNull(source.PODate),
    receivedDate: source.ReceivedDate,
    requestedDeliveryDate: orNull(source.RequestedDeliveryDate),
    quoteNumber: source.QuoteNumber,
    sourceRevision: source.SourceRevision,
    paymentTerms: source.PaymentTerms.trim(),
    incoterms: source.Incoterms.trim(),
  };
}

/** One priced leaf row at the depth its display group gives it. */
function leafOf(
  line: PurchaseOrderLine,
  depth: 0 | 1,
  amountMinor: number,
  currency: CurrencyInfo
): CustomerPOLeaf {
  return {
    kind: "line",
    depth,
    description: line.description,
    descriptionLines: [],
    quantity: line.quantity,
    uom: line.uom,
    unitPriceMinor: parseUnitPriceMinor(currency, line.unitPrice),
    amountMinor,
  };
}

/** One sub-sub bullet: presentation text, never a priced row. */
function bulletOf(text: string): CustomerPOLeaf {
  return {
    kind: "line",
    depth: 2,
    description: text,
    descriptionLines: [],
    quantity: "",
    uom: "",
    unitPriceMinor: null,
    amountMinor: 0,
  };
}

/**
 * The rows one display group contributes: a plain line is one depth-0 row with
 * its bullets; a split line is a depth-0 heading, one priced depth-1 row per
 * sub-item and one depth-2 row per sub-sub bullet.
 */
function childRowsOfGroup(
  group: PurchaseOrderLineGroup,
  currency: CurrencyInfo
): CustomerPOLeaf[] {
  if (group.groupKey === "") {
    const line = group.lines[0];
    if (line === undefined) return [];
    const row = leafOf(line, 0, lineNetMinor(line, currency) ?? 0, currency);
    return [{ ...row, descriptionLines: [...line.descriptionLines] }];
  }
  const rows: CustomerPOLeaf[] = [
    {
      kind: "line",
      depth: 0,
      description: effectiveParentDescription(group),
      descriptionLines: [],
      quantity: "",
      uom: "",
      unitPriceMinor: null,
      amountMinor: group.lines.reduce(
        (sum, line) => sum + (lineNetMinor(line, currency) ?? 0),
        0
      ),
    },
  ];
  for (const line of group.lines) {
    rows.push(leafOf(line, 1, lineNetMinor(line, currency) ?? 0, currency));
    for (const bullet of line.descriptionLines) rows.push(bulletOf(bullet));
  }
  return rows;
}

/** One aggregate header built from every line that names it. */
function aggregateRow(
  header: PurchaseOrderAggregateGroupState,
  members: PurchaseOrderLine[],
  currency: CurrencyInfo
): CustomerPOAggregate {
  const totalMinor = aggregateTotalMinor(members, header.key, currency) ?? 0;
  const quantityScaled = parseQuantityScaled(header.orderedQuantity);
  return {
    kind: "aggregate",
    title: header.title,
    quantity: header.orderedQuantity.trim(),
    uom: header.orderedUoM,
    unitPriceMinor:
      quantityScaled === null
        ? null
        : aggregateDisplayUnitPriceMinor(totalMinor, quantityScaled),
    totalMinor,
    children: header.showChildrenToCustomer
      ? groupPurchaseOrderLines(members).flatMap((group) =>
          childRowsOfGroup(group, currency)
        )
      : null,
  };
}

/**
 * The customer's reading order for the entered rows.
 *
 * A line is walked at its `line_number` position (array order is that order in
 * this editor). An aggregate emits exactly one header row, at its first child's
 * position, and its members are read as whole display groups — the same unit
 * the editor groups by — so a split package can never be torn across two
 * headers. A membership that names no header this build knows renders as
 * ordinary lines rather than an invented, empty header, and a header with no
 * children renders nothing: a customer document never shows a fake line.
 */
export function projectCustomerRows(
  lines: PurchaseOrderLine[],
  groups: PurchaseOrderAggregateGroupState[],
  currency: CurrencyInfo
): CustomerPORow[] {
  const headerByKey = new Map<number, PurchaseOrderAggregateGroupState>();
  for (const header of groups) {
    if (!headerByKey.has(header.key)) headerByKey.set(header.key, header);
  }
  // Membership is decided per display group, the unit the editor moves. Every
  // row of a member group stays with its group, so invalid data that splits a
  // package's membership can never silently drop a row from the document.
  const displayGroups = groupPurchaseOrderLines(lines);
  const membersByKey = new Map<number, PurchaseOrderLine[]>();
  for (const group of displayGroups) {
    const key = aggregateKeyOf(group.lines[0]);
    if (key === null) continue;
    const members = membersByKey.get(key);
    if (members === undefined) membersByKey.set(key, [...group.lines]);
    else members.push(...group.lines);
  }
  const rows: CustomerPORow[] = [];
  const emitted = new Set<number>();
  for (const group of displayGroups) {
    const key = aggregateKeyOf(group.lines[0]);
    const header = key === null ? undefined : headerByKey.get(key);
    if (key === null || header === undefined) {
      rows.push(...childRowsOfGroup(group, currency));
      continue;
    }
    if (emitted.has(key)) continue;
    emitted.add(key);
    rows.push(aggregateRow(header, membersByKey.get(key) ?? group.lines, currency));
  }
  return rows;
}
