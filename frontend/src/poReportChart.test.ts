import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { poreport } from "@backend/models";
import type { CurrencyInfo } from "./format";
import { normalizePurchaseOrderReport } from "./poReport";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
} from "./poReport";
import {
  purchaseOrderReportFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "./poReportFixtures";
import {
  buildReportChart,
  chartPeriodLabel,
  compactChartMoney,
  formatPercentTick,
  layoutReportChart,
  niceScale,
  reportChartDataKey,
  scaleY,
} from "./poReportChart";

// Issue #631: the chart's pure decisions. The scale is the shared contract with
// the Go PDF renderer, pinned by the same fixture both implementations read.
// Everything else here is geometry derived only from server figures: domains,
// column positions, line gaps, label thinning and the projection band. No test
// recomputes a server figure.

interface ScaleFixture {
  algorithm: string;
  tolerance: number;
  cases: Array<{
    name: string;
    input: { min: number; max: number; targetTicks: number };
    expected: { min: number; max: number; step: number; ticks: number[] };
  }>;
}

const SCALE_FIXTURE = JSON.parse(
  readFileSync(
    new URL(
      "../../backend/render/report/testdata/chart_scales.json",
      import.meta.url
    ),
    "utf8"
  )
) as ScaleFixture;

interface SnapshotFixture {
  report: poreport.Report;
}

const SHARED_SNAPSHOT = JSON.parse(
  readFileSync(
    new URL(
      "../../backend/render/report/testdata/report_snapshot.json",
      import.meta.url
    ),
    "utf8"
  )
) as SnapshotFixture;

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

function metrics(
  poCount: number,
  amountMinor: number,
  capitalMinor: number,
  marginMinor: number,
  marginBps: number | null
): PurchaseOrderReportMetrics {
  return {
    poCount,
    amountMinor,
    costedMinor: amountMinor,
    uncostedMinor: 0,
    extraExpenseMinor: 0,
    capitalMinor,
    marginMinor,
    marginBps,
    coverageBps: amountMinor === 0 ? null : 10_000,
  };
}

function emptyReport(): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  report.periods = [
    {
      periodStart: "2026-01-01",
      partial: false,
      inProgress: false,
      ...metrics(0, 0, 0, 0, null),
    },
  ];
  report.totals = metrics(0, 0, 0, 0, null);
  report.categories = [];
  report.categorySeries = [];
  return report;
}

function oneBucketReport(): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  report.periods = [report.periods[5]];
  report.categorySeries = [];
  return report;
}

function monthReport(count: number): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  report.periods = Array.from({ length: count }, (_, index) => {
    const year = 2005 + Math.floor(index / 12);
    const month = String((index % 12) + 1).padStart(2, "0");
    return {
      periodStart: `${year}-${month}-01`,
      partial: false,
      inProgress: false,
      ...metrics(1, 1_000_000 + index, 800_000, 200_000, 2500),
    };
  });
  report.categories = [];
  report.categorySeries = [];
  return report;
}

function sharedReport(): PurchaseOrderReportData {
  const report = normalizePurchaseOrderReport(SHARED_SNAPSHOT.report);
  // The shared snapshot is a historical 2025 range, so it carries no
  // current-period ghost; the chart tests use it for actual marks only.
  report.filter.bucket = "year";
  return report;
}

