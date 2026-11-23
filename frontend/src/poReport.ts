// Issues #629/#630/#631: the purchase-order report's honest desktop shape.
//
// Wails generates CategorySeries.marginBps as number[], but the wire carries a
// null entry for a period whose category had zero capital, and the scalar
// percentage pointers are nullable too. The generated declaration cannot state
// either truth, so this module owns the one desktop contract the chart and the
// PDF import. It is a pure mapping: no fetch, no formatting, no UI state.
//
// Null is a gap, never 0%: a category with no capital has an unknown margin,
// and a ratio of exactly zero is a real measured zero. The normalizer keeps
// that distinction and copies every field, so serializing the result cannot
// silently drop or invent a figure.

import type { poreport } from "@backend/models";

export interface PurchaseOrderReportQuery {
  from: string;
  to: string;
  bucket: string;
  category: string;
  currency: string;
}

export interface PurchaseOrderReportCompany {
  name: string;
  address: string;
}

export interface PurchaseOrderReportPopulation {
  availableCurrencies: string[];
  hasUnassigned: boolean;
  sourceMissingPoCount: number;
}

export interface PurchaseOrderReportMetrics {
  poCount: number;
  amountMinor: number;
  costedMinor: number;
  uncostedMinor: number;
  capitalMinor: number;
  marginMinor: number;
  /** Null when capital is zero; 0 is a real measured percentage. */
  marginBps: number | null;
  /** Null when amount is zero; 0 is a real measured percentage. */
  coverageBps: number | null;
}

/**
 * Open Unsold remaining on still-orderable quotations (#653). It is never
 * added to Actual and is not a loss.
 */
export interface PurchaseOrderReportPipeline {
  unsoldAmountMinor: number;
  unsoldMarginMinor: number;
}

/**
 * Realized commercial variance (#653). It is a comparison, not revenue: open
 * Unsold is mirrored for the screen but excluded from the net. Closed loss is
 * the unbought remainder of an explicitly closed quotation (#656); it is
 * subtracted from the net, and reopening the quotation restores Unsold.
 */
export interface PurchaseOrderReportVariance {
  quantityExtraIncomeAmountMinor: number;
  quantityExtraIncomeMarginMinor: number;
  unsoldAmountMinor: number;
  unsoldMarginMinor: number;
  closedLossAmountMinor: number;
  closedLossMarginMinor: number;
  priceExtraIncomeMinor: number;
  priceLossMinor: number;
  netRealizedVarianceAmountMinor: number;
  netRealizedVarianceMarginMinor: number;
}

export interface PurchaseOrderReportPeriod extends PurchaseOrderReportMetrics {
  periodStart: string;
  partial: boolean;
  inProgress: boolean;
}

export interface PurchaseOrderReportCategory extends PurchaseOrderReportMetrics {
  businessCategory: string;
  label: string;
}

export interface PurchaseOrderReportCategorySeries {
  businessCategory: string;
  /**
   * Aligned to the actual periods. A null entry is a gap (zero capital in that
   * period), never "0%"; a zero entry is a measured zero margin.
   */
  marginBps: Array<number | null>;
}

export interface PurchaseOrderReportProjectionBasis {
  /** First observed PO month; included in completeMonths. */
  historyFrom: string;
  /** Exclusive first day after the last complete month. */
  historyTo: string;
  completeMonths: number;
  poCount: number;
  asOf: string;
}

export interface PurchaseOrderReportProjectedBucket {
  periodStart: string;
  amountLowMinor: number;
  amountBaseMinor: number;
  amountHighMinor: number;
  capitalMinor: number;
  marginMinor: number;
  /** Null when capital is zero; 0 is a real measured percentage. */
  marginBps: number | null;
  sampleSize: number;
  source: string;
}

