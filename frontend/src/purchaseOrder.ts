// Issue #468: the PO draft editor's decisions, separated from its markup.
//
// The editor enters a customer order against one issued Quotation revision.
// Everything here is pure: parsing the exact quantity and money strings the
// fields hold, tracking whether the draft is dirty, and building the replace-
// lines payload. Exact totals and discounts are #470's slice, so none of this
// recalculates a price.

import {
  formatMoney,
  tryParseMoneyInput,
  type CurrencyInfo,
} from "./format";
import { legacyServiceCategory, legacyTaxType } from "./tax";

/** The fixed four-decimal quantity scale shared with the Go store. */
export const QUANTITY_SCALE = 10000;

/** One line as the desktop binding returns it (PascalCase = Wails models). */
export interface PurchaseOrderDTOItem {
  QuotationItemNo?: number | null;
  QuotationSubNo?: number | null;
  LineNumber: number;
  Description: string;
  QuantityScaled: number;
  UoM: string;
  UnitPriceMinor: number;
  DiscountEnabled: boolean;
  DiscountPercent: number;
  GrossMinor: number;
  DiscountMinor: number;
  NetMinor: number;
}

/** The printed source figures the quotation revision carried (issue #470). */
export interface PurchaseOrderPrintedTotals {
  subtotalMinor: number;
  discountMinor: number;
  ppnMinor: number;
  grandTotalMinor: number;
}

/**
 * The three live source states (#496). Machine logic keys off this, never off
 * a zero id: a linked revision 00 is a real link with SourceState "live".
 */
export type PurchaseOrderSourceState = "live" | "never" | "former";

/** One PO draft as the desktop binding returns it. */
export interface PurchaseOrderDTO {
  PODate?: string | null;
  RequestedDeliveryDate?: string | null;
  UUID: string;
  State: string;
  QuoteNumber: string;
  CustomerName: string;
  BillingAddress: string;
  DeliveryAddress: string;
  ShipToCode: string;
  ContactEmail: string;
  ContactPhone: string;
  PONumber: string;
  ReceivedDate: string;
  Incoterms: string;
  Currency: string;
  TaxZone: string;
  TaxType?: string;
  ServiceCategory?: string;
  TaxCategory: string;
  AdjustmentReason: string;
  CreatedAt: string;
  UpdatedAt: string;
  Items: PurchaseOrderDTOItem[];
  ID: number;
  QuotationID: number;
  SourceRevision: number;
  CustomerID: number;
  PaymentTerms: string;
  DiscountPercent: number;
  PpnBps: number;
  SubtotalMinor: number;
  DiscountMinor: number;
  PpnMinor: number;
  AdjustmentMinor: number;
  GrandTotalMinor: number;
  PrintedSubtotalMinor: number;
  PrintedDiscountMinor: number;
  PrintedPpnMinor: number;
  PrintedGrandTotalMinor: number;
  /** Set exactly once by the draft -> saved transition (#471). */
  SavedAt?: string | null;
  SavedBy?: number | null;
  /** The deliberate no-PDF confirmation recorded by the final save (#471). */
  PdfAbsentConfirmed?: boolean;
  /**
   * The #480 state: the Quotation this record followed was permanently
   * deleted. It is independent of Draft/Saved, and the record keeps its own
   * values plus a former-source reference until an admin repairs the link
   * (#481).
   */
  SourceMissing?: boolean;
  /**
   * The three live source states (#496). Older payloads omit it; derive from
   * SourceMissing/QuotationID with `sourceStateOf`.
   */
  SourceState?: string;
  /**
   * False exactly for a never-linked draft, whose printed source totals do not
   * exist yet. Older payloads omit it; `printedTotalsKnownOf` treats every
   * older shape as known because an old server could only send live/former.
   */
  PrintedTotalsKnown?: boolean;
  /**
   * The optional customer-system confirmation reference (#474). Null until
   * staff assign it; after that it is immutable. ConfirmationWarn follows the
   * stored customer-year sequence.
   */
  ConfirmationNumber?: string | null;
  ConfirmationAssignedAt?: string | null;
  ConfirmationWarn?: boolean;
  /**
   * The optimistic-concurrency revision (#473). It starts at 1 and moves
   * forward on every write; the editor submits the version it loaded.
   */
  Version: number;
}

/** One editable line. Quantity and money stay strings until they are parsed. */
export interface PurchaseOrderLine {
  key: number;
  description: string;
  quantity: string;
  uom: string;
  unitPrice: string;
  discountEnabled: boolean;
  discountPercent: string;
  quotationItemNo: number | null;
  quotationSubNo: number | null;
}

/** Editable header fields. The customer snapshot and source are read-only. */
export interface PurchaseOrderHeader {
  billingAddress: string;
  deliveryAddress: string;
  shipToCode: string;
  contactEmail: string;
  contactPhone: string;
  poNumber: string;
  poDate: string;
  receivedDate: string;
  requestedDeliveryDate: string;
  paymentTerms: string;
  incoterms: string;
  /** Whole-PO discount, in percent. The row discounts live on the lines. */
  discountPercent: string;
  /** Signed adjustment in the PO currency, as typed. Empty means none. */
  adjustment: string;
  adjustmentReason: string;
}

