// Issue #657 phase B: the Fulfillment drill-down's honest desktop shape.
//
// The server owns the status derivation and every figure; this module is a pure
// mapping plus the small decisions the screen makes: the four status labels in
// one order, contiguous aggregate children grouped under their title, the page
// bounds, and money/quantity formatting. It has no supplier unit cost field: an
// unknown wire field is dropped rather than rendered, and the aggregate
// realized margin is the only cost-side figure the report already publishes.
//
// Every amount is integer minor units in the page's one currency. Null currency
// is a missing catalogue entry, never a reason to substitute IDR.

import type { poreport } from "@backend/models";
import type { CurrencyInfo } from "./format";
import { formatMoney } from "./format";
import { formatQuantityScaled } from "./purchaseOrder";

export const FULFILLMENT_DEFAULT_PAGE_SIZE = 25;

/** The four derived states, in the order the filter and the badge vocabulary use. */
export const FULFILLMENT_STATUS_ORDER = [
  "open",
  "expired_unclosed",
  "all_bought",
  "closed",
] as const;

export type FulfillmentStatus = (typeof FULFILLMENT_STATUS_ORDER)[number];

export interface FulfillmentStatusOption {
  value: string;
  label: string;
}

/**
 * The filter's options. "all" is not a quotation state and never appears on a
 * row; it is the query's every-status value.
 */
export const FULFILLMENT_STATUS_OPTIONS: readonly FulfillmentStatusOption[] = [
  { value: "all", label: "All statuses" },
  { value: "open", label: "Open" },
  { value: "expired_unclosed", label: "Expired (unclosed)" },
  { value: "all_bought", label: "All bought" },
  { value: "closed", label: "Closed" },
];

const STATUS_LABELS = new Map(
  FULFILLMENT_STATUS_OPTIONS.map((option) => [option.value, option.label])
);

/** A known status by name; an unknown one is echoed, never renamed. */
export function fulfillmentStatusLabel(status: string): string {
  return STATUS_LABELS.get(status) ?? status;
}

export type FulfillmentStatusTone =
  | "neutral"
  | "success"
  | "accent"
  | "danger";

/**
 * Closed is a loss state; expired is attention, not a loss; open is healthy;
 * all-bought is a settled, neutral row.
 */
export function fulfillmentStatusTone(status: string): FulfillmentStatusTone {
  switch (status) {
    case "open":
      return "success";
    case "expired_unclosed":
      return "accent";
    case "closed":
      return "danger";
    default:
      return "neutral";
  }
}

export interface PurchaseOrderFulfillmentQuery {
  currency: string;
  status: string;
  search: string;
  page: number;
  pageSize: number;
}

export interface PurchaseOrderFulfillmentContribution {
  id: number;
  poNumber: string;
  state: string;
  receivedDate: string;
  allocatedQuantityScaled: number;
}

export interface PurchaseOrderFulfillmentItem {
  quotationItemNo: number;
  quotationSubNo: number | null;
  label: string;
  description: string;
  /** The aggregate title above this child; "" when ungrouped. */
  groupTitle: string;
  /** Bought on an earlier revision and absent from the latest one. */
  removed: boolean;

  quotedQuantityScaled: number;
  boughtQuantityScaled: number;
  unsoldQuantityScaled: number;
  overBoughtQuantityScaled: number;

  realizedAmountMinor: number;
  realizedMarginMinor: number;
  unsoldAmountMinor: number;
  unsoldMarginMinor: number;

  quantityExtraIncomeAmountMinor: number;
  quantityExtraIncomeMarginMinor: number;
  priceExtraIncomeMinor: number;
  priceLossMinor: number;
  closedLossAmountMinor: number;
  closedLossMarginMinor: number;

  purchaseOrders: PurchaseOrderFulfillmentContribution[];
}

export interface PurchaseOrderFulfillmentExtraExpense {
  purchaseOrderId: number;
  poNumber: string;
  lineNumber: number;
  description: string;
  quantityScaled: number;
  uom: string;
  amountMinor: number;
  groupTitle: string;
}

