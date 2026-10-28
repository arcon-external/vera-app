import { describe, expect, it } from "vitest";
import fixture from "../../backend/testdata/po_reconciliation_contract.json";
import type { CurrencyInfo } from "./format";
import { comparePurchaseOrder, sourceLabel, type PurchaseOrderSourceDTO } from "./poFinalize";
import {
  blankPurchaseOrderDraft,
  calculatePurchaseOrder,
  formatQuantityScaled,
  formatUnitPriceMinor,
  initialPurchaseOrderEditorState,
} from "./purchaseOrder";

// Ticket 04 binds the former test-only wire declarations to the production
// DTOs in ./poReconciliation: these checks pin shape and fixtures, not an
// as-yet unimplemented commit handler.
import type {
  ReconciliationHistoryEvent as HistoryEvent,
  ReconciliationItem as Item,
  ReconciliationLineIdentity as LineIdentity,
  ReconciliationSession as Session,
} from "./poReconciliation";

interface Discount { Enabled: boolean; Percent: number }
interface TaxBasis { TaxZone: string; TaxType: string; ServiceCategory: string }
interface Feedback { Notice: string | null; Status: string | null; Resolution: string | null }
interface Transition {
  Name: string;
  Field: string;
  Event: string;
  Before: Feedback;
  After: Feedback;
  CreatesItem: boolean;
  WritesPO: boolean;
  BlocksFinalSave: boolean;
}

// Each satisfies is exhaustive over its declaration. A TS-only key addition
// fails tsc; the runtime comparison also catches a fixture-only/Go-only key.
const shapes = {
  Session: {
    ID: true, PurchaseOrderID: true, SourceQuotationID: true, SourceRevision: true,
    POVersion: true, Active: true, ReadOnly: true, CreatedBy: true, CreatedAt: true,
    UpdatedAt: true, LineIdentities: true, Items: true,
  } satisfies Record<keyof Session, true>,
  Item: {
    Key: true, Field: true, Kind: true, LineKey: true, SourceLineRef: true,
    Label: true, POValue: true, SourceValue: true, Status: true, Resolution: true,
    ActorID: true, UpdatedAt: true, History: true,
  } satisfies Record<keyof Item, true>,
  HistoryEvent: {
    Kind: true, SourceLineRef: true, POValue: true, SourceValue: true, Status: true, Resolution: true, ActorID: true, At: true,
  } satisfies Record<keyof HistoryEvent, true>,
  LineIdentity: { Key: true, ID: true, Removed: true } satisfies Record<keyof LineIdentity, true>,
  Discount: { Enabled: true, Percent: true } satisfies Record<keyof Discount, true>,
  TaxBasis: { TaxZone: true, TaxType: true, ServiceCategory: true } satisfies Record<keyof TaxBasis, true>,
  Feedback: { Notice: true, Status: true, Resolution: true } satisfies Record<keyof Feedback, true>,
  Transition: {
    Name: true, Field: true, Event: true, Before: true, After: true,
    CreatesItem: true, WritesPO: true, BlocksFinalSave: true,
  } satisfies Record<keyof Transition, true>,
};

const fields = [
  { ID: "header:paymentTerms", KeyPattern: "header:paymentTerms", Label: "Payment terms", Mode: "commit" },
  { ID: "header:incoterms", KeyPattern: "header:incoterms", Label: "Incoterms", Mode: "commit" },
  { ID: "header:discountPercent", KeyPattern: "header:discountPercent", Label: "Whole-PO discount", Mode: "commit" },
  { ID: "line:description", KeyPattern: "line:{lineKey}:description", Label: "Line {lineNumber} description", Mode: "commit" },
  { ID: "line:uom", KeyPattern: "line:{lineKey}:uom", Label: "Line {lineNumber} unit of measure", Mode: "commit" },
  { ID: "line:quantity", KeyPattern: "line:{lineKey}:quantity", Label: "Line {lineNumber} quantity", Mode: "commit" },
  { ID: "line:unitPrice", KeyPattern: "line:{lineKey}:unitPrice", Label: "Line {lineNumber} unit price", Mode: "commit" },
  { ID: "line:discount", KeyPattern: "line:{lineKey}:discount", Label: "Line {lineNumber} discount", Mode: "commit" },
  { ID: "line:match", KeyPattern: "line:{lineKey}:match", Label: "Line {lineNumber} match", Mode: "line-set" },
  { ID: "source:match", KeyPattern: "source:{sourceLineRef}:match", Label: "Quotation line {sourceLineRef} missing", Mode: "line-set" },
  { ID: "link:taxBasis", KeyPattern: "link:taxBasis", Label: "Tax basis", Mode: "link" },
  { ID: "derived:totals", KeyPattern: "derived:totals", Label: "Totals", Mode: "derived" },
];

