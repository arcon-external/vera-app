import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
import type { POSourceAvailability } from "./poSelector";
import {
  formatQuantityScaled,
  formatUnitPriceMinor,
  parseQuantityScaled,
  parseUnitPriceMinor,
  type PurchaseOrderLine,
} from "./purchaseOrder";
import type { CurrencyInfo } from "./format";
import { newClientKey } from "./poReconciliation.lines";

export interface SourceSubItem {
  key: string;
  label: string;
  subNo: number;
  description: string;
  uom: string;
  quantityScaled: number;
  unitPriceMinor: number;
  /** The row-discounted quotation baseline for this leaf (#653). */
  quotedUnitPriceMinor: number;
  /** The quantity still orderable, or null when no availability read exists. */
  remainingQuantityScaled: number | null;
  discountEnabled: boolean;
  discountPercent: number;
  descriptionLines: string[];
}

export interface SourceItem {
  itemNo: number;
  label: string;
  description: string;
  uom: string;
  quantityScaled: number;
  unitPriceMinor: number;
  /** The row-discounted quotation baseline for this leaf (#653). */
  quotedUnitPriceMinor: number;
  /** The quantity still orderable, or null when no availability read exists. */
  remainingQuantityScaled: number | null;
  discountEnabled: boolean;
  discountPercent: number;
  descriptionLines: string[];
  subItems: SourceSubItem[];
}

function subTotal(sub: SourceSubItem): number {
  const gross = Math.round((sub.unitPriceMinor * sub.quantityScaled) / 10_000);
  if (!sub.discountEnabled) return gross;
  return Math.round((gross * (100 - sub.discountPercent)) / 100);
}

/**
 * Groups the frozen source lines into one item per quotation item number.
 * Priced parts stay inside the item; a plain line is an item of one.
 */
export function groupSourceItems(
  lines: PurchaseOrderSourceLineDTO[],
  availability?: POSourceAvailability | null
): SourceItem[] {
  const availabilityByLeaf = new Map<
    string,
    { remaining: number; quoted: number }
  >();
  for (const item of availability?.items ?? []) {
    availabilityByLeaf.set(
      `${item.quotationItemNo}.${item.quotationSubNo ?? ""}`,
      {
        remaining: item.remainingQuantityScaled,
        quoted: item.quotedUnitPriceMinor,
      }
    );
  }
  const leaf = (itemNo: number, subNo: number | null) =>
    availabilityByLeaf.get(`${itemNo}.${subNo ?? ""}`);
  const byItem = new Map<number, SourceItem>();
  const order: number[] = [];
  for (const line of lines) {
    const server = leaf(line.QuotationItemNo, line.QuotationSubNo ?? null);
    const existing = byItem.get(line.QuotationItemNo);
    if (line.QuotationSubNo === null || line.QuotationSubNo === undefined) {
      if (existing === undefined) order.push(line.QuotationItemNo);
      byItem.set(line.QuotationItemNo, {
        itemNo: line.QuotationItemNo,
        label: String(line.QuotationItemNo),
        description: line.Description,
        uom: line.UoM,
        quantityScaled: line.QuantityScaled,
        unitPriceMinor: line.UnitPriceMinor,
        quotedUnitPriceMinor:
          server?.quoted ?? line.QuotedUnitPriceMinor ?? line.UnitPriceMinor,
        remainingQuantityScaled: server?.remaining ?? line.QuantityScaled,
        discountEnabled: line.DiscountEnabled,
        discountPercent: line.DiscountPercent,
        descriptionLines: [...(line.DescriptionLines ?? [])],
        subItems: [],
      });
      continue;
    }
    let item = existing;
    if (item === undefined) {
      item = {
        itemNo: line.QuotationItemNo,
        label: String(line.QuotationItemNo),
        description: line.ParentDescription ?? `Quotation item ${line.QuotationItemNo}`,
        uom: line.UoM,
        quantityScaled: 10_000,
        unitPriceMinor: 0,
        quotedUnitPriceMinor: 0,
        remainingQuantityScaled: null,
        discountEnabled: false,
        discountPercent: 0,
        descriptionLines: [],
        subItems: [],
      };
      byItem.set(line.QuotationItemNo, item);
      order.push(line.QuotationItemNo);
    }
    item.subItems.push({
      key: `${line.QuotationItemNo}.${line.QuotationSubNo}`,
      label: `${line.QuotationItemNo}.${line.QuotationSubNo}`,
      subNo: line.QuotationSubNo,
      description: line.Description,
      uom: line.UoM,
      quantityScaled: line.QuantityScaled,
      unitPriceMinor: line.UnitPriceMinor,
      quotedUnitPriceMinor:
        server?.quoted ?? line.QuotedUnitPriceMinor ?? line.UnitPriceMinor,
      remainingQuantityScaled: server?.remaining ?? line.QuantityScaled,
      discountEnabled: line.DiscountEnabled,
      discountPercent: line.DiscountPercent,
      descriptionLines: [...(line.DescriptionLines ?? [])],
    });
  }
  for (const item of byItem.values()) {
    if (item.subItems.length > 0) {
      item.unitPriceMinor = item.subItems.reduce((sum, sub) => sum + subTotal(sub), 0);
    }
  }
  return order.map((itemNo) => byItem.get(itemNo)!);
}

