// Issue #630: the pure decisions behind the global Reports workspace.
//
// A report fetches exactly one filter, and the screen may not invent a figure
// to fill a gap. These helpers own the date translation (inclusive UI range to
// the API's exclusive `to`), the preset calendar windows, the filter key that
// decides when one fetch is enough, and the exact basis-point rendering the
// KPI strip and both tables share. They import the normalized desktop report
// shape from `poReport.ts`; the generated Wails model is never read here.

import type { PurchaseOrderReportData, PurchaseOrderReportQuery } from "./poReport";

export type ReportPreset =
  | "this-year"
  | "last-year"
  | "last-12-months"
  | "last-5-years"
  | "custom";

export type ReportBucket = "year" | "quarter" | "month";

/** The filter as the controls hold it: an inclusive calendar range. */
export interface ReportFilterState {
  preset: ReportPreset;
  /** Inclusive first day, YYYY-MM-DD. */
  from: string;
  /** Inclusive last day, YYYY-MM-DD; the API receives the next day. */
  to: string;
  bucket: ReportBucket;
  /** "all", one business code, or "UNASSIGNED". */
  category: string;
  currency: string;
}

export interface ReportOption<T extends string = string> {
  value: T;
  label: string;
}

/**
 * The KPI strip's responsive geometry, named so the layout and its width guard
 * cannot drift apart. `columns` is the number of KPI cells on one row at the
 * named viewport and `valueFontPx` is the font size the money value uses
 * there. The breakpoints are the AppShell/Card paddings plus Tailwind's
 * sm/lg/xl stops; a viewport below the first entry is one column at 19px.
 */
export interface ReportKpiLayout {
  viewport: number;
  columns: number;
  valueFontPx: number;
}

export const REPORT_KPI_LAYOUT: readonly ReportKpiLayout[] = [
  { viewport: 360, columns: 1, valueFontPx: 19 },
  { viewport: 390, columns: 1, valueFontPx: 19 },
  { viewport: 640, columns: 2, valueFontPx: 17 },
  { viewport: 1024, columns: 3, valueFontPx: 17 },
  { viewport: 1280, columns: 5, valueFontPx: 15 },
];

/**
 * The class strings the KPI strip renders with. They are exported so a width
 * test can assert the named geometry above describes the real layout rather
 * than a parallel guess. Tailwind scans this module and the component both.
 */
export const REPORT_KPI_GRID_CLASS =
  "grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5";
export const REPORT_KPI_VALUE_CLASS =
  "tnum mt-1 whitespace-nowrap text-title font-semibold sm:text-[17px] xl:text-lead";

/**
 * The usable width of one KPI cell at a viewport, after the page and card
 * padding and the 16px grid gap. It mirrors AppShell's `px-5 lg:px-8`, the
 * Card's `p-5`, and the strip's `gap-4`; the width guard uses it to prove the
 * widest exact figure still fits at every named breakpoint.
 */
export function reportKpiCellWidth(viewport: number, columns: number): number {
  const pagePadding = viewport >= 1024 ? 32 : 20;
  const cardPadding = 20;
  const gap = 16;
  return (
    (viewport - pagePadding * 2 - cardPadding * 2 - gap * (columns - 1)) /
    columns
  );
}

export const REPORT_PRESET_OPTIONS: readonly ReportOption<ReportPreset>[] = [
  { value: "this-year", label: "This year" },
  { value: "last-year", label: "Last year" },
  { value: "last-12-months", label: "Last 12 months" },
  { value: "last-5-years", label: "Last 5 years" },
  { value: "custom", label: "Custom" },
];

export const REPORT_BUCKET_OPTIONS: readonly ReportOption<ReportBucket>[] = [
  { value: "year", label: "Year" },
  { value: "quarter", label: "Quarter" },
  { value: "month", label: "Month" },
];

/**
 * The filter vocabulary in the client's workbook order. Unassigned is absent
 * from the base list because the endpoint reports whether the selected
 * population actually contains one.
 */
export const REPORT_CATEGORY_OPTIONS: readonly ReportOption[] = [
  { value: "all", label: "All categories" },
  { value: "FM", label: "Facility Management" },
  { value: "FAB", label: "Fabrication" },
  { value: "SUPPLY_GEN", label: "Goods Supply & General" },
  { value: "OTHERS", label: "Others" },
];

