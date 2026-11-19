import { readFileSync } from "node:fs";
import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { poreport } from "@backend/models";
import type { CurrencyInfo } from "./format";
import { PurchaseOrderReportsView } from "./components/PurchaseOrderReportsPage";
import { normalizePurchaseOrderReport } from "./poReport";
import {
  addDaysIso,
  reportRangeLabel,
  type ReportBucket,
  type ReportFilterState,
} from "./poReportPage";

// Issue #632 review: the Go PDF test reads this exact file and renders its
// `report` through the export path. This test normalizes the same report and
// server-renders the same view, then requires every expected metric string.
// Function-level parity in report_formats.json cannot catch a screen that
// formats the same value differently from the document; this can.

interface SharedSnapshotFixture {
  description: string;
  expected: {
    metrics: string[];
    pdf: { filterRange: string; generatedAt: string; asOf: string };
    screen: { asOfDate: string; filterRange: string };
  };
  report: poreport.Report;
}

const FIXTURE = JSON.parse(
  readFileSync(
    new URL(
      "../../backend/render/report/testdata/report_snapshot.json",
      import.meta.url
    ),
    "utf8"
  )
) as SharedSnapshotFixture;

const IDR: CurrencyInfo = {
  code: "IDR",
  exponent: 0,
  symbol: "Rp",
  position: "before",
  groupSep: ".",
  decimalSep: ",",
  symbolSpaced: false,
};

describe("shared report snapshot parity (issue #632)", () => {
  it("normalizes the shared wire report without inventing or dropping a ratio", () => {
    const report = normalizePurchaseOrderReport(FIXTURE.report);

    expect(report.generatedAt).toBe(FIXTURE.report.generatedAt);
    expect(report.asOf).toBe(FIXTURE.report.asOf);
    expect(report.filter).toEqual(FIXTURE.report.filter);
    expect(report.totals.marginBps).toBe(1235);
    expect(report.totals.coverageBps).toBe(7371);

    const series = report.categorySeries.flatMap((row) => row.marginBps);
    expect(series).toContain(null);
    expect(series.some((value) => typeof value === "number" && value < 0)).toBe(
      true
    );
  });

  it("renders every expected metric string exactly as the PDF does", () => {
    const report = normalizePurchaseOrderReport(FIXTURE.report);
    const filters: ReportFilterState = {
      preset: "custom",
      from: report.filter.from,
      to: addDaysIso(report.filter.to, -1),
      bucket: report.filter.bucket as ReportBucket,
      category: report.filter.category,
      currency: report.filter.currency,
    };

    const markup = renderToString(
      createElement(PurchaseOrderReportsView, {
        filters,
        report,
        loading: false,
        stale: false,
        error: "",
        validation: "",
        catalogue: [IDR],
        exporting: false,
        canExport: false,
        onPresetChange: () => undefined,
        onFromChange: () => undefined,
        onToChange: () => undefined,
        onBucketChange: () => undefined,
        onCategoryChange: () => undefined,
        onCurrencyChange: () => undefined,
        onRetry: () => undefined,
        onExport: () => undefined,
      })
    );

    // React separates adjacent text nodes with an empty comment; strip it so
    // the assertion is about the rendered text, not the serializer's markers.
    const text = markup.replace(/<!-- -->/g, "");
    for (const expected of FIXTURE.expected.metrics) {
      expect(text, expected).toContain(expected);
    }
    expect(text).toContain(`As of ${FIXTURE.expected.screen.asOfDate}`);
    expect(markup).toContain(`value="${filters.from}"`);
    expect(markup).toContain(`value="${filters.to}"`);
    expect(reportRangeLabel(report.filter)).toBe(
      FIXTURE.expected.screen.filterRange
    );
  });
});
