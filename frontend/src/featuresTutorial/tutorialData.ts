import type { CurrencyInfo } from "../format";
import type {
  PurchaseOrderReviewDTO,
  PurchaseOrderSourceLineDTO,
} from "../poFinalize";
import type {
  POSourcePage,
  POSourceQuotation,
  POSourceRevision,
  POSourceValidation,
} from "../poSelector";
import type {
  PurchaseOrderDTO,
  PurchaseOrderDTOItem,
} from "../purchaseOrder";
import type { ReconciliationTransport } from "../poReconciliationAdapter";

// Issue #595: the Purchase Order Features Tutorial runs on a client-only
// practice quotation. Nothing here reaches the API: the selector is handed
// these rows instead of ListPOSourceQuotations, the editor is handed the
// review instead of GetPurchaseOrderReview, and Save/Final Save are practice
// actions. The fixture exists so a first-time user can see the blue/green
// fields and the plain, split and unmatched ghosts without a real quotation.

export const TUTORIAL_QUOTE_NUMBER = "TUT-2026-001";
export const TUTORIAL_CUSTOMER = "Vera Features Tutorial (MOCK)";
export const TUTORIAL_SUBJECT =
  "Features Tutorial practice quotation (MOCK)";
export const TUTORIAL_PAYMENT_TERMS = "30 days";
export const TUTORIAL_INCOTERMS = "DAP";

const SOURCE_LINES: PurchaseOrderSourceLineDTO[] = [
  {
    QuotationItemNo: 1,
    QuotationSubNo: null,
    LineLabel: "1",
    Description: "Tutorial exact-match item (MOCK)",
    UoM: "Pc",
    QuantityScaled: 20000,
    UnitPriceMinor: 10_000_000,
    DiscountEnabled: false,
    DiscountPercent: 0,
  },
  {
    QuotationItemNo: 2,
    QuotationSubNo: 1,
    LineLabel: "2.1",
    ParentDescription: "Tutorial split package (MOCK)",
    Description: "Tutorial split part A (MOCK)",
    UoM: "Pc",
    QuantityScaled: 20000,
    UnitPriceMinor: 5_500_000,
    DiscountEnabled: false,
    DiscountPercent: 0,
    DescriptionLines: ["Tutorial scope A1", "Tutorial scope A2"],
  },
  {
    QuotationItemNo: 2,
    QuotationSubNo: 2,
    LineLabel: "2.2",
    ParentDescription: "Tutorial split package (MOCK)",
    Description: "Tutorial split part B (MOCK)",
    UoM: "Set",
    QuantityScaled: 10000,
    UnitPriceMinor: 2_850_000,
    DiscountEnabled: true,
    DiscountPercent: 5,
    DescriptionLines: ["Tutorial scope B1"],
  },
  {
    QuotationItemNo: 3,
    QuotationSubNo: null,
    LineLabel: "3",
    Description: "Tutorial plain ghost item (MOCK)",
    UoM: "Pc",
    QuantityScaled: 40000,
    UnitPriceMinor: 2_400_000,
    DiscountEnabled: false,
    DiscountPercent: 0,
  },
  {
    QuotationItemNo: 4,
    QuotationSubNo: null,
    LineLabel: "4",
    Description: "Tutorial unmatched item (MOCK)",
    UoM: "Lot",
    QuantityScaled: 10000,
    UnitPriceMinor: 1_500_000,
    DiscountEnabled: false,
    DiscountPercent: 0,
  },
];

/** The source revision as the PO review would return it. */
export function tutorialSourceLines(): PurchaseOrderSourceLineDTO[] {
  return SOURCE_LINES.map((line) => ({
    ...line,
    DescriptionLines: line.DescriptionLines
      ? [...line.DescriptionLines]
      : undefined,
  }));
}

const PRINTED_SUBTOTAL = 44_807_500;
const PRINTED_PPN = 4_928_825;
const PRINTED_GRAND = 49_736_325;

function item(
  overrides: Partial<PurchaseOrderDTOItem> & {
    LineNumber: number;
    Description: string;
    QuantityScaled: number;
    UoM: string;
    UnitPriceMinor: number;
  }
): PurchaseOrderDTOItem {
  return {
    QuotationItemNo: null,
    QuotationSubNo: null,
    DiscountEnabled: false,
    DiscountPercent: 0,
    GrossMinor: 0,
    DiscountMinor: 0,
    NetMinor: 0,
    ...overrides,
  };
}

/**
 * The practice PO the tutorial shows after the practice link. It deliberately
 * carries one exact line, one split group with a changed part, one plain line
 * with a changed quantity, one manual extra line, and leaves quotation item 4
 * unmatched.
 */