export interface PurchaseOrderFulfillmentQuotation {
  quotationId: number;
  quoteNumber: string;
  customerName: string;
  currency: string;
  latestRevision: number;
  issuedAt: string;
  validUntil: string;
  status: string;
  items: PurchaseOrderFulfillmentItem[];
  extraExpenses: PurchaseOrderFulfillmentExtraExpense[];
}

export interface PurchaseOrderFulfillmentPage {
  asOf: string;
  filter: PurchaseOrderFulfillmentQuery;
  quotations: PurchaseOrderFulfillmentQuotation[];
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
}

function normalizeContribution(
  contribution: poreport.FulfillmentContribution
): PurchaseOrderFulfillmentContribution {
  return {
    id: contribution.id,
    poNumber: contribution.poNumber,
    state: contribution.state,
    receivedDate: contribution.receivedDate,
    allocatedQuantityScaled: contribution.allocatedQuantityScaled,
  };
}

function normalizeItem(item: poreport.FulfillmentItem): PurchaseOrderFulfillmentItem {
  return {
    quotationItemNo: item.quotationItemNo,
    quotationSubNo:
      typeof item.quotationSubNo === "number" ? item.quotationSubNo : null,
    label: item.label,
    description: item.description,
    groupTitle: item.groupTitle ?? "",
    removed: item.removed === true,
    quotedQuantityScaled: item.quotedQuantityScaled,
    boughtQuantityScaled: item.boughtQuantityScaled,
    unsoldQuantityScaled: item.unsoldQuantityScaled,
    overBoughtQuantityScaled: item.overBoughtQuantityScaled,
    realizedAmountMinor: item.realizedAmountMinor,
    realizedMarginMinor: item.realizedMarginMinor,
    unsoldAmountMinor: item.unsoldAmountMinor,
    unsoldMarginMinor: item.unsoldMarginMinor,
    quantityExtraIncomeAmountMinor: item.quantityExtraIncomeAmountMinor,
    quantityExtraIncomeMarginMinor: item.quantityExtraIncomeMarginMinor,
    priceExtraIncomeMinor: item.priceExtraIncomeMinor,
    priceLossMinor: item.priceLossMinor,
    closedLossAmountMinor: item.closedLossAmountMinor,
    closedLossMarginMinor: item.closedLossMarginMinor,
    purchaseOrders: Array.isArray(item.purchaseOrders)
      ? item.purchaseOrders.map(normalizeContribution)
      : [],
  };
}

function normalizeExtra(
  extra: poreport.FulfillmentExtraExpense
): PurchaseOrderFulfillmentExtraExpense {
  return {
    purchaseOrderId: extra.purchaseOrderId,
    poNumber: extra.poNumber,
    lineNumber: extra.lineNumber,
    description: extra.description,
    quantityScaled: extra.quantityScaled,
    uom: extra.uom,
    amountMinor: extra.amountMinor,
    groupTitle: extra.groupTitle ?? "",
  };
}

function normalizeQuotation(
  quotation: poreport.FulfillmentQuotation
): PurchaseOrderFulfillmentQuotation {
  return {
    quotationId: quotation.quotationId,
    quoteNumber: quotation.quoteNumber,
    customerName: quotation.customerName,
    currency: quotation.currency,
    latestRevision: quotation.latestRevision,
    issuedAt: quotation.issuedAt,
    validUntil: quotation.validUntil,
    status: quotation.status,
    items: Array.isArray(quotation.items)
      ? quotation.items.map(normalizeItem)
      : [],
    extraExpenses: Array.isArray(quotation.extraExpenses)
      ? quotation.extraExpenses.map(normalizeExtra)
      : [],
  };
}

/**
 * Maps one server page to the screen's shape. The mapping is a whitelist: a
 * field the server must not send (a supplier unit cost, say) cannot reach the
 * component because only the fields named here are copied.
 */
