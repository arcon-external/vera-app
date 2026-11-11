import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderDraftLinkReviewDTO } from "../poDraftLink";
import {
  initialPurchaseOrderEditorState,
  type PurchaseOrderDTO,
} from "../purchaseOrder";
import { PurchaseOrderDraftLinkReviewPanel } from "./PurchaseOrderSourceLinkPanel";

// Issue #582: the tax-basis decision is a two-sided diff like every other
// review row. The whole card is the selection target, the PPN difference is
// stated, and the existing accessible names are unchanged.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const ORDER: PurchaseOrderDTO = {
  PODate: "2026-10-01",
  RequestedDeliveryDate: null,
  UUID: "uuid-tax-diff",
  State: "draft",
  QuoteNumber: "",
  CustomerName: "",
  BillingAddress: "",
  DeliveryAddress: "",
  ShipToCode: "",
  ContactEmail: "",
  ContactPhone: "",
  PONumber: "PO-TAX",
  ReceivedDate: "2026-10-02",
  Version: 1,
  Incoterms: "CIF",
  Currency: "USD",
  TaxZone: "FTZ",
  TaxCategory: "",
  AdjustmentReason: "",
  CreatedAt: "2026-10-02T10:00:00Z",
  UpdatedAt: "2026-10-02T10:00:00Z",
  Items: [],
  ID: 7,
  QuotationID: 0,
  SourceRevision: 0,
  CustomerID: 0,
  PaymentTerms: "45 days",
  DiscountPercent: 0,
  PpnBps: 0,
  SubtotalMinor: 0,
  DiscountMinor: 0,
  PpnMinor: 0,
  AdjustmentMinor: 0,
  GrandTotalMinor: 0,
  PrintedSubtotalMinor: 0,
  PrintedDiscountMinor: 0,
  PrintedPpnMinor: 0,
  PrintedGrandTotalMinor: 0,
};

const REVIEW: PurchaseOrderDraftLinkReviewDTO = {
  Order: ORDER,
  Source: {
    QuotationID: 12,
    RevisionNumber: 1,
    QuoteNumber: "1502260042",
    CustomerID: 2,
    CustomerName: "PT. Buyer",
    Currency: "USD",
    PaymentTerms: "45 days",
    Incoterms: "CIF",
    DiscountPercent: 0,
    Lines: [],
    PrintedSubtotalMinor: 0,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 0,
  },
  DocumentState: "none",
  DraftTax: {
    TaxZone: "FTZ",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxCategory: "",
    PpnBps: 0,
  },
  SourceTax: {
    TaxZone: "NonFTZ",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxCategory: "",
    PpnBps: 1100,
  },
  TaxChoiceRequired: true,
};

function markup(): string {
  const state = initialPurchaseOrderEditorState(ORDER, USD);
  return renderToString(
    <PurchaseOrderDraftLinkReviewPanel
      state={state}
      review={REVIEW}
      currency={USD}
      decisions={{}}
      acknowledgedMissing={[]}
      acknowledgedExtra={[]}
      taxChoice=""
      linking={false}
      error={null}
      onDecision={() => undefined}
      onToggleMissing={() => undefined}
      onToggleExtra={() => undefined}
      onMatchChange={() => undefined}
      onTaxChoice={() => undefined}
      onBack={() => undefined}
      onLink={() => undefined}
    />
  ).replace(/<!-- -->/g, "");
}

describe("PurchaseOrderDraftLinkReviewPanel tax-basis diff (#582)", () => {
  it("compares the two bases as labelled sides with a divider", () => {
    const html = markup();
    expect(html).toContain("PO ENTRY");
    expect(html).toContain("QUOTATION");
    expect(html).toContain("≠");
    expect(html).toContain("FTZ / Goods · no PPN");
    expect(html).toContain("NonFTZ / Goods · 11% PPN");
  });

  it("states the PPN difference between the two bases", () => {
    const html = markup();
    expect(html).toContain("PPN difference");
    expect(html).toContain("no PPN");
    expect(html).toContain("11%");
  });

  it("makes the whole card the selection target while keeping its name", () => {
    const html = markup();
    const keep = html.match(
      /<button[^>]*aria-label="Keep the draft tax basis"[^>]*>[^]*?<\/button>/
    )?.[0];
    const source = html.match(
      /<button[^>]*aria-label="Use the quotation tax basis"[^>]*>[^]*?<\/button>/
    )?.[0];
    expect(keep).toBeDefined();
    expect(keep).toContain("This purchase order");
    expect(source).toBeDefined();
    expect(source).toContain("Quotation revision");
    expect(source).toContain('aria-pressed="false"');
  });
});
