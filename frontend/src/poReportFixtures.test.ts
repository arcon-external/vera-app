import { describe, expect, it } from "vitest";
import {
  fixtureRatioBps,
  purchaseOrderReportExtraExpenseFixture,
  purchaseOrderReportFixture,
  purchaseOrderReportMaxFixture,
  purchaseOrderReportMonthFixture,
  purchaseOrderReportMonthWithRecordedFutureFixture,
  purchaseOrderReportYearFixture,
  purchaseOrderReportYearWithRecordedFutureFixture,
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
  expect(row.extraExpenseMinor, `${name} extra expense`).toBeGreaterThanOrEqual(0);
  expect(row.amountMinor, `${name} amount = costed + uncosted + extra`).toBe(
    row.costedMinor + row.uncostedMinor + row.extraExpenseMinor
  );
  expect(row.marginMinor, `${name} margin = costed - capital`).toBe(
    row.costedMinor - row.capitalMinor
  );
  expect(row.marginBps, `${name} marginBps`).toBe(
    fixtureRatioBps(row.marginMinor, row.capitalMinor)
  );
  expect(row.coverageBps, `${name} coverageBps`).toBe(
    fixtureRatioBps(row.costedMinor, row.amountMinor - row.extraExpenseMinor)
  );
}

function sumRows(rows: PurchaseOrderReportMetrics[]): PurchaseOrderReportMetrics {
  const total: PurchaseOrderReportMetrics = {
    poCount: 0,
    amountMinor: 0,
    costedMinor: 0,
    uncostedMinor: 0,
    extraExpenseMinor: 0,
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
    total.extraExpenseMinor += row.extraExpenseMinor;
    total.capitalMinor += row.capitalMinor;
    total.marginMinor += row.marginMinor;
  }
  return total;
}