function lineMatchesSub(
  line: PurchaseOrderLine,
  sub: SourceSubItem,
  currency: CurrencyInfo
): boolean {
  if (line.quotationItemNo !== undefined && line.quotationItemNo !== null) {
    if (line.quotationItemNo !== subDescriptionItemNo(sub)) return false;
  }
  if (line.quotationSubNo !== sub.subNo) return false;
  if (line.description.trim() !== sub.description.trim()) return false;
  if (line.uom !== sub.uom) return false;
  const quantity = parseQuantityScaled(line.quantity);
  const wantedQuantity = sub.remainingQuantityScaled ?? sub.quantityScaled;
  if (quantity === null || quantity !== wantedQuantity) return false;
  const price = parseUnitPriceMinor(currency, line.unitPrice);
  if (price === null || price !== sub.quotedUnitPriceMinor) return false;
  if (line.discountEnabled) return false;
  const bullets = line.descriptionLines;
  if (bullets.length !== sub.descriptionLines.length) return false;
  return bullets.every((value, index) => value === sub.descriptionLines[index]);
}

function subDescriptionItemNo(sub: SourceSubItem): number {
  return Number.parseInt(sub.key.split(".")[0] ?? "0", 10);
}

/**
 * True when the purchase order already carries exactly this quotation item:
 * every priced part is present with the same quantity, negotiated price and
 * description lines. A changed value reopens the ghost, because the order no
 * longer follows this quotation item exactly; #653's inline badges explain the
 * difference while the ghost is still on screen.
 */
export function sourceItemApplied(
  item: SourceItem,
  lines: PurchaseOrderLine[],
  currency: CurrencyInfo
): boolean {
  if (item.subItems.length > 0) {
    return item.subItems.every((sub) =>
      lines.some((line) => lineMatchesSub(line, sub, currency))
    );
  }
  const line = lines.find(
    (entry) =>
      entry.quotationItemNo === item.itemNo &&
      (entry.quotationSubNo === null || entry.quotationSubNo === undefined)
  );
  if (line === undefined) return false;
  if (line.description.trim() !== item.description.trim()) return false;
  if (line.uom !== item.uom) return false;
  const quantity = parseQuantityScaled(line.quantity);
  const wantedQuantity = item.remainingQuantityScaled ?? item.quantityScaled;
  const price = parseUnitPriceMinor(currency, line.unitPrice);
  if (quantity === null || quantity !== wantedQuantity) return false;
  if (price === null || price !== item.quotedUnitPriceMinor) return false;
  if (line.discountEnabled) return false;
  if (line.descriptionLines.length !== item.descriptionLines.length) return false;
  return line.descriptionLines.every(
    (value, index) => value === item.descriptionLines[index]
  );
}

/**
 * Converts one quotation item into the flat PO rows the store expects. A split
 * item has no parent row; its priced parts carry the shared group key.
 *
 * #654: an optional aggregate key lands on every fresh row, so a ghost
 * materialized inside an aggregate arrives already grouped. The default keeps
 * the normal ghost flow ungrouped.
 */
export function linesFromSourceItem(
  item: SourceItem,
  currency: CurrencyInfo,
  startKey: number,
  aggregateKey: number | null = null
): PurchaseOrderLine[] {
  if (item.subItems.length > 0) {
    return item.subItems.map((sub, index) => {
      const quantityScaled =
        sub.remainingQuantityScaled !== null && sub.remainingQuantityScaled > 0
          ? sub.remainingQuantityScaled
          : sub.quantityScaled;
      return {
        key: startKey + index,
        clientKey: newClientKey(),
        description: sub.description,
        quantity: formatQuantityScaled(quantityScaled),
        uom: sub.uom,
        unitPrice: formatUnitPriceMinor(currency, sub.quotedUnitPriceMinor),
        discountEnabled: false,
        discountPercent: "0",
        quotationItemNo: item.itemNo,
        quotationSubNo: sub.subNo,
        groupKey: `q:${item.itemNo}`,
        parentDescription: item.description,
        descriptionLines: [...sub.descriptionLines],
        aggregateKey,
        quotedUnitPriceMinor: sub.quotedUnitPriceMinor,
        remainingQuantityScaled:
          sub.remainingQuantityScaled !== null
            ? quantityScaled
            : null,
      };
    });
  }
  const quantityScaled =
    item.remainingQuantityScaled !== null && item.remainingQuantityScaled > 0
      ? item.remainingQuantityScaled
      : item.quantityScaled;
  return [
    {
      key: startKey,
      clientKey: newClientKey(),
      description: item.description,
      quantity: formatQuantityScaled(quantityScaled),
      uom: item.uom,
      unitPrice: formatUnitPriceMinor(currency, item.quotedUnitPriceMinor),
      discountEnabled: false,
      discountPercent: "0",
      quotationItemNo: item.itemNo,
      quotationSubNo: null,
      groupKey: "",
      parentDescription: "",
      descriptionLines: [...item.descriptionLines],
      aggregateKey,
      quotedUnitPriceMinor: item.quotedUnitPriceMinor,
      remainingQuantityScaled:
        item.remainingQuantityScaled !== null ? quantityScaled : null,
    },
  ];
}
