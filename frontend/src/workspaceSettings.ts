// Issue #492: per-user, device-local defaults for the Quotation and Purchase
// Order surfaces.
//
// The settings are a preference, not company data: they live in this device's
// localStorage, are keyed by the signed-in account, and are read only when
// something consumes them. Nothing here is a quotation or PO field on the
// server, so a corrupt record can always fall back to the built-in defaults
// without losing stored work.
//
// The module is deliberately React-free: the two settings pages render from
// these values, and App applies them when a new draft starts. `Storage` is
// injected rather than reached for, so the node suite drives every rule without
// a DOM.

import {
  MAX_PAYMENT_TERMS_LENGTH,
  validDateField,
  type PurchaseOrderDTO,
} from "./purchaseOrder";
import { PO_LIST_MAX_PAGE_SIZE, PO_LIST_MIN_PAGE_SIZE } from "./poList";

/** The record shape version. A mismatch reads as "no settings yet". */
export const WORKSPACE_SETTINGS_VERSION = 1;

/** Quotation validity/payment are uint8 on the server (1..255). */
export const MIN_QUOTATION_DAYS = 1;
export const MAX_QUOTATION_DAYS = 255;

/** The editor's subject field is free text; the preference is bounded. */
export const MAX_SUBJECT_LENGTH = 200;

/**
 * The list-page preference bounds, aliased from the list API's own range so a
 * saved size is always requestable. The archive server is paged, and the
 * response's effective PageSize replaces the preference after each load.
 */
export const MIN_PO_LIST_PAGE_SIZE = PO_LIST_MIN_PAGE_SIZE;
export const MAX_PO_LIST_PAGE_SIZE = PO_LIST_MAX_PAGE_SIZE;

export type SettingsTaxZone = "FTZ" | "NonFTZ";
export type SettingsTaxType = "Goods" | "Services";
export type SettingsServiceCategory = "" | "Construction" | "NonConstruction";

/** Quotation defaults applied when a new quotation starts. */
export interface QuotationSettings {
  validityDays: number;
  paymentTermDays: number;
  taxZone: SettingsTaxZone;
  taxType: SettingsTaxType;
  serviceCategory: SettingsServiceCategory;
  subject: string;
}

/** PO defaults. Values the source revision supplies always win. */
export interface PurchaseOrderSettings {
  paymentTerms: string;
  incoterms: string;
  /** "" means "today in WIB", decided when a draft starts. */
  receivedDate: string;
  listPageSize: number;
}

export interface WorkspaceSettings {
  quotation: QuotationSettings;
  purchaseOrder: PurchaseOrderSettings;
}

/** The behaviour before any preference existed: the editor's own defaults. */
export const DEFAULT_QUOTATION_SETTINGS: QuotationSettings = {
  validityDays: 30,
  paymentTermDays: 90,
  taxZone: "FTZ",
  taxType: "Goods",
  serviceCategory: "",
  subject: "",
};

/**
 * The PO defaults. Payment terms stay empty because the source quotation
 * normally seeds them; EXW is the quotation's own default Incoterm, and a blank
 * received date means today in WIB.
 */
export const DEFAULT_PURCHASE_ORDER_SETTINGS: PurchaseOrderSettings = {
  paymentTerms: "",
  incoterms: "EXW",
  receivedDate: "",
  listPageSize: 15,
};

export const DEFAULT_WORKSPACE_SETTINGS: WorkspaceSettings = {
  quotation: DEFAULT_QUOTATION_SETTINGS,
  purchaseOrder: DEFAULT_PURCHASE_ORDER_SETTINGS,
};

/** The four options the Quotation editor offers, as settings accept them. */
export const SETTINGS_INCOTERMS = [
  "EXW",
  "FCA",
  "FAS",
  "FOB",
  "CFR",
  "CIF",
  "CPT",
  "CIP",
  "DPU",
  "DAP",
  "DDP",
] as const;

/** The slice of `Storage` this module needs. */
export interface SettingsStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

/** One account's Quotation preference. */
export function quotationSettingsKey(userId: number): string {
  return `vera-quotation-settings:${userId}`;
}

/** One account's Purchase Order preference. */
export function purchaseOrderSettingsKey(userId: number): string {
  return `vera-po-settings:${userId}`;
}

