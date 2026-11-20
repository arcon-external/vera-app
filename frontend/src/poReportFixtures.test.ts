import { describe, expect, it } from "vitest";
import {
  fixtureRatioBps,
  purchaseOrderReportFixture,
  purchaseOrderReportMaxFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportYearFixture,
} from "./poReportFixtures";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
} from "./poReport";

// Issue #632 review: a fixture is evidence only while it tells the same
// financial story a server response would. These guards check the identities
// the screen relies on (amount = costed + uncosted, margin = costed - capital),
// that each row's percentages are derived from that row's own money, that the
// period and category tables foot to the totals, and that the partial /
// in-progress flags agree with the filter and the as-of date. A fixture that
// fails one of these can still render; it just cannot be cited as proof.

function assertRow(name: string, row: PurchaseOrderReportMetrics): void {
  expect(row.amountMinor, `${name} amount = costed + uncosted`).toBe(
    row.costedMinor + row.uncostedMinor
  );
  expect(row.marginMinor, `${name} margin = costed - capital`).toBe(
    row.costedMinor - row.capitalMinor
  );
  expect(row.marginBps, `${name} marginBps`).toBe(
    fixtureRatioBps(row.marginMinor, row.capitalMinor)
  );
  expect(row.coverageBps, `${name} coverageBps`).toBe(
    fixtureRatioBps(row.costedMinor, row.amountMinor)
  );
}

function sumRows(rows: PurchaseOrderReportMetrics[]): PurchaseOrderReportMetrics {
  const total: PurchaseOrderReportMetrics = {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    capitalMinor: 0,
    marginMinor: 0,
    marginBps: null,
    coverageBps: null,
  };
  for (const row of rows) {
    total.poCount += row.poCount;
    total.amountMinor += row.amountMinor;
    total.costedMinor += row.costedMinor;
    total.uncostedMinor += row.uncostedMinor;
    total.capitalMinor += row.capitalMinor;
    total.marginMinor += row.marginMinor;
  }
  return total;
}

function assertFooting(report: PurchaseOrderReportData): void {
  const periodSum = sumRows(report.periods);
  const categorySum = sumRows(report.categories);
  for (const [name, sum] of [
    ["period", periodSum],
    ["category", categorySum],
  ] as const) {
    expect(sum.poCount, `${name} count foots`).toBe(report.totals.poCount);
    expect(sum.amountMinor, `${name} amount foots`).toBe(
      report.totals.amountMinor
    );
    expect(sum.costedMinor, `${name} costed foots`).toBe(
      report.totals.costedMinor
    );
    expect(sum.uncostedMinor, `${name} uncosted foots`).toBe(
      report.totals.uncostedMinor
    );
    expect(sum.capitalMinor, `${name} capital foots`).toBe(
      report.totals.capitalMinor
    );
    expect(sum.marginMinor, `${name} margin foots`).toBe(
      report.totals.marginMinor
    );
  }
}

function nextMonthStart(periodStart: string): string {
  const date = new Date(`${periodStart}T00:00:00Z`);
  date.setUTCMonth(date.getUTCMonth() + 1);
  return date.toISOString().slice(0, 10);
}

function assertDateFlags(report: PurchaseOrderReportData): void {
  expect(report.asOf >= report.filter.from).toBe(true);
  expect(report.asOf < report.filter.to).toBe(true);
  for (const period of report.periods) {
    const next = nextMonthStart(period.periodStart);
    const partial =
      period.periodStart < report.filter.from || report.filter.to < next;
    const inProgress =
      report.asOf >= period.periodStart && report.asOf < next;
    expect(period.partial, `${period.periodStart} partial`).toBe(partial);
    expect(period.inProgress, `${period.periodStart} inProgress`).toBe(
      inProgress
    );
  }
}

function assertProjectionRatios(report: PurchaseOrderReportData): void {
  for (const series of [report.projection.monthly, report.projection.yearly]) {
    for (const bucket of series.buckets) {
      expect(
        bucket.marginMinor,
        `${bucket.periodStart} projected margin`
      ).toBe(bucket.amountBaseMinor - bucket.capitalMinor);
      expect(bucket.marginBps, `${bucket.periodStart} projected marginBps`).toBe(
        fixtureRatioBps(bucket.marginMinor, bucket.capitalMinor)
      );
    }
  }
}

