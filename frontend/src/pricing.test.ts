import { computePricing } from "./components/SummaryRail";
import type { PricedItem } from "./components/SummaryRail";
import { applyDiscount, applyMargin, MAX_DISCOUNT_PERCENT } from "./format";

// The live summary rail duplicates the backend's pricing in TypeScript so the
// editor can show totals before Prepare runs. Two implementations of one rule
// is the setup for drift, and drift here is a number the customer is quoted --
// so these cases are the ones Go asserts in TestRowDiscountFollowsTheConfirmedFlow
// and TestProfitFollowsTheConfirmedFlow, restated for the client. If one side
// changes a rule, the other side fails here rather than quietly disagreeing.

function line(
  cost: number,
  qty: number,
  margin: number,
  discount: number,
  discountOn = false
): PricedItem {
  return {
    qty: String(qty),
    unitPrice: String(cost),
    marginEnabled: margin > 0,
    marginPercent: String(margin),
    discountEnabled: discountOn,
    discountPercent: String(discount),
  };
}

const FTZ = { taxZone: "FTZ", taxType: "Goods", serviceCategory: "" };

describe("applyDiscount", () => {
  it("is off until it is switched on", () => {
    expect(applyDiscount(100000, 50, false)).toBe(100000);
  });

  it("truncates, exactly as the backend does", () => {
    // 108000 * 33 / 100 = 35640 exactly, but 100000 * 33 / 100 truncates.
    // The point is that both sides truncate at the same place.
    expect(applyDiscount(100000, 33, true)).toBe(67000);
    expect(applyMargin(333, 50, true)).toBe(499);
  });
});

describe("computePricing mirrors the backend", () => {
  it("margin then row discount, with the cost side untouched", () => {
    // cost 100.000 x 2, margin 20% -> 120.000 list, row discount 10% -> 108.000.
    const p = computePricing([line(100000, 2, 20, 10, true)], 0, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    expect(p.sellSubtotal).toBe(216000);
    expect(p.costSubtotal).toBe(200000);
    expect(p.sellGrand).toBe(216000);
    // Profit reads the sell grand, never the customer total, and the cost side
    // is not discounted -- the rule from issue #270.
    expect(p.netProfit).toBe(16000);
  });

  it("discount with no margin", () => {
    const p = computePricing([line(100000, 1, 0, 10, true)], 0, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    expect(p.sellSubtotal).toBe(90000);
    expect(p.netProfit).toBe(-10000);
  });

  it("discount below cost is a loss, not a clamp and not a wrap", () => {
    const p = computePricing([line(100000, 1, 10, 30, true)], 0, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    // 110.000 list, 30% off -> 77.000, against a 100.000 cost.
    expect(p.sellSubtotal).toBe(77000);
    expect(p.netProfit).toBe(-23000);
  });

  it("a hundred percent discount is a free line", () => {
    const p = computePricing([line(50000, 4, 0, 100, true)], 0, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    expect(p.sellSubtotal).toBe(0);
    expect(p.netProfit).toBe(-200000);
  });

  it("the whole-quotation discount applies on top of the per-row one", () => {
    const items = [line(100000, 2, 20, 10, true)];
    const withoutOverall = computePricing(items, 0, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    const withOverall = computePricing(items, 5, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    // 216.000 less 5% = 205.200, against an undiscounted 200.000 cost.
    expect(withOverall.sellGrand).toBe(205200);
    expect(withOverall.sellDiscount).toBe(10800);
    expect(withOverall.netProfit).toBe(5200);
    expect(withOverall.sellSubtotal).toBe(withoutOverall.sellSubtotal);
  });

  it("an undiscounted quotation is unchanged by this feature", () => {
    // The regression guard: a quotation with no row discounts has to price
    // exactly as it did before per-row discounts existed.
    const p = computePricing([line(15000000, 2, 20, 0)], 0, FTZ.taxZone, FTZ.taxType, FTZ.serviceCategory);
    expect(p.sellSubtotal).toBe(36000000);
    expect(p.sellGrand).toBe(36000000);
    expect(p.netProfit).toBe(6000000);
  });

  it("excludes PPN from both profits", () => {
    // 11% PPN on non-FTZ goods is collected for the state. It is on the
    // customer total and on neither profit figure.
    const p = computePricing([line(100000, 1, 0, 0)], 0, "NonFTZ", "Goods", "");
    expect(p.ppnAmount).toBe(11000);
    expect(p.customerGrand).toBe(111000);
    expect(p.netProfit).toBe(0);
    expect(p.profitBeforeTax).toBe(0);
  });

  it("takes PPH22 out of nett only", () => {
    const p = computePricing([line(100000, 1, 0, 0)], 0, "NonFTZ", "Services", "NonConstruction");
    expect(p.taxAmount).toBe(2000);
    expect(p.profitBeforeTax).toBe(0);
    expect(p.netProfit).toBe(-2000);
    expect(p.grandAfterTax).toBe(98000);
  });
});

describe("MAX_DISCOUNT_PERCENT", () => {
  it("allows a free line and nothing past it", () => {
    // Mirrors MaxDiscountPercent in the backend. Written down twice is a limit
    // that eventually disagrees with itself, so the parity is asserted.
    expect(MAX_DISCOUNT_PERCENT).toBe(100);
  });
});