describe("shared nice-scale fixture (issue #631)", () => {
  it("reads the Go fixture and pins all 4, 5 and 6 target-tick cases", () => {
    expect(SCALE_FIXTURE.algorithm).toBe("nice-1-2-5");
    const targets = new Set(SCALE_FIXTURE.cases.map((entry) => entry.input.targetTicks));
    expect(targets.has(4)).toBe(true);
    expect(targets.has(5)).toBe(true);
    expect(targets.has(6)).toBe(true);
    expect(SCALE_FIXTURE.cases.some((entry) => entry.name === "negative")).toBe(true);
    expect(SCALE_FIXTURE.cases.some((entry) => entry.name === "extreme")).toBe(true);
  });

  it("returns the same domain, step and ticks as the Go renderer", () => {
    for (const entry of SCALE_FIXTURE.cases) {
      const got = niceScale(
        entry.input.min,
        entry.input.max,
        entry.input.targetTicks
      );
      const context = `${entry.name} (target ${entry.input.targetTicks})`;
      expect(got.min, `${context} min`).toBeCloseTo(
        entry.expected.min,
        9
      );
      expect(got.max, `${context} max`).toBeCloseTo(
        entry.expected.max,
        9
      );
      expect(got.step, `${context} step`).toBeCloseTo(
        entry.expected.step,
        9
      );
      expect(got.ticks, context).toHaveLength(entry.expected.ticks.length);
      got.ticks.forEach((tick, index) => {
        expect(tick, `${context} tick ${index}`).toBeCloseTo(
          entry.expected.ticks[index],
          9
        );
      });
    }
  });

  it("always includes zero, never returns zero ticks and never divides by zero", () => {
    for (const [min, max] of [
      [1, 10],
      [-10, -1],
      [-3, 7],
      [0, 0],
      [1_000_000, 2_000_000],
      [-Number.MAX_SAFE_INTEGER, Number.MAX_SAFE_INTEGER],
    ]) {
      const scale = niceScale(min, max, 5);
      expect(scale.min).toBeLessThanOrEqual(0);
      expect(scale.max).toBeGreaterThanOrEqual(0);
      expect(scale.ticks.length).toBeGreaterThan(1);
      for (const tick of scale.ticks) expect(Number.isFinite(tick)).toBe(true);
      expect(scaleY(scale, scale.min, 10, 200)).toBeCloseTo(210, 9);
      expect(scaleY(scale, scale.max, 10, 200)).toBeCloseTo(10, 9);
    }
  });
});

describe("chart axes formatting (issue #631)", () => {
  it("formats compact money with the currency's own exponent and affixes", () => {
    expect(compactChartMoney(IDR, 150_000_000)).toBe("Rp150M");
    expect(compactChartMoney(IDR, -150_000_000)).toBe("Rp-150M");
    expect(compactChartMoney(IDR, 0)).toBe("Rp0");
    expect(compactChartMoney(IDR, 9_000_000_000_000_000)).toBe("Rp9P");
    expect(compactChartMoney(USD, 5)).toBe("$0.05");
    expect(compactChartMoney(USD, 123_456)).toBe("$1.2K");
    expect(compactChartMoney(EUR, 123_456)).toBe("1.2K €");
    expect(compactChartMoney(null, 150_000_000)).toBe("-");
  });

  it("formats percent ticks as a percentage, compacting only extreme values", () => {
    expect(formatPercentTick(0)).toBe("0%");
    expect(formatPercentTick(12.5)).toBe("12.5%");
    expect(formatPercentTick(-8.33)).toBe("-8.33%");
    expect(formatPercentTick(10_000)).toBe("10K%");
    expect(formatPercentTick(-10_000)).toBe("-10K%");
  });

  it("uses the bucket's own short vocabulary", () => {
    expect(chartPeriodLabel("2026-11-01", "month")).toBe("Nov 26");
    expect(chartPeriodLabel("2026-04-01", "quarter")).toBe("Q2 26");
    expect(chartPeriodLabel("2026-01-01", "year")).toBe("2026");
    expect(chartPeriodLabel("2026-11-19", "day")).toBe("19");
    expect(chartPeriodLabel("not-a-date", "month")).toBe("not-a-date");
  });
});

