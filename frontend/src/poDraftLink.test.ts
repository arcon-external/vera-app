import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "./format";
import type { PurchaseOrderReviewDTO } from "./poFinalize";
import {
  draftLinkConflictWarning,
  draftLinkCurrencyRemedy,
  draftLinkTaxChoiceError,
  draftTaxBasisLabel,
  formatPpnBps,
  planDraftLink,
  type PurchaseOrderDraftLinkReviewDTO,
  type PurchaseOrderDraftTaxChoice,
} from "./poDraftLink";
import {
  blankPurchaseOrderDraft,
  initialPurchaseOrderEditorState,
  type PurchaseOrderDTO,
} from "./purchaseOrder";

// Issue #497: the draft link's pure decisions. The comparison and the "Use
// Quotation" application are the finalize implementation; these pin the link's
// own rules -- eligibility, the tax choice, the currency remedy and the exact
// payload that follows the applied draft save.

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

function draft(overrides: Partial<PurchaseOrderDTO> = {}): PurchaseOrderDTO {
  return {
    ...blankPurchaseOrderDraft({
      currency: "IDR",
      taxZone: "FTZ",
      taxType: "Goods",
      serviceCategory: "",
      receivedDate: "2026-10-22",
      paymentTerms: "50% advance",
      incoterms: "EXW",
    }),
    ID: 41,
    UUID: "uuid-497",
    Version: 3,
    PONumber: "PO-497",
    Items: [
      {
        QuotationItemNo: null,
        QuotationSubNo: null,
        LineNumber: 1,
        Description: "Manual line",
        QuantityScaled: 10000,
        UoM: "Pc",
        UnitPriceMinor: 100,
        DiscountEnabled: false,
        DiscountPercent: 0,
        GrossMinor: 0,
        DiscountMinor: 0,
        NetMinor: 0,
      },
    ],
    ...overrides,
  };
}

function source(overrides: Partial<PurchaseOrderReviewDTO["Source"]> = {}) {
  return {
    QuotationID: 12,
    RevisionNumber: 0,
    QuoteNumber: "1502260001",
    CustomerID: 2,
    CustomerName: "PT. Source",
    Currency: "IDR",
    PaymentTerms: "50% advance",
    Incoterms: "EXW",
    DiscountPercent: 0,
    Lines: [
      {
        QuotationItemNo: 1,
        QuotationSubNo: null,
        LineLabel: "1",
        Description: "Source line",
        UoM: "Pc",
        QuantityScaled: 10000,
        UnitPriceMinor: 100,
        DiscountEnabled: false,
        DiscountPercent: 0,
      },
    ],
    PrintedSubtotalMinor: 100,
    PrintedDiscountMinor: 0,
    PrintedPpnMinor: 0,
    PrintedGrandTotalMinor: 100,
    ...overrides,
  };
}

function review(
  overrides: Partial<PurchaseOrderDraftLinkReviewDTO> = {}
): PurchaseOrderDraftLinkReviewDTO {
  return {
    Order: draft(),
    Source: source(),
    DocumentState: "none",
    DraftTax: {
      TaxZone: "FTZ",
      TaxType: "Goods",
      ServiceCategory: "",
      TaxCategory: "",
      PpnBps: 0,
    },
    SourceTax: {
      TaxZone: "FTZ",
      TaxType: "Goods",
      ServiceCategory: "",
      TaxCategory: "",
      PpnBps: 0,
    },
    TaxChoiceRequired: false,
    ...overrides,
  };
}

describe("draft link tax vocabulary", () => {
  it("renders the basis and the PPN rate", () => {
    expect(
      draftTaxBasisLabel({
        TaxZone: "NonFTZ",
        TaxType: "Goods",
        ServiceCategory: "",
        TaxCategory: "",
        PpnBps: 1100,
      })
    ).toBe("NonFTZ / Goods · 11% PPN");
    expect(formatPpnBps(175)).toBe("1.75%");
    expect(
      draftTaxBasisLabel({
        TaxZone: "FTZ",
        TaxType: "Goods",
        ServiceCategory: "",
        TaxCategory: "",
        PpnBps: 0,
      })
    ).toBe("FTZ / Goods · no PPN");
  });

  it("requires a choice only when the bases differ, and never sends an unknown one", () => {
    const differing = review({
      TaxChoiceRequired: true,
      SourceTax: { ...review().SourceTax, TaxZone: "NonFTZ", PpnBps: 1100 },
    });
    expect(draftLinkTaxChoiceError(differing, "")).toContain("Choose");
    expect(draftLinkTaxChoiceError(differing, "keep")).toBeNull();
    expect(draftLinkTaxChoiceError(differing, "source")).toBeNull();
    expect(draftLinkTaxChoiceError(review(), "")).toBeNull();
    expect(
      draftLinkTaxChoiceError(review(), "maybe" as PurchaseOrderDraftTaxChoice)
    ).toContain("Choose");
  });

  it("words the currency remedy for every draft that can still edit its basis", () => {
    const never = draftLinkCurrencyRemedy("never", "IDR", "USD");
    expect(never).toContain("Back without linking");
    expect(never).toContain("Source basis");
    expect(never).toContain("never converted");
    // A formerly-linked draft is still before a new link: same remedy, no
    // conversion.
    const former = draftLinkCurrencyRemedy("former", "IDR", "USD");
    expect(former).toContain("Back without linking");
    expect(former).toContain("Source basis");
    expect(former).toContain("then link again");
    expect(former).toContain("never converted");
    // A live draft is frozen: the remedy is another revision.
    const live = draftLinkCurrencyRemedy("live", "IDR", "USD");
    expect(live).toContain("Choose a quotation in IDR");
    expect(live).not.toContain("Source basis");
    expect(live).toContain("never converted");
  });
});