export function tutorialOrder(currency: CurrencyInfo): PurchaseOrderDTO {
  const items: PurchaseOrderDTOItem[] = [
    item({
      LineNumber: 1,
      QuotationItemNo: 1,
      QuotationSubNo: null,
      Description: "Tutorial exact-match item (MOCK)",
      QuantityScaled: 20000,
      UoM: "Pc",
      UnitPriceMinor: 10_000_000,
    }),
    item({
      LineNumber: 2,
      QuotationItemNo: 2,
      QuotationSubNo: 1,
      GroupKey: "q:2",
      ParentDescription: "Tutorial split package (MOCK)",
      Description: "Tutorial split part A (MOCK)",
      DescriptionLines: ["Tutorial scope A1", "Tutorial scope A2"],
      QuantityScaled: 30000,
      UoM: "Pc",
      UnitPriceMinor: 5_500_000,
    }),
    item({
      LineNumber: 3,
      QuotationItemNo: 2,
      QuotationSubNo: 2,
      GroupKey: "q:2",
      ParentDescription: "Tutorial split package (MOCK)",
      Description: "Tutorial split part B (MOCK)",
      DescriptionLines: ["Tutorial scope B1"],
      QuantityScaled: 10000,
      UoM: "Set",
      UnitPriceMinor: 2_850_000,
      DiscountEnabled: true,
      DiscountPercent: 5,
    }),
    item({
      LineNumber: 4,
      QuotationItemNo: 3,
      QuotationSubNo: null,
      Description: "Tutorial plain ghost item (MOCK)",
      QuantityScaled: 50000,
      UoM: "Pc",
      UnitPriceMinor: 2_400_000,
    }),
    item({
      LineNumber: 5,
      QuotationItemNo: null,
      QuotationSubNo: null,
      Description: "Tutorial manual extra line (MOCK)",
      QuantityScaled: 10000,
      UoM: "Lot",
      UnitPriceMinor: 750_000,
    }),
  ];
  return {
    PODate: "2026-11-01",
    RequestedDeliveryDate: "2026-12-01",
    UUID: "tutorial-purchase-order",
    State: "draft",
    QuoteNumber: TUTORIAL_QUOTE_NUMBER,
    CustomerName: TUTORIAL_CUSTOMER,
    BillingAddress: "Tutorial billing address (MOCK)",
    DeliveryAddress: "Tutorial delivery address (MOCK)",
    ShipToCode: "TUTORIAL",
    ContactEmail: "tutorial@example.invalid",
    ContactPhone: "+62 000 0000 0000",
    PONumber: "MOCK-TUTORIAL-PO",
    ReceivedDate: "2026-11-01",
    Incoterms: TUTORIAL_INCOTERMS,
    Currency: currency.code,
    TaxZone: "NonFTZ",
    TaxCategory: "SupplyOnly",
    TaxType: "Goods",
    ServiceCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-11-01T00:00:00Z",
    UpdatedAt: "2026-11-01T00:00:00Z",
    Items: items,
    ID: -1,
    QuotationID: -1,
    SourceRevision: 0,
    SourceMissing: false,
    SourceState: "live",
    PrintedTotalsKnown: true,
    CustomerID: -1,
    PaymentTerms: "45 days",
    DiscountPercent: 0,
    PpnBps: 1100,
    SubtotalMinor: 0,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 0,
    PrintedSubtotalMinor: PRINTED_SUBTOTAL,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: PRINTED_PPN,
    PrintedGrandTotalMinor: PRINTED_GRAND,
    Version: 1,
  };
}

export function tutorialReview(currency: CurrencyInfo): PurchaseOrderReviewDTO {
  return {
    Order: tutorialOrder(currency),
    Source: {
      QuotationID: -1,
      RevisionNumber: 0,
      QuoteNumber: TUTORIAL_QUOTE_NUMBER,
      CustomerID: -1,
      CustomerName: TUTORIAL_CUSTOMER,
      Currency: currency.code,
      PaymentTerms: TUTORIAL_PAYMENT_TERMS,
      Incoterms: TUTORIAL_INCOTERMS,
      DiscountPercent: 0,
      Lines: tutorialSourceLines(),
      PrintedSubtotalMinor: PRINTED_SUBTOTAL,
      PrintedDiscountMinor: 0,
      PrintedPpnMinor: PRINTED_PPN,
      PrintedGrandTotalMinor: PRINTED_GRAND,
    },
    DocumentState: "none",
  };
}

const TUTORIAL_ROW: POSourceQuotation = {
  id: -1,
  quoteNumber: TUTORIAL_QUOTE_NUMBER,
  subject: TUTORIAL_SUBJECT,
  customerName: TUTORIAL_CUSTOMER,
  latestRevision: 0,
  issuedRevisionCount: 1,
  activePO: null,
};

export const TUTORIAL_SELECTOR_PAGE: POSourcePage = {
  quotations: [TUTORIAL_ROW],
  page: 1,
  pageSize: 15,
  total: 1,
  totalPages: 1,
};

export const TUTORIAL_SELECTOR_REVISIONS: POSourceRevision[] = [
  { revision: 0, issuedAt: "2026-11-01T00:00:00Z" },
];

export const TUTORIAL_SELECTOR_VALIDATION: POSourceValidation = {
  valid: true,
};


/**
 * The practice order has no stored reconciliation row. Absence is the normal
 * hook state, and a save is a no-op because the editor is read-only in the
 * tutorial.
 */
export const TUTORIAL_RECONCILIATION_TRANSPORT: ReconciliationTransport = {
  load: async () => {
    throw new Error("no reconciliation recorded");
  },
  save: async (_poID, session) => session,
};
