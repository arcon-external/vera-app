import { describe, expect, it } from "vitest";
import { createElement } from "react";
import { renderToString } from "react-dom/server";
import type { CurrencyInfo } from "./format";
import { formatMoney } from "./format";
import { PurchaseOrderReportsView } from "./components/PurchaseOrderReportsPage";
import { purchaseOrderReportMaxFixture } from "./poReportFixtures";
import {
  REPORT_KPI_GRID_CLASS,
  REPORT_KPI_LAYOUT,
  REPORT_KPI_VALUE_CLASS,
  formatReportBps,
  formatReportCount,
  reportKpiCellWidth,
} from "./poReportPage";

// Issue #630 review: the KPI strip must show the complete money figure at
// 360, 390 and 1280, including JavaScript's largest exact amount and a
// trailing-symbol currency. A class-name assertion cannot prove that, so this
// guard measures the widest formatted strings against the real cell widths at
// the named breakpoints. The glyph budget is deliberately conservative:
// Inter/digits average below 0.56em per character, so a string that passes
// here has room in the browser as well. The assembled screenshots are the
// visual half of the same evidence.
//
// The maximum-percentage candidate below is synthetic width evidence only: the
// assembled max fixture carries the honest 0.00% of a near-zero margin against
// capital near MAX_SAFE_INTEGER. A synthetic figure may be used to measure the
// cell, never to stand in for a server calculation.

const MAX = Number.MAX_SAFE_INTEGER;
const MAX_GLYPH_EM = 0.56;

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
const EUR: CurrencyInfo = {
  code: "EUR",
  exponent: 2,
  symbol: "€",
  position: "after",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: true,
};
const AED: CurrencyInfo = {
  code: "AED",
  exponent: 2,
  symbol: "AED",
  position: "before",
  groupSep: ",",
  decimalSep: ".",
  symbolSpaced: true,
};
const VND: CurrencyInfo = {
  code: "VND",
  exponent: 0,
  symbol: "₫",
  position: "after",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: true,
};

const CATALOGUE = [IDR, USD, EUR, AED, VND];

function widestKpiText(): string {
  const candidates = [
    ...CATALOGUE.flatMap((entry) => [
      formatMoney(entry, MAX),
      formatMoney(entry, -MAX),
    ]),
    formatReportBps(MAX),
    formatReportBps(-MAX),
    formatReportCount(MAX),
  ];
  return candidates.reduce((widest, value) =>
    value.length > widest.length ? value : widest
  );
}

function estimatedWidth(text: string, fontPx: number): number {
  return text.length * fontPx * MAX_GLYPH_EM;
}

describe("Reports KPI width guard (issue #630)", () => {
  it("fits the widest exact figure in every named viewport's cell", () => {
    for (const layout of REPORT_KPI_LAYOUT) {
      const cellWidth = reportKpiCellWidth(layout.viewport, layout.columns);
      const textWidth = estimatedWidth(widestKpiText(), layout.valueFontPx);
      expect(
        textWidth,
        `${widestKpiText()} at ${layout.viewport}px (${layout.columns} columns, ${layout.valueFontPx}px) is ${textWidth.toFixed(1)}px in a ${cellWidth.toFixed(1)}px cell`
      ).toBeLessThanOrEqual(cellWidth - 2);
    }
  });

  it("uses one column until the two-column breakpoint, then never squeezes a cell", () => {
    expect(REPORT_KPI_LAYOUT.map((entry) => entry.columns)).toEqual([
      1, 1, 2, 3, 5,
    ]);
    expect(reportKpiCellWidth(360, 1)).toBe(280);
    expect(reportKpiCellWidth(390, 1)).toBe(310);
    expect(reportKpiCellWidth(1280, 5)).toBeCloseTo(222.4, 1);
  });

  it("keeps the exported classes on the same layout the guard measured", () => {
    expect(REPORT_KPI_GRID_CLASS).toContain("grid-cols-1");
    expect(REPORT_KPI_GRID_CLASS).toContain("sm:grid-cols-2");
    expect(REPORT_KPI_GRID_CLASS).toContain("lg:grid-cols-3");
    expect(REPORT_KPI_GRID_CLASS).toContain("xl:grid-cols-5");
    expect(REPORT_KPI_VALUE_CLASS).toContain("whitespace-nowrap");
    expect(REPORT_KPI_VALUE_CLASS).toContain("tnum");
    expect(REPORT_KPI_VALUE_CLASS).toContain("sm:text-[17px]");
    expect(REPORT_KPI_VALUE_CLASS).toContain("xl:text-lead");
  });

  it("renders the complete maximum amount, not a rounded stand-in", () => {
    const markup = renderToString(
      createElement(PurchaseOrderReportsView, {
        filters: {
          preset: "custom",
          period: null,
          from: "2026-01-01",
          to: "2026-12-31",
          bucket: "month",
          category: "all",
          currency: "IDR",
        },
        report: purchaseOrderReportMaxFixture(),
        loading: false,
        stale: false,
        error: "",
        validation: "",
        catalogue: CATALOGUE,
        exporting: false,
        canExport: false,
        onPresetChange: () => undefined,
        onPeriodChange: () => undefined,
        onFromChange: () => undefined,
        onToChange: () => undefined,
        onBucketChange: () => undefined,
        onCategoryChange: () => undefined,
        onCurrencyChange: () => undefined,
        onRetry: () => undefined,
        onExport: () => undefined,
      })
    );
    expect(markup).toContain("Rp9.007.199.254.740.991");
    expect(markup).toContain("0.00%");
    expect(markup).toContain("9,007,199,254,740,991");
  });
});