describe("planDraftLink", () => {
  it("builds the applied draft save and the link payload from the review", () => {
    const order = draft();
    const state = initialPurchaseOrderEditorState(order, IDR);
    const candidate = review({
      Source: source({ PaymentTerms: "45 days" }),
    });
    const plan = planDraftLink(
      state,
      candidate,
      IDR,
      { "header:paymentTerms": "po" },
      ["1"],
      [1],
      ""
    );
    expect(plan.error).toBeNull();
    expect(plan.update).not.toBeNull();
    expect(plan.update?.PaymentTerms).toBe("50% advance");
    expect(plan.update?.Items).toHaveLength(1);
    expect(plan.input).toEqual({
      ID: 41,
      ExpectedVersion: 3,
      TargetQuotationID: 12,
      TargetRevisionNumber: 0,
      Decisions: [{ Key: "header:paymentTerms", Choice: "po" }],
      AcknowledgedMissing: ["1"],
      AcknowledgedExtra: [1],
      TaxChoice: "keep",
    });
  });

  it("applies a Use Quotation choice to the values the draft save carries", () => {
    const state = initialPurchaseOrderEditorState(draft(), IDR);
    const candidate = review({
      Source: source({ PaymentTerms: "45 days" }),
    });
    const plan = planDraftLink(
      state,
      candidate,
      IDR,
      { "header:paymentTerms": "quotation" },
      ["1"],
      [1],
      ""
    );
    expect(plan.error).toBeNull();
    expect(plan.update?.PaymentTerms).toBe("45 days");
    expect(plan.input?.Decisions).toEqual([
      { Key: "header:paymentTerms", Choice: "quotation" },
    ]);
  });

  it("refuses an undecided difference, a missing tax choice and a currency mismatch", () => {
    const state = initialPurchaseOrderEditorState(draft(), IDR);

    const undecided = planDraftLink(
      state,
      review({ Source: source({ PaymentTerms: "45 days" }) }),
      IDR,
      {},
      ["1"],
      [1],
      ""
    );
    expect(undecided.input).toBeNull();
    expect(undecided.error).toContain("needs a choice");

    const taxRequired = review({
      TaxChoiceRequired: true,
      SourceTax: { ...review().SourceTax, TaxZone: "NonFTZ", PpnBps: 1100 },
    });
    const noTax = planDraftLink(state, taxRequired, IDR, {}, ["1"], [1], "");
    expect(noTax.input).toBeNull();
    expect(noTax.error).toContain("tax basis");

    const mismatched = review({ Source: source({ Currency: "USD" }) });
    const currency = planDraftLink(state, mismatched, IDR, {}, ["1"], [1], "");
    expect(currency.input).toBeNull();
    expect(currency.error).toContain("never compared");
  });

  it("links a ten-line draft once every line is explicitly matched", () => {
    const items = Array.from({ length: 10 }, (_, index) => ({
      QuotationItemNo: index + 1,
      QuotationSubNo: null,
      LineNumber: index + 1,
      Description: `Source line ${index + 1}`,
      QuantityScaled: 10000,
      UoM: "Pc",
      UnitPriceMinor: 100,
      DiscountEnabled: false,
      DiscountPercent: 0,
      GrossMinor: 0,
      DiscountMinor: 0,
      NetMinor: 0,
    }));
    const state = initialPurchaseOrderEditorState(
      draft({ Items: items, PONumber: "TEN-LINE" }),
      IDR
    );
    const candidate = review({
      Source: source({
        Lines: items.map((item, index) => ({
          QuotationItemNo: index + 1,
          QuotationSubNo: null,
          LineLabel: String(index + 1),
          Description: item.Description,
          UoM: item.UoM,
          QuantityScaled: item.QuantityScaled,
          UnitPriceMinor: item.UnitPriceMinor,
          DiscountEnabled: false,
          DiscountPercent: 0,
        })),
      }),
    });
    const plan = planDraftLink(state, candidate, IDR, {}, [], [], "");
    expect(plan.error).toBeNull();
    expect(plan.update?.Items).toHaveLength(10);
    expect(plan.update?.Items.map((item) => item.QuotationItemNo)).toEqual([
      1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
    ]);
    expect(plan.input?.AcknowledgedMissing).toEqual([]);
    expect(plan.input?.AcknowledgedExtra).toEqual([]);
  });
});

describe("draftLinkConflictWarning", () => {
  it("names the writer and says the link was not applied", () => {
    const warning = draftLinkConflictWarning({
      CurrentState: "draft",
      CurrentVersion: 9,
      UpdatedBy: 2,
      UpdatedByName: "Ada Admin",
      UpdatedAt: "2026-10-22T10:00:00Z",
    });
    expect(warning).toContain("Ada Admin");
    expect(warning).toContain("version 9");
    expect(warning).toContain("link was not applied");
  });
});