export interface PurchaseOrderEditorState {
  orderId: number;
  quoteNumber: string;
  customerName: string;
  sourceRevision: number;
  sourceState: PurchaseOrderSourceState;
  /** False for a never-linked draft: there are no printed source figures. */
  printedTotalsKnown: boolean;
  currency: string;
  taxZone: string;
  taxType: string;
  serviceCategory: string;
  taxCategory: string;
  header: PurchaseOrderHeader;
  lines: PurchaseOrderLine[];
  /** The customer-facing PPN rate resolved from the frozen tax basis. */
  ppnBps: number;
  /** What the source revision printed, kept separate from the calculation. */
  printedTotals: PurchaseOrderPrintedTotals;
  /** The version this editor loaded (#473); every save states it. */
  version: number;
  /** Canonical snapshot of the last loaded/saved values, for dirtiness. */
  baseline: string;
  nextKey: number;
  saving: boolean;
  error: string | null;
}

export type PurchaseOrderEditorAction =
  | { type: "headerChanged"; patch: Partial<PurchaseOrderHeader> }
  | { type: "lineChanged"; key: number; patch: Partial<PurchaseOrderLine> }
  | { type: "lineAdded"; uom: string }
  | { type: "lineRemoved"; key: number }
  | {
      type: "sourceBasisChanged";
      patch: Partial<
        Pick<
          PurchaseOrderEditorState,
          "taxZone" | "taxType" | "serviceCategory" | "taxCategory"
        >
      >;
    }
  | { type: "currencyChanged"; code: string; from: CurrencyInfo; to: CurrencyInfo }
  | { type: "created"; orderId: number; version: number }
  | { type: "saveStarted" }
  | { type: "saveConflict" }
  | { type: "versionAdopted"; version: number }
  | { type: "valuesReplaced"; header: PurchaseOrderHeader; lines: PurchaseOrderLine[] }
  | {
      type: "extractionApplied";
      header: Partial<PurchaseOrderHeader>;
      lines?: PurchaseOrderLine[];
      nextKey: number;
    }
  | { type: "saveFailed"; error: string }
  | { type: "saveSucceeded"; order: PurchaseOrderDTO; currency: CurrencyInfo };

/** One replace-lines payload entry (PascalCase = Wails binding). */
export interface PurchaseOrderItemInput {
  QuotationItemNo?: number | null;
  QuotationSubNo?: number | null;
  Description: string;
  QuantityScaled: number;
  UoM: string;
  UnitPriceMinor: number;
  DiscountEnabled: boolean;
  DiscountPercent: number;
}

/** The update payload (PascalCase = Wails binding). */
export interface PurchaseOrderUpdateInput {
  ID: number;
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
  Items: PurchaseOrderItemInput[];
  DiscountPercent: number;
  AdjustmentMinor: number;
  /** The currency and tax triple, editable only before a source is linked. */
  Currency: string;
  TaxZone: string;
  TaxType: string;
  ServiceCategory: string;
  /** The version the editor loaded (#473). */
  ExpectedVersion: number;
  /** True only after the Overwrite choice; keeps the replaced draft. */
  Overwrite: boolean;
}

/**
 * Parses a quantity typed as a decimal string into the exact 10^4-scaled
 * integer. Up to four decimal places; a comma is accepted as the decimal
 * separator because the office types both. Positive only: a zero-quantity
 * line is not an order line, and the database refuses it too. Null means the
 * field cannot be saved.
 */
