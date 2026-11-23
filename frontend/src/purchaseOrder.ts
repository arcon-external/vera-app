// Issue #468: the PO draft editor's decisions, separated from its markup.
//
// The editor enters a customer order against one issued Quotation revision.
// Everything here is pure: parsing the exact quantity and money strings the
// fields hold, tracking whether the draft is dirty, and building the replace-
// lines payload. Exact totals and discounts are #470's slice, so none of this
// recalculates a price.

import {
  formatMoney,
  type CurrencyInfo,
} from "./format";
import {
  MAX_DISCOUNT_PERCENT,
  MAX_EXACT_MINOR,
  QUANTITY_SCALE,
  buildAggregatePayload,
  lineGrossMinor,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
  pruneEmptyAggregates,
  rowDiscountMinor,
  toMoneyMinor,
  ungroupAggregate,
  validateAggregateLayout,
} from "./poAggregate";
import { legacyServiceCategory, legacyTaxType } from "./tax";

// The exact-input parsers and the money kernel live in poAggregate.ts, the
// lower layer, so that module can own the aggregate arithmetic without a
// runtime cycle. They are re-exported here because this module is the PO
// editor's public surface and every existing caller imports them from here.
export {
  MAX_DISCOUNT_PERCENT,
  MAX_EXACT_MINOR,
  QUANTITY_SCALE,
  parseDiscountPercent,
  parseQuantityScaled,
  parseUnitPriceMinor,
};

/** One line as the desktop binding returns it (PascalCase = Wails models). */
export interface PurchaseOrderDTOItem {
  /**
   * The persisted row id (#517). Required on rows the server returns;
   * optional here so payloads built against older bindings still decode.
   */
  ID?: number;
  /**
   * The row-carried editor alias (#517, migration 042). Null on pre-042 rows
   * and on payloads built against older bindings.
   */
  ClientKey?: string | null;
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
  GroupKey?: string;
  ParentDescription?: string;
  DescriptionLines?: string[];
  /**
   * #653: the frozen row-discounted quotation baseline and the quantity still
   * orderable on this leaf. Workspace-only customer-facing values, never
   * supplier cost; absent on payloads from an older server.
   */
  QuotedUnitPriceMinor?: number | null;
  RemainingQuantityScaled?: number | null;
  /**
   * #654: the aggregate presentation group this row belongs to. The desktop
   * binding carries 0 for ungrouped (its generated model has no pointers);
   * null/undefined on an older payload mean the same thing.
   */
  AggregateGroupID?: number | null;
}

/**
 * One PO-only aggregate presentation header (#654). TotalMinor and
 * DisplayUnitPriceMinor are derived on read; neither is stored, and no group
 * row carries supplier cost, margin or variance (Story 42).
 */
export interface PurchaseOrderDTOAggregateGroup {
  ID?: number;
  /** The first child's persisted line number; the read order is stable. */
  DisplayOrder: number;
  Title: string;
  OrderedQuantityScaled: number;
  OrderedUoM: string;
  ShowChildrenToCustomer: boolean;
  /** Derived on read: the sum of the child lines' nets. */
  TotalMinor: number;
  /** Derived on read; null when the exact value exceeds the safe range. */
  DisplayUnitPriceMinor?: number | null;
}

/** The printed source figures the quotation revision carried (issue #470). */
export interface PurchaseOrderPrintedTotals {
  subtotalMinor: number;
  discountMinor: number;
  ppnMinor: number;
  grandTotalMinor: number;
}

/**
 * The four business category codes (#628), the client's own reporting
 * vocabulary. The empty string is Unassigned: a draft that has not chosen one,
 * or a record saved before the category existed. It is never inferred.
 */
export type PurchaseOrderBusinessCategory = "" | "FM" | "FAB" | "SUPPLY_GEN" | "OTHERS";

/** One category option and the label staff read. */
export interface PurchaseOrderBusinessCategoryOption {
  value: PurchaseOrderBusinessCategory;
  label: string;
}

/**
 * The category picker's options, in the client's workbook order. Unassigned is
 * first so an empty draft is visibly empty rather than defaulting to a
 * business line, and so a pre-category record reads truthfully.
 */
export const PURCHASE_ORDER_BUSINESS_CATEGORY_OPTIONS: readonly PurchaseOrderBusinessCategoryOption[] =
  [
    { value: "", label: "Unassigned" },
    { value: "FM", label: "Facility Management" },
    { value: "FAB", label: "Fabrication" },
    { value: "SUPPLY_GEN", label: "Goods Supply & General" },
    { value: "OTHERS", label: "Others" },
  ];

/**
 * The label for a stored category. Only an empty value is Unassigned; a code
 * this build does not know is shown as stored, because recasting it as
 * Unassigned would silently erase a category someone else recorded.
 */