export interface PurchaseOrderReportProjectionSeries {
  status: string;
  /** Absent when the series is available; a stable reason code otherwise. */
  reason?: string;
  method: string;
  basis: PurchaseOrderReportProjectionBasis;
  buckets: PurchaseOrderReportProjectedBucket[];
}

export interface PurchaseOrderReportProjection {
  monthly: PurchaseOrderReportProjectionSeries;
  yearly: PurchaseOrderReportProjectionSeries;
}

/** The window and sample a current-period forecast used. */
export interface PurchaseOrderReportCurrentProjectionBasis {
  /** Inclusive first day (day mode) or first complete month (month mode). */
  from: string;
  /** Exclusive first day after the sample. */
  to: string;
  /** Elapsed days in day mode, complete months in month mode. */
  units: number;
  poCount: number;
  asOf: string;
}

/**
 * The projected rest of the current month, as totals for all remaining days.
 * The band is on the amount only; capital is a single derived figure and
 * margin is derived from the pair.
 */
export interface PurchaseOrderReportCurrentProjectionRemainder {
  low: number;
  base: number;
  high: number;
  capital: number;
  margin: number;
  /** Null when capital is zero; 0 is a real measured percentage. */
  marginBps: number | null;
  sampleSize: number;
  source: string;
}

/** The current month's month-to-date actuals beside its projected remainder. */
export interface PurchaseOrderReportCurrentMonthProjection {
  actual: PurchaseOrderReportMetrics;
  remainder: PurchaseOrderReportCurrentProjectionRemainder;
}

/**
 * The current month's forecast remainder, or the stable reason it could not be
 * produced. `remainder` is present only when available; a month-end zero
 * remainder is still an available forecast.
 */
export interface PurchaseOrderReportMonthSummaryProjection {
  status: string;
  /** Absent when available; a stable reason code otherwise. */
  reason?: string;
  method: string;
  remainder?: PurchaseOrderReportCurrentProjectionRemainder;
}

/**
 * The server-owned current-month readout. `actual` is month-to-date evidence,
 * `recordedFuture` is saved evidence dated after the as-of day, and the
 * projection is an estimate. They are three quantities and no two are summed.
 * A report outside the exact current-month/day or current-year/month shape has
 * no monthSummary at all (absent is not zero).
 */
export interface PurchaseOrderReportMonthSummary {
  /** Canonical YYYY-MM. */
  month: string;
  /** YYYY-MM-01, inclusive. */
  from: string;
  /** First day of the next month, exclusive. */
  to: string;
  /** WIB business date, YYYY-MM-DD. */
  asOf: string;
  /** "day" or "month". */
  mode: string;
  actual: PurchaseOrderReportMetrics;
  /** Present only when recorded future-dated evidence exists. */
  recordedFuture?: PurchaseOrderReportMetrics;
  projection: PurchaseOrderReportMonthSummaryProjection;
}

/**
 * The forecast for the period still in progress. `mode` is "day" when the
 * report is exactly the current month bucketed by day, "month" when it is
 * exactly the current year, and "none" otherwise. Buckets are future days
 * (day mode) or future months after the current month (month mode); no bucket
 * is ever added to totals or actual periods.
 */
export interface PurchaseOrderReportCurrentProjection {
  mode: string;
  status: string;
  /** Absent when available; a stable reason code otherwise. */
  reason?: string;
  method: string;
  basis: PurchaseOrderReportCurrentProjectionBasis;
  buckets: PurchaseOrderReportProjectedBucket[];
  /** Present in month mode only, and only when the forecast is available. */
  currentMonth?: PurchaseOrderReportCurrentMonthProjection;
}

export interface PurchaseOrderReportNote {
  code: string;
  text: string;
}

