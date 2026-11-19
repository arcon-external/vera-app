// Issue #630: a stable, normalized report fixture for the Reports page tests
// and the UI snapshot harness. It is deliberately shaped like
// `PurchaseOrderReportData`, never like the generated Wails model: the screen
// reads the normalized contract only.
//
// The figures foot by construction: the three periods and the five categories
// each sum to the totals. The FAB category carries a real loss and the
// Unassigned category carries zero capital, so a null percentage is visible in
// both the screen fixtures and the snapshots.

import type { PurchaseOrderReportData } from "./poReport";

export function purchaseOrderReportFixture(): PurchaseOrderReportData {
  return {
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
      address: "Batamindo Industrial Park, Lot 317, Mukakuning, Batam",
    },
    population: {
      availableCurrencies: ["IDR", "USD"],
      hasUnassigned: true,
      sourceMissingPoCount: 1,
    },
    totals: {
      poCount: 12,
      amountMinor: 480_000_000,
      costedMinor: 432_000_000,
      uncostedMinor: 48_000_000,
      capitalMinor: 390_000_000,
      marginMinor: 42_000_000,
      marginBps: 1077,
      coverageBps: 9000,
    },
    periods: [
      {
        periodStart: "2026-01-01",
        partial: false,
        inProgress: false,
        poCount: 5,
        amountMinor: 200_000_000,
        costedMinor: 180_000_000,
        uncostedMinor: 20_000_000,
        capitalMinor: 165_000_000,
        marginMinor: 15_000_000,
        marginBps: 909,
        coverageBps: 9000,
      },
      {
        periodStart: "2026-02-01",
        partial: false,
        inProgress: false,
        poCount: 4,
        amountMinor: 160_000_000,
        costedMinor: 144_000_000,
        uncostedMinor: 16_000_000,
        capitalMinor: 130_000_000,
        marginMinor: 14_000_000,
        marginBps: 1077,
        coverageBps: 9000,
      },
      {
        periodStart: "2026-03-01",
        partial: true,
        inProgress: true,
        poCount: 3,
        amountMinor: 120_000_000,
        costedMinor: 108_000_000,
        uncostedMinor: 12_000_000,
        capitalMinor: 95_000_000,
        marginMinor: 13_000_000,
        marginBps: 1368,
        coverageBps: 9000,
      },
    ],
    categories: [
      {
        businessCategory: "FM",
        label: "Facility Management",
        poCount: 4,
        amountMinor: 180_000_000,
        costedMinor: 162_000_000,
        uncostedMinor: 18_000_000,
        capitalMinor: 135_000_000,
        marginMinor: 27_000_000,
        marginBps: 2000,
        coverageBps: 9000,
      },
      {
        businessCategory: "FAB",
        label: "Fabrication",
        poCount: 3,
        amountMinor: 120_000_000,
        costedMinor: 120_000_000,
        uncostedMinor: 0,
        capitalMinor: 150_000_000,
        marginMinor: -30_000_000,
        marginBps: -2000,
        coverageBps: 10000,
      },
      {
        businessCategory: "SUPPLY_GEN",
        label: "Goods Supply & General",
        poCount: 3,
        amountMinor: 110_000_000,
        costedMinor: 99_000_000,
        uncostedMinor: 11_000_000,
        capitalMinor: 75_000_000,
        marginMinor: 24_000_000,
        marginBps: 3200,
        coverageBps: 9000,
      },
      {
        businessCategory: "OTHERS",
        label: "Others",
        poCount: 1,
        amountMinor: 40_000_000,
        costedMinor: 36_000_000,
        uncostedMinor: 4_000_000,
        capitalMinor: 30_000_000,
        marginMinor: 6_000_000,
        marginBps: 2000,
        coverageBps: 9000,
      },
      {
        businessCategory: "UNASSIGNED",
        label: "Unassigned",
        poCount: 1,
        amountMinor: 30_000_000,
        costedMinor: 15_000_000,
        uncostedMinor: 15_000_000,
        capitalMinor: 0,
        marginMinor: 15_000_000,
        marginBps: null,
        coverageBps: 5000,
      },
    ],
    categorySeries: [
      { businessCategory: "FM", marginBps: [2000, 2000, 2000] },
      { businessCategory: "FAB", marginBps: [-2000, null, -2000] },
      { businessCategory: "SUPPLY_GEN", marginBps: [3200, 3200, 3200] },
      { businessCategory: "OTHERS", marginBps: [2000, null, null] },
    ],
    projection: {
      monthly: {
        status: "available",
        method:
          "Median of the same calendar month across complete history; P25-P75 range (min-max under four observations), trailing three complete months when that calendar month has no history.",
        basis: {
          historyFrom: "2025-01-01",
          historyTo: "2026-01-01",
          completeMonths: 12,
          poCount: 24,
          asOf: "2026-11-19",
        },
        buckets: [
          {
            periodStart: "2026-04-01",
            amountLowMinor: 90_000_000,
            amountBaseMinor: 110_000_000,
            amountHighMinor: 130_000_000,
            capitalMinor: 80_000_000,
            marginMinor: 30_000_000,
            marginBps: 3750,
            sampleSize: 3,
            source: "same_calendar_month",
          },
          {
            periodStart: "2026-05-01",
            amountLowMinor: 100_000_000,
            amountBaseMinor: 120_000_000,
            amountHighMinor: 140_000_000,
            capitalMinor: 0,
            marginMinor: 120_000_000,
            marginBps: null,
            sampleSize: 4,
            source: "same_calendar_month",
          },
        ],
      },
      yearly: {
        status: "unavailable",
        reason: "insufficient_history",
        method:
          "The last twelve complete months held flat; P25-P75 range of rolling twelve-month totals, widened only to include the base.",
        basis: {
          historyFrom: "2025-01-01",
          historyTo: "2026-01-01",
          completeMonths: 12,
          poCount: 24,
          asOf: "2026-11-19",
        },
        buckets: [],
      },
    },
    notes: [
      {
        code: "legacy_order_adjustment",
        text: "One purchase order in this range carries a retired whole-order adjustment. The amount includes it; no margin is inferred from it.",
      },
    ],
  };
}
