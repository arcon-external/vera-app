// Issue #630: a stable, normalized report fixture for the Reports page tests
// and the UI snapshot harness. It is deliberately shaped like
// `PurchaseOrderReportData`, never like the generated Wails model: the screen
// reads the normalized contract only.
//
// The figures foot by construction: the eleven periods and the five categories
// each sum to the totals. Every period's money is split from the totals with
// integer weights, and every percentage is derived from its own margin/capital
// and costed/amount pair with `fixtureRatioBps`, so a fixture can never print
// 11.11% beside a 12.50% relationship. The FAB category carries a real loss and
// the Unassigned category carries zero capital, so a null percentage is visible
// in both the screen fixture and the snapshots.
//
// The dates are coherent with the filter: the report is "this year to today"
// on 19 November 2026, so November is the clipped (partial) month and the month
// that contains the as-of date. It is the only row allowed to say "Partial".

import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
} from "./poReport";

/**
 * Basis points as the server computes them: numerator/denominator rounded half
 * away from zero, and null when the denominator is not positive. Fixture-only
 * arithmetic; the screen never recalculates a server figure.
 */
export function fixtureRatioBps(
  numerator: number,
  denominator: number
): number | null {
  if (denominator <= 0) return null;
  const sign = numerator < 0 ? -1 : 1;
  const rounded = Math.round((Math.abs(numerator) * 10000) / denominator);
  return sign * rounded;
}

/** Splits an exact integer total into weighted parts that sum back to it. */
function splitWeighted(total: number, weights: number[]): number[] {
  const weightSum = weights.reduce((sum, weight) => sum + weight, 0);
  let assigned = 0;
  return weights.map((weight, index) => {
    if (index === weights.length - 1) return total - assigned;
    const part = Math.floor((total * weight) / weightSum);
    assigned += part;
    return part;
  });
}

function metrics(
  poCount: number,
  amountMinor: number,
  uncostedMinor: number,
  capitalMinor: number
): PurchaseOrderReportMetrics {
  const costedMinor = amountMinor - uncostedMinor;
  const marginMinor = costedMinor - capitalMinor;
  return {
    poCount,
    amountMinor,
    costedMinor,
    uncostedMinor,
    capitalMinor,
    marginMinor,
    marginBps: fixtureRatioBps(marginMinor, capitalMinor),
    coverageBps: fixtureRatioBps(costedMinor, amountMinor),
  };
}

const PERIOD_STARTS = [
  "2026-01-01",
  "2026-02-01",
  "2026-03-01",
  "2026-04-01",
  "2026-05-01",
  "2026-06-01",
  "2026-07-01",
  "2026-08-01",
  "2026-09-01",
  "2026-10-01",
  "2026-11-01",
];

// The last month is deliberately the smallest share: it is the month-to-date
// partial bucket, not an invented full month.
const COUNT_WEIGHTS = [2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1];
const AMOUNT_WEIGHTS = [12, 10, 9, 9, 9, 9, 9, 9, 9, 9, 6];
const CAPITAL_WEIGHTS = [13, 11, 9, 9, 9, 9, 9, 9, 8, 8, 6];

