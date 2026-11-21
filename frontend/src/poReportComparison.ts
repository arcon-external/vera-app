// Issue #641: the independent monthly browser's pure decisions.
//
// The overview report owns Performance. This module owns the second source
// identity (one exact calendar year per selected month), the selection rules
// for current/past/future months, and the normalized readout the table and the
// native export share. It performs no financial arithmetic: every value is
// selected from a server snapshot, and percentages are the server's own basis
// points.

import type {
  PurchaseOrderReportCurrentProjectionRemainder,
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
  PurchaseOrderReportQuery,
} from "./poReport";
import { reportFilterKey } from "./poReportPage";

export type MonthSourceStatus = "loading" | "error" | "ready";
export type MonthSourceLabel = "Performance snapshot" | "Monthly snapshot" | "";

const MONTH_NAMES = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
] as const;

const MONTH_PATTERN = /^(\d{4})-(\d{2})$/;

function parseMonth(month: string): { year: number; monthIndex: number } | null {
  const match = MONTH_PATTERN.exec(month);
  if (match === null) return null;
  const year = Number(match[1]);
  const monthIndex = Number(match[2]) - 1;
  if (year < 1 || monthIndex < 0 || monthIndex > 11) return null;
  return { year, monthIndex };
}

function isoDate(year: number, monthIndex: number, day: number): string {
  return `${String(year).padStart(4, "0")}-${String(monthIndex + 1).padStart(
    2,
    "0"
  )}-${String(day).padStart(2, "0")}`;
}

/** The selected month of a report's WIB as-of date, or "" for a malformed date. */
export function monthKeyFromDate(asOf: string): string {
  const trimmed = asOf.trim();
  if (/^\d{4}-\d{2}-\d{2}$/.test(trimmed)) return trimmed.slice(0, 7);
  return "";
}

/** "November 2026", the heading the table prints. */
export function monthLabel(month: string): string {
  const parsed = parseMonth(month);
  if (parsed === null) return month;
  return `${MONTH_NAMES[parsed.monthIndex]} ${parsed.year}`;
}

/** The first day of a canonical month, or "" when malformed. */
export function monthStart(month: string): string {
  const parsed = parseMonth(month);
  if (parsed === null) return "";
  return isoDate(parsed.year, parsed.monthIndex, 1);
}

/** The first day of the following month, or "" when malformed. */
export function nextMonthStart(month: string): string {
  const parsed = parseMonth(month);
  if (parsed === null) return "";
  const year = parsed.monthIndex === 11 ? parsed.year + 1 : parsed.year;
  const monthIndex = (parsed.monthIndex + 1) % 12;
  return isoDate(year, monthIndex, 1);
}

/** Moves a canonical month by whole calendar months. */
export function shiftMonth(month: string, delta: number): string {
  const parsed = parseMonth(month);
  if (parsed === null) return month;
  const absolute = parsed.year * 12 + parsed.monthIndex + delta;
  const year = Math.floor(absolute / 12);
  const monthIndex = ((absolute % 12) + 12) % 12;
  return `${String(year).padStart(4, "0")}-${String(monthIndex + 1).padStart(
    2,
    "0"
  )}`;
}

function normalizedCategory(category: string): string {
  return category.trim() === "" ? "all" : category.trim();
}

/**
 * The exact annual source query for a month's calendar year. It is the whole
 * year, not the "This year to today" preset: only the complete-year range earns
 * the current-year forecast and continuous zero months.
 */
export function annualMonthQuery(
  year: number,
  category: string,
  currency: string
): PurchaseOrderReportQuery {
  return {
    from: `${String(year).padStart(4, "0")}-01-01`,
    to: `${String(year + 1).padStart(4, "0")}-01-01`,
    bucket: "month",
    category: normalizedCategory(category),
    currency: currency.trim().toUpperCase(),
  };
}

/** The annual source key: exact query plus the overview's as-of generation. */
export function annualSourceKey(
  query: PurchaseOrderReportQuery,
  asOf: string
): string {
  return `${reportFilterKey(query)}#${asOf}`;
}

/** True when a source is exactly a complete calendar-year monthly query. */
export function annualSourceCoversMonth(
  source: PurchaseOrderReportData,
  month: string
): boolean {
  const year = month.slice(0, 4);
  return (
    source.filter.bucket === "month" &&
    source.filter.from === `${year}-01-01` &&
    source.filter.to === `${Number(year) + 1}-01-01`
  );
}

/** True when the overview itself is the exact annual source for the month. */
export function overviewIsAnnualForMonth(
  overview: PurchaseOrderReportData,
  month: string
): boolean {
  return annualSourceCoversMonth(overview, month);
}

