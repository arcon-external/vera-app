export interface CustomerPreviewItem {
  Description: string;
  UoM: string;
  DisplayOriginalUnitPrice: string;
  DisplayOriginalTotal: string;
  DisplayUnitPrice: string;
  DisplayTotal: string;
  OriginalUnitPrice: number;
  OriginalTotal: number;
  UnitPrice: number;
  Total: number;
  MarginEnabled: boolean;
  MarginPercent: number;
  MarginAmount: number;
  No: number;
  Qty: number;
}

export interface CustomerPreview {
  Items: CustomerPreviewItem[];
  Subject: string;
  RecipientName: string;
  RecipientCompany: string;
  RecipientAddress: string;
  QuoteNumber: string;
  RevisionID: number;
  Incoterm: string;
  DisplaySubTotal: string;
  DisplayDiscountAmount: string;
  DisplayGrandTotal: string;
  DisplayPpnAmount: string;
  SubTotal: number;
  DiscountAmount: number;
  GrandTotal: number;
  PpnAmount: number;
  OriginalSubTotal: number;
  OriginalDiscountAmount: number;
  OriginalGrandTotal: number;
  MarginalizedGrandAfterTax: number;
  ProfitAmount: number;
  NettProfit: number;
  DiscountPercent: number;
}

export interface InternalPreviewItem {
  Description: string;
  CostUnit: number;
  SellUnit: number;
  CostTotal: number;
  SellTotal: number;
  OriginalUnitPrice: number;
  OriginalTotal: number;
  MarginalizedUnitPrice: number;
  MarginalizedTotal: number;
  DisplayOriginalUnitPrice: string;
  DisplayOriginalTotal: string;
  DisplayMarginalizedUnit: string;
  DisplayMarginalizedTotal: string;
  DisplayMarginAmount: string;
  MarginEnabled: boolean;
  MarginPercent: number;
  MarginAmount: number;
  No: number;
  Qty: number;
}

export interface InternalBreakdown {
  Items: InternalPreviewItem[];
  TaxLabel: string;
  TaxType: string;
  ServiceCategory: string;
  PpnAmount: number;
  DisplayPpn: string;
  DisplayCost: string;
  DisplaySell: string;
  DisplayCostDisc: string;
  DisplaySellDisc: string;
  DisplayCostGrd: string;
  DisplayGrand: string;
  DisplayProfit: string;
  DisplayTax: string;
  DisplayNett: string;
  DisplayOriginalSubtotal: string;
  DisplayOriginalDiscount: string;
  DisplayOriginalGrand: string;
  DisplayMarginalizedSubtotal: string;
  DisplayMarginalizedDiscount: string;
  DisplayMarginalizedGrand: string;
  DisplayMarginalizedAfterTax: string;
  CostSubtotal: number;
  SellSubtotal: number;
  DiscountCost: number;
  DiscountSell: number;
  CostGrand: number;
  GrandTotal: number;
  ProfitAmount: number;
  TaxAmount: number;
  NettProfit: number;
  TaxRateBps: number;
  DiscountPercent: number;
  OriginalSubtotal: number;
  OriginalDiscount: number;
  OriginalGrand: number;
  MarginalizedSubtotal: number;
  MarginalizedDiscount: number;
  MarginalizedGrand: number;
  MarginalizedGrandAfterTax: number;
}
