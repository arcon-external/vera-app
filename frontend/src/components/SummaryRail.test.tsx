import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { SummaryRail, computePricing, type PricedItem } from "./SummaryRail";

// Issue #622: the Quotation editor computes pricing once and hands the same
// object to both the hero and this rail, so the two figures cannot disagree.
// These cases pin the rail's side of that contract: a supplied `pricing` is
// authoritative, and the rail still prices by itself when nobody supplies one.
//
// What this cannot show: the App wiring (the Quotation DOM suite drives the
// real editor) and pixel placement.

const LINE: PricedItem = {
  qty: "2",
  unitPrice: "1000",
  marginEnabled: false,
  marginPercent: "0",
  discountEnabled: false,
  discountPercent: "0",
};

describe("SummaryRail pricing", () => {
  it("prices the lines itself when no pricing is supplied", () => {
    const html = renderToString(
      <SummaryRail
        items={[LINE]}
        discountPercent={0}
        taxZone="FTZ"
        taxType="Goods"
        serviceCategory=""
      />
    );

    expect(html).toContain("Rp2.000");
  });

  it("renders the caller's pricing instead of computing a second one", () => {
    // The line would price at Rp1.000.000 on its own. The injected object says
    // Rp777, so a rail that recomputed would print the other figure.
    const supplied = {
      ...computePricing([], 0, "FTZ", "Goods", ""),
      costSubtotal: 777,
      sellSubtotal: 777,
      customerGrand: 777,
      grandAfterTax: 777,
      netProfit: 777,
      profitBeforeTax: 777,
    };
    const html = renderToString(
      <SummaryRail
        items={[{ ...LINE, qty: "1", unitPrice: "1000000" }]}
        discountPercent={0}
        taxZone="FTZ"
        taxType="Goods"
        serviceCategory=""
        pricing={supplied}
      />
    );

    expect(html).toContain("Rp777");
    expect(html).not.toContain("Rp1.000.000");
  });
});