export const REPORT_CATEGORY_UNASSIGNED = "UNASSIGNED";

const REPORT_CATEGORY_CODES = new Set([
  "all",
  "FM",
  "FAB",
  "SUPPLY_GEN",
  "OTHERS",
  REPORT_CATEGORY_UNASSIGNED,
]);

const REPORT_BUCKET_CODES = new Set(["year", "quarter", "month"]);

const MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
] as const;

const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;

function parseIsoDate(value: string): Date | null {
  const match = DATE_PATTERN.exec(value.trim());
  if (match === null) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > 31) return null;
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return null;
  }
  return date;
}

function isoDate(date: Date): string {
  const year = String(date.getUTCFullYear()).padStart(4, "0");
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/** Adds calendar days to a YYYY-MM-DD date. An invalid input passes through. */
export function addDaysIso(value: string, days: number): string {
  const date = parseIsoDate(value);
  if (date === null) return value;
  date.setUTCDate(date.getUTCDate() + days);
  return isoDate(date);
}

/** The inclusive calendar range a preset means on `today` (WIB date). */
export function reportPresetRange(
  preset: ReportPreset,
  today: string
): { from: string; to: string } {
  const date = parseIsoDate(today);
  if (date === null) return { from: today, to: today };
  const year = date.getUTCFullYear();
  switch (preset) {
    case "last-year":
      return { from: `${year - 1}-01-01`, to: `${year - 1}-12-31` };
    case "last-12-months": {
      // The trailing twelve calendar months, ending with today's partial
      // month. The server marks the open bucket, so the screen does not need
      // to pretend the month is complete.
      const start = new Date(Date.UTC(year, date.getUTCMonth() - 11, 1));
      return { from: isoDate(start), to: today };
    }
    case "last-5-years":
      return { from: `${year - 4}-01-01`, to: today };
    case "this-year":
      return { from: `${year}-01-01`, to: today };
    default:
      return { from: today, to: today };
  }
}

/** The initial filter for a newly opened Reports workspace. */
export function initialReportFilters(
  today: string,
  currency: string
): ReportFilterState {
  const range = reportPresetRange("this-year", today);
  return {
    preset: "this-year",
    ...range,
    bucket: "month",
    category: "all",
    currency: currency.trim().toUpperCase(),
  };
}

/**
 * The reason the controls cannot be fetched, or "" when they can. Invalid
 * input never reaches the endpoint: a partial date field must not turn into a
 * silent default range.
 */
export function reportFilterValidation(filters: ReportFilterState): string {
  if (parseIsoDate(filters.from) === null) {
    return "Choose a valid start date.";
  }
  if (parseIsoDate(filters.to) === null) {
    return "Choose a valid end date.";
  }
  if (filters.from > filters.to) {
    return "The start date must be on or before the end date.";
  }
  if (!REPORT_BUCKET_CODES.has(filters.bucket)) {
    return "Choose a valid bucket.";
  }
  if (!REPORT_CATEGORY_CODES.has(filters.category)) {
    return "Choose a valid category.";
  }
  if (filters.currency.trim() === "") {
    return "Choose a currency.";
  }
  return "";
}

/**
 * The exact request. `from` is the UI's first included day; `to` is the first
 * EXCLUDED day, so the last included day survives a month/year boundary.
 */
export function toReportQuery(filters: ReportFilterState): PurchaseOrderReportQuery {
  return {
    from: filters.from,
    to: addDaysIso(filters.to, 1),
    bucket: filters.bucket,
    category: filters.category === "" ? "all" : filters.category,
    currency: filters.currency.trim().toUpperCase(),
  };
}

/** A stable identity for one request; two equal keys are one fetch. */
export function reportFilterKey(query: PurchaseOrderReportQuery): string {
  return [
    query.from,
    query.to,
    query.bucket,
    query.category === "" ? "all" : query.category,
    query.currency.trim().toUpperCase(),
  ].join("|");
}

/** The readable name of a filter's category value. */
export function reportCategoryLabel(value: string): string {
  if (value === REPORT_CATEGORY_UNASSIGNED) return "Unassigned";
  return (
    REPORT_CATEGORY_OPTIONS.find((option) => option.value === value)?.label ??
    value
  );
}

/** The category selector's options, keeping a selected Unassigned visible. */
export function categoryFilterOptions(
  hasUnassigned: boolean,
  selected: string
): ReportOption[] {
  const options = [...REPORT_CATEGORY_OPTIONS];
  if (hasUnassigned || selected === REPORT_CATEGORY_UNASSIGNED) {
    options.push({ value: REPORT_CATEGORY_UNASSIGNED, label: "Unassigned" });
  }
  return options;
}

/**
 * The currency partition options. The current code is always present, even
 * when the previous snapshot's population did not list it, so changing
 * currency can never leave the selector on a value it cannot name.
 */
export function reportCurrencyOptions(
  available: readonly string[],
  selected: string
): string[] {
  const options = [...new Set(available.map((code) => code.trim().toUpperCase()))]
    .filter((code) => code !== "");
  const current = selected.trim().toUpperCase();
  if (current !== "" && !options.includes(current)) {
    options.unshift(current);
  }
  return options;
}

/** One calendar date as the screen reads it: "19 Nov 2026". */
export function formatReportDate(value: string): string {
  const date = parseIsoDate(value);
  if (date === null) return value;
  return `${date.getUTCDate()} ${MONTHS[date.getUTCMonth()]} ${date.getUTCFullYear()}`;
}

/** The inclusive range label derived from the API's exclusive filter. */
export function reportRangeLabel(query: PurchaseOrderReportQuery): string {
  return `${formatReportDate(query.from)} – ${formatReportDate(
    addDaysIso(query.to, -1)
  )}`;
}

/** A period's start rendered in the bucket's own vocabulary. */
export function reportPeriodLabel(periodStart: string, bucket: string): string {
  const date = parseIsoDate(periodStart);
  if (date === null) return periodStart;
  if (bucket === "year") return String(date.getUTCFullYear());
  if (bucket === "quarter") {
    return `Q${Math.floor(date.getUTCMonth() / 3) + 1} ${date.getUTCFullYear()}`;
  }
  return `${MONTHS[date.getUTCMonth()]} ${date.getUTCFullYear()}`;
}

/**
 * A count with en-US grouping, matching the PDF's formatCount exactly.
 * `toLocaleString` is deliberately not used: ICU data varies by runtime, and
 * the screen and the document must print the same bytes for 1,234.
 */
export function formatReportCount(value: number): string {
  if (!Number.isFinite(value)) return "-";
  const magnitude = Math.abs(Math.trunc(value));
  const digits = String(magnitude).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  return value < 0 ? `-${digits}` : digits;
}

/**
 * Basis points as an exact percentage. Null is unknown and prints "-", while
 * zero prints "0.00%": a measured zero is real evidence. Integer math keeps
 * 1250 from ever rendering as 12.5% spelled by a float.
 */
export function formatReportBps(value: number | null | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) return "-";
  const negative = value < 0;
  const magnitude = Math.abs(Math.trunc(value));
  const whole = Math.trunc(magnitude / 100);
  const fraction = String(magnitude % 100).padStart(2, "0");
  return `${negative ? "-" : ""}${whole}.${fraction}%`;
}

