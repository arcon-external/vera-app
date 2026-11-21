import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ReactNode } from "react";
import { PurchaseOrderReportComparisonGrid } from "./PurchaseOrderReportComparisonGrid";

// Issue #642: the comparison grid is presentation only. Its one job is the
// responsive column count and the odd-final-card span, so the suite renders
// supplied card content verbatim and checks the wrapper geometry. It cannot see
// a resolved 1024px media query; the classes are the browser-facing evidence.

function grid(children: ReactNode): string {
  return renderToStaticMarkup(
    <PurchaseOrderReportComparisonGrid>{children}</PurchaseOrderReportComparisonGrid>
  );
}

function wrappers(html: string): string[] {
  return html.split("data-comparison-card").slice(1);
}

describe("PurchaseOrderReportComparisonGrid", () => {
  it("renders one column below lg and two at lg or wider", () => {
    const html = grid(<div>January</div>);
    expect(html).toContain("data-comparison-grid");
    expect(html).toContain("grid-cols-1");
    expect(html).toContain("lg:grid-cols-2");
  });

  it("wraps every supplied child in canonical order, verbatim", () => {
    const html = grid(
      [
        <div key="january">January</div>,
        <div key="february">February</div>,
        <div key="march">March</div>,
      ]
    );
    const cards = wrappers(html);
    expect(cards).toHaveLength(3);
    const january = html.indexOf("January");
    const february = html.indexOf("February");
    const march = html.indexOf("March");
    expect(january).toBeGreaterThan(-1);
    expect(february).toBeGreaterThan(january);
    expect(march).toBeGreaterThan(february);
    expect(html).toContain("<div>January</div>");
    expect(html).toContain("<div>February</div>");
    expect(html).toContain("<div>March</div>");
  });

  it("spans only an odd final card, and only in the two-column layout", () => {
    const three = grid(
      [
        <div key="january">January</div>,
        <div key="february">February</div>,
        <div key="march">March</div>,
      ]
    );
    const cards = wrappers(three);
    expect(cards[0]).not.toContain("data-comparison-last-odd");
    expect(cards[1]).not.toContain("data-comparison-last-odd");
    expect(cards[2]).toContain("data-comparison-last-odd");
    expect(cards[2]).toContain("lg:col-span-2");
  });

  it("leaves an even final card unspanned and spans a lone odd card", () => {
    const four = grid(
      [
        <div key="january">January</div>,
        <div key="february">February</div>,
        <div key="march">March</div>,
        <div key="april">April</div>,
      ]
    );
    expect(four).not.toContain("data-comparison-last-odd");
    expect(four).not.toContain("lg:col-span-2");

    const one = wrappers(
      grid(<div>January</div>)
    );
    expect(one).toHaveLength(1);
    // A lone card is the odd final card and takes the full two-column row.
    expect(one[0]).toContain("data-comparison-last-odd");
    expect(one[0]).toContain("lg:col-span-2");
  });

  it("renders an empty grid when there are no children", () => {
    const html = grid(null);
    expect(html).toContain("data-comparison-grid");
    expect(html).not.toContain("data-comparison-card");
  });
});
