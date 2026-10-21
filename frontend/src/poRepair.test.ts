// Issue #475: the source repair's pure decisions.
//
// The repair panel owns no arithmetic of its own: it starts from the saved
// record's current values, repeats the #471 difference review against the
// candidate revision, and submits the same editable payload a correction uses
// plus the target pair and the required reason. These tests pin the parts that
// decide whether the submit button may run and what exactly it sends.

import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderDTO } from "./purchaseOrder";
import type { PurchaseOrderReviewDTO, PurchaseOrderSourceDTO } from "./poFinalize";
import {
  MAX_REPAIR_REASON_LENGTH,
  buildSourceRepairInput,
  canRepairSource,
  repairConflictWarning,
  repairReasonError,
  sourceRepairSummary,
} from "./poRepair";

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
    PODate: "2026-10-01",
    RequestedDeliveryDate: null,
    UUID: "uuid-475",
    State: "saved",
    QuoteNumber: "1502260001",
    CustomerName: "PT. Buyer",
    BillingAddress: "Jl. Billing A",
    DeliveryAddress: "Jl. Billing A",
    ShipToCode: "ST-1",
    ContactEmail: "buyer@example.com",
    ContactPhone: "0812",
    PONumber: "PO-475",
    ReceivedDate: "2026-10-02",
    Incoterms: "CIF",
    Currency: "USD",
    TaxZone: "FTZ",
    TaxCategory: "",
    AdjustmentReason: "",
    CreatedAt: "2026-10-01T00:00:00Z",
    UpdatedAt: "2026-10-01T00:00:00Z",
    Items: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Priced widget",
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
    ID: 7,
    QuotationID: 1,
    SourceRevision: 0,
    CustomerID: 2,
    PaymentTerms: "45 days",
    DiscountPercent: 0,
    PpnBps: 0,
    SubtotalMinor: 20000,
    DiscountMinor: 0,
    PpnMinor: 0,
    AdjustmentMinor: 0,
    GrandTotalMinor: 20000,
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    SavedAt: "2026-10-02T00:00:00Z",
    SavedBy: 1,
    PdfAbsentConfirmed: true,
    ConfirmationNumber: null,
    ConfirmationAssignedAt: null,
    ConfirmationWarn: false,
    Version: 4,
    ...overrides,
  };
}

function source(overrides: Partial<PurchaseOrderSourceDTO> = {}): PurchaseOrderSourceDTO {
  return {
    QuotationID: 9,
    RevisionNumber: 1,
    QuoteNumber: "1502260009",
    CustomerID: 2,
    CustomerName: "PT. Buyer",
    Currency: "USD",
    PaymentTerms: "45 days",
    Incoterms: "CIF",
    DiscountPercent: 0,
    Lines: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineLabel: "1",
        Description: "Priced widget",
        UoM: "Pc",
        QuantityScaled: 20000,
        UnitPriceMinor: 10000,
        DiscountEnabled: false,
        DiscountPercent: 0,
      },
    ],
    PrintedSubtotalMinor: 20000,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 20000,
    ...overrides,
  };
}

function review(sourceOverrides: Partial<PurchaseOrderSourceDTO> = {}): PurchaseOrderReviewDTO {
  const sourceRow = source(sourceOverrides);
  return {
    Order: order(),
    Source: sourceRow,
    DocumentState: "none",
  };
}

describe("source repair visibility", () => {
  it("is an admin action on a saved record only", () => {
    expect(canRepairSource(order(), true)).toBe(true);
    expect(canRepairSource(order(), false)).toBe(false);
    expect(canRepairSource(order({ State: "draft" }), true)).toBe(false);
  });
});

describe("repair reason", () => {
  it("is required and bounded to the server's 500 characters", () => {
    expect(repairReasonError("   ")).toBe("A source repair needs a reason.");
    expect(repairReasonError("x".repeat(MAX_REPAIR_REASON_LENGTH))).toBeNull();
    expect(repairReasonError("x".repeat(MAX_REPAIR_REASON_LENGTH + 1))).not.toBeNull();
  });
});