export interface PurchaseOrderReportData {
  generatedAt: string;
  asOf: string;
  filter: PurchaseOrderReportQuery;
  company: PurchaseOrderReportCompany;
  population: PurchaseOrderReportPopulation;
  totals: PurchaseOrderReportMetrics;
  /** Open Unsold, never added to Actual (#653). */
  pipeline: PurchaseOrderReportPipeline;
  /** Realized variance, never added to Actual (#653). */
  variance: PurchaseOrderReportVariance;
  periods: PurchaseOrderReportPeriod[];
  categories: PurchaseOrderReportCategory[];
  categorySeries: PurchaseOrderReportCategorySeries[];
  projection: PurchaseOrderReportProjection;
  /**
   * The current-period forecast. It is separate from every actual figure so it
   * can never be summed into totals or period rows.
   */
  currentProjection: PurchaseOrderReportCurrentProjection;
  /**
   * The server-owned current-month readout, absent outside an exact current
   * month/day or current year/month report.
   */
  monthSummary?: PurchaseOrderReportMonthSummary;
  notes: PurchaseOrderReportNote[];
}

/** Null or absent is unknown; a numeric zero is a measured zero. */
function nullableRatio(value: number | null | undefined): number | null {
  return typeof value === "number" ? value : null;
}

function normalizeMetrics(metrics: poreport.Metrics): PurchaseOrderReportMetrics {
  return {
    poCount: metrics.poCount,
    amountMinor: metrics.amountMinor,
    costedMinor: metrics.costedMinor,
    uncostedMinor: metrics.uncostedMinor,
    capitalMinor: metrics.capitalMinor,
    marginMinor: metrics.marginMinor,
    marginBps: nullableRatio(metrics.marginBps),
    coverageBps: nullableRatio(metrics.coverageBps),
  };
}

function normalizePeriod(period: poreport.Period): PurchaseOrderReportPeriod {
  return {
    periodStart: period.periodStart,
    partial: period.partial,
    inProgress: period.inProgress,
    ...normalizeMetrics(period),
  };
}

function normalizeCategory(category: poreport.Category): PurchaseOrderReportCategory {
  return {
    businessCategory: category.businessCategory,
    label: category.label,
    ...normalizeMetrics(category),
  };
}

function normalizeCategorySeries(series: poreport.CategorySeries): PurchaseOrderReportCategorySeries {
  return {
    businessCategory: series.businessCategory,
    marginBps: series.marginBps.map(nullableRatio),
  };
}

function normalizeBasis(basis: poreport.ProjectionBasis): PurchaseOrderReportProjectionBasis {
  return {
    historyFrom: basis.historyFrom,
    historyTo: basis.historyTo,
    completeMonths: basis.completeMonths,
    poCount: basis.poCount,
    asOf: basis.asOf,
  };
}

function normalizeBucket(bucket: poreport.ProjectedBucket): PurchaseOrderReportProjectedBucket {
  return {
    periodStart: bucket.periodStart,
    amountLowMinor: bucket.amountLowMinor,
    amountBaseMinor: bucket.amountBaseMinor,
    amountHighMinor: bucket.amountHighMinor,
    capitalMinor: bucket.capitalMinor,
    marginMinor: bucket.marginMinor,
    marginBps: nullableRatio(bucket.marginBps),
    sampleSize: bucket.sampleSize,
    source: bucket.source,
  };
}

function normalizeProjectionSeries(series: poreport.ProjectionSeries): PurchaseOrderReportProjectionSeries {
  const normalized: PurchaseOrderReportProjectionSeries = {
    status: series.status,
    method: series.method,
    basis: normalizeBasis(series.basis),
    buckets: series.buckets.map(normalizeBucket),
  };
  // Available series omit reason entirely (Go `omitempty`); keep the field
  // absent rather than inventing an empty string or null.
  if (typeof series.reason === "string") {
    normalized.reason = series.reason;
  }
  return normalized;
}

