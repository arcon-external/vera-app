import type { ArchivedQuotation } from "./archive";
import { legacyServiceCategory, legacyTaxType } from "./tax";
import type { DetailsState } from "./components/DetailsPanel";
import type { EditorItem, EditorSubItem } from "./components/ItemEditor";
import type { ToastType } from "./toast";

/**
 * Copy/paste for archived quotations (#485).
 *
 * The archived detail page copies the viewed revision's complete entry values
 * as versioned JSON text; pasting that text anywhere on the quotation editor
 * page refills the whole editor as a new, unsaved quotation. The archived
 * quote number and revision travel in `source` as information only and are
 * never applied -- the next export allocates a new number. Amounts travel as
 * raw minor-unit numbers (cost, margin, discount), never display strings, so
 * the editor recalculates exactly.
 *
 * Ordinary text pastes are never intercepted: only a payload with this kind
 * (and a supported version) fills the form. Everything else falls through to
 * the existing cell/description paste handlers.
 */

export const QUOTATION_COPY_KIND = "vera.quotation-copy";
export const QUOTATION_COPY_VERSION = 1;

export interface QuotationCopySource {
  quoteNumber: string;
  revision: number;
}

export interface QuotationCopySubItem {
  description: string;
  qty: number;
  uom: string;
  unitCost: number;
  marginEnabled: boolean;
  marginPercent: number;
  discountEnabled: boolean;
  discountPercent: number;
  /** Detail lines under the sub-item: plain text with no values of their own. */
  details: string[];
}

export interface QuotationCopyItem {
  /** The raw stored description, "- " bullet lines included (see splitRawDescription). */
  description: string;
  qty: number;
  uom: string;
  unitCost: number;
  marginEnabled: boolean;
  marginPercent: number;
  discountEnabled: boolean;
  discountPercent: number;
  showSubPrices: boolean;
  subItems: QuotationCopySubItem[];
}

export interface QuotationCopyQuotation {
  customerCode: number;
  recipientName: string;
  /** Informational: the editor has no address inputs, Prepare resolves both
   * from the client directory on the next export. Never applied. */
  recipientAddress: string;
  subject: string;
  incoterm: string;
  validityDays: number;
  paymentTermDays: number;
  leadTimeYears: number;
  leadTimeMonths: number;
  leadTimeWeeks: number;
  leadTimeDays: number;
  discountPercent: number;
  taxZone: string;
  taxType: string;
  serviceCategory: string;
  taxCategory: string;
  items: QuotationCopyItem[];
}

export interface QuotationCopyPayload {
  kind: typeof QUOTATION_COPY_KIND;
  version: typeof QUOTATION_COPY_VERSION;
  source: QuotationCopySource;
  quotation: QuotationCopyQuotation;
}

/**
 * Splits the raw stored description back into the editor's description +
 * bullets. The same rule App.tsx saves with: lines starting "- " (or "* ")
 * are bullets, every other non-blank line joins the main description.
 */
export function splitRawDescription(raw: string): { description: string; bullets: string[] } {
  const lines = raw.split("\n").filter((line) => line.trim() !== "");
  const mainParts: string[] = [];
  const bullets: string[] = [];
  for (const line of lines) {
    const trimmed = line.trimStart();
    if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
      bullets.push(trimmed.slice(2).trim());
    } else {
      mainParts.push(trimmed);
    }
  }
  return { description: mainParts.join(" "), bullets };
}

