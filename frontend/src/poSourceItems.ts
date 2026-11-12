import type { PurchaseOrderSourceLineDTO } from "./poFinalize";
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
export function groupSourceItems(lines: PurchaseOrderSourceLineDTO[]): SourceItem[] {
  const byItem = new Map<number, SourceItem>();
  const order: number[] = [];
  for (const line of lines) {
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
  if (quantity === null || quantity !== sub.quantityScaled) return false;
  const price = parseUnitPriceMinor(currency, line.unitPrice);
  if (price === null || price !== sub.unitPriceMinor) return false;
  if (line.discountEnabled !== sub.discountEnabled) return false;
  if (line.discountEnabled && Number.parseInt(line.discountPercent, 10) !== sub.discountPercent) {
    return false;
  }
  const bullets = line.descriptionLines;
  if (bullets.length !== sub.descriptionLines.length) return false;
  return bullets.every((value, index) => value === sub.descriptionLines[index]);
}

function subDescriptionItemNo(sub: SourceSubItem): number {
  return Number.parseInt(sub.key.split(".")[0] ?? "0", 10);
}

/**
 * True when the purchase order already carries exactly this quotation item:
 * every priced part is present with the same values and description lines.
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
  const price = parseUnitPriceMinor(currency, line.unitPrice);
  if (quantity !== item.quantityScaled || price !== item.unitPriceMinor) return false;
  if (line.discountEnabled !== item.discountEnabled) return false;
  if (line.discountEnabled && Number.parseInt(line.discountPercent, 10) !== item.discountPercent) {
    return false;
  }
  if (line.descriptionLines.length !== item.descriptionLines.length) return false;
  return line.descriptionLines.every(
    (value, index) => value === item.descriptionLines[index]
  );
}

/**
 * Converts one quotation item into the flat PO rows the store expects. A split
 * item has no parent row; its priced parts carry the shared group key.
 */
export function linesFromSourceItem(
  item: SourceItem,
  currency: CurrencyInfo,
  startKey: number
): PurchaseOrderLine[] {
  if (item.subItems.length > 0) {
    return item.subItems.map((sub, index) => ({
      key: startKey + index,
      clientKey: newClientKey(),
      description: sub.description,
      quantity: formatQuantityScaled(sub.quantityScaled),
      uom: sub.uom,
      unitPrice: formatUnitPriceMinor(currency, sub.unitPriceMinor),
      discountEnabled: sub.discountEnabled,
      discountPercent: String(sub.discountPercent),
      quotationItemNo: item.itemNo,
      quotationSubNo: sub.subNo,
      groupKey: `q:${item.itemNo}`,
      parentDescription: item.description,
      descriptionLines: [...sub.descriptionLines],
    }));
  }
  return [
    {
      key: startKey,
      clientKey: newClientKey(),
      description: item.description,
      quantity: formatQuantityScaled(item.quantityScaled),
      uom: item.uom,
      unitPrice: formatUnitPriceMinor(currency, item.unitPriceMinor),
      discountEnabled: item.discountEnabled,
      discountPercent: String(item.discountPercent),
      quotationItemNo: item.itemNo,
      quotationSubNo: null,
      groupKey: "",
      parentDescription: "",
      descriptionLines: [...item.descriptionLines],
    },
  ];
}
