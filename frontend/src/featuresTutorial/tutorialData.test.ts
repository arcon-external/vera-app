import { describe, expect, it } from "vitest";
import type { CurrencyInfo } from "../format";
import { initialPurchaseOrderEditorState } from "../purchaseOrder";
import { groupSourceItems, sourceItemApplied } from "../poSourceItems";
import {
  TUTORIAL_CUSTOMER,
  TUTORIAL_QUOTE_NUMBER,
  TUTORIAL_SELECTOR_PAGE,
  TUTORIAL_SELECTOR_VALIDATION,
  tutorialOrder,
  tutorialReview,
  tutorialSourceLines,
} from "./tutorialData";

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

describe("Features Tutorial practice data", () => {
  it("contains every ghost shape in one practice order", () => {
    const order = tutorialOrder(IDR);
    const state = initialPurchaseOrderEditorState(order, IDR);
    const items = groupSourceItems(tutorialSourceLines());

    expect(items.map((item) => item.itemNo)).toEqual([1, 2, 3, 4]);
    expect(items[1].subItems.map((sub) => sub.subNo)).toEqual([1, 2]);
    expect(items[1].subItems[0].descriptionLines).toEqual([
      "Tutorial scope A1",
      "Tutorial scope A2",
    ]);

    const applied = items.map((item) =>
      sourceItemApplied(item, state.lines, IDR)
    );
    // Exact item, split item with a changed part, plain item with a changed
    // quantity, and an item that never reached the order.
    expect(applied).toEqual([true, false, false, false]);
    expect(state.lines).toHaveLength(5);
    expect(state.lines.filter((line) => line.groupKey === "q:2")).toHaveLength(2);
    expect(state.lines.some((line) => line.quotationItemNo === null)).toBe(true);
  });

  it("builds a review whose source is the practice revision", () => {
    const review = tutorialReview(IDR);
    expect(review.Source.QuoteNumber).toBe(TUTORIAL_QUOTE_NUMBER);
    expect(review.Source.CustomerName).toBe(TUTORIAL_CUSTOMER);
    expect(review.Source.PaymentTerms).toBe("30 days");
    expect(review.Source.Incoterms).toBe("DAP");
    expect(review.Source.Lines).toHaveLength(5);
    expect(review.Order.PaymentTerms).toBe("45 days");
    expect(review.Order.Incoterms).toBe("DAP");
    expect(review.DocumentState).toBe("none");
  });

  it("exposes exactly one practice quotation to the tutorial selector", () => {
    expect(TUTORIAL_SELECTOR_PAGE.total).toBe(1);
    expect(TUTORIAL_SELECTOR_PAGE.quotations).toHaveLength(1);
    expect(TUTORIAL_SELECTOR_PAGE.quotations[0]).toMatchObject({
      id: -1,
      quoteNumber: TUTORIAL_QUOTE_NUMBER,
      customerName: TUTORIAL_CUSTOMER,
      latestRevision: 0,
      issuedRevisionCount: 1,
      activePO: null,
    });
    expect(TUTORIAL_SELECTOR_VALIDATION.valid).toBe(true);
  });
});