describe("report chart model (issue #631)", () => {
  it("includes actual money and every ghost bound in the money domain", () => {
    const report = purchaseOrderReportMonthFixture();
    report.currentProjection.buckets[0].amountHighMinor = 900_000_000;
    report.currentProjection.buckets[0].capitalMinor = 950_000_000;
    report.currentProjection.buckets[0].marginMinor = -200_000_000;
    const model = buildReportChart(report, []);
    expect(model.moneyScale.min).toBeLessThanOrEqual(-200_000_000);
    expect(model.moneyScale.max).toBeGreaterThanOrEqual(950_000_000);
    expect(model.projectedCount).toBe(11);
    expect(model.actualCount).toBe(19);
    expect(model.dividerIndex).toBe(19);
  });

  it("includes the current month's stacked remainder in the money domain", () => {
    const report = purchaseOrderReportYearFixture();
    report.currentProjection.currentMonth!.remainder.high = 900_000_000;
    const model = buildReportChart(report, []);
    const november = model.buckets.find(
      (bucket) => bucket.periodStart === "2026-11-01"
    )!;
    expect(model.moneyScale.max).toBeGreaterThanOrEqual(
      november.amountMinor + 900_000_000
    );
    expect(model.hasRemainder).toBe(true);
  });

  it("keeps null percentages as gaps and never turns them into a zero line", () => {
    const report = purchaseOrderReportFixture();
    report.periods[3] = { ...report.periods[3], marginBps: null };
    report.periods[4] = { ...report.periods[4], marginBps: null };
    report.categorySeries = [
      {
        businessCategory: "FM",
        marginBps: report.periods.map((period, index) =>
          index === 2 || index === 3 ? null : 1234
        ),
      },
    ];
    const model = buildReportChart(report, ["FM"]);
    const layout = layoutReportChart(model, 960);
    expect(model.percentScale).not.toBeNull();
    // Two separate runs: Jan-Mar and May-Nov. A gap is not a segment to zero.
    expect(layout.marginPath?.match(/M/g)).toHaveLength(2);
    expect(layout.marginPoints).toHaveLength(report.periods.length - 2);
    for (const point of layout.marginPoints) {
      expect(point.y).toBeGreaterThanOrEqual(layout.plot.y);
      expect(point.y).toBeLessThanOrEqual(layout.plot.y + layout.plot.height);
    }
    const last = layout.marginPoints[layout.marginPoints.length - 1];
    const lastBps = report.periods[last.index].marginBps;
    expect(lastBps).not.toBeNull();
    const tenPercent = layout.percentTicks.find((tick) => tick.value === 10);
    if (tenPercent === undefined) throw new Error("expected a 10% tick");
    // The fixture's latest margin is 10.77%; it must sit by the 10% line, not
    // 100x higher from reading basis points as percent.
    expect(Math.abs(last.y - tenPercent.y)).toBeLessThan(20);
    expect(layout.categoryPaths[0].path.match(/M/g)).toHaveLength(2);
  });

  it("hides category lines outside All and footnotes the ones with no costed data", () => {
    const filtered = purchaseOrderReportFixture();
    filtered.filter.category = "FM";
    const hidden = buildReportChart(filtered, ["FM"]);
    expect(hidden.categories).toHaveLength(0);
    expect(hidden.categoryFootnote).toContain("All categories");

    const all = purchaseOrderReportFixture();
    const omitted = buildReportChart(all, []);
    expect(omitted.categoryFootnote).toContain("omitted");
  });

  it("omits a present category series whose every value is null and says why", () => {
    const report = purchaseOrderReportFixture();
    report.categorySeries = [
      {
        businessCategory: "FM",
        marginBps: report.periods.map(() => null),
      },
    ];
    const model = buildReportChart(report, ["FM"]);
    expect(model.categories).toHaveLength(0);
    expect(model.categoryFootnote).toContain("Facility Management");
    expect(model.categoryFootnote).toContain("omitted");
  });

  it("draws current-period ghosts for Month and Year, never for quarters or years", () => {
    const month = buildReportChart(purchaseOrderReportMonthFixture(), []);
    expect(month.projectionKind).toBe("day");
    expect(month.projectedCount).toBe(11);
    expect(month.actualCount).toBe(19);

    const year = buildReportChart(purchaseOrderReportYearFixture(), []);
    expect(year.projectionKind).toBe("month");
    expect(year.projectedCount).toBe(1);
    expect(year.actualCount).toBe(11);
    expect(year.hasRemainder).toBe(true);

    for (const bucket of ["quarter", "year"] as const) {
      const report = purchaseOrderReportFixture();
      report.filter.bucket = bucket;
      const model = buildReportChart(report, []);
      expect(model.projectedCount).toBe(0);
      expect(model.projectionKind).toBe("none");
      expect(model.projectionNote).toContain("Month and Year views");
    }
  });

  it("explains an unavailable current-period projection instead of drawing a flat line", () => {
    const report = purchaseOrderReportMonthFixture();
    report.currentProjection.status = "unavailable";
    report.currentProjection.reason = "insufficient_days";
    report.currentProjection.buckets = [];
    const model = buildReportChart(report, []);
    expect(model.projectedCount).toBe(0);
    expect(model.projectionNote).toContain("Not enough elapsed days");
    expect(model.hasRemainder).toBe(false);
  });

  it("draws month ghost days after the last elapsed day with day-number labels", () => {
    const report = purchaseOrderReportMonthFixture();
    const model = buildReportChart(report, []);
    expect(model.actualCount).toBe(19);
    expect(model.projectedCount).toBe(11);
    expect(model.buckets[18].periodStart).toBe("2026-11-19");
    expect(model.buckets[18].label).toBe("19");
    expect(model.buckets[19].periodStart).toBe("2026-11-20");
    expect(model.buckets[19].label).toBe("20");
    expect(model.buckets.at(-1)?.periodStart).toBe("2026-11-30");
    expect(model.buckets.at(-1)?.label).toBe("30");
    expect(model.dividerIndex).toBe(19);

    const layout = layoutReportChart(model, 960);
    expect(layout.bandWhiskers).toHaveLength(11);
    expect(layout.remainderWhisker).toBeNull();
    expect(layout.xLabels[0]?.label).toBe("1");
    expect(layout.xLabels.at(-1)?.label).toBe("30");
    for (const whisker of layout.bandWhiskers) {
      expect(Number.isFinite(whisker.lowY)).toBe(true);
      expect(whisker.highY).toBeLessThan(whisker.lowY);
    }
  });

  it("stacks the current month's remainder and ghosts later months in Year mode", () => {
    const report = purchaseOrderReportYearFixture();
    const model = buildReportChart(report, []);
    expect(model.actualCount).toBe(11);
    expect(model.projectedCount).toBe(1);
    expect(model.buckets[10].periodStart).toBe("2026-11-01");
    expect(model.buckets[10].amountMinor).toBe(
      report.currentProjection.currentMonth!.actual.amountMinor
    );
    expect(model.buckets[10].remainder).toEqual(
      report.currentProjection.currentMonth!.remainder
    );
    expect(model.buckets[11].periodStart).toBe("2026-12-01");
    expect(model.buckets[11].label).toBe("Dec 26");

    const layout = layoutReportChart(model, 960);
    const november = layout.columns[10];
    expect(november.remainder).not.toBeNull();
    for (const segment of [
      november.remainder!.amount,
      november.remainder!.capital,
      november.remainder!.margin,
    ]) {
      expect(segment.height).toBeGreaterThan(0);
      expect(Number.isFinite(segment.y)).toBe(true);
    }
    // The stacked amount segment sits above the actual bar: its bottom is the
    // actual amount's top, so it is a remainder, not a second full bar.
    expect(november.remainder!.amount.y).toBeLessThan(november.amount.y);
    expect(
      november.remainder!.amount.y + november.remainder!.amount.height
    ).toBeCloseTo(november.amount.y, 6);
    expect(layout.remainderWhisker).not.toBeNull();
    expect(layout.bandWhiskers).toHaveLength(1);
    expect(layout.marginPoints).toHaveLength(11);
  });

  it("draws no ghost on a historical range and states why", () => {
    const report = purchaseOrderReportFixture();
    const model = buildReportChart(report, []);
    expect(model.projectedCount).toBe(0);
    expect(model.actualCount).toBe(report.periods.length);
    expect(model.projectionNote).toContain("not the current month or year");
    const layout = layoutReportChart(model, 960);
    expect(layout.bandWhiskers).toEqual([]);
    expect(layout.remainderWhisker).toBeNull();
    expect(layout.dividerX).toBeNull();
  });

  it("orders category toggles by the workbook vocabulary and includes Unassigned when defined", () => {
    const report = purchaseOrderReportFixture();
    report.categorySeries = [
      {
        businessCategory: "UNASSIGNED",
        marginBps: report.periods.map(() => 500),
      },
      {
        businessCategory: "FAB",
        marginBps: report.periods.map((_, index) => (index === 0 ? 500 : null)),
      },
    ];
    const model = buildReportChart(report, ["UNASSIGNED"]);
    expect(model.categories.map((category) => category.code)).toEqual([
      "FAB",
      "UNASSIGNED",
    ]);
    expect(model.categories[1].label).toBe("Unassigned");
    expect(model.categories[1].visible).toBe(true);
  });

  it("keeps a stable data identity key across equal snapshots and changes it with the data", () => {
    const report = purchaseOrderReportFixture();
    const clone = structuredClone(report);
    expect(reportChartDataKey(clone)).toBe(reportChartDataKey(report));

    clone.filter.bucket = "year";
    expect(reportChartDataKey(clone)).not.toBe(reportChartDataKey(report));

    const changed = purchaseOrderReportFixture();
    changed.periods[0] = {
      ...changed.periods[0],
      amountMinor: changed.periods[0].amountMinor + 1,
    };
    expect(reportChartDataKey(changed)).not.toBe(reportChartDataKey(report));
  });

  it("ignores the retired projection series and follows the current-period ghost", () => {
    const report = purchaseOrderReportMonthFixture();
    const base = reportChartDataKey(report);

    const clone = structuredClone(report);
    clone.projection.monthly.buckets = [
      {
        periodStart: "2027-01-01",
        amountLowMinor: 1,
        amountBaseMinor: 2,
        amountHighMinor: 3,
        capitalMinor: 1,
        marginMinor: 1,
        marginBps: 100,
        sampleSize: 1,
        source: "same_calendar_month",
      },
    ];
    expect(reportChartDataKey(clone)).toBe(base);

    clone.currentProjection.buckets[0].amountBaseMinor += 1;
    expect(reportChartDataKey(clone)).not.toBe(base);

    const year = purchaseOrderReportYearFixture();
    const yearBase = reportChartDataKey(year);
    const yearClone = structuredClone(year);
    yearClone.currentProjection.currentMonth!.remainder.base += 1;
    expect(reportChartDataKey(yearClone)).not.toBe(yearBase);
  });

  it("never returns NaN geometry for empty, one-bucket or zero-capital data", () => {
    for (const report of [emptyReport(), oneBucketReport()]) {
      const layout = layoutReportChart(buildReportChart(report, []), 360);
      expect(layout.width).toBeGreaterThan(0);
      expect(layout.plot.width).toBeGreaterThan(0);
      expect(Number.isFinite(layout.zeroY)).toBe(true);
      for (const column of layout.columns) {
        for (const bar of [column.amount, column.capital, column.margin]) {
          expect(Number.isFinite(bar.x)).toBe(true);
          expect(Number.isFinite(bar.y)).toBe(true);
          expect(Number.isFinite(bar.width)).toBe(true);
          expect(Number.isFinite(bar.height)).toBe(true);
          expect(bar.height).toBeGreaterThanOrEqual(0);
        }
      }
    }

    const zero = buildReportChart(emptyReport(), []);
    expect(zero.percentScale).toBeNull();
    expect(zero.moneyScale.max).toBeGreaterThan(0);
  });

  it("puts negative margins below the zero baseline and keeps uncosted inside the amount column", () => {
    const report = sharedReport();
    report.filter.bucket = "month";
    const layout = layoutReportChart(buildReportChart(report, []), 960);
    const negative = layout.columns.find((column) => column.margin.negative);
    expect(negative).toBeDefined();
    expect(negative?.margin.y).toBeGreaterThanOrEqual(layout.zeroY);
    expect(negative?.margin.height).toBeGreaterThan(0);

    const uncosted = layout.columns.find((column) => column.uncosted !== null);
    expect(uncosted).toBeDefined();
    const segment = uncosted!.uncosted!;
    const amountTop = Math.min(uncosted!.amount.y, layout.zeroY);
    const amountBottom = Math.max(
      uncosted!.amount.y,
      uncosted!.amount.y + uncosted!.amount.height
    );
    expect(segment.y).toBeGreaterThanOrEqual(amountTop - 0.01);
    expect(segment.y + segment.height).toBeLessThanOrEqual(amountBottom + 0.01);
  });

  it("gives 120 buckets a scrollable width, finite geometry and thinned readable labels", () => {
    const report = monthReport(120);
    const model = buildReportChart(report, []);
    expect(model.actualCount).toBe(120);
    const layout = layoutReportChart(model, 960);
    expect(layout.width).toBeGreaterThan(960);
    expect(layout.columns).toHaveLength(120);
    expect(layout.xLabels.length).toBeLessThan(120);
    expect(layout.xLabels.at(-1)?.index).toBe(119);
    expect(layout.xLabels.every((label) => Number.isFinite(label.x))).toBe(true);
    for (const label of layout.xLabels) {
      expect(label.x).toBeGreaterThanOrEqual(layout.plot.x);
      expect(label.x).toBeLessThanOrEqual(layout.plot.x + layout.plot.width);
    }
  });

  it("keeps an 11+2 bucket month chart inside a 360px card without shrinking its text", () => {
    const report = purchaseOrderReportFixture();
    const layout = layoutReportChart(buildReportChart(report, []), 280);
    expect(layout.width).toBe(280);
    expect(layout.plot.width).toBeGreaterThan(150);
    expect(layout.groupWidth).toBeGreaterThanOrEqual(12);
  });

  it("rescales the percentage axis when a category line is shown and leaves money untouched", () => {
    const report = sharedReport();
    report.filter.bucket = "month";
    report.categorySeries[0].marginBps[0] = 12_000;
    const off = buildReportChart(report, []);
    const on = buildReportChart(report, ["FM"]);
    const offLayout = layoutReportChart(off, 960);
    const onLayout = layoutReportChart(on, 960);

    expect(off.percentScale?.max).not.toBe(on.percentScale?.max);
    expect(onLayout.categoryPaths).toHaveLength(1);
    expect(offLayout.categoryPaths).toHaveLength(0);
    expect(onLayout.columns[0].amount.y).toBe(offLayout.columns[0].amount.y);
  });
});
