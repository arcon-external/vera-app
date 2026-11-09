import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ArchivedQuotation } from "../archive";
import { QuotationCopyButton, QuotationDetailPage } from "./QuotationDetailPage";
import { ToastProvider } from "../toast";

// The copy action on the archived detail page (#485). The suite runs in node,
// so interaction is not automatable here: these pins prove the button is
// absent while the revision loads, renders its label once loaded, and honours
// the switching-disabled state. The clipboard write itself is unit-tested
// through copyQuotationToClipboard; the click-through stays hand-driven.

function archived(): ArchivedQuotation {
  return {
    ID: 5,
    QuoteNumber: "1502260059",
    RevisionNumber: 3,
    CustomerName: "Acme",
    CustomerCode: 4,
    CustomerAddress: "Jl. Klien 12",
    Subject: "Pump package",
    Currency: "IDR",
    CreatedAt: "",
    GrandTotal: 0,
    DisplayGrand: "",
    RecipientName: "Jane",
    RecipientAddress: "Gedung Penerima 3",
    Incoterm: "DAP",
    ValidityDays: 45,
    PaymentTermDays: 60,
    DisplayLeadTime: "-",
    LeadTimeYears: 0,
    LeadTimeMonths: 1,
    LeadTimeWeeks: 0,
    LeadTimeDays: 0,
    DiscountPercent: 0,
    TaxZone: "FTZ",
    TaxCategory: "",
    TaxType: "Goods",
    ServiceCategory: "",
    TaxRateBps: 0,
    OriginalSubtotal: 0,
    OriginalDiscount: 0,
    OriginalGrand: 0,
    MarginalizedSubtotal: 0,
    MarginalizedDiscount: 0,
    MarginalizedGrand: 0,
    PpnAmount: 0,
    DisplayPpn: "",
    TaxAmount: 0,
    MarginalizedAfterTax: 0,
    ProfitAmount: 0,
    NettProfit: 0,
    DisplayOriginalSubtotal: "",
    DisplayOriginalDiscount: "",
    DisplayOriginalGrand: "",
    DisplayMarginalizedSubtotal: "",
    DisplayMarginalizedDiscount: "",
    DisplayMarginalizedGrand: "",
    DisplayTax: "",
    DisplayMarginalizedAfterTax: "",
    DisplayProfit: "",
    DisplayNett: "",
    HTMLSnapshot: "",
    AvailableRevisions: [3],
    Items: [],
  };
}

const noop = () => {};

describe("QuotationDetailPage copy action", () => {
  it("shows no copy button while the revision loads", () => {
    const html = renderToString(
      <ToastProvider>
        <QuotationDetailPage
          quotationID={5}
          revision={3}
          onBack={noop}
          onEdit={noop}
          onCreatePurchaseOrder={async () => {}}
          onOpenPurchaseOrder={noop}
        />
      </ToastProvider>
    );
    expect(html).toContain("Loading quotation");
    expect(html).not.toContain("Copy to clipboard");
  });

  it("renders the copy button with its label once the revision is loaded", () => {
    const html = renderToString(
      <QuotationCopyButton quotation={archived()} disabled={false} onCopy={noop} />
    );
    expect(html).toContain("Copy to clipboard");
    expect(html).not.toContain("disabled=");
  });

  it("disables the copy button while switching revisions", () => {
    const html = renderToString(
      <QuotationCopyButton quotation={archived()} disabled={true} onCopy={noop} />
    );
    expect(html).toContain("Copy to clipboard");
    expect(html).toContain("disabled=");
  });
});