function assertFooting(report: PurchaseOrderReportData): void {
  // A row with no purchase order cannot carry revenue. Totals can foot while
  // this is false, which is exactly the lie the month fixture used to print.
  for (const period of report.periods) {
    if (period.poCount !== 0) continue;
    expect(period.amountMinor, `${period.periodStart} empty amount`).toBe(0);
    expect(period.costedMinor, `${period.periodStart} empty costed`).toBe(0);
    expect(period.uncostedMinor, `${period.periodStart} empty uncosted`).toBe(0);
    expect(period.extraExpenseMinor, `${period.periodStart} empty extra`).toBe(0);
    expect(period.capitalMinor, `${period.periodStart} empty capital`).toBe(0);
    expect(period.marginMinor, `${period.periodStart} empty margin`).toBe(0);
  }

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
    expect(sum.extraExpenseMinor, `${name} extra expense foots`).toBe(
      report.totals.extraExpenseMinor
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
      extraExpenseMinor: november.extraExpenseMinor,
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

  it("shares the current month's day buckets and month summary arithmetic", () => {
    const month = purchaseOrderReportMonthFixture();
    const summary = month.monthSummary!;
    expect(summary.actual).toEqual(month.totals);
    expect(summary.recordedFuture).toBeUndefined();
    const buckets = month.currentProjection.buckets;
    const summed = buckets.reduce(
      (total, bucket) => ({
        low: total.low + bucket.amountLowMinor,
        base: total.base + bucket.amountBaseMinor,
        high: total.high + bucket.amountHighMinor,
        capital: total.capital + bucket.capitalMinor,
        margin: total.margin + bucket.marginMinor,
      }),
      { low: 0, base: 0, high: 0, capital: 0, margin: 0 }
    );
    expect(summary.projection.remainder).toEqual({
      low: summed.low,
      base: summed.base,
      high: summed.high,
      capital: summed.capital,
      margin: summed.margin,
      marginBps: fixtureRatioBps(summed.margin, summed.capital),
      sampleSize: 19,
      source: "elapsed_days",
    });

    const year = purchaseOrderReportYearFixture();
    const yearSummary = year.monthSummary!;
    expect(yearSummary.mode).toBe("month");
    expect(yearSummary.actual).toEqual(month.totals);
    expect(yearSummary.projection.remainder).toEqual(
      summary.projection.remainder
    );
    expect(year.currentProjection.currentMonth!.actual).toEqual(month.totals);
    expect(year.currentProjection.currentMonth!.remainder).toEqual(
      summary.projection.remainder
    );
    const november = year.periods.find(
      (period) => period.periodStart === "2026-11-01"
    )!;
    expect(november.amountMinor).toBe(month.totals.amountMinor);
    // The annual source's complete months are fully costed, so its available
    // month-mode forecast is legal rather than an incomplete-cost estimate.
    for (const period of year.periods.slice(0, 10)) {
      expect(period.uncostedMinor).toBe(0);
    }
  });


  it("adds recorded future evidence to the raw rows, totals and categories", () => {
    const cases = [
      {
        report: purchaseOrderReportMonthWithRecordedFutureFixture(),
        baseline: purchaseOrderReportMonthFixture(),
        futurePeriod: "2026-11-25",
        annual: false,
      },
      {
        report: purchaseOrderReportYearWithRecordedFutureFixture(),
        baseline: purchaseOrderReportYearFixture(),
        futurePeriod: "2026-11-01",
        annual: true,
      },
    ];
    for (const { report, baseline, futurePeriod, annual } of cases) {
      assertFooting(report);
      assertRow("future totals", report.totals);
      for (const category of report.categories) {
        assertRow(`future category ${category.businessCategory}`, category);
      }

      const summary = report.monthSummary!;
      const recorded = summary.recordedFuture!;
      expect(recorded.poCount).toBe(1);
      expect(recorded.amountMinor).toBe(777_000);
      expect(baseline.totals.poCount + recorded.poCount).toBe(
        report.totals.poCount
      );
      expect(baseline.totals.amountMinor + recorded.amountMinor).toBe(
        report.totals.amountMinor
      );
      expect(baseline.totals.costedMinor + recorded.costedMinor).toBe(
        report.totals.costedMinor
      );
      expect(baseline.totals.uncostedMinor + recorded.uncostedMinor).toBe(
        report.totals.uncostedMinor
      );
      expect(baseline.totals.capitalMinor + recorded.capitalMinor).toBe(
        report.totals.capitalMinor
      );
      expect(baseline.totals.marginMinor + recorded.marginMinor).toBe(
        report.totals.marginMinor
      );
      if (!annual) {
        // A day report's totals are current-month evidence, so its actual plus
        // the recorded future order foots to the snapshot's totals.
        expect(summary.actual.poCount + recorded.poCount).toBe(
          report.totals.poCount
        );
        expect(summary.actual.amountMinor + recorded.amountMinor).toBe(
          report.totals.amountMinor
        );
      }

      const futureRow = report.periods.find(
        (period) => period.periodStart === futurePeriod
      )!;
      assertRow(`future row ${futurePeriod}`, futureRow);
      if (annual) {
        // The annual current-month row is the raw month (actual + future).
        expect(futureRow.poCount).toBe(summary.actual.poCount + recorded.poCount);
        expect(futureRow.amountMinor).toBe(
          summary.actual.amountMinor + recorded.amountMinor
        );
        expect(report.currentProjection.currentMonth?.actual).toEqual(
          summary.actual
        );
      } else {
        // The day fixture's future day row is exactly the recorded order; the
        // elapsed actual rows and the projection basis exclude it.
        expect(futureRow.poCount).toBe(recorded.poCount);
        expect(futureRow.amountMinor).toBe(recorded.amountMinor);
        expect(report.currentProjection.basis.poCount).toBe(
          summary.actual.poCount
        );
      }
    }
  });

  it("buckets extra expense separately and never folds it into cost or coverage (#655)", () => {
    const report = purchaseOrderReportExtraExpenseFixture();
    assertRow("totals", report.totals);
    for (const period of report.periods) {
      assertRow(`period ${period.periodStart}`, period);
    }
    for (const category of report.categories) {
      assertRow(`category ${category.businessCategory}`, category);
    }
    assertFooting(report);

    // March is a quoted line (5 x 10.000, cost 6.000) beside a pass-through
    // freight of 2 x 500.000. April carries only an extra expense.
    expect(report.totals).toMatchObject({
      poCount: 2,
      amountMinor: 2_050_000,
      costedMinor: 50_000,
      uncostedMinor: 0,
      extraExpenseMinor: 2_000_000,
      capitalMinor: 30_000,
      marginMinor: 20_000,
      // 50.000 / (2.050.000 - 2.000.000): the non-extra amount only.
      coverageBps: 10000,
    });

    const [mixed, extraOnly] = report.periods;
    expect(mixed.periodStart).toBe("2026-03-01");
    expect(mixed.extraExpenseMinor).toBe(1_000_000);
    expect(mixed.marginMinor).toBe(20_000);
    expect(mixed.coverageBps).toBe(10000);

    expect(extraOnly.periodStart).toBe("2026-04-01");
    expect(extraOnly.amountMinor).toBe(1_000_000);
    expect(extraOnly.costedMinor).toBe(0);
    expect(extraOnly.extraExpenseMinor).toBe(1_000_000);
    expect(extraOnly.uncostedMinor).toBe(0);
    expect(extraOnly.marginMinor).toBe(0);
    // A zero non-extra denominator is an unknown coverage, not 0% or 100%.
    expect(extraOnly.coverageBps).toBeNull();
    expect(extraOnly.marginBps).toBeNull();
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