function expectKeys(value: object, shape: object) {
  expect(Object.keys(value).sort()).toEqual(Object.keys(shape).sort());
}

const USD: CurrencyInfo = {
  code: "USD", exponent: 2, symbol: "$", position: "before",
  groupSep: ",", decimalSep: ".", symbolSpaced: false,
};

describe("PO reconciliation shared contract (ticket 01)", () => {
  it("pins every field and every wire key, including nested specimens", () => {
    expect(fixture.Version).toBe(1);
    expect(fields).toEqual(fixture.Fields);
    expect(fixture.Enums).toEqual({
      ItemKind: ["difference", "missing_line", "extra_line", "duplicate_match", "tax_basis"],
      ItemStatus: ["unresolved", "resolved"],
      Resolution: ["kept_po", "used_quotation", "values_now_match", "acknowledged_missing", "acknowledged_extra", "match_corrected", "line_removed"],
      Notice: ["empty", "matched"],
    });
    expectKeys(fixture.Shapes, shapes);
    for (const name of Object.keys(shapes) as Array<keyof typeof shapes>) {
      expect(Object.keys(shapes[name]).sort(), name).toEqual([...fixture.Shapes[name]].sort());
    }
    const session: Session = fixture.Session;
    expectKeys(session, shapes.Session);
    for (const line of session.LineIdentities) expectKeys(line, shapes.LineIdentity);
    expect(session.Items.length).toBeGreaterThan(0);
    for (const item of session.Items) {
      expectKeys(item, shapes.Item);
      for (const event of item.History) expectKeys(event, shapes.HistoryEvent);
      if (item.Field === "line:discount" || item.Field === "link:taxBasis") {
        const shape = item.Field === "line:discount" ? shapes.Discount : shapes.TaxBasis;
        for (const value of [item.POValue, item.SourceValue]) {
          expect(value).toBeTypeOf("object");
          if (typeof value === "object" && value !== null) expectKeys(value, shape);
        }
      }
    }
    const discount: Discount = fixture.Values.Discount;
    const taxBasis: TaxBasis = fixture.Values.TaxBasis;
    expectKeys(discount, shapes.Discount);
    expectKeys(taxBasis, shapes.TaxBasis);
    for (const transition of fixture.Transitions) {
      const typed: Transition = transition;
      expectKeys(typed, shapes.Transition);
      expectKeys(typed.Before, shapes.Feedback);
      expectKeys(typed.After, shapes.Feedback);
    }
  });

  for (const tc of fixture.ComparisonCases) {
    it(`runs the real comparison: ${tc.Name}`, () => {
      const source: PurchaseOrderSourceDTO = {
        ...tc.Source,
        Lines: tc.Source.Lines.map((line) => ({
          ...line,
          LineLabel: line.QuotationSubNo === null
            ? String(line.QuotationItemNo) : `${line.QuotationItemNo}.${line.QuotationSubNo}`,
        })),
      };
      const state = initialPurchaseOrderEditorState(blankPurchaseOrderDraft({
        currency: "USD", taxZone: "FTZ", taxType: "Goods", serviceCategory: "",
        receivedDate: "2026-10-28",
      }), USD);
      state.header.paymentTerms = tc.Entered.PaymentTerms;
      state.header.incoterms = tc.Entered.Incoterms;
      state.header.discountPercent = String(tc.Entered.DiscountPercent);
      state.lines = tc.Entered.Lines.map((line, index) => ({
        key: index + 100,
        clientKey: `client:contract-${index}`,
        description: line.Description,
        uom: line.UoM,
        quantity: formatQuantityScaled(line.QuantityScaled),
        unitPrice: formatUnitPriceMinor(USD, line.UnitPriceMinor),
        discountEnabled: line.DiscountEnabled,
        discountPercent: String(line.DiscountPercent),
        quotationItemNo: line.QuotationItemNo,
        quotationSubNo: line.QuotationSubNo,
      }));
      const got = comparePurchaseOrder(state, source, USD);
      const want = tc.Want.Differences.map((difference) => {
        const priceValues = Object.entries(tc.FrontendPriceValues)
          .find(([key]) => key === difference.Key)?.[1];
        return {
          key: difference.Key, label: difference.Label,
          poValue: priceValues?.[0] ?? difference.POValue,
          sourceValue: priceValues?.[1] ?? difference.SourceValue,
        };
      });
      expect(got.differences).toEqual(want);
      expect(got.missing.map(sourceLabel)).toEqual(tc.Want.Missing);
      expect(got.extraLineNumbers).toEqual(tc.Want.ExtraLineNumbers);
      expect(got.duplicateLineNumbers).toEqual(tc.Want.DuplicateLineNumbers);
      expect(got.blocking).toEqual([]);
      // No ComparableKeys is exposed on the current TS result. The
      // all-fields-differ case below pins its whole observable field set.
      if (tc.Name === "every-current-comparable-field-differs") {
        expect(got.differences.map((difference) => difference.key).sort())
          .toEqual([...tc.Want.ComparableKeys].sort());
      }
    });
  }

  for (const tc of fixture.TotalsCases) {
    it(`derives totals without forcing printed evidence: ${tc.Name}`, () => {
      const state = initialPurchaseOrderEditorState(blankPurchaseOrderDraft({
        currency: "USD", taxZone: "FTZ", taxType: "Goods", serviceCategory: "",
        receivedDate: "2026-10-28",
      }), USD);
      state.ppnBps = tc.Input.PpnBps;
      state.header.discountPercent = String(tc.Input.DiscountPercent);
      state.header.adjustment = formatUnitPriceMinor(USD, tc.Input.AdjustmentMinor);
      state.header.adjustmentReason = tc.Input.AdjustmentReason;
      state.lines = tc.Input.Lines.map((line, index) => ({
        key: index, clientKey: `client:candidate-${index}`, description: "Widget", uom: "Pc",
        quotationItemNo: 1, quotationSubNo: null,
        quantity: formatQuantityScaled(line.QuantityScaled),
        unitPrice: formatUnitPriceMinor(USD, line.UnitPriceMinor),
        discountEnabled: line.DiscountEnabled, discountPercent: String(line.DiscountPercent),
      }));
      const got = calculatePurchaseOrder(state, USD);
      expect(got.error).toBeNull();
      expect(got.totals).not.toBeNull();
      expect({ ...got.totals, printedDifferenceMinor: tc.PrintedGrandTotalMinor - got.totals!.grandTotalMinor })
        .toEqual(tc.Want);
    });
  }

  it("keeps a complete planned transition catalogue without claiming it is implemented", () => {
    const states = new Set<string>();
    for (const transition of fixture.Transitions) {
      for (const value of Object.values(transition.After)) if (value !== null) states.add(value);
      if (transition.After.Notice === "matched") expect(transition.After.Status).toBeNull();
    }
    expect([...states].sort()).toEqual([
      "empty", "matched", "unresolved", "resolved", "kept_po", "used_quotation",
      "acknowledged_missing", "acknowledged_extra", "match_corrected", "line_removed", "values_now_match",
    ].sort());
    for (const tc of fixture.IdentityCases) expect(tc.AfterKey, tc.Name).toBe(tc.BeforeKey);
    for (const tc of fixture.GateCases) expect(tc.CanSave, tc.Name).toBe(tc.Blockers.length === 0);
  });
});