describe("purchase order report fixtures (issue #632)", () => {
  it("derives every sample row's figures and percentages from one footed set", () => {
    const report = purchaseOrderReportFixture();
    assertRow("totals", report.totals);
    for (const period of report.periods) {
      assertRow(`period ${period.periodStart}`, period);
    }
    for (const category of report.categories) {
      assertRow(`category ${category.businessCategory}`, category);
    }
    assertFooting(report);
    assertProjectionRatios(report);

    // No category-by-period money exists in this payload, so the fixture
    // omits the series rather than inventing one. The shared full snapshot
    // carries a genuinely derived series.
    expect(report.categorySeries).toEqual([]);
  });

  it("marks exactly the month that contains the as-of date and is clipped by the range", () => {
    const report = purchaseOrderReportFixture();
    assertDateFlags(report);
    const marked = report.periods.filter(
      (period) => period.partial || period.inProgress
    );
    expect(marked).toHaveLength(1);
    expect(marked[0].periodStart).toBe("2026-11-01");
    expect(marked[0].partial).toBe(true);
    expect(marked[0].inProgress).toBe(true);
  });

  it("carries the current-period contract in the month and year fixtures", () => {
    const month = purchaseOrderReportMonthFixture();
    expect(month.currentProjection.mode).toBe("day");
    expect(month.currentProjection.status).toBe("available");
    expect(month.periods).toHaveLength(30);
    expect(month.periods.filter((period) => period.periodStart <= month.asOf)).toHaveLength(19);
    expect(month.currentProjection.buckets).toHaveLength(11);
    expect(month.currentProjection.buckets[0].periodStart).toBe("2026-11-20");
    expect(month.currentProjection.currentMonth).toBeUndefined();
    assertFooting(month);
    for (const bucket of month.currentProjection.buckets) {
      expect(bucket.amountLowMinor).toBeLessThanOrEqual(bucket.amountBaseMinor);
      expect(bucket.amountBaseMinor).toBeLessThanOrEqual(bucket.amountHighMinor);
      expect(bucket.marginMinor).toBe(bucket.amountBaseMinor - bucket.capitalMinor);
      expect(bucket.marginBps).toBe(
        fixtureRatioBps(bucket.marginMinor, bucket.capitalMinor)
      );
    }

    const year = purchaseOrderReportYearFixture();
    expect(year.currentProjection.mode).toBe("month");
    expect(year.currentProjection.status).toBe("available");
    expect(year.periods).toHaveLength(12);
    expect(year.periods.filter((period) => period.periodStart <= "2026-11-01")).toHaveLength(11);
    assertFooting(year);
    expect(year.currentProjection.buckets).toHaveLength(1);
    expect(year.currentProjection.buckets[0].periodStart).toBe("2026-12-01");
    const currentMonth = year.currentProjection.currentMonth;
    expect(currentMonth).toBeDefined();
    const november = year.periods.find(
      (period) => period.periodStart === "2026-11-01"
    )!;
    expect(currentMonth!.actual).toEqual({
      poCount: november.poCount,
      amountMinor: november.amountMinor,
      costedMinor: november.costedMinor,
      uncostedMinor: november.uncostedMinor,
      capitalMinor: november.capitalMinor,
      marginMinor: november.marginMinor,
      marginBps: november.marginBps,
      coverageBps: november.coverageBps,
    });
    const remainder = currentMonth!.remainder;
    expect(remainder.low).toBeLessThanOrEqual(remainder.base);
    expect(remainder.base).toBeLessThanOrEqual(remainder.high);
    expect(remainder.margin).toBe(remainder.base - remainder.capital);
    expect(remainder.marginBps).toBe(
      fixtureRatioBps(remainder.margin, remainder.capital)
    );
  });

  it("keeps the maximum exact fixture footed with derived ratios", () => {
    const report = purchaseOrderReportMaxFixture();
    assertRow("totals", report.totals);
    for (const period of report.periods) {
      assertRow(`period ${period.periodStart}`, period);
    }
    for (const category of report.categories) {
      assertRow(`category ${category.businessCategory}`, category);
    }
    assertFooting(report);
    assertProjectionRatios(report);

    // The honest ratio is the near-zero margin against near-MAX capital, not
    // a synthetic MAX_SAFE_INTEGER percentage.
    expect(report.totals.marginBps).toBe(0);
    expect(report.totals.coverageBps).toBe(10000);
  });
});

