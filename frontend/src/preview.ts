export interface CustomerPreviewItem {
  Description: string;
  UoM: string;
  DisplayUnitPrice: string;
  DisplayTotal: string;
  UnitPrice: number;
  Total: number;
  No: number;
  Qty: number;
}

export interface CustomerPreview {
  Items: CustomerPreviewItem[];
  Subject: string;
  RecipientName: string;
  RecipientCompany: string;
  RecipientAddress: string;
  Incoterm: string;
  DisplaySubTotal: string;
  DisplayDiscountAmount: string;
  DisplayGrandTotal: string;
  SubTotal: number;
  DiscountAmount: number;
  GrandTotal: number;
  DiscountPercent: number;
}

export interface InternalPreviewItem {
  Description: string;
  CostUnit: number;
  SellUnit: number;
  CostTotal: number;
  SellTotal: number;
  No: number;
  Qty: number;
}

export interface InternalBreakdown {
  Items: InternalPreviewItem[];
  TaxLabel: string;
  DisplayCost: string;
  DisplaySell: string;
  DisplayCostDisc: string;
  DisplaySellDisc: string;
  DisplayCostGrd: string;
  DisplayGrand: string;
  DisplayProfit: string;
  DisplayTax: string;
  DisplayNett: string;
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
  MarginPercent: number;
  DiscountPercent: number;
}
