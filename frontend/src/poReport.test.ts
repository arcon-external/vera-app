import { describe, expect, it } from "vitest";
import type { poreport } from "@backend/models";
import { normalizePurchaseOrderReport } from "./poReport";

// Issue #629's generated Wails model says CategorySeries.marginBps is number[],
// but the runtime payload contains null entries for periods where a category
// had no capital. These tests pin the honest normalized shape #630/#631 use:
// null is a gap, zero is a measured zero, and an absent scalar ratio becomes
// null rather than a fake 0%.

function reportFixture(): poreport.Report {
  const wire = {
    generatedAt: "2026-11-19T05:00:00Z",
    asOf: "2026-11-19",
    filter: {
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    },
    company: {
      name: "PT Arcon Penta Persada",
      address: "Jl. Example 1",
    },
    population: {
      availableCurrencies: ["IDR", "USD"],
      hasUnassigned: true,
      sourceMissingPoCount: 0,
    },
    totals: {
      poCount: 3,
      amountMinor: 1050,
      costedMinor: 600,
      uncostedMinor: 300,
      extraExpenseMinor: 150,
      capitalMinor: 600,
      marginMinor: 0,
      marginBps: 0,
      // Costed / (amount - extra) = 600 / 900.
      coverageBps: 6667,
    },
    pipeline: {
      unsoldAmountMinor: 0,
      unsoldMarginMinor: 0,
    },
    variance: {
      quantityExtraIncomeAmountMinor: 0,
      quantityExtraIncomeMarginMinor: 0,
      unsoldAmountMinor: 0,
      unsoldMarginMinor: 0,
      closedLossAmountMinor: 0,
      closedLossMarginMinor: 0,
      priceExtraIncomeMinor: 0,
      priceLossMinor: 0,
      netRealizedVarianceAmountMinor: 0,
      netRealizedVarianceMarginMinor: 0,
    },
    periods: [
      {
        periodStart: "2026-03-01",
        // The fixture range starts in January and the as-of date is in
        // November, so March is neither clipped nor the open bucket.
        partial: false,
        inProgress: false,
        poCount: 1,
        amountMinor: 340,
        costedMinor: 200,
        uncostedMinor: 100,
        extraExpenseMinor: 40,
        capitalMinor: 200,
        marginMinor: 0,
        marginBps: 0,
        coverageBps: 6667,
      },
    ],
    categories: [
      {
        businessCategory: "FM",
        label: "Facility Management",
        poCount: 1,
        amountMinor: 340,
        costedMinor: 200,
        uncostedMinor: 100,
        extraExpenseMinor: 40,
        capitalMinor: 200,
        marginMinor: 0,
        marginBps: 0,
        coverageBps: 6667,
      },
    ],
    categorySeries: [
      {
        businessCategory: "FM",
        // The generated declaration says number[]; the wire really carries null.
        marginBps: [0, null, 1250],
      },
    ],
    projection: {
      monthly: {
        status: "available",
        method: "Median of the same calendar month.",
        basis: {
          historyFrom: "2025-01-01",
          historyTo: "2026-01-01",
          completeMonths: 12,
          poCount: 12,
          asOf: "2026-11-19",
        },
        buckets: [
          {
            periodStart: "2026-01-01",
            amountLowMinor: 100,
            amountBaseMinor: 200,
            amountHighMinor: 300,
            capitalMinor: 0,
            marginMinor: 200,
            marginBps: null,
            sampleSize: 3,
            source: "same_calendar_month",
          },
        ],
      },
      yearly: {
        status: "unavailable",
        reason: "insufficient_history",
        method: "The last twelve complete months held flat.",
        basis: {
          historyFrom: "2025-01-01",
          historyTo: "2026-01-01",
          completeMonths: 12,
          poCount: 12,
          asOf: "2026-11-19",
        },
        buckets: [],
      },
    },
    currentProjection: {
      mode: "day",
      status: "available",
      method: "Elapsed-day average of the current month.",
      basis: {
        from: "2026-11-01",
        to: "2026-11-20",
        units: 19,
        poCount: 3,
        asOf: "2026-11-19",
      },
      buckets: [
        {
          periodStart: "2026-11-20",
          amountLowMinor: 100,
          amountBaseMinor: 200,
          amountHighMinor: 300,
          capitalMinor: 0,
          marginMinor: 200,
          marginBps: null,
          sampleSize: 19,
          source: "elapsed_days",
        },
      ],
    },
    notes: [{ code: "legacy_order_adjustment", text: "A disclosed residual." }],
  };
  return wire as unknown as poreport.Report;
}

