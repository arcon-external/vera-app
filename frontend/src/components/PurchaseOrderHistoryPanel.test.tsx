import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderHistoryEntry } from "../poCorrection";
import { PurchaseOrderHistoryPanel } from "./PurchaseOrderHistoryPanel";

// Issue #473: the readable history panel. Every value is the snapshot's, not the
// live record's: reason, actor, time, attachment version and the replaced
// header, lines and totals.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const CORRECTION: PurchaseOrderHistoryEntry = {
  UUID: "corr-1",
  Kind: "correction",
  State: "saved",
  Reason: "The customer moved to a new warehouse",
  ActorName: "Budi Santoso",
  CreatedAt: "2026-10-21T10:15:00+07:00",
  QuoteNumber: "1502260044",
  CustomerName: "PT. Buyer Nusantara",
  SourceRevision: 7,
  Currency: "USD",
  TaxZone: "FTZ",
  TaxCategory: "Goods",
  BillingAddress: "Jl. Old Billing 7",
  DeliveryAddress: "Jl. Old Delivery 9",
  ShipToCode: "SHIP-OLD-3",
  ContactEmail: "old.buyer@example.com",
  ContactPhone: "+62 21 555 0101",
  PONumber: "PO-473",
  PODate: "2026-09-30",
  ReceivedDate: "2026-10-01",
  RequestedDeliveryDate: "2026-11-15",
  PaymentTerms: "30 days",
  Incoterms: "CIF",
  AdjustmentReason: "Rounded to the printed total",
  PreviousVersion: 4,
  ResultingVersion: 5,
  DiscountPercent: 5,
  SubtotalMinor: 20000,
  DiscountMinor: 2000,
  PpnBps: 1100,
  PpnMinor: 1980,
  AdjustmentMinor: -250,
  GrandTotalMinor: 19730,
  Attachment: {
    Version: 2,
    SHA256: "d2f4a6c8e0b1".padEnd(64, "0"),
    Filename: "Customer PO 4500098123.pdf",
  },
  Items: [
    {
      LineNumber: 1,
      Description: "Widget",
      QuantityScaled: 20000,
      UoM: "Pc",
      UnitPriceMinor: 10000,
      DiscountMinor: 2000,
      NetMinor: 18000,
    },
  ],
};

const SOURCE_REPAIR: PurchaseOrderHistoryEntry = {
  ...CORRECTION,
  UUID: "corr-3",
  Kind: "source_repair",
  Reason: "The order followed the wrong revision",
  PreviousVersion: 5,
  ResultingVersion: 6,
  Attachment: null,
  Repair: {
    PreviousQuoteNumber: "1502260044",
    PreviousSourceRevision: 7,
    PreviousCustomerName: "PT. Buyer Nusantara",
    NewQuoteNumber: "1502260051",
    NewSourceRevision: 0,
    NewCustomerName: "PT. Buyer Nusantara",
    Decisions: [
      { Key: "header:paymentTerms", Choice: "quotation" },
      { Key: "line:1:unitPrice", Choice: "po" },
    ],
  },
};

const DRAFT_OVERWRITE: PurchaseOrderHistoryEntry = {
  ...CORRECTION,
  UUID: "corr-2",
  Kind: "draft_overwrite",
  State: "draft",
  Reason: "Draft overwritten after a concurrent save",
  ActorName: "Sari",
  CreatedAt: "2026-10-21T09:00:00+07:00",
  PreviousVersion: 3,
  ResultingVersion: 4,
  Attachment: null,
};

function markup(entries: PurchaseOrderHistoryEntry[]) {
  return renderToStaticMarkup(
    <PurchaseOrderHistoryPanel entries={entries} currency={USD} />
  );
}