/** Builds the copy text for the currently viewed revision. */
export function buildQuotationCopy(quotation: ArchivedQuotation): string {
  const payload: QuotationCopyPayload = {
    kind: QUOTATION_COPY_KIND,
    version: QUOTATION_COPY_VERSION,
    source: { quoteNumber: quotation.QuoteNumber, revision: quotation.RevisionNumber },
    quotation: {
      customerCode: quotation.CustomerCode,
      recipientName: quotation.RecipientName,
      recipientAddress: quotation.RecipientAddress,
      subject: quotation.Subject,
      incoterm: quotation.Incoterm,
      validityDays: quotation.ValidityDays,
      paymentTermDays: quotation.PaymentTermDays,
      leadTimeYears: quotation.LeadTimeYears,
      leadTimeMonths: quotation.LeadTimeMonths,
      leadTimeWeeks: quotation.LeadTimeWeeks,
      leadTimeDays: quotation.LeadTimeDays,
      discountPercent: quotation.DiscountPercent,
      taxZone: quotation.TaxZone,
      taxType: quotation.TaxType,
      serviceCategory: quotation.ServiceCategory,
      taxCategory: quotation.TaxCategory,
      items: quotation.Items.map((item) => ({
        description: item.Description,
        qty: item.Qty,
        uom: item.UoM,
        // The raw cost, not the marginalised figure: the editor re-applies
        // margin itself, so copying the marginalised price would margin twice.
        unitCost: item.UnitPrice,
        marginEnabled: item.MarginEnabled,
        marginPercent: item.MarginPercent,
        discountEnabled: item.DiscountEnabled ?? false,
        discountPercent: item.DiscountPercent ?? 0,
        showSubPrices: item.ShowSubPrices ?? false,
        subItems: (item.Subs ?? []).map((sub) => ({
          description: sub.Description,
          qty: sub.Qty,
          uom: sub.UoM,
          unitCost: sub.OriginalUnitPrice,
          marginEnabled: sub.MarginEnabled,
          marginPercent: sub.MarginPercent,
          discountEnabled: sub.DiscountEnabled ?? false,
          discountPercent: sub.DiscountPercent ?? 0,
          details: [...(sub.Details ?? [])],
        })),
      })),
    },
  };
  return JSON.stringify(payload);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isBoolean(value: unknown): value is boolean {
  return typeof value === "boolean";
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function stringArray(value: unknown): string[] | null {
  if (!Array.isArray(value)) return null;
  if (!value.every(isString)) return null;
  return [...value];
}

function parseSubItem(raw: unknown): QuotationCopySubItem | null {
  if (!isRecord(raw)) return null;
  if (
    !isString(raw.description) ||
    !isFiniteNumber(raw.qty) ||
    !isString(raw.uom) ||
    !isFiniteNumber(raw.unitCost) ||
    !isBoolean(raw.marginEnabled) ||
    !isFiniteNumber(raw.marginPercent) ||
    !isBoolean(raw.discountEnabled) ||
    !isFiniteNumber(raw.discountPercent)
  ) {
    return null;
  }
  const details = stringArray(raw.details);
  if (details === null) return null;
  return {
    description: raw.description,
    qty: Math.trunc(raw.qty),
    uom: raw.uom,
    unitCost: Math.trunc(raw.unitCost),
    marginEnabled: raw.marginEnabled,
    marginPercent: Math.trunc(raw.marginPercent),
    discountEnabled: raw.discountEnabled,
    discountPercent: Math.trunc(raw.discountPercent),
    details,
  };
}

function parseItem(raw: unknown): QuotationCopyItem | null {
  if (!isRecord(raw)) return null;
  if (
    !isString(raw.description) ||
    !isFiniteNumber(raw.qty) ||
    !isString(raw.uom) ||
    !isFiniteNumber(raw.unitCost) ||
    !isBoolean(raw.marginEnabled) ||
    !isFiniteNumber(raw.marginPercent) ||
    !isBoolean(raw.discountEnabled) ||
    !isFiniteNumber(raw.discountPercent) ||
    !isBoolean(raw.showSubPrices) ||
    !Array.isArray(raw.subItems)
  ) {
    return null;
  }
  const subItems: QuotationCopySubItem[] = [];
  for (const sub of raw.subItems) {
    const parsed = parseSubItem(sub);
    if (parsed === null) return null;
    subItems.push(parsed);
  }
  return {
    description: raw.description,
    qty: Math.trunc(raw.qty),
    uom: raw.uom,
    unitCost: Math.trunc(raw.unitCost),
    marginEnabled: raw.marginEnabled,
    marginPercent: Math.trunc(raw.marginPercent),
    discountEnabled: raw.discountEnabled,
    discountPercent: Math.trunc(raw.discountPercent),
    showSubPrices: raw.showSubPrices,
    subItems,
  };
}

/**
 * Parses clipboard text into a copy payload, or null when the text is not one.
 * Ordinary text -- prose, Excel rows, description fragments -- returns null so
 * the existing paste handlers keep it. Only the marked kind with a supported
 * version and a structurally valid quotation is accepted.
 */
export function parseQuotationCopy(text: string): QuotationCopyPayload | null {
  // Fast path before the JSON parse: ordinary pastes never contain the marker.
  if (!text.includes(QUOTATION_COPY_KIND)) return null;
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return null;
  }
  if (!isRecord(raw)) return null;
  if (raw.kind !== QUOTATION_COPY_KIND) return null;
  if (raw.version !== QUOTATION_COPY_VERSION) return null;
  if (!isRecord(raw.source) || !isString(raw.source.quoteNumber) || !isFiniteNumber(raw.source.revision)) {
    return null;
  }
  const q = raw.quotation;
  if (!isRecord(q)) return null;
  if (
    !isFiniteNumber(q.customerCode) ||
    !isString(q.recipientName) ||
    !isString(q.recipientAddress) ||
    !isString(q.subject) ||
    !isString(q.incoterm) ||
    !isFiniteNumber(q.validityDays) ||
    !isFiniteNumber(q.paymentTermDays) ||
    !isFiniteNumber(q.leadTimeYears) ||
    !isFiniteNumber(q.leadTimeMonths) ||
    !isFiniteNumber(q.leadTimeWeeks) ||
    !isFiniteNumber(q.leadTimeDays) ||
    !isFiniteNumber(q.discountPercent) ||
    !isString(q.taxZone) ||
    !isString(q.taxType) ||
    !isString(q.serviceCategory) ||
    !isString(q.taxCategory) ||
    !Array.isArray(q.items)
  ) {
    return null;
  }
  const items: QuotationCopyItem[] = [];
  for (const item of q.items) {
    const parsed = parseItem(item);
    if (parsed === null) return null;
    items.push(parsed);
  }
  return {
    kind: QUOTATION_COPY_KIND,
    version: QUOTATION_COPY_VERSION,
    source: { quoteNumber: raw.source.quoteNumber, revision: Math.trunc(raw.source.revision) },
    quotation: {
      customerCode: Math.trunc(q.customerCode),
      recipientName: q.recipientName,
      recipientAddress: q.recipientAddress,
      subject: q.subject,
      incoterm: q.incoterm,
      validityDays: Math.trunc(q.validityDays),
      paymentTermDays: Math.trunc(q.paymentTermDays),
      leadTimeYears: Math.trunc(q.leadTimeYears),
      leadTimeMonths: Math.trunc(q.leadTimeMonths),
      leadTimeWeeks: Math.trunc(q.leadTimeWeeks),
      leadTimeDays: Math.trunc(q.leadTimeDays),
      discountPercent: Math.trunc(q.discountPercent),
      taxZone: q.taxZone,
      taxType: q.taxType,
      serviceCategory: q.serviceCategory,
      taxCategory: q.taxCategory,
      items,
    },
  };
}

/** One editor line without its React identity; the caller assigns key/id. */
export type EditorItemDraft = Omit<EditorItem, "key" | "subItems"> & {
  subItems: Array<Omit<EditorSubItem, "id">>;
};

export interface ArchivedEditorDraft {
  details: DetailsState;
  customerCode: number;
  items: EditorItemDraft[];
}

/**
 * The single archive -> editor mapping. The detail page's Edit action and the
 * copy/paste apply path both read through here, so the two cannot drift: one
 * rule for the description split, one rule for legacy tax, one rule for raw
 * numbers as editor strings.
 */
export function archivedToEditorDraft(quotation: ArchivedQuotation): ArchivedEditorDraft {
  return {
    details: {
      subject: quotation.Subject,
      recipientName: quotation.RecipientName,
      validity: quotation.ValidityDays,
      paymentTerm: quotation.PaymentTermDays,
      leadTimeYears: quotation.LeadTimeYears,
      leadTimeMonths: quotation.LeadTimeMonths,
      leadTimeWeeks: quotation.LeadTimeWeeks,
      leadTimeDays: quotation.LeadTimeDays,
      discountPercent: quotation.DiscountPercent,
      incoterm: quotation.Incoterm,
      taxZone: quotation.TaxZone,
      taxType: quotation.TaxType || legacyTaxType(quotation.TaxCategory),
      serviceCategory: quotation.ServiceCategory || legacyServiceCategory(quotation.TaxCategory),
    },
    customerCode: quotation.CustomerCode,
    items: quotation.Items.map((item) => {
      const parsed = splitRawDescription(item.Description);
      return {
        description: parsed.description,
        bullets: parsed.bullets,
        qty: String(item.Qty),
        uom: item.UoM,
        unitPrice: String(item.UnitPrice),
        marginEnabled: item.MarginEnabled,
        marginPercent: String(item.MarginPercent),
        // An old revision has no per-row discount, which reads as "this row
        // carries none" -- the same legacy rule the editor already used.
        discountEnabled: item.DiscountEnabled ?? false,
        discountPercent: String(item.DiscountPercent ?? 0),
        showSubPrices: item.ShowSubPrices ?? false,
        subItems: (item.Subs ?? []).map((sub) => ({
          description: sub.Description,
          qty: String(sub.Qty),
          uom: sub.UoM,
          unitCost: String(sub.OriginalUnitPrice),
          marginEnabled: sub.MarginEnabled,
          marginPercent: String(sub.MarginPercent),
          discountEnabled: sub.DiscountEnabled ?? false,
          discountPercent: String(sub.DiscountPercent ?? 0),
          bullets: [...(sub.Details ?? [])],
        })),
      };
    }),
  };
}

export interface QuotationCopyApplied {
  ok: true;
  details: DetailsState;
  customerCode: number;
  /** Lines with React keys assigned (0..n-1). */
  items: EditorItem[];
  /** One past the highest assigned sub-item id, for the editor's counters. */
  nextSubId: number;
}

export interface QuotationCopyRefused {
  ok: false;
  reason: "unknown-customer";
  customerCode: number;
}

/**
 * Turns a parsed payload into editor state. Refuses -- with nothing applied --
 * when the payload's customer code is not in the served client list, so a
 * paste can never land on the wrong customer. The result carries no quote
 * number, revision, draft id or PDF reference: applying always starts a new,
 * unsaved quotation.
 */
export function applyQuotationCopy(
  payload: QuotationCopyPayload,
  customerOptions: ReadonlyArray<{ value: number; label: string }>
): QuotationCopyApplied | QuotationCopyRefused {
  const code = payload.quotation.customerCode;
  if (!customerOptions.some((option) => option.value === code)) {
    return { ok: false, reason: "unknown-customer", customerCode: code };
  }
  const q = payload.quotation;
  let subId = 1;
  const items: EditorItem[] = q.items.map((item, index) => {
    const parsed = splitRawDescription(item.description);
    return {
      key: index,
      description: parsed.description,
      bullets: parsed.bullets,
      qty: String(item.qty),
      uom: item.uom,
      unitPrice: String(item.unitCost),
      marginEnabled: item.marginEnabled,
      marginPercent: String(item.marginPercent),
      discountEnabled: item.discountEnabled,
      discountPercent: String(item.discountPercent),
      showSubPrices: item.showSubPrices,
      subItems: item.subItems.map((sub) => ({
        id: subId++,
        description: sub.description,
        qty: String(sub.qty),
        uom: sub.uom,
        unitCost: String(sub.unitCost),
        marginEnabled: sub.marginEnabled,
        marginPercent: String(sub.marginPercent),
        discountEnabled: sub.discountEnabled,
        discountPercent: String(sub.discountPercent),
        bullets: [...sub.details],
      })),
    };
  });
  return {
    ok: true,
    details: {
      subject: q.subject,
      recipientName: q.recipientName,
      validity: q.validityDays,
      paymentTerm: q.paymentTermDays,
      leadTimeYears: q.leadTimeYears,
      leadTimeMonths: q.leadTimeMonths,
      leadTimeWeeks: q.leadTimeWeeks,
      leadTimeDays: q.leadTimeDays,
      discountPercent: q.discountPercent,
      incoterm: q.incoterm,
      taxZone: q.taxZone,
      taxType: q.taxType || legacyTaxType(q.taxCategory),
      serviceCategory: q.serviceCategory || legacyServiceCategory(q.taxCategory),
    },
    customerCode: code,
    items,
    nextSubId: subId,
  };
}

export type CopyPasteDecision =
  | { action: "ignore" }
  | { action: "apply"; payload: QuotationCopyPayload }
  | { action: "confirm"; payload: QuotationCopyPayload }
  | { action: "refuse-unknown-customer"; customerCode: number };

/**
 * The paste-capture decision: marked payloads are applied (blank editor),
 * confirmed (editor has content) or refused (unknown customer); everything
 * else is ignored so ordinary pastes keep their existing behaviour.
 */
export function resolveCopyPasteText(
  text: string | null | undefined,
  opts: { dirty: boolean; customerOptions: ReadonlyArray<{ value: number; label: string }> }
): CopyPasteDecision {
  if (!text) return { action: "ignore" };
  const payload = parseQuotationCopy(text);
  if (payload === null) return { action: "ignore" };
  if (!opts.customerOptions.some((option) => option.value === payload.quotation.customerCode)) {
    return { action: "refuse-unknown-customer", customerCode: payload.quotation.customerCode };
  }
  return opts.dirty ? { action: "confirm", payload } : { action: "apply", payload };
}

/** "03" for revision 3: the label both copy and paste toasts name. */
export function formatCopyRevision(revision: number): string {
  return String(revision).padStart(2, "0");
}

function copySourceLabel(quoteNumber: string, revision: number): string {
  return `quotation ${quoteNumber} revision ${formatCopyRevision(revision)}`;
}

/**
 * Writes the viewed revision's copy text. The clipboard and the toast are
 * injected so the detail page stays thin and this is unit-testable.
 */
export async function copyQuotationToClipboard(
  quotation: ArchivedQuotation,
  clipboard: { writeText(text: string): Promise<void> },
  pushToast: (type: ToastType, message: string) => void
): Promise<void> {
  try {
    await clipboard.writeText(buildQuotationCopy(quotation));
    pushToast(
      "success",
      `Copied ${copySourceLabel(quotation.QuoteNumber, quotation.RevisionNumber)}. ` +
        "Paste it on the quotation editor to refill the form."
    );
  } catch {
    pushToast("error", "We couldn't copy this quotation. Try again.");
  }
}

/** The success toast for a paste, naming the source quotation and revision. */
export function pasteSuccessMessage(payload: QuotationCopyPayload): string {
  return (
    `Pasted ${copySourceLabel(payload.source.quoteNumber, payload.source.revision)}. ` +
    "Review and export when ready."
  );
}

/** The refusal toast when the payload's customer is not in the client list. */
export function unknownCustomerMessage(customerCode: number): string {
  return (
    `The copied quotation is for a customer (code ${customerCode}) that isn't ` +
    "in the client list, so nothing was pasted."
  );
}