/**
 * True when the overview is a full current-month/day report whose server
 * monthSummary can own the selected current month. That is the daily-parity
 * path: the table reads the same snapshot and readout the day chart used.
 */
export function overviewCanSupplyCurrentMonth(
  overview: PurchaseOrderReportData,
  month: string
): boolean {
  return (
    month === monthKeyFromDate(overview.asOf) &&
    overview.filter.bucket === "day" &&
    overview.filter.from === monthStart(month) &&
    overview.filter.to === nextMonthStart(month) &&
    overview.monthSummary !== undefined &&
    overview.monthSummary.month === month
  );
}

/** The annual source identity the selected month needs, when it needs one. */
export interface AnnualMonthSourceRequest {
  year: number;
  query: PurchaseOrderReportQuery;
  key: string;
}

export function annualMonthRequest(
  month: string,
  category: string,
  currency: string,
  asOf: string
): AnnualMonthSourceRequest | null {
  const parsed = parseMonth(month);
  if (parsed === null) return null;
  const query = annualMonthQuery(parsed.year, category, currency);
  return { year: parsed.year, query, key: annualSourceKey(query, asOf) };
}

export interface MonthSourceResolution {
  status: MonthSourceStatus;
  source: PurchaseOrderReportData | null;
  error: string;
  sourceLabel: MonthSourceLabel;
  /** The current month is read from the overview's server monthSummary. */
  usesOverviewMonthSummary: boolean;
}

/**
 * Chooses the one source that owns the selected month. The current month is
 * taken from a matching full-current-month/day overview first (daily parity),
 * then from an exact annual source; an exact annual overview serves as its own
 * annual source. A loading or failed annual source leaves the section without
 * old money under its new heading.
 */
export function resolveMonthSource(input: {
  overview: PurchaseOrderReportData;
  month: string;
  annualSource: PurchaseOrderReportData | null;
  annualStatus: MonthSourceStatus;
  annualError: string;
}): MonthSourceResolution {
  const { overview, month, annualSource, annualStatus, annualError } = input;
  if (parseMonth(month) === null) {
    return {
      status: "error",
      source: null,
      error: `"${month}" is not a canonical YYYY-MM month.`,
      sourceLabel: "",
      usesOverviewMonthSummary: false,
    };
  }
  if (overviewCanSupplyCurrentMonth(overview, month)) {
    return {
      status: "ready",
      source: overview,
      error: "",
      sourceLabel: "Performance snapshot",
      usesOverviewMonthSummary: true,
    };
  }
  if (overviewIsAnnualForMonth(overview, month)) {
    return {
      status: "ready",
      source: overview,
      error: "",
      sourceLabel: "Performance snapshot",
      usesOverviewMonthSummary: false,
    };
  }
  if (annualStatus === "error") {
    return {
      status: "error",
      source: null,
      error:
        annualError === ""
          ? "The monthly snapshot could not be loaded."
          : annualError,
      sourceLabel: "Monthly snapshot",
      usesOverviewMonthSummary: false,
    };
  }
  if (annualStatus !== "ready" || annualSource === null) {
    return {
      status: "loading",
      source: null,
      error: "",
      sourceLabel: "Monthly snapshot",
      usesOverviewMonthSummary: false,
    };
  }
  if (!annualSourceCoversMonth(annualSource, month)) {
    return {
      status: "error",
      source: null,
      error: `The loaded monthly snapshot does not cover ${monthLabel(month)}.`,
      sourceLabel: "Monthly snapshot",
      usesOverviewMonthSummary: false,
    };
  }
  return {
    status: "ready",
    source: annualSource,
    error: "",
    sourceLabel: "Monthly snapshot",
    usesOverviewMonthSummary: false,
  };
}

/** One estimated quantity: the current month's remainder or a future month's forecast. */
export interface PurchaseOrderMonthEstimate {
  kind: "remainder" | "forecast";
  low: number;
  base: number;
  high: number;
  capital: number;
  margin: number;
  marginBps: number | null;
  sampleSize: number;
  source: string;
  method: string;
}

export type MonthPrimaryLabel = "Actual to date" | "Recorded" | "";