describe("PurchaseOrderHistoryPanel (issue #473)", () => {
  it("says so when nothing has been corrected", () => {
    const html = markup([]);
    expect(html).toContain("No corrections have been recorded for this purchase order.");
    expect(html).toContain("Nothing recorded yet");
  });

  it("renders the reason, actor, time and versions of each snapshot", () => {
    const html = markup([CORRECTION, DRAFT_OVERWRITE]);
    expect(html).toContain("Correction");
    expect(html).toContain("Draft overwritten");
    expect(html).toContain("The customer moved to a new warehouse");
    expect(html).toContain("Draft overwritten after a concurrent save");
    expect(html).toContain("Budi Santoso");
    expect(html).toContain("Sari");
    expect(html).toContain("2026-10-21 10:15 +07:00");
    expect(html).toContain("Version 4 → 5");
    expect(html).toContain("Version 3 → 4");
  });

  it("renders the attachment reference the snapshot kept", () => {
    const html = markup([CORRECTION, DRAFT_OVERWRITE]);
    expect(html).toContain("Customer PO PDF v2 · Customer PO 4500098123.pdf");
    expect(html).toContain("sha256 d2f4a6c8e0b1…");
    expect(html).toContain("No customer PO PDF was attached at that moment.");
  });

  it("renders the replaced header, lines and totals", () => {
    const html = markup([CORRECTION]);
    expect(html).toContain("Previous values");
    expect(html).toContain("PO-473");
    expect(html).toContain("Jl. Old Delivery 9");
    expect(html).toContain("Rounded to the printed total");
    expect(html).toContain("Widget");
    expect(html).toContain("$100.00");
    expect(html).toContain("$180.00");
    expect(html).toContain("$200.00");
    expect(html).toContain("$197.30");
  });

  it("renders every retained header value, not just the handful it once showed", () => {
    // Wave 2 of #473: a correction that changed only the billing address, the
    // ship-to code, the contact details or a date must be understandable from
    // this panel. Each value below is distinct, so an omitted field cannot be
    // satisfied by another field's text.
    const html = markup([CORRECTION]);
    for (const [surface, value] of [
      ["quote number", "1502260044"],
      ["customer", "PT. Buyer Nusantara"],
      ["source revision", ">7<"],
      ["state", ">saved<"],
      ["currency", "USD"],
      ["tax zone", "FTZ"],
      ["tax category", "Goods"],
      ["billing address", "Jl. Old Billing 7"],
      ["delivery address", "Jl. Old Delivery 9"],
      ["ship-to code", "SHIP-OLD-3"],
      ["contact email", "old.buyer@example.com"],
      ["contact phone", "+62 21 555 0101"],
      ["PO date", "2026-09-30"],
      ["received date", "2026-10-01"],
      ["requested delivery date", "2026-11-15"],
      ["payment terms", "30 days"],
      ["incoterms", "CIF"],
      ["PO number", "PO-473"],
      ["whole-PO discount", "5%"],
      ["PPN rate", "1100 bps"],
      ["adjustment reason", "Rounded to the printed total"],
      ["attachment", "Customer PO PDF v2"],
    ] as const) {
      expect(html, `${surface} is missing from the retained snapshot`).toContain(value);
    }
    for (const group of [
      "Identity",
      "Addresses and contact",
      "Dates and terms",
      "Totals",
      "Lines",
    ]) {
      expect(html, `history group ${group} is missing`).toContain(group);
    }
  });

  it("reports a history read failure instead of an empty success", () => {
    const html = renderToStaticMarkup(
      <PurchaseOrderHistoryPanel
        entries={[]}
        currency={USD}
        error="purchase order not found"
      />
    );
    expect(html).toContain("purchase order not found");
    expect(html).not.toContain("No corrections have been recorded");
  });

describe("PurchaseOrderHistoryPanel source repair (issue #475)", () => {
  it("names the old and new source and every recorded choice", () => {
    const html = markup([SOURCE_REPAIR]);
    expect(html).toContain("Source repaired");
    expect(html).toContain(
      "Quotation 1502260044 · revision 7 → Quotation 1502260051 · revision 0"
    );
    expect(html).toContain("header:paymentTerms");
    expect(html).toContain("Used Quotation");
    expect(html).toContain("line:1:unitPrice");
    expect(html).toContain("Kept PO entry");
    expect(html).toContain("The order followed the wrong revision");
  });

  it("does not claim a customer move when both sides name the same customer", () => {
    const html = markup([SOURCE_REPAIR]);
    expect(html).not.toContain("customer PT. Buyer Nusantara →");
  });
});

});