export function purchaseOrderReportFixture(): PurchaseOrderReportData {
  const counts = splitWeighted(12, COUNT_WEIGHTS);
  const amounts = splitWeighted(480_000_000, AMOUNT_WEIGHTS);
  const uncosted = splitWeighted(48_000_000, AMOUNT_WEIGHTS);
  const capitals = splitWeighted(390_000_000, CAPITAL_WEIGHTS);
  const periods = PERIOD_STARTS.map((periodStart, index) => ({
    periodStart,
    partial: index === PERIOD_STARTS.length - 1,
    inProgress: index === PERIOD_STARTS.length - 1,
    ...metrics(counts[index], amounts[index], uncosted[index], capitals[index]),
  }));

  const totals = metrics(12, 480_000_000, 48_000_000, 390_000_000);
  const basis = {
    historyFrom: "2026-03-01",
    historyTo: "2026-11-01",
    completeMonths: 8,
    poCount: 8,
    asOf: "2026-11-19",
  };

  return {
    generatedAt: "2026-11-19T05:00:00Z",
    asOf: "2026-11-19",
    filter: {
      from: "2026-01-01",
      to: "2026-11-20",
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
    totals,
    periods,
    categories: [
      {
        businessCategory: "FM",
        label: "Facility Management",
        ...metrics(4, 180_000_000, 18_000_000, 135_000_000),
      },
      {
        businessCategory: "FAB",
        label: "Fabrication",
        ...metrics(3, 120_000_000, 0, 150_000_000),
      },
      {
        businessCategory: "SUPPLY_GEN",
        label: "Goods Supply & General",
        ...metrics(3, 110_000_000, 11_000_000, 75_000_000),
      },
      {
        businessCategory: "OTHERS",
        label: "Others",
        ...metrics(1, 40_000_000, 4_000_000, 30_000_000),
      },
      {
        businessCategory: "UNASSIGNED",
        label: "Unassigned",
        ...metrics(1, 30_000_000, 15_000_000, 0),
      },
    ],
    // There is no category-by-period money in the payload, so a truthful
    // aligned series cannot be constructed from this fixture. The chart is a
    // later consumer (#631); an empty series is honest, a fabricated one is
    // not. The shared full snapshot carries a truthful series instead.
    categorySeries: [],
    projection: {
      monthly: {
        status: "available",
        method:
          "Median of the same calendar month across complete history; P25-P75 range (min-max under four observations), trailing three complete months when that calendar month has no history.",
        basis,
        buckets: [
          {
            periodStart: "2026-12-01",
            amountLowMinor: 40_000_000,
            amountBaseMinor: 44_000_000,
            amountHighMinor: 49_000_000,
            capitalMinor: 36_000_000,
            marginMinor: 8_000_000,
            marginBps: fixtureRatioBps(8_000_000, 36_000_000),
            sampleSize: 8,
            source: "same_calendar_month",
          },
          {
            periodStart: "2027-01-01",
            amountLowMinor: 35_000_000,
            amountBaseMinor: 40_000_000,
            amountHighMinor: 46_000_000,
            capitalMinor: 0,
            marginMinor: 40_000_000,
            marginBps: null,
            sampleSize: 2,
            source: "same_calendar_month",
          },
        ],
      },
      yearly: {
        status: "unavailable",
        reason: "insufficient_history",
        method:
          "The last twelve complete months held flat; P25-P75 range of rolling twelve-month totals, widened only to include the base.",
        basis,
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

/** Splits an exact integer total into `parts` whole parts that sum back to it. */
function splitExact(total: number, parts: number): number[] {
  const base = Math.floor(total / parts);
  const remainder = total - base * parts;
  return Array.from({ length: parts }, (_, index) =>
    index < remainder ? base + 1 : base
  );
}

/**
 * Issue #630 review: the same footed shape at JavaScript's largest exact
 * figure. It exists so the 360px KPI strip and the snapshot harness are read
 * against `MAX_SAFE_INTEGER`, not only against a comfortable Rp480m. Every row
 * still sums to the total and every percentage is derived from that row's own
 * money: at these figures the margin is only 1000 minor units against capital
 * near MAX_SAFE_INTEGER, so the honest percentage is 0.00%, not a synthetic
 * MAX_SAFE_INTEGER string. The widest percentage text is guarded separately as
 * a standalone formatting/width case.
 */
export function purchaseOrderReportMaxFixture(): PurchaseOrderReportData {
  const report = purchaseOrderReportFixture();
  const max = Number.MAX_SAFE_INTEGER;
  const costed = max - 5;
  const uncosted = 5;
  const capital = max - 1005;

  const periodCounts = splitExact(max, report.periods.length);
  const periodAmounts = splitExact(max, report.periods.length);
  const periodUncosted = splitExact(uncosted, report.periods.length);
  const periodCapital = splitExact(capital, report.periods.length);

  report.periods = report.periods.map((period, index) => {
    // Costed is the amount's own remainder, not a second independent split:
    // two independent splits round their remainders onto different rows and
    // the row would stop footing by one minor unit.
    const costedMinor = periodAmounts[index] - periodUncosted[index];
    const margin = costedMinor - periodCapital[index];
    return {
      ...period,
      poCount: periodCounts[index],
      amountMinor: periodAmounts[index],
      costedMinor,
      uncostedMinor: periodUncosted[index],
      capitalMinor: periodCapital[index],
      marginMinor: margin,
      marginBps: fixtureRatioBps(margin, periodCapital[index]),
      coverageBps: fixtureRatioBps(costedMinor, periodAmounts[index]),
    };
  });

  const categoryCounts = splitExact(max, report.categories.length);
  const categoryAmounts = splitExact(max, report.categories.length);
  const categoryUncosted = splitExact(uncosted, report.categories.length);
  const categoryCapital = splitExact(capital, report.categories.length);

  report.categories = report.categories.map((category, index) => {
    const costedMinor = categoryAmounts[index] - categoryUncosted[index];
    const margin = costedMinor - categoryCapital[index];
    return {
      ...category,
      poCount: categoryCounts[index],
      amountMinor: categoryAmounts[index],
      costedMinor,
      uncostedMinor: categoryUncosted[index],
      capitalMinor: categoryCapital[index],
      marginMinor: margin,
      marginBps: fixtureRatioBps(margin, categoryCapital[index]),
      coverageBps: fixtureRatioBps(costedMinor, categoryAmounts[index]),
    };
  });

  const margin = costed - capital;
  report.totals = {
    poCount: max,
    amountMinor: max,
    costedMinor: costed,
    uncostedMinor: uncosted,
    capitalMinor: capital,
    marginMinor: margin,
    marginBps: fixtureRatioBps(margin, capital),
    coverageBps: fixtureRatioBps(costed, max),
  };
  return report;
}
