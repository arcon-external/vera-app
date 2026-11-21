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
    // The fixture is "this year to 19 November 2026", not exactly the current
    // year, so the current-period contract says: no ghost, with a stated
    // reason. The Month and Year fixtures below carry an available forecast.
    currentProjection: {
      mode: "none",
      status: "unavailable",
      reason: "not_current_period",
      method:
        "The report range is not exactly the current calendar month or year; no current-period projection is computed.",
      basis: {
        from: "",
        to: "",
        units: 0,
        poCount: 0,
        asOf: "2026-11-19",
      },
      buckets: [],
    },
    notes: [
      {
        code: "legacy_order_adjustment",
        text: "One purchase order in this range carries a retired whole-order adjustment. The amount includes it; no margin is inferred from it.",
      },
    ],
  };
}

/**
 * Issue #638: the current calendar month, bucketed by day. Nineteen days have
 * elapsed, eleven are still to come; the fixture materializes all thirty
 * server periods (the store fills empty days with zero) and the current-period
 * projection supplies the ghost days after the as-of date. The figures foot:
 * period rows sum to totals, and every category row derives its own ratios.
 */
export function purchaseOrderReportMonthFixture(): PurchaseOrderReportData {
  const elapsed = 19;
  const totalDays = 30;
  const counts = splitExact(12, elapsed);
  // Revenue belongs only to a day that received an order. Splitting the money
  // across all elapsed days produced "0 POs, Rp10M" rows: the totals footed but
  // the table lied line by line.
  const active = counts.filter((count) => count > 0).length;
  const amounts = splitExact(190_000_000, active);
  // The current-period forecast requires a fully costed sample: an uncosted
  // figure is never treated as margin. The day and year fixtures share this
  // current-month arithmetic so their tables and remainder agree.
  const uncosted = splitExact(0, active);
  const capitals = splitExact(152_000_000, active);
  let activeIndex = 0;

  const periods = Array.from({ length: totalDays }, (_, index) => {
    if (index >= elapsed) {
      return {
        periodStart: `2026-11-${String(index + 1).padStart(2, "0")}`,
        partial: false,
        inProgress: false,
        ...metrics(0, 0, 0, 0),
        marginBps: null,
        coverageBps: null,
      };
    }
    if (counts[index] === 0) {
      return {
        periodStart: `2026-11-${String(index + 1).padStart(2, "0")}`,
        partial: false,
        inProgress: index === elapsed - 1,
        ...metrics(0, 0, 0, 0),
        marginBps: null,
        coverageBps: null,
      };
    }
    const amount = amounts[activeIndex] ?? 0;
    const uncostedPart = uncosted[activeIndex] ?? 0;
    const capital = capitals[activeIndex] ?? 0;
    activeIndex += 1;
    return {
      periodStart: `2026-11-${String(index + 1).padStart(2, "0")}`,
      partial: false,
      inProgress: index === elapsed - 1,
      ...metrics(counts[index], amount, uncostedPart, capital),
    };
  });

  const totalCount = periods.reduce((sum, period) => sum + period.poCount, 0);
  const totalAmount = periods.reduce((sum, period) => sum + period.amountMinor, 0);
  const totalUncosted = periods.reduce(
    (sum, period) => sum + period.uncostedMinor,
    0
  );
  const totalCapital = periods.reduce(
    (sum, period) => sum + period.capitalMinor,
    0
  );
  const totals = metrics(totalCount, totalAmount, totalUncosted, totalCapital);

  const categoryAmounts = splitExact(totalAmount, 3);
  const categoryUncosted = splitExact(totalUncosted, 3);
  const categoryCapital = splitExact(totalCapital, 3);
  const categoryCounts = splitExact(totalCount, 3);
  const categoryRows = [
    { businessCategory: "FM", label: "Facility Management" },
    { businessCategory: "FAB", label: "Fabrication" },
    { businessCategory: "OTHERS", label: "Others" },
  ].map((row, index) => ({
    ...row,
    ...metrics(
      categoryCounts[index],
      categoryAmounts[index],
      categoryUncosted[index],
      categoryCapital[index]
    ),
  }));

  const low = 8_000_000;
  const base = 10_000_000;
  const high = 12_000_000;
  const capital = 8_000_000;
  const margin = base - capital;
  const buckets = Array.from({ length: totalDays - elapsed }, (_, index) => ({
    periodStart: `2026-11-${String(elapsed + index + 1).padStart(2, "0")}`,
    amountLowMinor: low,
    amountBaseMinor: base,
    amountHighMinor: high,
    capitalMinor: capital,
    marginMinor: margin,
    marginBps: fixtureRatioBps(margin, capital),
    sampleSize: elapsed,
    source: "elapsed_days",
  }));

  return {
    generatedAt: "2026-11-19T05:00:00Z",
    asOf: "2026-11-19",
    filter: {
      from: "2026-11-01",
      to: "2026-12-01",
      bucket: "day",
      category: "all",
      currency: "IDR",
    },
    company: {
      name: "PT Arcon Penta Persada",
      address: "Batamindo Industrial Park, Lot 317, Mukakuning, Batam",
    },
    population: {
      availableCurrencies: ["IDR", "USD"],
      hasUnassigned: false,
      sourceMissingPoCount: 0,
    },
    totals,
    periods,
    categories: categoryRows,
    categorySeries: [],
    projection: {
      monthly: {
        status: "unavailable",
        reason: "not_current_period",
        method: "",
        basis: {
          historyFrom: "",
          historyTo: "",
          completeMonths: 0,
          poCount: 0,
          asOf: "2026-11-19",
        },
        buckets: [],
      },
      yearly: {
        status: "unavailable",
        reason: "not_current_period",
        method: "",
        basis: {
          historyFrom: "",
          historyTo: "",
          completeMonths: 0,
          poCount: 0,
          asOf: "2026-11-19",
        },
        buckets: [],
      },
    },
    currentProjection: {
      mode: "day",
      status: "available",
      method:
        "Elapsed-day average of the current month; P25-P75 range of elapsed daily totals (min-max under four observations).",
      basis: {
        from: "2026-11-01",
        to: "2026-11-20",
        units: elapsed,
        poCount: totalCount,
        asOf: "2026-11-19",
      },
      buckets,
    },
    monthSummary: {
      month: "2026-11",
      from: "2026-11-01",
      to: "2026-12-01",
      asOf: "2026-11-19",
      mode: "day",
      actual: totals,
      projection: {
        status: "available",
        method:
          "Elapsed-day average of the current month; P25-P75 range of elapsed daily totals (min-max under four observations).",
        remainder: {
          low: low * (totalDays - elapsed),
          base: base * (totalDays - elapsed),
          high: high * (totalDays - elapsed),
          capital: capital * (totalDays - elapsed),
          margin: margin * (totalDays - elapsed),
          marginBps: fixtureRatioBps(margin, capital),
          sampleSize: elapsed,
          source: "elapsed_days",
        },
      },
    },
    notes: [],
  };
}

