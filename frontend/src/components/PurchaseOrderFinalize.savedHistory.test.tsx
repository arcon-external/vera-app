import { renderToStaticMarkup } from "react-dom/server";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";
import type { backend } from "@backend/models";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderDTO } from "../purchaseOrder";
import { PurchaseOrderSavedView } from "./PurchaseOrderFinalize";

// #518: the saved record's read-only reconciliation history.
//
// The saved record keeps everything #471/#473/#475 gave it and adds one
// read-only surface: the current saved session plus every earlier generation.
// This suite renders the composition without a DOM, so it proves what exists
// in the markup — the trigger, the rail, the exact state sentences, the
// saved-only filters and the return control — and that no editor action leaks
// into a saved record. Interaction and focus live in the jsdom suite.

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function order(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    UUID: "po-uuid-518",
    State: "saved",
    QuoteNumber: "1502260044",
    CustomerName: "PT. Buyer Nusantara",
    BillingAddress: "Jl. Satu 1",
    DeliveryAddress: "Jl. Dua 2",
    ShipToCode: "SHIP-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "021-555",
    PONumber: "PO-518",
    ReceivedDate: "2026-10-20",
    Incoterms: "FOB",
    Currency: "USD",
    TaxZone: "JKT",
    TaxCategory: "standard",
    AdjustmentReason: "",
    CreatedAt: "2026-10-20T09:00:00Z",
    UpdatedAt: "2026-10-21T09:00:00Z",
    Items: [
      {
        ID: 1,
        ClientKey: "client:line-1",
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Widget",
        QuantityScaled: 20000,
        UoM: "Pc",
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 20000,
        DiscountMinor: 0,
        NetMinor: 20000,
      },
    ],
    ID: 518,
    QuotationID: 44,
    SourceRevision: 2,
    CustomerID: 9,
    PaymentTerms: "50% advance",
    DiscountPercent: 0,
    PpnBps: 1100,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 2200,
    AdjustmentMinor: 0,
    GrandTotalMinor: 22200,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 2200,
    PrintedGrandTotalMinor: 22200,
    Version: 6,
    SavedAt: "2026-10-21T09:00:00Z",
    SavedBy: 3,
    PdfAbsentConfirmed: true,
    ...overrides,
  };
}

const CURRENT_SESSION: backend.POReconciliationSession = {
  ID: 12,
  PurchaseOrderID: 518,
  SourceQuotationID: 44,
  SourceRevision: 2,
  POVersion: 6,
  Active: true,
  ReadOnly: true,
  CreatedBy: 7,
  CreatedAt: "2026-10-21T09:00:00Z",
  UpdatedAt: "2026-10-21T09:05:00Z",
  LineIdentities: [],
  Items: [
    {
      Key: "header:paymentTerms",
      Field: "header:paymentTerms",
      Kind: "difference",
      LineKey: null,
      SourceLineRef: null,
      Label: "Payment terms",
      POValue: "50% advance",
      SourceValue: "30 days",
      Status: "resolved",
      Resolution: "kept_po",
      ActorID: 7,
      UpdatedAt: "2026-10-21T09:05:00Z",
      History: [],
    },
    {
      Key: "link:taxBasis",
      Field: "link:taxBasis",
      Kind: "tax_basis",
      LineKey: null,
      SourceLineRef: null,
      Label: "Tax basis",
      POValue: { TaxZone: "JKT", TaxType: "PPN", ServiceCategory: "" },
      SourceValue: { TaxZone: "JKT", TaxType: "PPN", ServiceCategory: "" },
      Status: "resolved",
      Resolution: "used_quotation",
      ActorID: 7,
      UpdatedAt: "2026-10-21T09:05:00Z",
      History: [],
    },
  ],
} as unknown as backend.POReconciliationSession;

const READ: backend.POReconciliationSessions = {
  PurchaseOrderID: 518,
  POVersion: 6,
  Sessions: [
    {
      Generation: 1,
      SourceAvailable: true,
      RepairCorrectionID: null,
      Session: CURRENT_SESSION,
    },
    {
      Generation: 0,
      SourceAvailable: true,
      RepairCorrectionID: null,
      Session: {
        ...CURRENT_SESSION,
        ID: 9,
        Active: false,
        ReadOnly: true,
        SourceRevision: 1,
        POVersion: 4,
      },
    },
  ],
} as unknown as backend.POReconciliationSessions;