function normalizeCurrentBasis(
  basis: poreport.CurrentProjectionBasis | null | undefined,
  asOf: string
): PurchaseOrderReportCurrentProjectionBasis {
  if (basis === null || basis === undefined) {
    return { from: "", to: "", units: 0, poCount: 0, asOf };
  }
  return {
    from: typeof basis.from === "string" ? basis.from : "",
    to: typeof basis.to === "string" ? basis.to : "",
    units:
      typeof basis.units === "number" && Number.isFinite(basis.units)
        ? basis.units
        : 0,
    poCount:
      typeof basis.poCount === "number" && Number.isFinite(basis.poCount)
        ? basis.poCount
        : 0,
    asOf: typeof basis.asOf === "string" ? basis.asOf : asOf,
  };
}

function normalizeCurrentRemainder(
  input: poreport.CurrentProjectionRemainder | null | undefined
): PurchaseOrderReportCurrentProjectionRemainder | null {
  if (input === null || input === undefined || typeof input !== "object") {
    return null;
  }
  const numbers = [input.low, input.base, input.high, input.capital, input.margin];
  if (!numbers.every((value) => typeof value === "number" && Number.isFinite(value))) {
    // A malformed remainder is not a zero-padded forecast. Dropping it keeps
    // the chart from drawing an invented stack.
    return null;
  }
  return {
    low: input.low,
    base: input.base,
    high: input.high,
    capital: input.capital,
    margin: input.margin,
    marginBps: nullableRatio(input.marginBps),
    sampleSize:
      typeof input.sampleSize === "number" && Number.isFinite(input.sampleSize)
        ? input.sampleSize
        : 0,
    source: typeof input.source === "string" ? input.source : "",
  };
}

function normalizeCurrentMonth(
  input: poreport.CurrentMonthProjection | null | undefined
): PurchaseOrderReportCurrentMonthProjection | null {
  if (input === null || input === undefined || typeof input !== "object") {
    return null;
  }
  const remainder = normalizeCurrentRemainder(input.remainder);
  if (remainder === null || input.actual === null || input.actual === undefined) {
    return null;
  }
  return {
    actual: normalizeMetrics(input.actual),
    remainder,
  };
}

/**
 * The server-owned current-month readout. A malformed or incomplete summary is
 * absent rather than patched with invented figures: the month browser can then
 * show its unavailable state instead of zeros that never came from the server.
 */
function normalizeMonthSummary(
  input: poreport.MonthSummary | null | undefined
): PurchaseOrderReportMonthSummary | null {
  if (input === null || input === undefined || typeof input !== "object") {
    return null;
  }
  if (
    typeof input.month !== "string" ||
    typeof input.from !== "string" ||
    typeof input.to !== "string" ||
    typeof input.asOf !== "string" ||
    typeof input.mode !== "string" ||
    input.actual === null ||
    input.actual === undefined ||
    input.projection === null ||
    input.projection === undefined
  ) {
    return null;
  }
  const projection: PurchaseOrderReportMonthSummaryProjection = {
    status:
      typeof input.projection.status === "string"
        ? input.projection.status
        : "unavailable",
    method:
      typeof input.projection.method === "string"
        ? input.projection.method
        : "",
  };
  if (typeof input.projection.reason === "string") {
    projection.reason = input.projection.reason;
  }
  const remainder = normalizeCurrentRemainder(input.projection.remainder);
  if (remainder !== null) {
    projection.remainder = remainder;
  }
  const summary: PurchaseOrderReportMonthSummary = {
    month: input.month,
    from: input.from,
    to: input.to,
    asOf: input.asOf,
    mode: input.mode,
    actual: normalizeMetrics(input.actual),
    projection,
  };
  if (input.recordedFuture !== null && input.recordedFuture !== undefined) {
    summary.recordedFuture = normalizeMetrics(input.recordedFuture);
  }
  return summary;
}

/**
 * The current-period forecast, normalized even when the server omitted it (an
 * older API, or a hand-built fixture). A missing object is mode none and
 * unavailable with the stable not-current-period reason; it is never treated as
 * an available forecast with zero buckets.
 */
