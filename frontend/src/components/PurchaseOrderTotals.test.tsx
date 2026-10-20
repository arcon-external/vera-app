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
  discountMinor: 16168,
  afterDiscountMinor: 307195,
  ppnMinor: 33791,
  adjustmentMinor: -250,
  grandTotalMinor: 340736,
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
      discountPercent={5}
      ppnBps={1100}
      adjustmentReason="Round to the printed total"
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
    expect(html).toContain("Whole-PO discount (5%)");
    expect(html).toContain("$-161.68");
    expect(html).toContain("Net after discount");
    expect(html).toContain("$3,071.95");
    expect(html).toContain("PPN (11%)");
    expect(html).toContain("$337.91");
    expect(html).toContain("Adjustment");
    expect(html).toContain("$-2.50");
    expect(html).toContain("Grand total");
    expect(html).toContain("$3,407.36");
  });

  it("shows the adjustment's reason beside its amount", () => {
    expect(render()).toContain("Round to the printed total");
  });

  it("hides PPN, discounts and the adjustment when they do not apply", () => {
    const html = render({
      totals: {
        grossMinor: 1000,
        rowDiscountMinor: 0,
        subtotalMinor: 1000,
        discountMinor: 0,
        afterDiscountMinor: 1000,
        ppnMinor: 0,
        adjustmentMinor: 0,
        grandTotalMinor: 1000,
      },
      discountPercent: 0,
      ppnBps: 0,
      adjustmentReason: "",
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
});
