import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CustomerPreview, InternalBreakdown } from "../preview";
import { ReviewPanel } from "./ReviewPanel";

// Regression: go marshals a nil slice to JSON null, and this panel read .length
// on it. The throw unmounted the whole tree and left a blank white window right
// after Prepare -- the most destructive failure the app has had. The API now
// sends [], and these tests hold the client to surviving null anyway, because a
// crash here costs the user their whole screen, not one field.

function customer(): CustomerPreview {
  return {
    Items: [
      {
        No: 1,
        Description: "Control panel replacement",
        Qty: 2,
        UoM: "Lot",
        DisplayOriginalUnitPrice: "15.000.000",
        DisplayOriginalTotal: "30.000.000",
      },
    ],
    DisplayOriginalGrandTotal: "30.000.000",
    DisplayGrandTotal: "30.000.000",
  } as unknown as CustomerPreview;
}

function internal(subItems: unknown): InternalBreakdown {
  return {
    Items: [
      {
        No: 1,
        Description: "Control panel replacement",
        Qty: 2,
        SubItems: subItems,
        MarginEnabled: false,
        MarginPercent: 0,
        DiscountEnabled: false,
        DiscountPercent: 0,
        DisplayMarginalizedUnit: "15.000.000",
        DisplayMarginalizedTotal: "30.000.000",
        DisplayMarginAmount: "0",
        DisplayMarginBlended: "0%",
        ListUnitPrice: 15000000,
      },
    ],
    DisplayOriginalGrand: "30.000.000",
  } as unknown as InternalBreakdown;
}

function render(subItems: unknown): string {
  return renderToString(
    <ReviewPanel customer={customer()} internal={internal(subItems)} quotationHTML="" />
  );
}

describe("ReviewPanel", () => {
  it("renders when the API sends null for a line's sub-items", () => {
    expect(() => render(null)).not.toThrow();
    expect(render(null)).toContain("Control panel replacement");
  });

  it("renders an empty array and a populated array alike", () => {
    expect(() => render([])).not.toThrow();

    const withSubs = [
      {
        SubNo: "1.1",
        Description: "Panel beds",
        Details: null, // the same nil-slice shape, one level down
        Qty: 2,
        UoM: "Pc",
        DisplayOriginalUnitPrice: "8.500.000",
        DisplayOriginalTotal: "17.000.000",
        DisplayMarginalizedUnit: "10.200.000",
        DisplayMarginalizedTotal: "20.400.000",
        SubSubItems: [],
      },
    ];
    expect(() => render(withSubs)).not.toThrow();
    expect(render(withSubs)).toContain("Panel beds");
  });

  it("still shows the composition when sub-items are present", () => {
    const html = render([
      {
        SubNo: "1.1",
        Description: "Panel beds",
        Details: ["Mounted and aligned"],
        Qty: 1,
        UoM: "Pc",
        DisplayOriginalUnitPrice: "1.000",
        DisplayOriginalTotal: "1.000",
        DisplayMarginalizedUnit: "1.200",
        DisplayMarginalizedTotal: "1.200",
      },
    ]);
    expect(html).toContain("Panel beds");
    expect(html).toContain("Mounted and aligned");
  });
});
