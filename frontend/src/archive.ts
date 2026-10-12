import type { CurrencyInfo } from "./format";

export interface ArchivedQuotationSummary {
  ID: number;
  QuoteNumber: string;
  RevisionNumber: number;
  CustomerName: string;
  Subject: string;
  /** The code the latest revision was issued in (server-normalised). */
  Currency: string;
  CreatedAt: string;
  GrandTotal: number;
  DisplayGrand: string;
}

/**
 * Resolves a revision's currency code to its catalogue entry for display.
 * The archive/detail pages fetch the catalogue over GetCurrencyCatalogue();
 * the code comes from the revision, the fallback is the live profile entry
 * the caller passed. A missing or unknown code (a catalogue that has not
 * loaded yet -- the server normalises codes, so unknown means unreachable,
 * not a third currency) renders in the live currency rather than breaking
 * the row. Matching is case-insensitive, mirroring Go's Lookup.
 */
export function resolveCurrencyEntry(
  catalogue: readonly CurrencyInfo[] | null | undefined,
  code: string | null | undefined,
  fallback: CurrencyInfo
): CurrencyInfo {
  const want = (code ?? "").trim().toUpperCase();
  if (want !== "") {
    const found = catalogue?.find((entry) => entry.code === want);
    if (found) return found;
  }
  return fallback;
}

export interface ArchivedQuotationSubItem {
  SubNo: number;
  Description: string;
  Details: string[];
  Qty: number;
  UoM: string;
  OriginalUnitPrice: number;
  DisplayOriginalUnit: string;
  DisplayOriginalTotal: string;
  MarginEnabled: boolean;
  MarginPercent: number;
  MarginAmount: number;
  DisplayMarginAmount: string;
  MarginalizedUnitPrice: number;
  DisplayMarginalizedUnit: string;
  MarginalizedTotal: number;
  DisplayMarginalizedTotal: string;
  /** The row's own discount. Zero on every revision exported before #274. */
  DiscountEnabled: boolean;
  DiscountPercent: number;
  ListUnitPrice: number;
  DiscountAmount: number;
}

export interface ArchivedQuotationItem {
  ItemNo: number;
  Description: string;
  Qty: number;
  UoM: string;
  UnitPrice: number;
  DisplayUnitPrice: string;
  MarginEnabled: boolean;
  MarginPercent: number;
  MarginAmount: number;
  /** The row's own discount. Zero on every revision exported before #274. */
  DiscountEnabled: boolean;
  DiscountPercent: number;
  ListUnitPrice: number;
  DiscountAmount: number;
  DisplayMarginAmount: string;
  OriginalTotal: number;
  DisplayOriginalTotal: string;
  MarginalizedUnitPrice: number;
  DisplayMarginalizedUnit: string;
  MarginalizedTotal: number;
  DisplayMarginalizedTotal: string;
  Subs: ArchivedQuotationSubItem[];
  ShowSubPrices: boolean;
}

export interface ArchivedQuotation extends ArchivedQuotationSummary {
  CustomerCode: number;
  CustomerAddress: string;
  RecipientName: string;
  RecipientAddress: string;
  Incoterm: string;
  /** The code the revision was issued in. Editing under a live profile with a
   * different code is blocked rather than reinterpreting the stored amounts. */
  Currency: string;
  ValidityDays: number;
  PaymentTermDays: number;
  LeadTimeDays: number;
  DiscountPercent: number;
  TaxZone: string;
  TaxCategory: string;
  TaxType: string;
  ServiceCategory: string;
  TaxRateBps: number;
  OriginalSubtotal: number;
  OriginalDiscount: number;
  OriginalGrand: number;
  MarginalizedSubtotal: number;
  MarginalizedDiscount: number;
  MarginalizedGrand: number;
  PpnAmount: number;
  DisplayPpn: string;
  TaxAmount: number;
  MarginalizedAfterTax: number;
  ProfitAmount: number;
  NettProfit: number;
  DisplayOriginalSubtotal: string;
  DisplayOriginalDiscount: string;
  DisplayOriginalGrand: string;
  DisplayMarginalizedSubtotal: string;
  DisplayMarginalizedDiscount: string;
  DisplayMarginalizedGrand: string;
  DisplayTax: string;
  DisplayMarginalizedAfterTax: string;
  DisplayProfit: string;
  DisplayNett: string;
  Items: ArchivedQuotationItem[];
  HTMLSnapshot: string;
  AvailableRevisions: number[];
}