export function parseQuantityScaled(raw: string): number | null {
  const text = raw.trim();
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

/** Renders a scaled quantity as the shortest decimal with at most 4 places. */
export function formatQuantityScaled(scaled: number): string {
  if (!Number.isFinite(scaled) || scaled <= 0) return "0";
  const whole = Math.trunc(scaled / QUANTITY_SCALE);
  const fraction = String(scaled % QUANTITY_SCALE)
    .padStart(4, "0")
    .replace(/0+$/, "");
  return fraction === "" ? String(whole) : `${whole}.${fraction}`;
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

/** Formats a stored minor amount for its money input. */
export function formatUnitPriceMinor(
  currency: CurrencyInfo,
  minor: number
): string {
  return formatMoney(currency, minor);
}

/** The ceiling on a row or whole-PO discount; mirrors MaxDiscountPercent in Go. */
export const MAX_DISCOUNT_PERCENT = 100;

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

/**
 * The customer-facing PPN rate for a persisted tax basis, mirroring
 * PurchaseOrderPPNBps in backend/store/po_calc.go. The server is authoritative;
 * this is the editor's live display and the same rule the save payload relies
 * on. Null means the triple is not a supported combination.
 */
export function purchaseOrderPpnBps(
  zone: string,
  taxType: string,
  serviceCategory: string,
  legacyCategory: string
): number | null {
  let type = taxType;
  let service = serviceCategory;
  if (type === "") {
    if (zone === "FTZ") {
      if (legacyCategory !== "") return null;
      type = "Goods";
    } else {
      const mapped = legacyPOToTaxType(legacyCategory);
      if (mapped === null) return null;
      type = mapped.taxType;
      service = mapped.serviceCategory;
    }
  }
  if (zone !== "FTZ" && zone !== "NonFTZ") return null;
  if (type === "Goods") {
    if (service !== "") return null;
    return zone === "NonFTZ" ? 1100 : 0;
  }
  if (type === "Services") {
    if (service !== "NonConstruction" && service !== "Construction") return null;
    return 0;
  }
  return null;
}

function legacyPOToTaxType(
  category: string
): { taxType: string; serviceCategory: string } | null {
  switch (category) {
    case "SupplyOnly":
      return { taxType: "Goods", serviceCategory: "" };
    case "SupplyInstall":
      return { taxType: "Services", serviceCategory: "NonConstruction" };
    case "SupplyInstallConstruction":
      return { taxType: "Services", serviceCategory: "Construction" };
    default:
      return null;
  }
}

/**
 * Reads the source state from a payload. An older server sent only a live or a
 * former link, so an absent state is derived from the fields it did send. A
 * linked revision 00 keeps SourceState "live": zero is not absence.
 */
export function sourceStateOf(
  order: Pick<PurchaseOrderDTO, "SourceState" | "SourceMissing" | "QuotationID">
): PurchaseOrderSourceState {
  if (
    order.SourceState === "live" ||
    order.SourceState === "never" ||
    order.SourceState === "former"
  ) {
    return order.SourceState;
  }
  if (order.SourceMissing === true) return "former";
  if ((order.QuotationID ?? 0) > 0) return "live";
  return "never";
}

/**
 * Whether the printed source totals are real evidence. Only a never-linked
 * draft is unknown; an older payload could only be live or former, both of
 * which copied the printed figures when they were created.
 */
export function printedTotalsKnownOf(
  order: Pick<PurchaseOrderDTO, "PrintedTotalsKnown" | "SourceState" | "SourceMissing" | "QuotationID">
): boolean {
  if (order.PrintedTotalsKnown !== undefined) return order.PrintedTotalsKnown;
  return sourceStateOf(order) !== "never";
}

/** The inputs a transient PO-first draft starts from. */
export interface BlankPurchaseOrderOptions {
  currency: string;
  taxZone: string;
  taxType: string;
  serviceCategory: string;
  taxCategory?: string;
  receivedDate: string;
  paymentTerms?: string;
  incoterms?: string;
}

/**
 * Builds the transient editor DTO for a source-less draft. It is never stored
 * until Save draft, a PDF upload or an extraction asks for a row; opening the
 * editor creates nothing. Every identity field is empty, and SourceState
 * "never" tells the UI not to show a Customer, a quotation revision or printed
 * totals.
 */
export function blankPurchaseOrderDraft(
  options: BlankPurchaseOrderOptions
): PurchaseOrderDTO {
  const taxCategory = options.taxCategory ?? "";
  return {
    PODate: null,
    RequestedDeliveryDate: null,
    UUID: "",
    State: "draft",
    QuoteNumber: "",
    CustomerName: "",
    BillingAddress: "",
    DeliveryAddress: "",
    ShipToCode: "",
    ContactEmail: "",
    ContactPhone: "",
    PONumber: "",
    ReceivedDate: options.receivedDate,
    Incoterms: options.incoterms ?? "",
    Currency: options.currency,
    TaxZone: options.taxZone,
    TaxType: options.taxType,
    ServiceCategory: options.serviceCategory,
    TaxCategory: taxCategory,
    AdjustmentReason: "",
    CreatedAt: "",
    UpdatedAt: "",
    Items: [],
    ID: 0,
    QuotationID: 0,
    SourceRevision: 0,
    CustomerID: 0,
    PaymentTerms: options.paymentTerms ?? "",
    DiscountPercent: 0,
    PpnBps:
      purchaseOrderPpnBps(
        options.taxZone,
        options.taxType,
        options.serviceCategory,
        taxCategory
      ) ?? 0,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
    SavedAt: null,
    SavedBy: null,
    PdfAbsentConfirmed: false,
    SourceMissing: true,
    SourceState: "never",
    PrintedTotalsKnown: false,
    ConfirmationNumber: null,
    ConfirmationAssignedAt: null,
    ConfirmationWarn: false,
    Version: 0,
  };
}

/** The Go store's bound on an adjustment reason, in Unicode code points. */
export const MAX_ADJUSTMENT_REASON_LENGTH = 200;

/**
 * The code points Go's `strings.TrimSpace` removes, mirrored exactly.
 *
 * The editor, the store and the schema's CHECK must agree on what "blank"
 * means, or an invisible reason passes one layer and fails another. This is
 * deliberately not `String.prototype.trim()`: JavaScript also strips U+FEFF
 * (which Go and Postgres keep as content) and does not strip U+0085 (which both
 * remove). The SQL backstop lists the same set.
 */
function isAdjustmentReasonWhitespace(codePoint: number): boolean {
  return (
    (codePoint >= 0x0009 && codePoint <= 0x000d) ||
    codePoint === 0x0020 ||
    codePoint === 0x0085 ||
    codePoint === 0x00a0 ||
    codePoint === 0x1680 ||
    (codePoint >= 0x2000 && codePoint <= 0x200a) ||
    codePoint === 0x2028 ||
    codePoint === 0x2029 ||
    codePoint === 0x202f ||
    codePoint === 0x205f ||
    codePoint === 0x3000
  );
}

/**
 * Trims exactly what `strings.TrimSpace` trims, so a reason is stored and
 * compared on the same characters in every layer. Works in code points, so an
 * astral character is never split.
 */
export function trimAdjustmentReason(value: string): string {
  const characters = Array.from(value);
  let start = 0;
  let end = characters.length;
  while (
    start < end &&
    isAdjustmentReasonWhitespace(characters[start].codePointAt(0)!)
  ) {
    start++;
  }
  while (
    end > start &&
    isAdjustmentReasonWhitespace(characters[end - 1].codePointAt(0)!)
  ) {
    end--;
  }
  return characters.slice(start, end).join("");
}

const MAX_EXACT_MINOR_BIGINT = BigInt(MAX_EXACT_MINOR);

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

/**
 * Parses the signed adjustment in the PO's own currency. Empty means none;
 * a leading minus is a real, signed reduction. Null means the text cannot be
 * money, so the editor can name the field instead of storing zero.
 */
export function parseAdjustmentMinor(
  currency: CurrencyInfo,
  raw: string
): number | null {
  const text = raw.trim();
  if (text === "") return 0;
  const parsed = tryParseMoneyInput(currency, text);
  if (
    parsed === null ||
    !Number.isSafeInteger(parsed) ||
    Math.abs(parsed) > MAX_EXACT_MINOR
  ) {
    return null;
  }
  return parsed;
}

/** One row's exact figures: what it costs before and after its own discount. */
export interface PurchaseOrderLineCalculation {
  grossMinor: number;
  discountMinor: number;
  netMinor: number;
}

/** Every stage of the calculation, so the editor can show where a total came from. */
export interface PurchaseOrderTotals {
  /**
   * Sum of row gross figures, before any row discount.
   *
   * Display-only: Go neither bounds nor stores this aggregate, because the
   * order is already fully described by the per-line figures and their net
   * subtotal. When the sum exceeds 2^53-1 it cannot be named exactly as a
   * JavaScript number, so it is `null` and the panel says so. Refusing the
   * order here would reject an order the API accepts and can reopen.
   */
  grossMinor: number | null;
  /** Sum of row discounts. Display-only for the same reason as `grossMinor`. */
  rowDiscountMinor: number | null;
  /** Sum of row nets: the base of the whole-PO discount, bounded by Go too. */
  subtotalMinor: number;
  /** Whole-PO discount amount, truncated to a whole minor unit. */
  discountMinor: number;
  afterDiscountMinor: number;
  ppnMinor: number;
  adjustmentMinor: number;
  grandTotalMinor: number;
}

export interface PurchaseOrderCalculation {
  /** One entry per line; null when that line cannot be parsed yet. */
  lines: Array<PurchaseOrderLineCalculation | null>;
  /**
   * The full calculation, or null while any input cannot be parsed. A partial
   * sum would understate the order, and zero must never stand in for a
   * calculation that did not happen; the panel renders an explicit pending
   * state instead.
   */
  totals: PurchaseOrderTotals | null;
  /** The first problem, named for the field or line it belongs to. */
  error: string | null;
}

function toMoney(value: bigint): number | null {
  if (value < 0n || value > MAX_EXACT_MINOR_BIGINT) return null;
  return Number(value);
}

/**
 * Exact `quantity × price ÷ 10^4` with the half-away-from-zero rounding the Go
 * calculator uses. A big-int intermediate is what makes an overflowing line
 * detectable instead of silently wrapped.
 */
function lineGrossMinor(
  quantityScaled: number,
  unitPriceMinor: number
): number | null {
  const product = BigInt(quantityScaled) * BigInt(unitPriceMinor);
  const divisor = BigInt(QUANTITY_SCALE);
  const quotient = product / divisor;
  const remainder = product % divisor;
  const rounded = remainder * 2n >= divisor ? quotient + 1n : quotient;
  return toMoney(rounded);
}

/** `amount × percent ÷ 100`, truncated, exactly as the quotation does it. */
function percentageMinor(amount: number, percent: number): number | null {
  return toMoney((BigInt(amount) * BigInt(percent)) / 100n);
}

/** `amount × basis points ÷ 10000`, truncated, exactly as calcTax does it. */
function basisPointsMinor(amount: number, bps: number): number | null {
  return toMoney((BigInt(amount) * BigInt(bps)) / 10000n);
}

/**
 * The explained customer-facing calculation, mirroring CalculatePurchaseOrder
 * in backend/store/po_calc.go stage for stage:
 *
 *   1. line gross    half away from zero(quantity × unit price ÷ 10^4)
 *   2. row discount  truncate(gross × percent ÷ 100); net = gross − discount
 *   3. subtotal      sum of row nets
 *   4. PO discount   truncate(subtotal × percent ÷ 100)
 *   5. PPN           truncate(after × bps ÷ 10000), added on top
 *   6. adjustment    signed, added last, reason required
 *   7. grand total   after + PPN + adjustment, never negative
 *
 * Internal cost, margin, PPH22 and profit have no place in this module: a PO is
 * compared against customer-facing figures only.
 */
export function calculatePurchaseOrder(
  state: PurchaseOrderEditorState,
  currency: CurrencyInfo
): PurchaseOrderCalculation {
  const lineResults: Array<PurchaseOrderLineCalculation | null> = [];
  // BigInt accumulators keep the sum exact even when two individually valid
  // lines add up past the safe-number bound; a Number sum would lose a unit
  // before the range check could see it.
  let grossTotal = 0n;
  let rowDiscountTotal = 0n;
  let subtotal = 0n;
  let firstError: string | null = null;

  for (let index = 0; index < state.lines.length; index++) {
    const line = state.lines[index];
    const quantity = parseQuantityScaled(line.quantity);
    if (quantity === null) {
      lineResults.push(null);
      firstError ??= `Line ${index + 1}: quantity must be positive with up to 4 decimal places and within the supported range.`;
      continue;
    }
    const price = parseUnitPriceMinor(currency, line.unitPrice);
    if (price === null) {
      lineResults.push(null);
      firstError ??= `Line ${index + 1}: unit price must be a nonnegative ${currency.code} amount within the supported range.`;
      continue;
    }
    const percent = parseDiscountPercent(line.discountPercent);
    if (percent === null) {
      lineResults.push(null);
      firstError ??= `Line ${index + 1}: discount must be between 0 and ${MAX_DISCOUNT_PERCENT}.`;
      continue;
    }
    const gross = lineGrossMinor(quantity, price);
    if (gross === null) {
      lineResults.push(null);
      firstError ??= `Line ${index + 1}: the line total exceeds the supported amount range.`;
      continue;
    }
    const discount = line.discountEnabled ? percentageMinor(gross, percent) : 0;
    if (discount === null) {
      lineResults.push(null);
      firstError ??= `Line ${index + 1}: the line discount exceeds the supported amount range.`;
      continue;
    }
    const net = gross - discount;
    lineResults.push({ grossMinor: gross, discountMinor: discount, netMinor: net });
    grossTotal += BigInt(gross);
    rowDiscountTotal += BigInt(discount);
    subtotal += BigInt(net);
  }

  const wholePercent = parseDiscountPercent(state.header.discountPercent);
  if (wholePercent === null) {
    return {
      lines: lineResults,
      totals: null,
      error:
        firstError ??
        `Whole-PO discount must be between 0 and ${MAX_DISCOUNT_PERCENT}.`,
    };
  }
  const adjustment = parseAdjustmentMinor(currency, state.header.adjustment);
  if (adjustment === null) {
    return {
      lines: lineResults,
      totals: null,
      error:
        firstError ??
        `Adjustment must be a signed ${currency.code} amount.`,
    };
  }
  const adjustmentReason = trimAdjustmentReason(state.header.adjustmentReason);

  // The reason rule lives here so the panel and the payload cannot disagree:
  // Unicode code points, and a nonzero adjustment needs more than whitespace.
  // The length is checked even with no amount, exactly as the Go store does.
  let error = firstError;
  if (error === null && characterCount(adjustmentReason) > MAX_ADJUSTMENT_REASON_LENGTH) {
    error = `Adjustment reason must be ${MAX_ADJUSTMENT_REASON_LENGTH} characters or fewer.`;
  }
  if (error === null && adjustment !== 0 && adjustmentReason === "") {
    error = "An adjustment requires a reason.";
  }
  // No totals while an input is unparseable: a partial sum would understate the
  // order, and zero is not a calculation.
  if (error !== null) {
    return { lines: lineResults, totals: null, error };
  }

  // Only the subtotal is checked here: it is the value Go stores and the base
  // of the whole-PO discount. The aggregate gross and row-discount sums are
  // display-only and Go does not bound them; refusing them here would reject an
  // order the API accepts. They are converted below, and `null` means "too
  // large to name exactly", never a refusal.
  const subtotalMinor = toMoney(subtotal);
  if (subtotalMinor === null) {
    return {
      lines: lineResults,
      totals: null,
      error: "The subtotal exceeds the supported amount range.",
    };
  }

  const discount = percentageMinor(subtotalMinor, wholePercent);
  if (discount === null) {
    return {
      lines: lineResults,
      totals: null,
      error: "The whole-PO discount exceeds the supported amount range.",
    };
  }
  const afterDiscount = subtotalMinor - discount;
  const ppn = basisPointsMinor(afterDiscount, state.ppnBps);
  if (ppn === null) {
    return {
      lines: lineResults,
      totals: null,
      error: "PPN exceeds the supported amount range.",
    };
  }
  const grandBig = BigInt(afterDiscount) + BigInt(ppn) + BigInt(adjustment);
  if (grandBig < -MAX_EXACT_MINOR_BIGINT || grandBig > MAX_EXACT_MINOR_BIGINT) {
    return {
      lines: lineResults,
      totals: null,
      error: "The purchase order total exceeds the supported amount range.",
    };
  }
  const grand = Number(grandBig);
  // The arithmetic is still shown while a rule blocks saving: a negative total
  // is an error beside the figures, not a reason to blank them out.
  if (grand < 0) {
    error = "The adjustment would make the purchase order total negative.";
  }

  return {
    lines: lineResults,
    totals: {
      grossMinor: toMoney(grossTotal),
      rowDiscountMinor: toMoney(rowDiscountTotal),
      subtotalMinor,
      discountMinor: discount,
      afterDiscountMinor: afterDiscount,
      ppnMinor: ppn,
      adjustmentMinor: adjustment,
      grandTotalMinor: grand,
    },
    error,
  };
}

/** The server's maximum free-text length for payment terms. */
export const MAX_PAYMENT_TERMS_LENGTH = 500;

/**
 * Counts Unicode code points, the unit the schema's char_length and the Go
 * store's utf8.RuneCountInString use. String.prototype.length counts UTF-16
 * units, so an astral character would consume two of the 500 and the same
 * wording would be accepted by one layer and refused by another.
 */
export function characterCount(value: string): number {
  return Array.from(value).length;
}

/** True for a blank field or a real YYYY-MM-DD calendar date. */
export function validDateField(raw: string): boolean {
  const text = raw.trim();
  if (text === "") return true;
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(text);
  if (!match) return false;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day
  );
}

function headerFromOrder(
  order: PurchaseOrderDTO,
  currency: CurrencyInfo
): PurchaseOrderHeader {
  return {
    billingAddress: order.BillingAddress,
    deliveryAddress: order.DeliveryAddress,
    shipToCode: order.ShipToCode,
    contactEmail: order.ContactEmail,
    contactPhone: order.ContactPhone,
    poNumber: order.PONumber,
    poDate: order.PODate ?? "",
    receivedDate: order.ReceivedDate,
    requestedDeliveryDate: order.RequestedDeliveryDate ?? "",
    paymentTerms: order.PaymentTerms,
    incoterms: order.Incoterms,
    discountPercent: String(order.DiscountPercent ?? 0),
    adjustment:
      order.AdjustmentMinor === 0 ? "" : formatMoney(currency, order.AdjustmentMinor),
    adjustmentReason: order.AdjustmentReason,
  };
}

/**
 * Builds the editor state from a stored draft. Money re-renders in the PO's
 * currency, and quantities re-render as decimals, so a saved 2.5000 reads
 * back as "2.5" rather than as a raw integer. The stored calculation's inputs
 * (discounts, adjustment) restore exactly; the stored figures themselves are
 * not copied into the editor, because the editor recalculates as the user
 * types and must agree with what the server will store.
 */
export function initialPurchaseOrderEditorState(
  order: PurchaseOrderDTO,
  currency: CurrencyInfo
): PurchaseOrderEditorState {
  const lines = order.Items.map((item, index) => ({
    key: index,
    description: item.Description,
    quantity: formatQuantityScaled(item.QuantityScaled),
    uom: item.UoM,
    unitPrice: formatUnitPriceMinor(currency, item.UnitPriceMinor),
    discountEnabled: item.DiscountEnabled ?? false,
    discountPercent: String(item.DiscountPercent ?? 0),
    quotationItemNo: item.QuotationItemNo ?? null,
    quotationSubNo: item.QuotationSubNo ?? null,
  }));
  const state: PurchaseOrderEditorState = {
    orderId: order.ID,
    quoteNumber: order.QuoteNumber,
    customerName: order.CustomerName,
    sourceRevision: order.SourceRevision,
    sourceState: sourceStateOf(order),
    printedTotalsKnown: printedTotalsKnownOf(order),
    currency: order.Currency,
    taxZone: order.TaxZone,
    taxType:
      order.TaxType !== undefined && order.TaxType !== ""
        ? order.TaxType
        : legacyTaxType(order.TaxCategory),
    serviceCategory:
      order.ServiceCategory !== undefined && order.ServiceCategory !== ""
        ? order.ServiceCategory
        : legacyServiceCategory(order.TaxCategory),
    taxCategory: order.TaxCategory,
    header: headerFromOrder(order, currency),
    lines,
    ppnBps: order.PpnBps ?? 0,
    printedTotals: {
      subtotalMinor: order.PrintedSubtotalMinor ?? 0,
      discountMinor: order.PrintedDiscountMinor ?? 0,
      ppnMinor: order.PrintedPpnMinor ?? 0,
      grandTotalMinor: order.PrintedGrandTotalMinor ?? 0,
    },
    version: order.Version,
    baseline: "",
    nextKey: lines.length,
    saving: false,
    error: null,
  };
  state.baseline = serializePurchaseOrderEditor(state);
  return state;
}

/**
 * The canonical value snapshot dirtiness compares. Keys are excluded because
 * they are UI identity, not data; line order is included because it is the
 * customer's order.
 */
export function serializePurchaseOrderEditor(
  state: PurchaseOrderEditorState
): string {
  return JSON.stringify({
    header: state.header,
    currency: state.currency,
    taxZone: state.taxZone,
    taxType: state.taxType,
    serviceCategory: state.serviceCategory,
    taxCategory: state.taxCategory,
    lines: state.lines.map((line) => ({
      description: line.description,
      quantity: line.quantity,
      uom: line.uom,
      unitPrice: line.unitPrice,
      discountEnabled: line.discountEnabled,
      discountPercent: line.discountPercent,
      quotationItemNo: line.quotationItemNo,
      quotationSubNo: line.quotationSubNo,
    })),
  });
}

/** Whether the editor holds changes that Cancel would discard. */
export function isPurchaseOrderDirty(state: PurchaseOrderEditorState): boolean {
  return serializePurchaseOrderEditor(state) !== state.baseline;
}

/** The display value for an absent confirmation number. Zero is never used. */
export const CONFIRMATION_NUMBER_NOT_ASSIGNED = "Not assigned";

/**
 * The stored confirmation number, or "" when the record is unassigned. The
 * schema stores null, never an empty issued value; trimming here only guards a
 * value that crossed a boundary with padding.
 */
export function confirmationNumberOf(order: PurchaseOrderDTO): string {
  return (order.ConfirmationNumber ?? "").trim();
}

/** What the saved view shows in its identifier slot. */
export function confirmationNumberLabel(order: PurchaseOrderDTO): string {
  const number = confirmationNumberOf(order);
  return number === "" ? CONFIRMATION_NUMBER_NOT_ASSIGNED : number;
}

/**
 * A later Assign action is available only on a saved record with no number.
 * Once assigned, the identifier is read-only; a draft never carries one.
 */
export function canAssignConfirmationNumber(order: PurchaseOrderDTO): boolean {
  return order.State === "saved" && confirmationNumberOf(order) === "";
}

/**
 * A full replace-lines payload, or the reason it cannot be built. Drafts may
 * be incomplete in content, but a line that cannot be parsed must not be
 * silently stored as zero: the reason names the field so the editor can show
 * it.
 */
export function buildPurchaseOrderUpdate(
  state: PurchaseOrderEditorState,
  currency: CurrencyInfo
): { input: PurchaseOrderUpdateInput | null; error: string | null } {
  if (!validDateField(state.header.poDate)) {
    return { input: null, error: "PO date must be YYYY-MM-DD." };
  }
  if (state.header.receivedDate.trim() === "" || !validDateField(state.header.receivedDate)) {
    return { input: null, error: "Received date must be YYYY-MM-DD." };
  }
  if (!validDateField(state.header.requestedDeliveryDate)) {
    return {
      input: null,
      error: "Requested delivery date must be YYYY-MM-DD.",
    };
  }
  // Commercial wording, not a day count: "50% advance" and
  // "30 days after delivery" are both real terms. Only the length is bounded,
  // by the same 500 the schema's CHECK enforces.
  const paymentTerms = state.header.paymentTerms.trim();
  if (characterCount(paymentTerms) > MAX_PAYMENT_TERMS_LENGTH) {
    return {
      input: null,
      error: `Payment terms must be ${MAX_PAYMENT_TERMS_LENGTH} characters or fewer.`,
    };
  }

  const items: PurchaseOrderItemInput[] = [];
  for (let index = 0; index < state.lines.length; index++) {
    const line = state.lines[index];
    const quantity = parseQuantityScaled(line.quantity);
    if (quantity === null) {
      return {
        input: null,
        error: `Line ${index + 1}: quantity must be positive with up to 4 decimal places and within the supported range.`,
      };
    }
    const price = parseUnitPriceMinor(currency, line.unitPrice);
    if (price === null) {
      return {
        input: null,
        error: `Line ${index + 1}: unit price must be a nonnegative ${currency.code} amount within the supported range.`,
      };
    }
    const discountPercent = parseDiscountPercent(line.discountPercent);
    if (discountPercent === null) {
      return {
        input: null,
        error: `Line ${index + 1}: discount must be between 0 and ${MAX_DISCOUNT_PERCENT}.`,
      };
    }
    items.push({
      QuotationItemNo: line.quotationItemNo,
      QuotationSubNo: line.quotationSubNo,
      Description: line.description,
      QuantityScaled: quantity,
      UoM: line.uom,
      UnitPriceMinor: price,
      DiscountEnabled: line.discountEnabled,
      DiscountPercent: discountPercent,
    });
  }
  const discountPercent = parseDiscountPercent(state.header.discountPercent);
  if (discountPercent === null) {
    return {
      input: null,
      error: `Whole-PO discount must be between 0 and ${MAX_DISCOUNT_PERCENT}.`,
    };
  }
  const adjustment = parseAdjustmentMinor(currency, state.header.adjustment);
  if (adjustment === null) {
    return {
      input: null,
      error: `Adjustment must be a signed ${currency.code} amount.`,
    };
  }
  const adjustmentReason = trimAdjustmentReason(state.header.adjustmentReason);

  // The calculation is also the save-time guard: an unexplained adjustment, a
  // negative total, or one out of range is named here rather than after a round
  // trip. The reason rule lives in one place, so it cannot diverge between the
  // live totals panel and the payload.
  const calculation = calculatePurchaseOrder(state, currency);
  if (calculation.error !== null) {
    return { input: null, error: calculation.error };
  }

  return {
    input: {
      ID: state.orderId,
      BillingAddress: state.header.billingAddress,
      DeliveryAddress: state.header.deliveryAddress,
      ShipToCode: state.header.shipToCode,
      ContactEmail: state.header.contactEmail,
      ContactPhone: state.header.contactPhone,
      PONumber: state.header.poNumber,
      PODate: state.header.poDate,
      ReceivedDate: state.header.receivedDate,
      RequestedDeliveryDate: state.header.requestedDeliveryDate,
      Incoterms: state.header.incoterms,
      PaymentTerms: paymentTerms,
      AdjustmentReason: adjustment === 0 ? "" : adjustmentReason,
      Items: items,
      DiscountPercent: discountPercent,
      AdjustmentMinor: adjustment,
      // A never- or formerly-linked draft may move currency/basis before a
      // link exists. A live row's stored basis is frozen; empty means "keep"
      // and prevents the editor's legacy display derivation from being
      // submitted as a change.
      Currency: state.sourceState !== "live" ? state.currency : "",
      TaxZone: state.sourceState !== "live" ? state.taxZone : "",
      TaxType: state.sourceState !== "live" ? state.taxType : "",
      ServiceCategory: state.sourceState !== "live" ? state.serviceCategory : "",
      ExpectedVersion: state.version,
      Overwrite: false,
    },
    error: null,
  };
}

/** The reducer; pure, so the dirty guard is testable without a DOM. */
export function purchaseOrderEditorReducer(
  state: PurchaseOrderEditorState,
  action: PurchaseOrderEditorAction
): PurchaseOrderEditorState {
  switch (action.type) {
    case "headerChanged":
      return {
        ...state,
        header: { ...state.header, ...action.patch },
      };
    case "lineChanged":
      return {
        ...state,
        lines: state.lines.map((line) =>
          line.key === action.key ? { ...line, ...action.patch } : line
        ),
      };
    case "lineAdded":
      return {
        ...state,
        lines: [
          ...state.lines,
          {
            key: state.nextKey,
            description: "",
            quantity: "1",
            uom: action.uom,
            unitPrice: "0",
            discountEnabled: false,
            discountPercent: "0",
            quotationItemNo: null,
            quotationSubNo: null,
          },
        ],
        nextKey: state.nextKey + 1,
      };
    case "lineRemoved":
      return {
        ...state,
        lines: state.lines.filter((line) => line.key !== action.key),
      };
    case "sourceBasisChanged": {
      const next = { ...state, ...action.patch };
      if (action.patch.taxType === "Goods") next.serviceCategory = "";
      if (
        action.patch.taxType === "Services" &&
        (next.serviceCategory === "" || next.serviceCategory === undefined)
      ) {
        next.serviceCategory = "NonConstruction";
      }
      next.ppnBps =
        purchaseOrderPpnBps(
          next.taxZone,
          next.taxType,
          next.serviceCategory,
          next.taxCategory
        ) ?? 0;
      return next;
    }
    case "currencyChanged": {
      // No conversion: the stored minor units stay the same. Only the display
      // changes, so a currency edited before amounts are entered reads
      // correctly in the new denomination.
      const lines = state.lines.map((line) => {
        const minor = parseUnitPriceMinor(action.from, line.unitPrice);
        return minor === null
          ? line
          : { ...line, unitPrice: formatMoney(action.to, minor) };
      });
      const adjustment = parseAdjustmentMinor(action.from, state.header.adjustment);
      const header = {
        ...state.header,
        adjustment:
          adjustment === null
            ? state.header.adjustment
            : adjustment === 0
              ? ""
              : formatMoney(action.to, adjustment),
      };
      return { ...state, currency: action.code, header, lines };
    }
    case "created":
      // The first Save/upload persisted the transient row. Keep the working
      // copy and only adopt the server identity, so a failed follow-up update
      // retries against this row instead of creating a second one.
      return { ...state, orderId: action.orderId, version: action.version };
    case "saveStarted":
      return { ...state, saving: true, error: null };
    case "saveConflict":
      // The conflict dialog owns the explanation; the editor keeps its working
      // copy and shows no duplicate error line.
      return { ...state, saving: false, error: null };
    case "versionAdopted":
      // Wave 2 of #473: a draft write that met a saved record continues as a
      // correction against the version the server named, without discarding the
      // working copy or the version it must submit.
      return { ...state, version: action.version };
    case "valuesReplaced":
      // Wave 2 of #473: a finalize review can hold "Use Quotation" choices that
      // exist only as decisions. Applying them to the working copy before the
      // correction transition keeps the values the reviewer confirmed.
      return { ...state, header: action.header, lines: action.lines, error: null };
    case "extractionApplied":
      // #477: only the chosen proposals. Lines are replaced only when the
      // staff member ticked at least one extracted row; otherwise the current
      // entries stay exactly as they were.
      return {
        ...state,
        header: { ...state.header, ...action.header },
        lines: action.lines ?? state.lines,
        nextKey: Math.max(state.nextKey, action.nextKey),
        error: null,
      };
    case "saveFailed":
      return { ...state, saving: false, error: action.error };
    case "saveSucceeded": {
      const saved = initialPurchaseOrderEditorState(action.order, action.currency);
      return { ...saved, nextKey: Math.max(saved.nextKey, state.nextKey) };
    }
    default:
      return state;
  }
}
