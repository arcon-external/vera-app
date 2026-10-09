import type { DetailsState } from "./components/DetailsPanel";
import type { EditorItem, EditorSubItem } from "./components/ItemEditor";
import type { backend } from "@backend/models";

/**
 * A saved, unexported quotation.
 *
 * The payload is the editor's own state, stored verbatim. Nothing here prices,
 * validates, reserves a quote number or signs anything -- that is the whole
 * point of a draft, and Prepare still does all of it properly on export. So
 * reading one back must never throw on unexpected content: a draft written by
 * an older build, or half-written by hand, still has to open into something
 * the user can keep typing into.
 */
export interface DraftPayload {
  items: EditorItem[];
  details: DetailsState;
  customerId: number;
}

/** A draft as the backend returns it. Payload is a JSON string, not an object. */
export type LoadedDraft = backend.DraftDTO;

/** One row of the drafts list. */
export type DraftSummaryDTO = backend.DraftSummaryDTO;

function asString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function asBoolean(value: unknown): boolean {
  return value === true;
}

function asNumber(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((entry): entry is string => typeof entry === "string");
}

/**
 * Restores one sub-item, tolerating a shape the current build does not
 * recognise. A missing field falls back to what newSubItem() would have set,
 * rather than leaving the row unusable.
 */
function restoreSubItem(raw: unknown, id: number): EditorSubItem {
  const sub = (raw ?? {}) as Record<string, unknown>;
  return {
    id,
    description: asString(sub.description),
    qty: asString(sub.qty, "1"),
    uom: asString(sub.uom, "Lot"),
    unitCost: asString(sub.unitCost),
    marginEnabled: asBoolean(sub.marginEnabled),
    marginPercent: asString(sub.marginPercent, "0"),
    bullets: asStringArray(sub.bullets),
  };
}

function restoreItem(raw: unknown, key: number): EditorItem {
  const item = (raw ?? {}) as Record<string, unknown>;
  const subs = Array.isArray(item.subItems) ? item.subItems : [];
  return {
    key,
    description: asString(item.description),
    bullets: asStringArray(item.bullets),
    qty: asString(item.qty, "1"),
    uom: asString(item.uom, "Lot"),
    unitPrice: asString(item.unitPrice),
    marginEnabled: asBoolean(item.marginEnabled),
    marginPercent: asString(item.marginPercent, "0"),
    subItems: subs.map((sub, index) => restoreSubItem(sub, index + 1)),
    showSubPrices: asBoolean(item.showSubPrices),
  };
}

/**
 * Parses a draft payload back into editor state.
 *
 * Returns null when the payload is not an object at all, so the caller can
 * tell the user rather than silently blanking the editor. Anything that *is*
 * an object is restored field by field: half a draft that opens is far more
 * useful than a draft that refuses to open.
 */
export function parseDraftPayload(payload: string): DraftPayload | null {
  let decoded: unknown;
  try {
    decoded = JSON.parse(payload);
  } catch {
    return null;
  }
  if (typeof decoded !== "object" || decoded === null || Array.isArray(decoded)) {
    return null;
  }
  const draft = decoded as Record<string, unknown>;
  const rawItems = Array.isArray(draft.items) ? draft.items : [];
  const rawDetails = (draft.details ?? {}) as Record<string, unknown>;
  const details = rawDetails as unknown as DetailsState;
  return {
    // Keys and sub-item ids are reissued on load. They are React identity and
    // row handles, never data, and reusing stale ones is how a reopened draft
    // ends up with two rows editing each other.
    items: rawItems.map((item, index) => restoreItem(item, index)),
    details: {
      ...details,
      subject: asString(details.subject),
      recipientName: asString(details.recipientName),
      validity: asNumber(details.validity, 30),
      paymentTerm: asNumber(details.paymentTerm, 90),
      leadTime: asNumber(details.leadTime, 60),
      discountPercent: asNumber(details.discountPercent, 0),
      incoterm: asString(details.incoterm, "EXW"),
      taxZone: asString(details.taxZone, "FTZ"),
      taxType: asString(details.taxType, "Goods"),
      serviceCategory: asString(details.serviceCategory),
    },
    customerId: asNumber(draft.customerId, 0),
  };
}

/** How many rows of a draft hold something worth listing. */
export function countDraftItems(items: EditorItem[]): number {
  return items.filter(
    (item) =>
      item.description.trim() !== "" ||
      item.bullets.some((line) => line.trim() !== "") ||
      item.subItems.some((sub) => sub.description.trim() !== "")
  ).length;
}

/** "3 minutes ago" style stamp for the drafts list. */
export function relativeSavedAt(iso: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? "" : "s"} ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} hour${hours === 1 ? "" : "s"} ago`;
  const days = Math.round(hours / 24);
  if (days < 31) return `${days} day${days === 1 ? "" : "s"} ago`;
  return new Date(then).toLocaleDateString("id-ID", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}