/** The sentence a projection's stable reason code stands for. */
export function projectionReasonLabel(reason: string | undefined): string {
  switch (reason) {
    case "no_data_in_range":
      return "No purchase orders in this range to forecast from.";
    case "insufficient_history":
      return "Not enough complete-month history to forecast from.";
    case "incomplete_cost_basis":
      return "Some historical lines have no resolved cost, so margin cannot be projected.";
    case "value_out_of_range":
      return "The historical figures are outside the exact range.";
    case "invalid_query":
      return "The filter was not valid for a projection.";
    default:
      return reason === undefined || reason === ""
        ? "Not enough complete-month history to forecast from."
        : `Projection unavailable (${reason}).`;
  }
}

/** The method label for a projected bucket's observations. */
export function projectionSourceLabel(source: string): string {
  switch (source) {
    case "same_calendar_month":
      return "same calendar month";
    case "trailing_three_complete_months":
      return "trailing 3 complete months";
    case "trailing_twelve_complete_months":
      return "trailing 12 complete months";
    default:
      return source;
  }
}

/**
 * Whether the report on screen belongs to the current filter. Comparing the
 * exclusive `to` is intentional: the server echoes the same half-open range.
 */
export function reportMatchesFilter(
  report: PurchaseOrderReportData,
  query: PurchaseOrderReportQuery
): boolean {
  return reportFilterKey(report.filter) === reportFilterKey(query);
}