export interface PurchaseOrderMonthReadout {
  /** Canonical YYYY-MM. */
  month: string;
  status: MonthSourceStatus;
  error: string;
  /** The source's own WIB business date; "" while loading or failed. */
  asOf: string;
  generatedAt: string;
  category: string;
  currency: string;
  sourceLabel: MonthSourceLabel;
  /** True when the current month came from the overview's monthSummary. */
  usesOverviewMonthSummary: boolean;
  /** The source supplied the server-owned monthSummary for this month. */
  hasMonthSummary: boolean;
  primary: PurchaseOrderReportMetrics | null;
  primaryLabel: MonthPrimaryLabel;
  /** Recorded future-dated evidence for the current month; omitted when none. */
  recordedFuture: PurchaseOrderReportMetrics | null;
  /** A forecast/remainder; null when unavailable or not applicable. */
  estimate: PurchaseOrderMonthEstimate | null;
  /** A stable reason code, or a local code such as outside_current_year. */
  unavailableReason: string;
  /** A non-forecast explanation (for example a missing monthly row). */
  message: string;
  /** Same as-of, Business category and currency as the overview. */
  scopeMatchesOverview: boolean;
}

function estimateFromRemainder(
  kind: "remainder" | "forecast",
  remainder: PurchaseOrderReportCurrentProjectionRemainder,
  method: string
): PurchaseOrderMonthEstimate {
  return {
    kind,
    low: remainder.low,
    base: remainder.base,
    high: remainder.high,
    capital: remainder.capital,
    margin: remainder.margin,
    marginBps: remainder.marginBps,
    sampleSize: remainder.sampleSize,
    source: remainder.source,
    method,
  };
}

function estimateFromBucket(
  bucket: {
    amountLowMinor: number;
    amountBaseMinor: number;
    amountHighMinor: number;
    capitalMinor: number;
    marginMinor: number;
    marginBps: number | null;
    sampleSize: number;
    source: string;
  },
  method: string
): PurchaseOrderMonthEstimate {
  return {
    kind: "forecast",
    low: bucket.amountLowMinor,
    base: bucket.amountBaseMinor,
    high: bucket.amountHighMinor,
    capital: bucket.capitalMinor,
    margin: bucket.marginMinor,
    marginBps: bucket.marginBps,
    sampleSize: bucket.sampleSize,
    source: bucket.source,
    method,
  };
}

/** True when a source's scope matches the overview for a combined export. */
export function sourceScopeMatches(
  overview: PurchaseOrderReportData,
  source: PurchaseOrderReportData
): boolean {
  return (
    source.asOf === overview.asOf &&
    normalizedCategory(source.filter.category) ===
      normalizedCategory(overview.filter.category) &&
    source.filter.currency.trim().toUpperCase() ===
      overview.filter.currency.trim().toUpperCase()
  );
}

function periodForMonth(
  source: PurchaseOrderReportData,
  month: string
): PurchaseOrderReportData["periods"][number] | null {
  const start = monthStart(month);
  return source.periods.find((period) => period.periodStart === start) ?? null;
}

/**
 * The section readout for one selected month. It selects fields only: actual,
 * recorded future-dated evidence, and an estimate stay three distinct
 * quantities; no percentage is recomputed and no forecast is summed with a
 * recorded value.
 */
