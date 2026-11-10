import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import type { PurchaseOrderTotals as PurchaseOrderTotalsValue } from "../purchaseOrder";
import { PurchaseOrderTotals } from "./PurchaseOrderTotals";

// Issue #470: the totals panel's markup, rendered without a DOM. The pure
// suite owns the arithmetic; this suite proves every stage is on screen, that
// PPN prints separately only when charged, and that the source document's
// printed figures keep their own group.
//
// What it cannot prove: the real page at 360/1280 (snapshot harness) and any
// interaction (the editor DOM suite).

const USD: CurrencyInfo = {
  code: "USD",
  exponent: 2,
  symbol: "$",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: false,
};

const TOTALS: PurchaseOrderTotalsValue = {
  grossMinor: 354625,
  rowDiscountMinor: 31262,
  subtotalMinor: 323363,
  discountMinor: 0,
  afterDiscountMinor: 323363,
  ppnMinor: 33791,
  grandTotalMinor: 357154,
};

const PRINTED = {
  subtotalMinor: 325000,
  discountMinor: 0,
  ppnMinor: 35750,
  grandTotalMinor: 360750,
};

function render(overrides: Partial<Parameters<typeof PurchaseOrderTotals>[0]> = {}) {
  return renderToString(
    <PurchaseOrderTotals
      totals={TOTALS}
      currency={USD}
      ppnBps={1100}
      printedTotals={PRINTED}
      sourceRevision={2}
      {...overrides}
    />
  ).replace(/<!-- -->/g, "");
}

describe("PurchaseOrderTotals (issue #470)", () => {
  it("shows every calculation stage with its money", () => {
    const html = render();
    expect(html).toContain("Line totals");
    expect(html).toContain("$3,546.25");
    expect(html).toContain("Row discounts");
    expect(html).toContain("$-312.62");
    expect(html).toContain("Subtotal");
    expect(html).toContain("$3,233.63");
    // The whole-PO discount and the signed adjustment are retired (#573).
    expect(html).not.toContain("Whole-PO discount");
    expect(html).not.toContain("Net after discount");
    expect(html).not.toContain("Adjustment");
    expect(html).toContain("PPN (11%)");
    expect(html).toContain("$337.91");
    expect(html).toContain("Grand total");
    expect(html).toContain("$3,571.54");
  });

  it("hides PPN and row discounts when they do not apply", () => {
    const html = render({
      totals: {
        grossMinor: 1000,
        rowDiscountMinor: 0,
        subtotalMinor: 1000,
        discountMinor: 0,
        afterDiscountMinor: 1000,
        ppnMinor: 0,
        grandTotalMinor: 1000,
      },
      ppnBps: 0,
    });
    expect(html).not.toContain("PPN (0%)");
    expect(html).not.toContain("Charged on top");
    expect(html).not.toContain("Row discounts");
    expect(html).not.toContain("Whole-PO discount");
    expect(html).not.toContain("Adjustment");
    expect(html).toContain("Grand total");
    expect(html).toContain("$10.00");
  });

  it("keeps the source's printed totals in their own group", () => {
    const html = render();
    expect(html).toContain("Printed on quotation revision 2");
    expect(html).toContain("Printed subtotal");
    expect(html).toContain("$3,250.00");
    expect(html).toContain("Printed PPN");
    expect(html).toContain("$357.50");
    expect(html).toContain("Printed grand total");
    expect(html).toContain("$3,607.50");
    expect(html).toContain("kept as printed");
  });

  it("shows a pending state instead of fabricated zeros", () => {
    const html = render({ totals: null });
    expect(html).toContain("Calculation pending");
    expect(html).toContain("Fix the highlighted field");
    expect(html).not.toContain("$0.00");
    expect(html).not.toContain("Grand total");
    // The source document's evidence is independent of our arithmetic and
    // stays visible while the calculation is pending.
    expect(html).toContain("Printed grand total");
    expect(html).toContain("$3,607.50");
  });

  it("marks printed totals unavailable when a value cannot be exact", () => {
    const html = render({
      printedTotals: {
        subtotalMinor: Number.MAX_SAFE_INTEGER + 1,
        discountMinor: 0,
        ppnMinor: 0,
        grandTotalMinor: Number.MAX_SAFE_INTEGER + 1,
      },
    });
    expect(html).toContain("Printed totals unavailable");
    expect(html).not.toContain("$90,071,992,547,409.92");
  });

  it("says when a display-only aggregate cannot be named exactly", () => {
    // The Judge's two-line case: each line is exact, the subtotal is zero, and
    // the aggregate gross/row-discount figures exceed 2^53-1. They are never
    // stored or sent, so the panel says so instead of printing an inexact
    // number or refusing the order.
    const html = render({
      totals: {
        ...TOTALS,
        grossMinor: null,
        rowDiscountMinor: null,
      },
    });
    expect(html).toContain("Above the exact display range");
    expect(html).not.toContain("$0.00");
    expect(html).toContain("$3,233.63");
    expect(html).toContain("$3,571.54");
  });
});
