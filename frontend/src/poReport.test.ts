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
      amountMinor: 900,
      costedMinor: 600,
      uncostedMinor: 300,
      capitalMinor: 600,
      marginMinor: 0,
      marginBps: 0,
      coverageBps: 6667,
    },
    periods: [
      {
        periodStart: "2026-03-01",
        // The fixture range starts in January and the as-of date is in
        // November, so March is neither clipped nor the open bucket.
        partial: false,
        inProgress: false,
        poCount: 1,
        amountMinor: 300,
        costedMinor: 200,
        uncostedMinor: 100,
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
        amountMinor: 300,
        costedMinor: 200,
        uncostedMinor: 100,
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

  it("preserves every field, keeps an absent reason absent, and serializes faithfully", () => {
    const input = reportFixture();
    const report = normalizePurchaseOrderReport(input);

    expect(report).toEqual(input);
    expect(JSON.parse(JSON.stringify(report))).toEqual(JSON.parse(JSON.stringify(input)));
    expect("reason" in report.projection.monthly).toBe(false);
    expect(report.projection.yearly.reason).toBe("insufficient_history");
  });
});