describe("buildSourceRepairInput", () => {
  it("starts from the current values and names the target and reason", () => {
    const result = buildSourceRepairInput(
      order(),
      review(),
      USD,
      {},
      [],
      [],
      "the order followed the wrong revision"
    );
    expect(result.error).toBeNull();
    expect(result.input).not.toBeNull();
    expect(result.input?.TargetQuotationID).toBe(9);
    expect(result.input?.TargetRevisionNumber).toBe(1);
    expect(result.input?.ExpectedVersion).toBe(4);
    expect(result.input?.Reason).toBe("the order followed the wrong revision");
    expect(result.input?.PONumber).toBe("PO-475");
    expect(result.input?.Items[0].UnitPriceMinor).toBe(10000);
  });

  it("refuses a blank reason before anything is sent", () => {
    const result = buildSourceRepairInput(order(), review(), USD, {}, [], [], "  ");
    expect(result.input).toBeNull();
    expect(result.error).toBe("A source repair needs a reason.");
  });

  it("blocks an undecided difference and sends the chosen one", () => {
    const differing = review({ PaymentTerms: "30 days" });
    const blocked = buildSourceRepairInput(
      order(),
      differing,
      USD,
      {},
      [],
      [],
      "right revision"
    );
    expect(blocked.input).toBeNull();
    expect(blocked.error).toContain("Payment terms");

    const chosen = buildSourceRepairInput(
      order(),
      differing,
      USD,
      { "header:paymentTerms": "quotation" },
      [],
      [],
      "right revision"
    );
    expect(chosen.error).toBeNull();
    expect(chosen.input?.PaymentTerms).toBe("30 days");
    expect(chosen.input?.Decisions).toEqual([
      { Key: "header:paymentTerms", Choice: "quotation" },
    ]);
  });

  it("carries every explicit acknowledgement the review needs", () => {
    const missing = review();
    missing.Source = source({
      Lines: [
        ...source().Lines,
        {
          QuotationItemNo: 2,
          QuotationSubNo: null,
          LineLabel: "2",
          Description: "Omitted line",
          UoM: "Lot",
          QuantityScaled: 10000,
          UnitPriceMinor: 5000,
          DiscountEnabled: false,
          DiscountPercent: 0,
        },
      ],
    });
    const result = buildSourceRepairInput(
      order(),
      missing,
      USD,
      {},
      ["2"],
      [],
      "right revision"
    );
    expect(result.error).toBeNull();
    expect(result.input?.AcknowledgedMissing).toEqual(["2"]);
  });
});

describe("repairConflictWarning", () => {
  it("names the writer and asks for a reload instead of an overwrite", () => {
    const warning = repairConflictWarning({
      CurrentState: "saved",
      CurrentVersion: 5,
      UpdatedBy: 9,
      UpdatedByName: "Budi",
      UpdatedAt: "2026-10-21T10:15:00+07:00",
    });
    expect(warning).toContain("Budi saved version 5 at 2026-10-21 10:15 +07:00");
    expect(warning).toContain("reload the purchase order");
    expect(warning).not.toContain("Overwriting");
  });
});

describe("sourceRepairSummary", () => {
  it("renders both sides of the link", () => {
    expect(
      sourceRepairSummary({
        PreviousQuoteNumber: "1502260001",
        PreviousSourceRevision: 2,
        PreviousCustomerName: "PT. Buyer",
        NewQuoteNumber: "1502260009",
        NewSourceRevision: 0,
        NewCustomerName: "PT. Buyer",
      })
    ).toBe("Quotation 1502260001 · revision 2 → Quotation 1502260009 · revision 0");
  });

  it("names a customer change explicitly", () => {
    expect(
      sourceRepairSummary({
        PreviousQuoteNumber: "1502260001",
        PreviousSourceRevision: 2,
        PreviousCustomerName: "PT. Buyer",
        NewQuoteNumber: "1502260009",
        NewSourceRevision: 0,
        NewCustomerName: "PT. Other",
      })
    ).toBe(
      "Quotation 1502260001 · revision 2 → Quotation 1502260009 · revision 0 · customer PT. Buyer → PT. Other"
    );
  });
});
