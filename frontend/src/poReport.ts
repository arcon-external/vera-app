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
  periods: PurchaseOrderReportPeriod[];
  categories: PurchaseOrderReportCategory[];
  categorySeries: PurchaseOrderReportCategorySeries[];
  projection: PurchaseOrderReportProjection;
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

/**
 * Maps the generated Wails report to the desktop contract. Every field is
 * copied, nullable percentages stay null, and absent optional fields stay
 * absent, so the result is safe to serialize or hand to #630 and #631.
 */
export function normalizePurchaseOrderReport(input: poreport.Report): PurchaseOrderReportData {
  return {
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
    periods: input.periods.map(normalizePeriod),
    categories: input.categories.map(normalizeCategory),
    categorySeries: input.categorySeries.map(normalizeCategorySeries),
    projection: {
      monthly: normalizeProjectionSeries(input.projection.monthly),
      yearly: normalizeProjectionSeries(input.projection.yearly),
    },
    notes: input.notes.map((note) => ({ code: note.code, text: note.text })),
  };
}