describe("normalizePurchaseOrderReport (issue #629)", () => {
  it("keeps null category percentages as gaps and zero as a real zero", () => {
    const report = normalizePurchaseOrderReport(reportFixture());

    expect(report.categorySeries[0].marginBps).toEqual([0, null, 1250]);
    expect(report.categorySeries[0].marginBps[0]).toBe(0);
    expect(report.categorySeries[0].marginBps[1]).toBeNull();
  });

  it("represents nullable scalar ratios honestly, including absent values", () => {
    const input = reportFixture();
    // A generated optional field can be absent at runtime; it is unknown, not
    // 0%. Null and 0 must stay distinguishable from that absence.
    input.periods[0].marginBps = undefined;
    input.categories[0].coverageBps = undefined;

    const report = normalizePurchaseOrderReport(input);

    expect(report.totals.marginBps).toBe(0);
    expect(report.totals.coverageBps).toBe(6667);
    expect(report.periods[0].marginBps).toBeNull();
    expect(report.periods[0].coverageBps).toBe(6667);
    expect(report.categories[0].coverageBps).toBeNull();
    expect(report.projection.monthly.buckets[0].marginBps).toBeNull();
  });

  it("copies extra expense as its own pass-through bucket (issue #655)", () => {
    const report = normalizePurchaseOrderReport(reportFixture());

    expect(report.totals.extraExpenseMinor).toBe(150);
    expect(report.periods[0].extraExpenseMinor).toBe(40);
    expect(report.categories[0].extraExpenseMinor).toBe(40);
    // The extra is neither costed nor uncosted: it is its own bucket, so the
    // amount identity only foots when it is named separately.
    expect(report.totals.amountMinor).toBe(
      report.totals.costedMinor +
        report.totals.uncostedMinor +
        report.totals.extraExpenseMinor
    );
  });

  it("reads a payload without extra expense as zero, never a nonzero stand-in", () => {
    const input = reportFixture();
    const omitExtraExpense = (metrics: object): void => {
      delete (metrics as { extraExpenseMinor?: number }).extraExpenseMinor;
    };
    omitExtraExpense(input.totals);
    omitExtraExpense(input.periods[0]);
    omitExtraExpense(input.categories[0]);

    const report = normalizePurchaseOrderReport(input);

    expect(report.totals.extraExpenseMinor).toBe(0);
    expect(report.periods[0].extraExpenseMinor).toBe(0);
    expect(report.categories[0].extraExpenseMinor).toBe(0);
  });

  it("preserves every field, keeps an absent reason absent, and serializes faithfully", () => {
    const input = reportFixture();
    const report = normalizePurchaseOrderReport(input);

    expect(report).toEqual(input);
    expect(JSON.parse(JSON.stringify(report))).toEqual(JSON.parse(JSON.stringify(input)));
    expect("reason" in report.projection.monthly).toBe(false);
    expect("reason" in report.currentProjection).toBe(false);
    expect(report.projection.yearly.reason).toBe("insufficient_history");
  });

  it("normalizes a missing current projection as unavailable, never as an empty forecast", () => {
    const input = reportFixture();
    delete (input as unknown as Record<string, unknown>).currentProjection;

    const report = normalizePurchaseOrderReport(input);

    expect(report.currentProjection.mode).toBe("none");
    expect(report.currentProjection.status).toBe("unavailable");
    expect(report.currentProjection.reason).toBe("not_current_period");
    expect(report.currentProjection.buckets).toEqual([]);
    expect(report.currentProjection.currentMonth).toBeUndefined();
    expect(report.currentProjection.basis.asOf).toBe("2026-11-19");
  });

  it("normalizes the current month's actual and remainder without recomputing them", () => {
    const input = reportFixture();
    input.currentProjection.mode = "month";
    input.currentProjection.currentMonth = {
      actual: input.periods[0],
      remainder: {
        low: 100,
        base: 200,
        high: 300,
        capital: 80,
        margin: 120,
        marginBps: 15_000,
        sampleSize: 19,
        source: "elapsed_days",
      },
    } as unknown as poreport.CurrentMonthProjection;

    const report = normalizePurchaseOrderReport(input);

    expect(report.currentProjection.mode).toBe("month");
    expect(report.currentProjection.currentMonth?.actual.amountMinor).toBe(
      input.periods[0].amountMinor
    );
    expect(report.currentProjection.currentMonth?.remainder).toEqual({
      low: 100,
      base: 200,
      high: 300,
      capital: 80,
      margin: 120,
      marginBps: 15_000,
      sampleSize: 19,
      source: "elapsed_days",
    });
  });

  it("drops a malformed current-month remainder instead of inventing zeros", () => {
    const input = reportFixture();
    input.currentProjection.mode = "month";
    input.currentProjection.currentMonth = {
      actual: input.totals,
      remainder: {
        low: 100,
        base: 200,
        high: 300,
        capital: 0,
        margin: 200,
        marginBps: null,
        sampleSize: 19,
        source: "elapsed_days",
      },
    } as unknown as poreport.CurrentMonthProjection;
    // The generated type says these numbers are required; the wire is not
    // trusted to have sent them, and a partial remainder must not print as 0.
    (input.currentProjection.currentMonth.remainder as unknown as Record<string, unknown>).base =
      undefined;

    const report = normalizePurchaseOrderReport(input);

    expect(report.currentProjection.currentMonth).toBeUndefined();
    expect(report.currentProjection.buckets).toHaveLength(1);
  });
});