function normalizeCurrentProjection(
  input: poreport.CurrentProjection | null | undefined,
  asOf: string
): PurchaseOrderReportCurrentProjection {
  if (input === null || input === undefined || typeof input !== "object") {
    return {
      mode: "none",
      status: "unavailable",
      reason: "not_current_period",
      method: "",
      basis: normalizeCurrentBasis(null, asOf),
      buckets: [],
    };
  }
  const normalized: PurchaseOrderReportCurrentProjection = {
    mode: typeof input.mode === "string" ? input.mode : "none",
    status: typeof input.status === "string" ? input.status : "unavailable",
    method: typeof input.method === "string" ? input.method : "",
    basis: normalizeCurrentBasis(input.basis, asOf),
    buckets: Array.isArray(input.buckets)
      ? input.buckets.map(normalizeBucket)
      : [],
  };
  if (typeof input.reason === "string") {
    normalized.reason = input.reason;
  }
  const currentMonth = normalizeCurrentMonth(input.currentMonth);
  if (currentMonth !== null) {
    normalized.currentMonth = currentMonth;
  }
  return normalized;
}

/**
 * Maps the generated Wails report to the desktop contract. Every field is
 * copied, nullable percentages stay null, and absent optional fields stay
 * absent, so the result is safe to serialize or hand to #630 and #631.
 */
export function normalizePurchaseOrderReport(input: poreport.Report): PurchaseOrderReportData {
  const report: PurchaseOrderReportData = {
    generatedAt: input.generatedAt,
    asOf: input.asOf,
    filter: { ...input.filter },
    company: { ...input.company },
    population: {
      availableCurrencies: [...input.population.availableCurrencies],
      hasUnassigned: input.population.hasUnassigned,
      sourceMissingPoCount: input.population.sourceMissingPoCount,
    },
    totals: normalizeMetrics(input.totals),
    pipeline: {
      unsoldAmountMinor: input.pipeline?.unsoldAmountMinor ?? 0,
      unsoldMarginMinor: input.pipeline?.unsoldMarginMinor ?? 0,
    },
    variance: {
      quantityExtraIncomeAmountMinor:
        input.variance?.quantityExtraIncomeAmountMinor ?? 0,
      quantityExtraIncomeMarginMinor:
        input.variance?.quantityExtraIncomeMarginMinor ?? 0,
      unsoldAmountMinor: input.variance?.unsoldAmountMinor ?? 0,
      unsoldMarginMinor: input.variance?.unsoldMarginMinor ?? 0,
      closedLossAmountMinor: input.variance?.closedLossAmountMinor ?? 0,
      closedLossMarginMinor: input.variance?.closedLossMarginMinor ?? 0,
      priceExtraIncomeMinor: input.variance?.priceExtraIncomeMinor ?? 0,
      priceLossMinor: input.variance?.priceLossMinor ?? 0,
      netRealizedVarianceAmountMinor:
        input.variance?.netRealizedVarianceAmountMinor ?? 0,
      netRealizedVarianceMarginMinor:
        input.variance?.netRealizedVarianceMarginMinor ?? 0,
    },
    periods: input.periods.map(normalizePeriod),
    categories: input.categories.map(normalizeCategory),
    categorySeries: input.categorySeries.map(normalizeCategorySeries),
    projection: {
      monthly: normalizeProjectionSeries(input.projection.monthly),
      yearly: normalizeProjectionSeries(input.projection.yearly),
    },
    currentProjection: normalizeCurrentProjection(
      input.currentProjection as
        | poreport.CurrentProjection
        | null
        | undefined,
      input.asOf
    ),
    notes: input.notes.map((note) => ({ code: note.code, text: note.text })),
  };
  const monthSummary = normalizeMonthSummary(
    input.monthSummary as poreport.MonthSummary | null | undefined
  );
  if (monthSummary !== null) {
    report.monthSummary = monthSummary;
  }
  return report;
}