function markup(element: ReactElement): string {
  return renderToStaticMarkup(element);
}

describe("PurchaseOrderSavedView reconciliation history (#518)", () => {
  it("shows the read-only current session in a saved rail and the archive return", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
        historyInitial={[]}
        reconciliationInitial={READ}
      />
    );
    // The saved surface exists at both widths: a rail region (shown >=1024)
    // and the same panel's trigger (shown below it).
    expect(html).toContain('aria-label="Reconciliation"');
    expect(html).toContain('aria-label="Reconciliation, read-only. Open panel."');
    expect(html).toContain("Read-only");
    expect(html).toContain("Saved session · revision 2 · generation 1");
    expect(html).toContain("2 resolved");
    // Saved-only filters: All and Resolved, never Unresolved.
    expect(html).toContain("All");
    expect(html).toContain("Resolved");
    expect(html).not.toContain("Unresolved");
    // The exact return control (the baseline said "Back to purchase orders").
    expect(html).toContain("Back to archive");
    expect(html).not.toContain("Back to purchase orders");
    // A saved record exposes no reconciliation decision or editor control.
    expect(html).not.toContain("Keep PO");
    expect(html).not.toContain("Use Quotation");
    expect(html).not.toContain("Acknowledge");
    // The current generation is not duplicated as history, and a lone older
    // generation is disclosed.
    expect(html).toContain("Session history (1)");
  });

  it("shows the exact no-session sentence when the PO predates sessions", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
        historyInitial={[]}
        reconciliationInitial={{ PurchaseOrderID: 518, POVersion: 6, Sessions: [] }}
      />
    );
    expect(html).toContain(
      "No reconciliation session was recorded for this purchase order."
    );
    expect(html).not.toContain("Session history");
  });

  it("shows the saved history error sentence and never a no-session claim", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
        historyInitial={[]}
        reconciliationInitialError="connection reset"
      />
    );
    expect(html).toContain("The saved reconciliation history could not be loaded.");
    expect(html).toContain("Try again");
    expect(html).not.toContain(
      "No reconciliation session was recorded for this purchase order."
    );
  });

  it("shows the empty-session sentence and a zero count with no filters", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
        historyInitial={[]}
        reconciliationInitial={{
          PurchaseOrderID: 518,
          POVersion: 6,
          Sessions: [
            {
              Generation: 1,
              SourceAvailable: true,
              RepairCorrectionID: null,
              Session: { ...CURRENT_SESSION, Items: [] } as unknown as backend.POReconciliationSession,
            },
          ],
        }}
      />
    );
    expect(html).toContain("No differences were recorded for this session.");
    expect(html).toContain("0 resolved");
    expect(html).toContain("Recorded when this purchase order was finalized.");
    // No filters and no history when there is nothing to filter.
    expect(html).not.toContain('role="tablist"');
    expect(html).not.toContain("Session history");
  });

  it("stacks the saved lines below a 720px container and keeps the table above it", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
        historyInitial={[]}
        reconciliationInitial={READ}
      />
    );
    // Layout B: the stacked read-only list is the sub-720 presentation, the
    // table returns at 720 of *container* width, and the table no longer
    // demands a 720px minimum that would force a sideways scroll at 1024.
    expect(html).toContain("@min-[720px]:hidden");
    expect(html).toContain("hidden @min-[720px]:block");
    expect(html).toContain("Line 1");
    expect(html).toContain("Widget");
    expect(html).not.toContain("min-w-[720px]");
    expect(html).not.toContain("overflow-x-auto scroll-thin");
  });

  it("refuses to combine a stale read with the displayed record", () => {
    const html = markup(
      <PurchaseOrderSavedView
        order={order()}
        currency={USD}
        onBack={() => undefined}
        documentInitialState={{ Document: null, Versions: [] }}
        historyInitial={[]}
        reconciliationInitial={{ ...READ, POVersion: 5 }}
      />
    );
    expect(html).toContain(
      "This purchase order changed while its reconciliation was loading."
    );
    expect(html).toContain("Reload");
    expect(html).not.toContain("Saved session · revision 2 · generation 1");
  });
});
