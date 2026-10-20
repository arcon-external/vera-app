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
}

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
  TaxCategory: string;
  CreatedAt: string;
  UpdatedAt: string;
  Items: PurchaseOrderDTOItem[];
  ID: number;
  QuotationID: number;
  SourceRevision: number;
  CustomerID: number;
  PaymentTermsDays: number;
}

/** One editable line. Quantity and money stay strings until they are parsed. */
export interface PurchaseOrderLine {
  key: number;
  description: string;
  quantity: string;
  uom: string;
  unitPrice: string;
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
  paymentTermsDays: string;
  incoterms: string;
}

export interface PurchaseOrderEditorState {
  orderId: number;
  quoteNumber: string;
  customerName: string;
  sourceRevision: number;
  currency: string;
  taxZone: string;
  taxCategory: string;
  header: PurchaseOrderHeader;
  lines: PurchaseOrderLine[];
  /** Canonical snapshot of the last loaded/saved values, for dirtiness. */
  baseline: string;
  nextKey: number;
  saving: boolean;
  error: string | null;
  confirmDiscard: boolean;
}

export type PurchaseOrderEditorAction =
  | { type: "headerChanged"; patch: Partial<PurchaseOrderHeader> }
  | { type: "lineChanged"; key: number; patch: Partial<PurchaseOrderLine> }
  | { type: "lineAdded"; uom: string }
  | { type: "lineRemoved"; key: number }
  | { type: "saveStarted" }
  | { type: "saveFailed"; error: string }
  | { type: "saveSucceeded"; order: PurchaseOrderDTO; currency: CurrencyInfo }
  | { type: "cancelRequested" }
  | { type: "cancelDismissed" };

/** One replace-lines payload entry (PascalCase = Wails binding). */
export interface PurchaseOrderItemInput {
  QuotationItemNo?: number | null;
  QuotationSubNo?: number | null;
  Description: string;
  QuantityScaled: number;
  UoM: string;
  UnitPriceMinor: number;
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
  PaymentTermsDays: number;
  Items: PurchaseOrderItemInput[];
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
  if (!Number.isSafeInteger(whole) || whole > Number.MAX_SAFE_INTEGER / QUANTITY_SCALE) {
    return null;
  }
  const scaled = whole * QUANTITY_SCALE + fraction;
  if (!Number.isSafeInteger(scaled) || scaled <= 0) return null;
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
  if (parsed === null || parsed < 0 || !Number.isSafeInteger(parsed)) return null;
  return parsed;
}

/** Formats a stored minor amount for its money input. */
export function formatUnitPriceMinor(
  currency: CurrencyInfo,
  minor: number
): string {
  return formatMoney(currency, minor);
}

/** Parses the payment-terms field. Empty is 0 (not agreed yet). */
export function parsePaymentTermsDays(raw: string): number | null {
  const text = raw.trim();
  if (text === "") return 0;
  if (!/^\d+$/.test(text)) return null;
  const value = Number(text);
  if (!Number.isSafeInteger(value) || value > 3650) return null;
  return value;
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

function headerFromOrder(order: PurchaseOrderDTO): PurchaseOrderHeader {
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
    paymentTermsDays: String(order.PaymentTermsDays),
    incoterms: order.Incoterms,
  };
}

/**
 * Builds the editor state from a stored draft. Money re-renders in the PO's
 * currency, and quantities re-render as decimals, so a saved 2.5000 reads
 * back as "2.5" rather than as a raw integer.
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
    quotationItemNo: item.QuotationItemNo ?? null,
    quotationSubNo: item.QuotationSubNo ?? null,
  }));
  const state: PurchaseOrderEditorState = {
    orderId: order.ID,
    quoteNumber: order.QuoteNumber,
    customerName: order.CustomerName,
    sourceRevision: order.SourceRevision,
    currency: order.Currency,
    taxZone: order.TaxZone,
    taxCategory: order.TaxCategory,
    header: headerFromOrder(order),
    lines,
    baseline: "",
    nextKey: lines.length,
    saving: false,
    error: null,
    confirmDiscard: false,
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
    lines: state.lines.map((line) => ({
      description: line.description,
      quantity: line.quantity,
      uom: line.uom,
      unitPrice: line.unitPrice,
      quotationItemNo: line.quotationItemNo,
      quotationSubNo: line.quotationSubNo,
    })),
  });
}

/** Whether the editor holds changes that Cancel would discard. */
export function isPurchaseOrderDirty(state: PurchaseOrderEditorState): boolean {
  return serializePurchaseOrderEditor(state) !== state.baseline;
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
  const paymentTerms = parsePaymentTermsDays(state.header.paymentTermsDays);
  if (paymentTerms === null) {
    return {
      input: null,
      error: "Payment terms must be a whole number of days between 0 and 3650.",
    };
  }

  const items: PurchaseOrderItemInput[] = [];
  for (let index = 0; index < state.lines.length; index++) {
    const line = state.lines[index];
    const quantity = parseQuantityScaled(line.quantity);
    if (quantity === null) {
      return {
        input: null,
        error: `Line ${index + 1}: quantity must be positive with up to 4 decimal places.`,
      };
    }
    const price = parseUnitPriceMinor(currency, line.unitPrice);
    if (price === null) {
      return {
        input: null,
        error: `Line ${index + 1}: unit price must be a nonnegative ${currency.code} amount.`,
      };
    }
    items.push({
      QuotationItemNo: line.quotationItemNo,
      QuotationSubNo: line.quotationSubNo,
      Description: line.description,
      QuantityScaled: quantity,
      UoM: line.uom,
      UnitPriceMinor: price,
    });
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
      PaymentTermsDays: paymentTerms,
      Items: items,
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
        confirmDiscard: false,
      };
    case "lineChanged":
      return {
        ...state,
        lines: state.lines.map((line) =>
          line.key === action.key ? { ...line, ...action.patch } : line
        ),
        confirmDiscard: false,
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
            quotationItemNo: null,
            quotationSubNo: null,
          },
        ],
        nextKey: state.nextKey + 1,
        confirmDiscard: false,
      };
    case "lineRemoved":
      return {
        ...state,
        lines: state.lines.filter((line) => line.key !== action.key),
        confirmDiscard: false,
      };
    case "saveStarted":
      return { ...state, saving: true, error: null };
    case "saveFailed":
      return { ...state, saving: false, error: action.error };
    case "saveSucceeded": {
      const saved = initialPurchaseOrderEditorState(action.order, action.currency);
      return { ...saved, nextKey: Math.max(saved.nextKey, state.nextKey) };
    }
    case "cancelRequested":
      return isPurchaseOrderDirty(state)
        ? { ...state, confirmDiscard: true }
        : state;
    case "cancelDismissed":
      return { ...state, confirmDiscard: false };
    default:
      return state;
  }
}