interface StoredRecord<T> {
  version: number;
  settings: T;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function codePointLength(value: string): number {
  return Array.from(value).length;
}

function boundedInteger(
  value: unknown,
  min: number,
  max: number,
  fallback: number
): number {
  return typeof value === "number" &&
    Number.isInteger(value) &&
    value >= min &&
    value <= max
    ? value
    : fallback;
}

function boundedString(value: unknown, max: number, fallback = ""): string {
  return typeof value === "string" && codePointLength(value) <= max
    ? value
    : fallback;
}

function sanitizeQuotation(value: unknown): QuotationSettings {
  const raw = isRecord(value) ? value : {};
  const taxType: SettingsTaxType =
    raw.taxType === "Services" ? "Services" : "Goods";
  const taxZone: SettingsTaxZone =
    raw.taxZone === "NonFTZ" ? "NonFTZ" : "FTZ";
  // Goods carries no service category, mirroring the editor's rule, and
  // Services always carries one because the server refuses a service without a
  // category. An unreadable category therefore lands on Non-construction rather
  // than storing a default every new quotation would fail to Prepare.
  const serviceCategory: SettingsServiceCategory =
    taxType === "Services"
      ? raw.serviceCategory === "Construction"
        ? "Construction"
        : "NonConstruction"
      : "";
  return {
    validityDays: boundedInteger(
      raw.validityDays,
      MIN_QUOTATION_DAYS,
      MAX_QUOTATION_DAYS,
      DEFAULT_QUOTATION_SETTINGS.validityDays
    ),
    paymentTermDays: boundedInteger(
      raw.paymentTermDays,
      MIN_QUOTATION_DAYS,
      MAX_QUOTATION_DAYS,
      DEFAULT_QUOTATION_SETTINGS.paymentTermDays
    ),
    taxZone,
    taxType,
    serviceCategory,
    subject: boundedString(
      raw.subject,
      MAX_SUBJECT_LENGTH,
      DEFAULT_QUOTATION_SETTINGS.subject
    ),
  };
}

function sanitizePurchaseOrder(value: unknown): PurchaseOrderSettings {
  const raw = isRecord(value) ? value : {};
  const incoterms =
    typeof raw.incoterms === "string" &&
    (SETTINGS_INCOTERMS as readonly string[]).includes(raw.incoterms)
      ? raw.incoterms
      : DEFAULT_PURCHASE_ORDER_SETTINGS.incoterms;
  const receivedDate =
    typeof raw.receivedDate === "string" && validDateField(raw.receivedDate)
      ? raw.receivedDate.trim()
      : DEFAULT_PURCHASE_ORDER_SETTINGS.receivedDate;
  return {
    paymentTerms: boundedString(
      raw.paymentTerms,
      MAX_PAYMENT_TERMS_LENGTH,
      DEFAULT_PURCHASE_ORDER_SETTINGS.paymentTerms
    ),
    incoterms,
    receivedDate,
    listPageSize: boundedInteger(
      raw.listPageSize,
      MIN_PO_LIST_PAGE_SIZE,
      MAX_PO_LIST_PAGE_SIZE,
      DEFAULT_PURCHASE_ORDER_SETTINGS.listPageSize
    ),
  };
}

/** One section's value, or the built-in default for anything unreadable. */
function readSection<T>(
  storage: SettingsStorage,
  key: string,
  sanitize: (value: unknown) => T,
  fallback: T
): T {
  let text: string | null;
  try {
    text = storage.getItem(key);
  } catch {
    return fallback;
  }
  if (text === null) return fallback;
  let decoded: unknown;
  try {
    decoded = JSON.parse(text);
  } catch {
    return fallback;
  }
  if (!isRecord(decoded) || decoded.version !== WORKSPACE_SETTINGS_VERSION) {
    return fallback;
  }
  return sanitize(decoded.settings);
}

function storageForUser(
  storage: SettingsStorage | null,
  userId: number | null
): { storage: SettingsStorage; userId: number } | null {
  if (storage === null || userId === null) return null;
  if (!Number.isInteger(userId) || userId < 1) return null;
  return { storage, userId };
}

/**
 * The effective settings for one account. Signed out (or with no storage)
 * answers the built-in defaults without reading anything; keep saved values
 * across sign-out is the caller's business, and the records stay in place.
 */
export function readWorkspaceSettings(
  storage: SettingsStorage | null,
  userId: number | null
): WorkspaceSettings {
  const target = storageForUser(storage, userId);
  if (target === null) return DEFAULT_WORKSPACE_SETTINGS;
  return {
    quotation: readSection(
      target.storage,
      quotationSettingsKey(target.userId),
      sanitizeQuotation,
      DEFAULT_QUOTATION_SETTINGS
    ),
    purchaseOrder: readSection(
      target.storage,
      purchaseOrderSettingsKey(target.userId),
      sanitizePurchaseOrder,
      DEFAULT_PURCHASE_ORDER_SETTINGS
    ),
  };
}

function writeSection<T>(
  storage: SettingsStorage | null,
  userId: number | null,
  key: (userId: number) => string,
  sanitize: (value: unknown) => T,
  settings: T
): void {
  const target = storageForUser(storage, userId);
  if (target === null) return;
  const record: StoredRecord<T> = {
    version: WORKSPACE_SETTINGS_VERSION,
    // Sanitize on the way in as well, so an out-of-range value never reaches
    // the device even if a caller skipped the page's own guard.
    settings: sanitize(settings),
  };
  try {
    target.storage.setItem(key(target.userId), JSON.stringify(record));
  } catch {
    // A full or unavailable store must not break the page. The next read
    // answers the previous value, so the UI stays truthful.
  }
}

export function writeQuotationSettings(
  storage: SettingsStorage | null,
  userId: number | null,
  settings: QuotationSettings
): void {
  writeSection(storage, userId, quotationSettingsKey, sanitizeQuotation, settings);
}

export function writePurchaseOrderSettings(
  storage: SettingsStorage | null,
  userId: number | null,
  settings: PurchaseOrderSettings
): void {
  writeSection(
    storage,
    userId,
    purchaseOrderSettingsKey,
    sanitizePurchaseOrder,
    settings
  );
}

function clearSection(
  storage: SettingsStorage | null,
  userId: number | null,
  key: (userId: number) => string
): void {
  const target = storageForUser(storage, userId);
  if (target === null) return;
  try {
    target.storage.removeItem(key(target.userId));
  } catch {
    // Nothing to recover from: the overlay still reads back if the remove
    // failed, which is the safe direction.
  }
}

/** Removes one account's Quotation overlay; the page then shows the defaults. */
export function clearQuotationSettings(
  storage: SettingsStorage | null,
  userId: number | null
): void {
  clearSection(storage, userId, quotationSettingsKey);
}

/** Removes one account's PO overlay; the page then shows the defaults. */
export function clearPurchaseOrderSettings(
  storage: SettingsStorage | null,
  userId: number | null
): void {
  clearSection(storage, userId, purchaseOrderSettingsKey);
}

/** The Quotation editor's field names, with the default service category rule. */
export interface QuotationDefaults {
  subject: string;
  validity: number;
  paymentTerm: number;
  taxZone: string;
  taxType: string;
  serviceCategory: string;
}

/** Applies the saved Quotation defaults to a brand-new draft. */
export function quotationDefaultsFromSettings(
  settings: QuotationSettings
): QuotationDefaults {
  return {
    subject: settings.subject,
    validity: settings.validityDays,
    paymentTerm: settings.paymentTermDays,
    taxZone: settings.taxZone,
    taxType: settings.taxType,
    serviceCategory:
      settings.taxType === "Services" ? settings.serviceCategory : "",
  };
}

/** The calendar date in WIB (UTC+7, no DST) as YYYY-MM-DD. */
export function todayWIB(now: Date = new Date()): string {
  const wib = new Date(now.getTime() + 7 * 60 * 60 * 1000);
  const year = wib.getUTCFullYear();
  const month = String(wib.getUTCMonth() + 1).padStart(2, "0");
  const day = String(wib.getUTCDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/**
 * Fills a newly created PO draft from the saved defaults, without ever
 * replacing a value the quotation revision supplied.
 *
 * The source revision carries the customer, currency, tax basis, payment terms,
 * Incoterms and lines; only a blank payment term/Incoterm (a quotation issued
 * with neither) is filled, and the received date -- which is a PO fact, not a
 * quotation one -- falls back to today in WIB when the preference is blank.
 */
export function applyPurchaseOrderDefaults(
  order: PurchaseOrderDTO,
  settings: PurchaseOrderSettings,
  now: Date = new Date()
): PurchaseOrderDTO {
  return {
    ...order,
    PaymentTerms:
      order.PaymentTerms.trim() !== ""
        ? order.PaymentTerms
        : settings.paymentTerms,
    Incoterms:
      order.Incoterms.trim() !== "" ? order.Incoterms : settings.incoterms,
    ReceivedDate:
      order.ReceivedDate.trim() !== ""
        ? order.ReceivedDate
        : settings.receivedDate !== ""
          ? settings.receivedDate
          : todayWIB(now),
  };
}
