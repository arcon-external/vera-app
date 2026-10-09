export interface ArchivedQuotationSummary {
  ID: number;
  QuoteNumber: string;
  RevisionNumber: number;
  CustomerName: string;
  Subject: string;
  CreatedAt: string;
  GrandTotal: number;
  DisplayGrand: string;
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