export function resolveMonthReadout(input: {
  overview: PurchaseOrderReportData;
  month: string;
  annualSource: PurchaseOrderReportData | null;
  annualStatus: MonthSourceStatus;
  annualError: string;
}): PurchaseOrderMonthReadout {
  const resolution = resolveMonthSource(input);
  const { overview, month } = input;
  const readout: PurchaseOrderMonthReadout = {
    month,
    status: resolution.status,
    error: resolution.error,
    asOf: "",
    generatedAt: "",
    category: overview.filter.category,
    currency: overview.filter.currency,
    sourceLabel: resolution.sourceLabel,
    usesOverviewMonthSummary: resolution.usesOverviewMonthSummary,
    hasMonthSummary: false,
    primary: null,
    primaryLabel: "",
    recordedFuture: null,
    estimate: null,
    unavailableReason: "",
    message: "",
    scopeMatchesOverview: false,
  };
  if (resolution.status !== "ready" || resolution.source === null) {
    return readout;
  }

  const source = resolution.source;
  const currentMonth = monthKeyFromDate(overview.asOf);
  const isCurrent = month === currentMonth;
  const isFuture = month > currentMonth;
  readout.asOf = source.asOf;
  readout.generatedAt = source.generatedAt;
  readout.category = source.filter.category;
  readout.currency = source.filter.currency;
  readout.scopeMatchesOverview = sourceScopeMatches(overview, source);

  if (isCurrent) {
    const summary =
      source.monthSummary !== undefined && source.monthSummary.month === month
        ? source.monthSummary
        : undefined;
    const legacyMonth = source.currentProjection.currentMonth;
    if (summary !== undefined) {
      readout.hasMonthSummary = true;
      readout.primary = summary.actual;
      readout.primaryLabel = "Actual to date";
      readout.recordedFuture = summary.recordedFuture ?? null;
      if (
        summary.projection.status === "available" &&
        summary.projection.remainder !== undefined
      ) {
        readout.estimate = estimateFromRemainder(
          "remainder",
          summary.projection.remainder,
          summary.projection.method
        );
      } else {
        readout.unavailableReason =
          summary.projection.reason ?? "not_current_period";
      }
    } else if (legacyMonth !== null && legacyMonth !== undefined) {
      // Older API compatibility: the #638 shape has the month-to-date actual
      // and remainder but no recorded-future line. The values remain
      // server-owned; this is not a client forecast.
      readout.primary = legacyMonth.actual;
      readout.primaryLabel = "Actual to date";
      if (
        source.currentProjection.status === "available" &&
        legacyMonth.remainder !== undefined
      ) {
        readout.estimate = estimateFromRemainder(
          "remainder",
          legacyMonth.remainder,
          source.currentProjection.method
        );
      } else {
        readout.unavailableReason =
          source.currentProjection.reason ?? "not_current_period";
      }
    } else {
      readout.message =
        "This snapshot does not carry a month-to-date readout for the current month.";
      if (source.currentProjection.status !== "available") {
        readout.unavailableReason =
          source.currentProjection.reason ?? "not_current_period";
      }
    }
    return readout;
  }

  const period = periodForMonth(source, month);
  if (!isFuture) {
    if (period === null) {
      readout.message = "No monthly row is available for the selected month.";
      return readout;
    }
    readout.primary = period;
    readout.primaryLabel = "Recorded";
    return readout;
  }

  // Future month: recorded evidence (if any) stays solid; the full-month
  // forecast is a separate alternative. A zero future period is not displayed
  // as a recorded zero.
  if (period !== null && period.poCount > 0) {
    readout.primary = period;
    readout.primaryLabel = "Recorded";
  }
  if (month.slice(0, 4) !== currentMonth.slice(0, 4)) {
    readout.unavailableReason = "outside_current_year";
    return readout;
  }
  const projection = source.currentProjection;
  if (projection.mode !== "month" || projection.status !== "available") {
    readout.unavailableReason =
      projection.mode !== "month"
        ? "not_current_period"
        : projection.reason ?? "not_current_period";
    return readout;
  }
  const bucket = projection.buckets.find(
    (entry) => entry.periodStart === monthStart(month)
  );
  if (bucket === undefined) {
    readout.unavailableReason = "no_bucket";
    return readout;
  }
  readout.estimate = estimateFromBucket(bucket, projection.method);
  return readout;
}

/**
 * Whether the selected month is complete enough for an export. Current and
 * past months need actual/recorded evidence; a future month may legitimately
 * carry only an unavailable-forecast reason or no recorded row. A scope
 * mismatch always blocks the combined current view.
 */
export function monthReadoutExportable(
  readout: PurchaseOrderMonthReadout,
  overview: PurchaseOrderReportData
): boolean {
  if (
    readout.status !== "ready" ||
    !readout.scopeMatchesOverview ||
    readout.month === ""
  ) {
    return false;
  }
  if (readout.month === monthKeyFromDate(overview.asOf)) {
    // The current month must come from the server-owned monthSummary; the
    // older currentMonth fallback can display but cannot be exported because
    // the native view requires the frozen summary.
    return readout.hasMonthSummary && readout.primary !== null;
  }
  if (readout.month < monthKeyFromDate(overview.asOf)) {
    return readout.primary !== null;
  }
  return (
    readout.primary !== null ||
    readout.estimate !== null ||
    readout.unavailableReason !== ""
  );
}

/** The sentence a readout's unavailable reason stands for. */
export function monthUnavailableLabel(reason: string): string {
  switch (reason) {
    case "outside_current_year":
      return "Future months beyond the current year have no current-year forecast.";
    case "no_bucket":
      return "No current-year forecast bucket is available for this month.";
    case "":
      return "No forecast is available for this month.";
    default:
      return projectionReasonLabelOrFallback(reason);
  }
}

function projectionReasonLabelOrFallback(reason: string): string {
  switch (reason) {
    case "not_current_period":
      return "This month is outside the current-year forecast. No retired projection is used as a fallback.";
    case "no_data":
      return "No purchase orders in the current period to forecast from.";
    case "insufficient_days":
      return "Not enough elapsed days and purchase orders in the current month to forecast from.";
    case "insufficient_months":
      return "Not enough complete months in trailing history to forecast from.";
    case "incomplete_cost_basis":
      return "Some historical lines have no resolved cost, so margin cannot be projected.";
    case "value_out_of_range":
      return "The forecast figures are outside the exact range.";
    case "invalid_query":
      return "The filter was not valid for a projection.";
    default:
      return `Forecast unavailable (${reason}).`;
  }
}