export function normalizePurchaseOrderFulfillmentPage(
  page: poreport.FulfillmentPage
): PurchaseOrderFulfillmentPage {
  return {
    asOf: page.asOf,
    filter: {
      currency: page.filter.currency,
      status: page.filter.status,
      search: page.filter.search,
      page: page.filter.page,
      pageSize: page.filter.pageSize,
    },
    quotations: Array.isArray(page.quotations)
      ? page.quotations.map(normalizeQuotation)
      : [],
    page: page.page,
    pageSize: page.pageSize,
    total: page.total,
    totalPages: page.totalPages,
  };
}

/** One contiguous run of items under the same aggregate title. */
export interface FulfillmentItemGroup {
  /** "" for an ungrouped run. */
  title: string;
  items: PurchaseOrderFulfillmentItem[];
}

/**
 * Splits items into contiguous title runs. A group header is presentation only:
 * two children that share a title are two item rows under one heading, and a
 * title that reappears after another run gets its own heading rather than
 * merging two separated sets of lines.
 */
export function groupFulfillmentItems(
  items: readonly PurchaseOrderFulfillmentItem[]
): FulfillmentItemGroup[] {
  const groups: FulfillmentItemGroup[] = [];
  for (const item of items) {
    const title = item.groupTitle.trim();
    const current = groups[groups.length - 1];
    if (current !== undefined && current.title === title) {
      current.items.push(item);
    } else {
      groups.push({ title, items: [item] });
    }
  }
  return groups;
}

export interface FulfillmentPageBounds {
  /** The page to display: clamped to the real range. */
  page: number;
  totalPages: number;
  hasPrevious: boolean;
  hasNext: boolean;
}

/** One empty result is one empty page, never "page 1 of 0". */
export function fulfillmentPageBounds(
  total: number,
  pageSize: number,
  page: number
): FulfillmentPageBounds {
  const safeSize =
    Number.isFinite(pageSize) && pageSize > 0
      ? Math.trunc(pageSize)
      : FULFILLMENT_DEFAULT_PAGE_SIZE;
  const safeTotal =
    Number.isFinite(total) && total > 0 ? Math.trunc(total) : 0;
  const totalPages = Math.max(1, Math.ceil(safeTotal / safeSize));
  const requested = Number.isFinite(page) ? Math.trunc(page) : 1;
  const current = Math.min(Math.max(1, requested), totalPages);
  return {
    page: current,
    totalPages,
    hasPrevious: current > 1,
    hasNext: current < totalPages,
  };
}

/** A stable key for one server read. Every field that changes it is included. */
export function fulfillmentQueryKey(
  query: PurchaseOrderFulfillmentQuery
): string {
  return JSON.stringify([
    query.currency,
    query.status,
    query.search,
    query.page,
    query.pageSize,
  ]);
}

/** Money in the page's currency; a missing catalogue entry is "-", never IDR. */
export function formatFulfillmentMoney(
  entry: CurrencyInfo | null,
  minor: number
): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

/** A loss magnitude with a true minus sign; zero has none. */
export function formatFulfillmentLoss(
  entry: CurrencyInfo | null,
  minor: number
): string {
  if (entry === null) return "-";
  if (minor === 0 || !Number.isFinite(minor)) {
    return formatFulfillmentMoney(entry, 0);
  }
  return `−${formatFulfillmentMoney(entry, Math.abs(minor))}`;
}

/** The store's 1/10000 scale as the shortest decimal. */
export function formatFulfillmentQuantity(scaled: number): string {
  return formatQuantityScaled(scaled);
}

const FULFILLMENT_MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
] as const;

/**
 * A WIB calendar date as the compact readable form a dense cell can hold:
 * `2026-10-05` becomes `05 Oct 2026`. Anything that is not a plain date is
 * echoed unchanged rather than guessed at; the server's own date is the truth.
 */
export function formatFulfillmentDate(value: string): string {
  const trimmed = value.trim();
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(trimmed);
  if (match === null) return value;
  const month = Number(match[2]);
  if (!Number.isInteger(month) || month < 1 || month > 12) return value;
  return `${match[3]} ${FULFILLMENT_MONTHS[month - 1]} ${match[1]}`;
}