describe("monthSummary normalization (issue #641)", () => {
  it("copies the server-owned current-month readout without arithmetic", () => {
    const input = reportFixture();
    input.monthSummary = {
      month: "2026-11",
      from: "2026-11-01",
      to: "2026-12-01",
      asOf: "2026-11-19",
      mode: "day",
      actual: input.totals,
      recordedFuture: {
        poCount: 1,
        amountMinor: 500,
        costedMinor: 450,
        uncostedMinor: 50,
        extraExpenseMinor: 25,
        capitalMinor: 400,
        marginMinor: 50,
        marginBps: 1250,
        coverageBps: 9000,
      },
      projection: {
        status: "available",
        method: "Elapsed-day average",
        remainder: {
          low: 100,
          base: 200,
          high: 300,
          capital: 150,
          margin: 50,
          marginBps: 3333,
          sampleSize: 19,
          source: "elapsed_days",
        },
      },
    } as unknown as poreport.MonthSummary;

    const report = normalizePurchaseOrderReport(input);
    expect(report.monthSummary).toEqual({
      month: "2026-11",
      from: "2026-11-01",
      to: "2026-12-01",
      asOf: "2026-11-19",
      mode: "day",
      actual: normalizePurchaseOrderReport(input).totals,
      recordedFuture: {
        poCount: 1,
        amountMinor: 500,
        costedMinor: 450,
        uncostedMinor: 50,
        extraExpenseMinor: 25,
        capitalMinor: 400,
        marginMinor: 50,
        marginBps: 1250,
        coverageBps: 9000,
      },
      projection: {
        status: "available",
        method: "Elapsed-day average",
        remainder: {
          low: 100,
          base: 200,
          high: 300,
          capital: 150,
          margin: 50,
          marginBps: 3333,
          sampleSize: 19,
          source: "elapsed_days",
        },
      },
    });
  });

  it("drops an unavailable summary instead of inventing a zero one", () => {
    const input = reportFixture();
    input.monthSummary = {
      month: "2026-11",
    } as unknown as poreport.MonthSummary;

    const report = normalizePurchaseOrderReport(input);
    expect(report.monthSummary).toBeUndefined();
  });

  it("keeps a null remainder percentage null and absent reasons absent", () => {
    const input = reportFixture();
    input.monthSummary = {
      month: "2026-11",
      from: "2026-11-01",
      to: "2026-12-01",
      asOf: "2026-11-19",
      mode: "month",
      actual: { ...input.totals, marginBps: null, coverageBps: null },
      projection: {
        status: "available",
        method: "Month view",
        remainder: {
          low: 0,
          base: 0,
          high: 0,
          capital: 0,
          margin: 0,
          marginBps: null,
          sampleSize: 19,
          source: "elapsed_days",
        },
      },
    } as unknown as poreport.MonthSummary;

    const summary = normalizePurchaseOrderReport(input).monthSummary!;
    expect(summary.actual.marginBps).toBeNull();
    expect(summary.actual.coverageBps).toBeNull();
    expect(summary.projection.remainder?.marginBps).toBeNull();
    expect("reason" in summary.projection).toBe(false);
    expect("recordedFuture" in summary).toBe(false);
  });
});