/**
 * Issue #641: the current calendar year, bucketed by month. It shares the
 * month fixture's current-month actual and remainder so the day table, the
 * monthly table and the chart quote one arithmetic. January to October are
 * complete, fully costed months; November is the month-to-date current month;
 * December is the single ghost month. No forecast is summed into an actual.
 */
export function purchaseOrderReportYearFixture(): PurchaseOrderReportData {
  const month = purchaseOrderReportMonthFixture();
  const base = purchaseOrderReportFixture();
  const monthSummary = month.monthSummary!;

  const periods = base.periods.slice(0, 10).map((period) => ({
    periodStart: period.periodStart,
    partial: false,
    inProgress: false,
    ...metrics(period.poCount, period.amountMinor, 0, period.capitalMinor),
  }));
  periods.push({
    periodStart: "2026-11-01",
    partial: false,
    inProgress: true,
    ...month.totals,
  });
  periods.push({
    periodStart: "2026-12-01",
    partial: false,
    inProgress: false,
    ...metrics(0, 0, 0, 0),
  });

  const totalCount = periods.reduce((sum, period) => sum + period.poCount, 0);
  const totalAmount = periods.reduce((sum, period) => sum + period.amountMinor, 0);
  const totalCapital = periods.reduce(
    (sum, period) => sum + period.capitalMinor,
    0
  );
  const totals = metrics(totalCount, totalAmount, 0, totalCapital);

  const categoryDefinitions = [
    { businessCategory: "FM", label: "Facility Management" },
    { businessCategory: "FAB", label: "Fabrication" },
    { businessCategory: "SUPPLY_GEN", label: "Goods Supply & General" },
    { businessCategory: "OTHERS", label: "Others" },
    { businessCategory: "UNASSIGNED", label: "Unassigned" },
  ] as const;
  const categoryCounts = splitExact(totals.poCount, categoryDefinitions.length);
  const categoryAmounts = splitExact(
    totals.amountMinor,
    categoryDefinitions.length
  );
  const categoryCapitals = splitExact(
    totals.capitalMinor,
    categoryDefinitions.length
  );
  const categories = categoryDefinitions.map((row, index) => ({
    ...row,
    ...metrics(
      categoryCounts[index],
      categoryAmounts[index],
      0,
      categoryCapitals[index]
    ),
  }));

  const basis = {
    historyFrom: "2026-03-01",
    historyTo: "2026-11-01",
    completeMonths: 8,
    poCount: 8,
    asOf: "2026-11-19",
  };

  return {
    ...base,
    filter: {
      from: "2026-01-01",
      to: "2027-01-01",
      bucket: "month",
      category: "all",
      currency: "IDR",
    },
    totals,
    periods,
    categories,
    categorySeries: [],
    projection: {
      monthly: {
        status: "unavailable",
        reason: "not_current_period",
        method: "",
        basis,
        buckets: [],
      },
      yearly: {
        status: "unavailable",
        reason: "not_current_period",
        method: "",
        basis,
        buckets: [],
      },
    },
    currentProjection: {
      mode: "month",
      status: "available",
      method:
        "Median of the complete months in trailing history; P25-P75 range of their monthly totals. The current month's remainder uses the elapsed-day average.",
      basis: {
        from: "2026-08-01",
        to: "2026-11-01",
        units: 3,
        poCount: 5,
        asOf: "2026-11-19",
      },
      buckets: [
        {
          periodStart: "2026-12-01",
          amountLowMinor: 40_000_000,
          amountBaseMinor: 50_000_000,
          amountHighMinor: 60_000_000,
          capitalMinor: 40_000_000,
          marginMinor: 10_000_000,
          marginBps: fixtureRatioBps(10_000_000, 40_000_000),
          sampleSize: 3,
          source: "trailing_complete_months",
        },
      ],
      currentMonth: {
        actual: month.totals,
        remainder: monthSummary.projection.remainder!,
      },
    },
    monthSummary: {
      month: "2026-11",
      from: "2026-11-01",
      to: "2026-12-01",
      asOf: "2026-11-19",
      mode: "month",
      actual: month.totals,
      projection: {
        status: "available",
        method:
          "Median of the complete months in trailing history; P25-P75 range of their monthly totals. The current month's remainder uses the elapsed-day average.",
        remainder: monthSummary.projection.remainder!,
      },
    },
    notes: base.notes,
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

/**
 * Issue #641 correction F2: a recorded future-dated purchase order is real
 * saved evidence, not a line added only to monthSummary. These fixtures add the
 * same order to the raw period row, totals, the FM category row and the
 * monthSummary.recordedFuture, so the month browser's actual + recorded future
 * foots to the snapshot's totals and every category/coverage ratio is derived
 * from the updated money.
 */
const RECORDED_FUTURE_METRICS: PurchaseOrderReportMetrics = {
  poCount: 1,
  amountMinor: 777_000,
  costedMinor: 700_000,
  uncostedMinor: 77_000,
  capitalMinor: 600_000,
  marginMinor: 100_000,
  marginBps: fixtureRatioBps(100_000, 600_000),
  coverageBps: fixtureRatioBps(700_000, 777_000),
};

function addFixtureMetrics(
  base: PurchaseOrderReportMetrics,
  add: PurchaseOrderReportMetrics
): PurchaseOrderReportMetrics {
  return metrics(
    base.poCount + add.poCount,
    base.amountMinor + add.amountMinor,
    base.uncostedMinor + add.uncostedMinor,
    base.capitalMinor + add.capitalMinor
  );
}

function withRecordedFuture(
  report: PurchaseOrderReportData,
  periodStart: string
): PurchaseOrderReportData {
  const future = { ...RECORDED_FUTURE_METRICS };
  const periods = report.periods.map((period) =>
    period.periodStart === periodStart
      ? { ...period, ...addFixtureMetrics(period, future) }
      : period
  );
  const categories = report.categories.map((category) =>
    category.businessCategory === "FM"
      ? { ...category, ...addFixtureMetrics(category, future) }
      : category
  );
  return {
    ...report,
    totals: addFixtureMetrics(report.totals, future),
    periods,
    categories,
    monthSummary: {
      ...report.monthSummary!,
      recordedFuture: future,
    },
  };
}

/**
 * The coherent day fixture with one future-dated saved order. The day row is
 * future-dated, so the chart's actual period filter still excludes it while
 * the raw totals and category rows include it; monthSummary.actual remains the
 * elapsed sample and recordedFuture carries the one order separately.
 */
export function purchaseOrderReportMonthWithRecordedFutureFixture(): PurchaseOrderReportData {
  return withRecordedFuture(
    structuredClone(purchaseOrderReportMonthFixture()),
    "2026-11-25"
  );
}

/**
 * The coherent year fixture with one future-dated saved order. November's raw
 * row and the year totals include it; monthSummary.actual and
 * currentProjection.currentMonth.actual stay month-to-date, and the difference
 * is recordedFuture.
 */
export function purchaseOrderReportYearWithRecordedFutureFixture(): PurchaseOrderReportData {
  return withRecordedFuture(
    structuredClone(purchaseOrderReportYearFixture()),
    "2026-11-01"
  );
}