export function purchaseOrderBusinessCategoryLabel(
  value: string | undefined
): string {
  if (value === undefined || value.trim() === "") return "Unassigned";
  const option = PURCHASE_ORDER_BUSINESS_CATEGORY_OPTIONS.find(
    (entry) => entry.value === value
  );
  return option?.label ?? value;
}

/** Whether a stored value is one of the four codes (empty is not a category). */
export function isPurchaseOrderBusinessCategory(
  value: string
): value is Exclude<PurchaseOrderBusinessCategory, ""> {
  return (
    value === "FM" ||
    value === "FAB" ||
    value === "SUPPLY_GEN" ||
    value === "OTHERS"
  );
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
  /**
   * The #628 reporting category. Older payloads omit it; an absent value is
   * Unassigned, never inferred.
   */
  BusinessCategory?: string;
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
   * Whether the draft asked for a confirmation number (#581). It allocates
   * nothing; the final save's checkbox remains the authority.
   */
  ConfirmationRequested?: boolean;
  /**
   * The optimistic-concurrency revision (#473). It starts at 1 and moves
   * forward on every write; the editor submits the version it loaded.
   */
  Version: number;
  /**
   * #654: the PO-only aggregate presentation headers, in read order. An older
   * server omits the field; absence is "no aggregates", never inferred.
   */
  AggregateGroups?: PurchaseOrderDTOAggregateGroup[];
}

/** One editable line. Quantity and money stay strings until they are parsed. */
export interface PurchaseOrderLine {
  key: number;
  /**
   * #517: the editor alias (`client:<UUID>`), generated per line and always
   * submitted. It is the line's session key while the session first sees it;
   * a session's first-seen key never re-keys afterwards, so this alias is
   * carried across saves, reorder and reopen verbatim.
   */
  clientKey: string;
  /**
   * #517: the persisted row id from the DTO, once loaded. Never edited
   * locally; submitted back so the server matches the row by id first.
   */
  persistedId?: number;
  /**
   * #517: the row-carried alias from the DTO (migration 042). A loaded line
   * reuses it as its editor alias so reopen keeps the same session key; a
   * pre-042 row has none and keeps a fresh local alias while its session
   * key stays `id:<row>`.
   */
  persistedClientKey?: string | null;
  description: string;
  quantity: string;
  uom: string;
  unitPrice: string;
  discountEnabled: boolean;
  discountPercent: string;
  quotationItemNo: number | null;
  quotationSubNo: number | null;
  /**
   * Split-line identity (#573). Empty for a plain line; every row of one split
   * line shares the key, and the parent is a presentation of those rows.
   */
  groupKey: string;
  /** The split line's title; empty for a plain line. */
  parentDescription: string;
  /** Description bullets on a plain line, or sub-sub lines on a sub-item. */
  descriptionLines: string[];
  /**
   * #653: the quotation baseline this negotiated line is compared against and
   * the quantity still orderable. Null means the server did not know (a blank
   * or former-source draft), so no variance badge is invented.
   */
  quotedUnitPriceMinor?: number | null;
  remainingQuantityScaled?: number | null;
  /**
   * #654: the editor-stable aggregate group this row belongs to, or null when
   * it stands on its own. It is UI identity like `key`, never submitted; the
   * payload sends the group's 1-based position instead.
   */
  aggregateKey: number | null;
}

/**
 * One editable aggregate header (#654). The quantity stays a string until it
 * is parsed on save, exactly like a line's quantity and price.
 */
export interface PurchaseOrderAggregateGroupState {
  /** Editor-stable key; never submitted. */
  key: number;
  /** Read-only convenience: the persisted row id, absent until a save. */
  persistedId?: number;
  title: string;
  /** The user-typed display quantity; parsed on save. */
  orderedQuantity: string;
  orderedUoM: string;
  showChildrenToCustomer: boolean;
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
  /** The #628 reporting category; empty while a draft may be Unassigned. */
  businessCategory: PurchaseOrderBusinessCategory;
  /** The draft's confirmation-number request (#581); off by default. */
  confirmationRequested: boolean;
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
  /** The aggregate headers in display order (#654). */
  aggregateGroups: PurchaseOrderAggregateGroupState[];
  /** The next unused editor key for a new aggregate header. */
  nextAggregateKey: number;
  saving: boolean;
  error: string | null;
}

/**
 * Ticket 05: one collision-resistant client key per editor line. Stored on
 * the line so edits, reorder and validation all keep the same identity the
 * reconciliation items key off.
 */
export function newPurchaseOrderLineKey(): string {
  try {
    // Called as a method: a detached randomUUID loses its receiver.
    const id: unknown = globalThis.crypto?.randomUUID?.();
    if (typeof id === "string" && id.length > 0) return `client:${id}`;
  } catch {
    // A missing or unbound implementation falls through to the counter.
  }
  return `client:local-${Date.now().toString(36)}-${Math.floor(Math.random() * 0xffffffff).toString(36)}`;
}

export type PurchaseOrderEditorAction =
  | { type: "headerChanged"; patch: Partial<PurchaseOrderHeader> }
  | { type: "lineChanged"; key: number; patch: Partial<PurchaseOrderLine> }
  | { type: "lineAdded"; uom: string }
  | { type: "lineAddedFromSource"; uom: string; quotationItemNo: number; quotationSubNo: number | null }
  | { type: "lineRemoved"; key: number }
  | { type: "linesReplaced"; lines: PurchaseOrderLine[] }
  /** #654: record a new aggregate header and set its rows' membership. */
  | { type: "aggregateAdded"; group: PurchaseOrderAggregateGroupState; lineKeys: number[] }
  /** #654: edit a header's title, display quantity, unit or print toggle. */
  | {
      type: "aggregateChanged";
      key: number;
      patch: Partial<
        Pick<
          PurchaseOrderAggregateGroupState,
          "title" | "orderedQuantity" | "orderedUoM" | "showChildrenToCustomer"
        >
      >;
    }
  /** #654: clear membership and remove the header, without touching the rows. */
  | { type: "aggregateUngrouped"; key: number }
  /** #654: drop headers left with no children after a removal. */
  | { type: "aggregatePruned" }
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
  | { type: "saveFailed"; error: string }
  | { type: "saveSucceeded"; order: PurchaseOrderDTO; currency: CurrencyInfo };

/** One submitted aggregate header (#654, PascalCase = Wails binding). */
export interface PurchaseOrderAggregateGroupInput {
  Title: string;
  OrderedQuantityScaled: number;
  OrderedUoM: string;
  ShowChildrenToCustomer: boolean;
}

/** One replace-lines payload entry (PascalCase = Wails binding). */
export interface PurchaseOrderItemInput {
  /**
   * The persisted row id when the line was loaded from the server (#517).
   * Null for editor-entered lines; the server matches ID, then alias, then
   * exact values, and refuses unknown ids and duplicate claims.
   */
  ID?: number | null;
  /**
   * The editor alias, always submitted (#517). The server adopts it onto the
   * matched row, binds it in the session identities and refuses an alias
   * owned by another line; an unbound alias is refused with save guidance.
   */
  ClientKey?: string | null;
  QuotationItemNo?: number | null;
  QuotationSubNo?: number | null;
  Description: string;
  QuantityScaled: number;
  UoM: string;
  UnitPriceMinor: number;
  DiscountEnabled: boolean;
  DiscountPercent: number;
  GroupKey: string;
  ParentDescription: string;
  DescriptionLines: string[];
  /**
   * #654: the 1-based position of this row's aggregate group in
   * `AggregateGroups`; 0 means ungrouped. Position, never the editor key, so
   * a save that rekeys the groups does not mark the editor dirty.
   */
  AggregateGroupIndex?: number;
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
  /** The #628 reporting category. Empty is Unassigned on a draft. */
  BusinessCategory: PurchaseOrderBusinessCategory;
  /** The draft's confirmation-number request (#581). Allocates nothing. */
  ConfirmationRequested: boolean;
  Items: PurchaseOrderItemInput[];
  /**
   * #654: always present, empty when the order has no aggregates. The server
   * distinguishes an absent slice (an old client, refused on a grouped record)
   * from an empty one (the deliberate ungroup).
   */
  AggregateGroups: PurchaseOrderAggregateGroupInput[];
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

/** One inline variance badge on a purchase-order line (#653). */
export interface PurchaseOrderVarianceBadge {
  kind: "unsold" | "quantityExtra" | "priceLoss" | "priceExtra";
  tone: "neutral" | "positive" | "danger";
  label: string;
}

/**
 * The inline badges a PO line earns against its quotation baseline. The
 * function is pure and takes scaled quantities and minor-unit prices, so the
 * parent line, a package part and a unit test all use one rule.
 *
 * Unsold is never a loss: a quantity below the remaining quotation quantity is
 * neutral. A quantity above it is extra income. A negotiated price below the
 * baseline is a price loss; above it is price extra income. No supplier cost
 * is an input or an output (Story 42).
 */
export function purchaseOrderVarianceBadges(input: {
  quantityScaled: number;
  unitPriceMinor: number | null;
  quotedUnitPriceMinor?: number | null;
  remainingQuantityScaled?: number | null;
}, currency: CurrencyInfo): PurchaseOrderVarianceBadge[] {
  const badges: PurchaseOrderVarianceBadge[] = [];
  const remaining = input.remainingQuantityScaled;
  if (
    typeof remaining === "number" &&
    Number.isFinite(remaining) &&
    remaining >= 0 &&
    Number.isFinite(input.quantityScaled) &&
    input.quantityScaled > 0
  ) {
    if (input.quantityScaled < remaining) {
      badges.push({
        kind: "unsold",
        tone: "neutral",
        label: `${formatQuantityScaled(remaining - input.quantityScaled)} remaining (Unsold)`,
      });
    } else if (input.quantityScaled > remaining) {
      badges.push({
        kind: "quantityExtra",
        tone: "positive",
        label: `+${formatQuantityScaled(input.quantityScaled - remaining)} over quoted (Extra income)`,
      });
    }
  }
  const baseline = input.quotedUnitPriceMinor;
  if (
    typeof baseline === "number" &&
    Number.isFinite(baseline) &&
    baseline >= 0 &&
    typeof input.unitPriceMinor === "number" &&
    Number.isFinite(input.unitPriceMinor)
  ) {
    const difference = input.unitPriceMinor - baseline;
    if (difference < 0) {
      badges.push({
        kind: "priceLoss",
        tone: "danger",
        label: `−${formatMoney(currency, -difference)}/unit vs quoted (Price loss)`,
      });
    } else if (difference > 0) {
      badges.push({
        kind: "priceExtra",
        tone: "positive",
        label: `+${formatMoney(currency, difference)}/unit vs quoted (Price extra income)`,
      });
    }
  }
  return badges;
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

/** Formats a stored minor amount for its money input. */
export function formatUnitPriceMinor(
  currency: CurrencyInfo,
  minor: number
): string {
  return formatMoney(currency, minor);
}

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
 * until Save draft or a PDF upload asks for a row; opening the editor creates
 * nothing. Every identity field is empty, and SourceState "never" tells the UI
 * not to show a Customer, a quotation revision or printed totals.
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
    BusinessCategory: "",
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
  /**
   * Always zero: the PO has no whole-order discount (#573). Kept as a stage so
   * the totals check can still compare the printed discount with the PO's
   * absence of one.
   */
  discountMinor: number;
  afterDiscountMinor: number;
  ppnMinor: number;
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

/** `amount × basis points ÷ 10000`, truncated, exactly as calcTax does it. */
function basisPointsMinor(amount: number, bps: number): number | null {
  return toMoneyMinor((BigInt(amount) * BigInt(bps)) / 10000n);
}

/**
 * The explained customer-facing calculation, mirroring CalculatePurchaseOrder
 * in backend/store/po_calc.go stage for stage:
 *
 *   1. line gross    half away from zero(quantity × unit price ÷ 10^4)
 *   2. row discount  truncate(gross × percent ÷ 100); net = gross − discount
 *   3. subtotal      sum of row nets
 *   4. PPN           truncate(subtotal × bps ÷ 10000), added on top
 *   5. grand total   subtotal + PPN
 *
 * The whole-PO discount and the signed adjustment are retired (#573); the
 * store keeps their dormant columns at zero. Internal cost, margin, PPH23 and
 * profit have no place in this module: a PO is compared against
 * customer-facing figures only.
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
    const discount = line.discountEnabled ? rowDiscountMinor(gross, percent) : 0;
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

  // No whole-PO discount and no adjustment (#573): the subtotal is the sum of
  // the row nets, and PPN is charged on top of it. The printed source may have
  // carried a whole discount; the totals check shows that difference rather
  // than reproducing it here.
  const error = firstError;
  if (error !== null) {
    return { lines: lineResults, totals: null, error };
  }

  const subtotalMinor = toMoneyMinor(subtotal);
  if (subtotalMinor === null) {
    return {
      lines: lineResults,
      totals: null,
      error: "The subtotal exceeds the supported amount range.",
    };
  }

  const ppn = basisPointsMinor(subtotalMinor, state.ppnBps);
  if (ppn === null) {
    return {
      lines: lineResults,
      totals: null,
      error: "PPN exceeds the supported amount range.",
    };
  }
  const grandBig = BigInt(subtotalMinor) + BigInt(ppn);
  if (grandBig > BigInt(MAX_EXACT_MINOR)) {
    return {
      lines: lineResults,
      totals: null,
      error: "The purchase order total exceeds the supported amount range.",
    };
  }

  return {
    lines: lineResults,
    totals: {
      grossMinor: toMoneyMinor(grossTotal),
      rowDiscountMinor: toMoneyMinor(rowDiscountTotal),
      subtotalMinor,
      discountMinor: 0,
      afterDiscountMinor: subtotalMinor,
      ppnMinor: ppn,
      grandTotalMinor: Number(grandBig),
    },
    error: null,
  };
}

/** The server's maximum free-text length for payment terms. */
export const MAX_PAYMENT_TERMS_LENGTH = 500;

/** The split-line text caps, mirrored by the store and migration 048. */
export const MAX_PARENT_DESCRIPTION_LENGTH = 500;
export const MAX_DESCRIPTION_LINES = 50;
export const MAX_DESCRIPTION_LINE_LENGTH = 500;

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
    // An older payload has no category; that is Unassigned, not a guess.
    businessCategory: isPurchaseOrderBusinessCategory(order.BusinessCategory ?? "")
      ? (order.BusinessCategory as Exclude<PurchaseOrderBusinessCategory, "">)
      : "",
    confirmationRequested: order.ConfirmationRequested ?? false,
  };
}

/**
 * Builds the editor state from a stored draft. Money re-renders in the PO's
 * currency, and quantities re-render as decimals, so a saved 2.5000 reads
 * back as "2.5" rather than as a raw integer. The stored row discounts restore
 * exactly; the stored figures themselves are not copied into the editor,
 * because the editor recalculates as the user types and must agree with what
 * the server will store. The retired whole-PO discount and adjustment are
 * ignored on read and pinned to zero on write (#573).
 */
export function initialPurchaseOrderEditorState(
  order: PurchaseOrderDTO,
  currency: CurrencyInfo
): PurchaseOrderEditorState {
  // #654: the aggregate headers are keyed by their array position, which is the
  // store's display order. Each line's persisted group id resolves to that
  // key; an id this payload does not carry (or 0, the desktop binding's
  // "ungrouped") is null rather than a guessed group.
  const groupKeyByID = new Map<number, number>();
  const aggregateGroups: PurchaseOrderAggregateGroupState[] = (
    order.AggregateGroups ?? []
  ).map((entry, index) => {
    if (typeof entry.ID === "number" && entry.ID > 0) {
      groupKeyByID.set(entry.ID, index);
    }
    return {
      key: index,
      persistedId: entry.ID,
      title: entry.Title,
      orderedQuantity: formatQuantityScaled(entry.OrderedQuantityScaled),
      orderedUoM: entry.OrderedUoM,
      showChildrenToCustomer: entry.ShowChildrenToCustomer ?? false,
    };
  });
  const lines = order.Items.map((item, index) => ({
    key: index,
    // #517: a row-carried alias is reused verbatim so reopen keeps the same
    // session key; otherwise the session key is derived at commit time and a
    // fresh local alias is submitted for the server to adopt.
    clientKey:
      typeof item.ClientKey === "string" && item.ClientKey !== ""
        ? item.ClientKey
        : newPurchaseOrderLineKey(),
    persistedId: item.ID,
    persistedClientKey: item.ClientKey ?? null,
    description: item.Description,
    quantity: formatQuantityScaled(item.QuantityScaled),
    uom: item.UoM,
    unitPrice: formatUnitPriceMinor(currency, item.UnitPriceMinor),
    discountEnabled: item.DiscountEnabled ?? false,
    discountPercent: String(item.DiscountPercent ?? 0),
    quotationItemNo: item.QuotationItemNo ?? null,
    quotationSubNo: item.QuotationSubNo ?? null,
    groupKey: item.GroupKey ?? "",
    parentDescription: item.ParentDescription ?? "",
    descriptionLines: item.DescriptionLines ?? [],
    quotedUnitPriceMinor: item.QuotedUnitPriceMinor ?? null,
    remainingQuantityScaled: item.RemainingQuantityScaled ?? null,
    aggregateKey:
      typeof item.AggregateGroupID === "number" && item.AggregateGroupID > 0
        ? groupKeyByID.get(item.AggregateGroupID) ?? null
        : null,
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
    aggregateGroups,
    nextAggregateKey: aggregateGroups.length,
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
 *
 * #654: an aggregate is part of the customer-facing order, so its four values
 * and each line's membership are included — but as the group's array
 * position, never the editor key. A save that rekeys the groups therefore
 * produces the same snapshot and does not make a saved order look dirty.
 */
export function serializePurchaseOrderEditor(
  state: PurchaseOrderEditorState
): string {
  const positionByGroupKey = new Map<number, number>();
  state.aggregateGroups.forEach((group, index) => {
    positionByGroupKey.set(group.key, index + 1);
  });
  return JSON.stringify({
    header: state.header,
    currency: state.currency,
    taxZone: state.taxZone,
    taxType: state.taxType,
    serviceCategory: state.serviceCategory,
    taxCategory: state.taxCategory,
    aggregateGroups: state.aggregateGroups.map((group) => ({
      title: group.title,
      orderedQuantity: group.orderedQuantity,
      orderedUoM: group.orderedUoM,
      showChildrenToCustomer: group.showChildrenToCustomer,
    })),
    lines: state.lines.map((line) => ({
      description: line.description,
      quantity: line.quantity,
      uom: line.uom,
      unitPrice: line.unitPrice,
      discountEnabled: line.discountEnabled,
      discountPercent: line.discountPercent,
      quotationItemNo: line.quotationItemNo,
      quotationSubNo: line.quotationSubNo,
      groupKey: line.groupKey,
      parentDescription: line.parentDescription,
      descriptionLines: line.descriptionLines,
      aggregateIndex:
        line.aggregateKey === null
          ? 0
          : positionByGroupKey.get(line.aggregateKey) ?? 0,
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
 * The version a write may name for a stored row (#584). The editor state and
 * the retained draft can each be a render behind the other: a draft save
 * advances the state through `saveSucceeded`, while a session save advances
 * the retained draft before React renders it. The write must name the newer of
 * the two, and a retained version may never move backwards.
 */
export function latestPersistedVersion(
  orderId: number,
  stateVersion: number,
  retained: { id: number; version: number } | null
): number {
  if (retained === null || retained.id !== orderId) return stateVersion;
  return Math.max(stateVersion, retained.version);
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
  // #654: the aggregate layout is checked before the lines are rendered into
  // the payload, so an impossible placement is named by its group instead of
  // being sent for the server to refuse after the round trip.
  const aggregateError = validateAggregateLayout(state, currency);
  if (aggregateError !== null) {
    return { input: null, error: aggregateError };
  }
  const aggregatePayload = buildAggregatePayload(state.aggregateGroups, state.lines);
  if (aggregatePayload.error !== null) {
    // validateAggregateLayout already runs the same rules; this is a guard,
    // not a second rule set.
    return { input: null, error: aggregatePayload.error };
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
    if (characterCount(line.parentDescription) > MAX_PARENT_DESCRIPTION_LENGTH) {
      return {
        input: null,
        error: `Line ${index + 1}: the split-line title must be ${MAX_PARENT_DESCRIPTION_LENGTH} characters or fewer.`,
      };
    }
    if (line.descriptionLines.length > MAX_DESCRIPTION_LINES) {
      return {
        input: null,
        error: `Line ${index + 1}: at most ${MAX_DESCRIPTION_LINES} description lines are allowed.`,
      };
    }
    for (const descriptionLine of line.descriptionLines) {
      if (characterCount(descriptionLine) > MAX_DESCRIPTION_LINE_LENGTH) {
        return {
          input: null,
          error: `Line ${index + 1}: each description line must be ${MAX_DESCRIPTION_LINE_LENGTH} characters or fewer.`,
        };
      }
    }
    const aggregateGroupIndex =
      line.aggregateKey === null
        ? 0
        : aggregatePayload.indexByGroupKey.get(line.aggregateKey) ?? 0;
    items.push({
      ID: line.persistedId ?? null,
      ClientKey: line.clientKey,
      QuotationItemNo: line.quotationItemNo,
      QuotationSubNo: line.quotationSubNo,
      Description: line.description,
      QuantityScaled: quantity,
      UoM: line.uom,
      UnitPriceMinor: price,
      DiscountEnabled: line.discountEnabled,
      DiscountPercent: discountPercent,
      GroupKey: line.groupKey,
      ParentDescription: line.parentDescription,
      DescriptionLines: line.descriptionLines,
      // #654: the group's 1-based position. An ungrouped row omits the field
      // rather than sending 0, so every existing payload keeps its exact shape
      // and the server's "0 or absent means ungrouped" rule reads it plainly.
      ...(aggregateGroupIndex > 0 ? { AggregateGroupIndex: aggregateGroupIndex } : {}),
    });
  }
  // The calculation is also the save-time guard: an out-of-range total is
  // named here rather than after a round trip.
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
      BusinessCategory: state.header.businessCategory,
      // The whole-PO discount and adjustment are retired (#573). The columns
      // stay dormant; every write pins them to zero so no new record carries a
      // value the editor can no longer show or explain.
      AdjustmentReason: "",
      ConfirmationRequested: state.header.confirmationRequested,
      Items: items,
      // #654: always an array. The server reads an absent slice as an old
      // client and refuses to ungroup a grouped record it cannot represent;
      // an empty array is the deliberate ungroup.
      AggregateGroups: aggregatePayload.groups,
      DiscountPercent: 0,
      AdjustmentMinor: 0,
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

/** One named validation blocker with its focus target. The gate names it;
 * activating it focuses the field so its error becomes visible. */
export interface ValidationBlocker {
  key: string;
  label: string;
  message: string;
  /** The input's accessible name, for focusing the field. */
  fieldLabel: string;
}

/**
 * Every invalid or incomplete entry that blocks a save, named per field.
 * Mirrors buildPurchaseOrderUpdate's checks one for one (same messages, same
 * order) so the gate can name blockers the payload builder would only report
 * as the first failure. No new requiredness is introduced: empty fields are
 * only blockers where the payload already refuses them. The saved-record
 * minimum (at least one line, #580) is checked by Final Save itself, so a
 * blank draft does not open with a blocker chip.
 */
export function listValidationBlockers(
  state: PurchaseOrderEditorState,
  currency: CurrencyInfo
): ValidationBlocker[] {
  const blockers: ValidationBlocker[] = [];
  if (!validDateField(state.header.poDate)) {
    blockers.push({
      key: "header:poDate",
      label: "PO date",
      message: "PO date must be YYYY-MM-DD.",
      fieldLabel: "PO date",
    });
  }
  if (state.header.receivedDate.trim() === "" || !validDateField(state.header.receivedDate)) {
    blockers.push({
      key: "header:receivedDate",
      label: "Received date",
      message: "Received date must be YYYY-MM-DD.",
      fieldLabel: "Received date",
    });
  }
  if (!validDateField(state.header.requestedDeliveryDate)) {
    blockers.push({
      key: "header:requestedDeliveryDate",
      label: "Requested delivery date",
      message: "Requested delivery date must be YYYY-MM-DD.",
      fieldLabel: "Requested delivery date",
    });
  }
  const paymentTerms = state.header.paymentTerms.trim();
  if (characterCount(paymentTerms) > MAX_PAYMENT_TERMS_LENGTH) {
    blockers.push({
      key: "header:paymentTerms",
      label: "Payment terms",
      message: `Payment terms must be ${MAX_PAYMENT_TERMS_LENGTH} characters or fewer.`,
      fieldLabel: "Payment terms",
    });
  }
  // #654: the aggregate layout is checked before the lines, exactly as the
  // payload builder orders it. It is one blocker because the layout validator
  // names the first problem by its group and line; a second aggregate blocker
  // would only repeat it.
  const aggregateError = validateAggregateLayout(state, currency);
  if (aggregateError !== null) {
    blockers.push({
      key: "aggregate:layout",
      label: "Aggregate lines",
      message: aggregateError,
      fieldLabel: "Aggregate lines",
    });
  }
  // #628: the category is a Final Save requirement, not a draft-save field
  // rule. It is deliberately absent from this list so Save draft is never
  // blocked by it; the editor's Final Save gate and the select's required hint
  // enforce it, and buildFinalizeInput repeats the server's refusal.
  state.lines.forEach((line, index) => {
    const display = `Line ${index + 1}`;
    if (parseQuantityScaled(line.quantity) === null) {
      blockers.push({
        key: `line:${index + 1}:quantity`,
        label: `${display} quantity`,
        message: `Line ${index + 1}: quantity must be positive with up to 4 decimal places and within the supported range.`,
        fieldLabel: `Quantity for line ${index + 1}`,
      });
    }
    if (parseUnitPriceMinor(currency, line.unitPrice) === null) {
      blockers.push({
        key: `line:${index + 1}:unitPrice`,
        label: `${display} unit price`,
        message: `Line ${index + 1}: unit price must be a nonnegative ${currency.code} amount within the supported range.`,
        fieldLabel: `Unit price for line ${index + 1}`,
      });
    }
    if (parseDiscountPercent(line.discountPercent) === null) {
      blockers.push({
        key: `line:${index + 1}:discount`,
        label: `${display} discount`,
        message: `Line ${index + 1}: discount must be between 0 and ${MAX_DISCOUNT_PERCENT}.`,
        fieldLabel: `Discount percentage for line ${index + 1}`,
      });
    }
    if (characterCount(line.parentDescription) > MAX_PARENT_DESCRIPTION_LENGTH) {
      blockers.push({
        key: `line:${index + 1}:parentDescription`,
        label: `${display} split-line title`,
        message: `Line ${index + 1}: the split-line title must be ${MAX_PARENT_DESCRIPTION_LENGTH} characters or fewer.`,
        fieldLabel: `Description for line ${index + 1}`,
      });
    }
    if (line.descriptionLines.length > MAX_DESCRIPTION_LINES) {
      blockers.push({
        key: `line:${index + 1}:descriptionLines`,
        label: `${display} description lines`,
        message: `Line ${index + 1}: at most ${MAX_DESCRIPTION_LINES} description lines are allowed.`,
        fieldLabel: `Description for line ${index + 1}`,
      });
    }
    for (const descriptionLine of line.descriptionLines) {
      if (characterCount(descriptionLine) > MAX_DESCRIPTION_LINE_LENGTH) {
        blockers.push({
          key: `line:${index + 1}:descriptionLine`,
          label: `${display} description line`,
          message: `Line ${index + 1}: each description line must be ${MAX_DESCRIPTION_LINE_LENGTH} characters or fewer.`,
          fieldLabel: `Description for line ${index + 1}`,
        });
        break;
      }
    }
  });
  // Mirrors the payload builder's final guard: range refusals that no single
  // line input names. Skipped when an earlier blocker already names the same
  // failure so the gate never double-counts one invalid input.
  const calculation = calculatePurchaseOrder(state, currency);
  if (calculation.error !== null && !blockers.some((entry) => entry.message === calculation.error)) {
    blockers.push({
      key: "calculation:error",
      label: "Totals",
      message: calculation.error,
      fieldLabel: "Totals",
    });
  }
  return blockers;
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
            clientKey: newPurchaseOrderLineKey(),
            persistedId: undefined,
            persistedClientKey: null,
            description: "",
            quantity: "1",
            uom: action.uom,
            unitPrice: "0",
            discountEnabled: false,
            discountPercent: "0",
            quotationItemNo: null,
            quotationSubNo: null,
            groupKey: "",
            parentDescription: "",
            descriptionLines: [],
            aggregateKey: null,
          },
        ],
        nextKey: state.nextKey + 1,
      };
    case "lineAddedFromSource":
      return {
        ...state,
        lines: [
          ...state.lines,
          {
            key: state.nextKey,
            clientKey: newPurchaseOrderLineKey(),
            persistedId: undefined,
            persistedClientKey: null,
            description: "",
            quantity: "1",
            uom: action.uom,
            unitPrice: "0",
            discountEnabled: false,
            discountPercent: "0",
            quotationItemNo: action.quotationItemNo,
            quotationSubNo: action.quotationSubNo,
            groupKey: "",
            parentDescription: "",
            descriptionLines: [],
            aggregateKey: null,
          },
        ],
        nextKey: state.nextKey + 1,
      };
    case "lineRemoved": {
      // #654: removing the last child leaves no header behind; a group is
      // never allowed to become an empty fake row on the customer document.
      return pruneEmptyAggregates({
        ...state,
        lines: state.lines.filter((line) => line.key !== action.key),
      });
    }
    case "linesReplaced": {
      const highest = action.lines.reduce((max, line) => Math.max(max, line.key + 1), 0);
      return {
        ...state,
        lines: action.lines,
        nextKey: Math.max(state.nextKey, highest),
      };
    }
    case "aggregateAdded": {
      // Upsert by key: a repeated dispatch replaces the header rather than
      // creating a duplicate that could never be placed on a save.
      const exists = state.aggregateGroups.some(
        (group) => group.key === action.group.key
      );
      const lineKeys = new Set(action.lineKeys);
      return {
        ...state,
        aggregateGroups: exists
          ? state.aggregateGroups.map((group) =>
              group.key === action.group.key ? action.group : group
            )
          : [...state.aggregateGroups, action.group],
        lines: state.lines.map((line) =>
          lineKeys.has(line.key)
            ? { ...line, aggregateKey: action.group.key }
            : line
        ),
        nextAggregateKey: Math.max(state.nextAggregateKey, action.group.key + 1),
      };
    }
    case "aggregateChanged":
      return {
        ...state,
        aggregateGroups: state.aggregateGroups.map((group) =>
          group.key === action.key ? { ...group, ...action.patch } : group
        ),
      };
    case "aggregateUngrouped":
      return {
        ...state,
        aggregateGroups: state.aggregateGroups.filter(
          (group) => group.key !== action.key
        ),
        lines: ungroupAggregate(state.lines, action.key),
      };
    case "aggregatePruned":
      return pruneEmptyAggregates(state);
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
      return { ...state, currency: action.code, lines };
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
    case "saveFailed":
      return { ...state, saving: false, error: action.error };
    case "saveSucceeded": {
      const saved = initialPurchaseOrderEditorState(action.order, action.currency);
      // #517: the save round-trip rebuilds lines from the returned DTO, which
      // now carries IDs and row aliases. The editor alias is the session key
      // while the session first sees the line, so it is carried over by
      // alias first (reused verbatim on reopen) and by persisted id second.
      // Position is never consulted: a reorder swap must not re-key a line.
      const byAlias = new Map(
        state.lines.map((line) => [line.clientKey, line] as const)
      );
      const byID = new Map(
        state.lines
          .filter((line) => line.persistedId !== undefined)
          .map((line) => [line.persistedId as number, line] as const)
      );
      const lines = saved.lines.map((line) => {
        const previous =
          byAlias.get(line.clientKey) ??
          (line.persistedId !== undefined ? byID.get(line.persistedId) : undefined);
        if (previous === undefined) return line;
        return {
          ...line,
          clientKey: previous.clientKey,
          persistedClientKey: line.persistedClientKey,
        };
      });
      return { ...saved, lines, nextKey: Math.max(saved.nextKey, state.nextKey), nextAggregateKey: Math.max(saved.nextAggregateKey, state.nextAggregateKey) };
    }
    default:
      return state;
  }
}
