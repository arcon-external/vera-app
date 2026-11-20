// Issue #631: the pure decisions behind the Reports performance chart.
//
// Nothing here fetches, formats a table, or recomputes a report figure. The
// module maps the normalized server snapshot into chart buckets, rounds one
// zero-inclusive domain per axis with the SAME nice-scale algorithm the Go PDF
// renderer uses (backend/render/report/scale.go, pinned by the shared
// testdata/chart_scales.json fixture), and lays out marks for a measured
// container width.
//
// Projections are drawn from projection.monthly for a monthly view and
// projection.yearly for a yearly view. A quarterly view gets no forecast:
// there is no quarterly projection in the payload, and inventing one here
// would be a number the server never produced.

import type { CurrencyInfo } from "./format";
import { formatReportCount, projectionReasonLabel } from "./poReportPage";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
  PurchaseOrderReportProjectedBucket,
} from "./poReport";

/** One axis: zero-inclusive domain rounded to a 1/2/5 step. */
export interface ChartScale {
  min: number;
  max: number;
  step: number;
  ticks: number[];
}

export const CHART_TARGET_MONEY_TICKS = 5;
export const CHART_TARGET_PERCENT_TICKS = 4;

export const CHART_HEIGHT = 300;
export const CHART_PLOT_TOP = 18;
export const CHART_PLOT_BOTTOM = 48;
export const CHART_GUTTER_LEFT = 62;
export const CHART_GUTTER_RIGHT = 54;
export const CHART_MIN_GROUP_WIDTH = 12;

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

export const CHART_CATEGORY_ORDER = [
  "FM",
  "FAB",
  "SUPPLY_GEN",
  "OTHERS",
  "UNASSIGNED",
] as const;

// The category line colours and dash patterns are the workbook vocabulary
// made visible. Colour is never the only distinction: every line also carries
// its own dash pattern and its legend label/code.
const CATEGORY_STYLE: Record<
  string,
  { color: string; dash: string }
> = {
  FM: { color: "#C2410C", dash: "0" },
  FAB: { color: "#0F766E", dash: "6 3" },
  SUPPLY_GEN: { color: "#7C3AED", dash: "2 3" },
  OTHERS: { color: "#B45309", dash: "9 3 2 3" },
  UNASSIGNED: { color: "#64748B", dash: "1 4" },
};

const CURRENCY_UNITS = [
  { divisor: 1e18, suffix: "E" },
  { divisor: 1e15, suffix: "P" },
  { divisor: 1e12, suffix: "T" },
  { divisor: 1e9, suffix: "B" },
  { divisor: 1e6, suffix: "M" },
  { divisor: 1e3, suffix: "K" },
] as const;

export interface ReportChartBucket {
  key: string;
  periodStart: string;
  label: string;
  projected: boolean;
  partial: boolean;
  inProgress: boolean;
  /** Present for an actual bucket; null for a projected bucket. */
  poCount: number | null;
  amountMinor: number;
  /** Present for an actual bucket; null for a projected bucket. */
  costedMinor: number | null;
  /** Present for an actual bucket; null for a projected bucket. */
  uncostedMinor: number | null;
  capitalMinor: number;
  marginMinor: number;
  marginBps: number | null;
  /** Present for an actual bucket; null for a projected bucket. */
  coverageBps: number | null;
  projectedDetail: PurchaseOrderReportProjectedBucket | null;
}

export interface ReportChartCategory {
  code: string;
  label: string;
  color: string;
  dash: string;
  /** Aligned to the actual periods; null is a gap, never 0%. */
  values: Array<number | null>;
  visible: boolean;
}

export interface ReportChartModel {
  dataKey: string;
  bucket: string;
  buckets: ReportChartBucket[];
  actualCount: number;
  projectedCount: number;
  /** Index of the first projected bucket, or null when no divider is needed. */
  dividerIndex: number | null;
  moneyScale: ChartScale;
  percentScale: ChartScale | null;
  categories: ReportChartCategory[];
  categoryFootnote: string | null;
  /** Quarter/unavailable/empty explanation; null when a projection is drawn. */
  projectionNote: string | null;
  projectionKind: "monthly" | "yearly" | "none";
  hasNegativeMargin: boolean;
  hasUncosted: boolean;
  hasPartial: boolean;
  summary: string;
}

export interface ChartColumn {
  x: number;
  y: number;
  width: number;
  height: number;
  negative: boolean;
  value: number;
}

export interface ReportChartBucketGeometry {
  index: number;
  center: number;
  projected: boolean;
  amount: ChartColumn;
  capital: ChartColumn;
  margin: ChartColumn;
  uncosted: { x: number; y: number; width: number; height: number } | null;
}

export interface ReportChartXLabel {
  index: number;
  label: string;
  x: number;
  anchor: "start" | "middle" | "end";
  partial: boolean;
}

export interface ReportChartLayout {
  width: number;
  height: number;
  plot: { x: number; y: number; width: number; height: number };
  groupWidth: number;
  barWidth: number;
  barGap: number;
  zeroY: number;
  centers: number[];
  moneyTicks: Array<{ value: number; y: number }>;
  percentTicks: Array<{ value: number; y: number }>;
  xLabels: ReportChartXLabel[];
  dividerX: number | null;
  bandPath: string | null;
  columns: ReportChartBucketGeometry[];
  marginPath: string | null;
  marginPoints: Array<{ index: number; x: number; y: number }>;
  categoryPaths: Array<{ code: string; path: string }>;
}

/**
 * The Go scale algorithm, line for line: zero-inclusive, 1/2/5 steps,
 * outward-rounded bounds, both bounds ticks, exact zero never -0.
 */
export function niceScale(
  min: number,
  max: number,
  targetTicks: number
): ChartScale {
  if (targetTicks < 2) targetTicks = 2;
  if (
    Number.isNaN(min) ||
    Number.isNaN(max) ||
    !Number.isFinite(min) ||
    !Number.isFinite(max)
  ) {
    return { min: 0, max: 1, step: 0.2, ticks: [0, 0.2, 0.4, 0.6, 0.8, 1] };
  }
  if (min > max) {
    const swap = min;
    min = max;
    max = swap;
  }
  if (min > 0) min = 0;
  if (max < 0) max = 0;
  if (min === 0 && max === 0) max = 1;

  const intervals = targetTicks - 1;
  let step = niceStep((max - min) / intervals);
  if (!Number.isFinite(step) || step <= 0) step = 1;

  let niceMin = Math.floor(min / step + 1e-9) * step;
  let niceMax = Math.ceil(max / step - 1e-9) * step;
  if (niceMin === 0) niceMin = 0;
  if (niceMax === 0) niceMax = 0;

  let count = Math.round((niceMax - niceMin) / step);
  if (count < 1) count = 1;

  const ticks: number[] = [];
  for (let index = 0; index <= count; index += 1) {
    const value = roundTick(niceMin + index * step, step);
    ticks.push(value === 0 ? 0 : value);
  }

  return { min: niceMin, max: niceMax, step, ticks };
}

function niceStep(raw: number): number {
  if (!Number.isFinite(raw) || raw <= 0) return 1;
  const exponent = Math.floor(Math.log10(raw));
  const base = Math.pow(10, exponent);
  const fraction = raw / base;
  if (fraction < 1.5) return base;
  if (fraction < 3) return 2 * base;
  if (fraction < 7) return 5 * base;
  return 10 * base;
}

function roundTick(value: number, step: number): number {
  if (!Number.isFinite(step) || step <= 0) return value;
  let decimals = -Math.floor(Math.log10(step));
  if (decimals < 0) decimals = 0;
  if (decimals > 15) decimals = 15;
  const factor = Math.pow(10, decimals);
  return Math.round(value * factor) / factor;
}

/** Maps a value in a nice scale to the plot's y coordinate. */
export function scaleY(
  scale: ChartScale,
  value: number,
  top: number,
  height: number
): number {
  if (!Number.isFinite(value)) return top + height;
  if (scale.max === scale.min) return top + height;
  return top + height - ((value - scale.min) / (scale.max - scale.min)) * height;
}

// Two significant digits keep 1.234K at "1.2K" while still naming a small
// major-unit tick as "0.01" instead of collapsing it to "0". The exact figure
// stays in the tooltip and the table.
function trimDecimal(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return String(Number(value.toPrecision(2)));
}

function compactMagnitude(value: number): string {
  const magnitude = Math.abs(value);
  for (const unit of CURRENCY_UNITS) {
    if (magnitude >= unit.divisor) {
      return trimDecimal(magnitude / unit.divisor) + unit.suffix;
    }
  }
  return trimDecimal(magnitude);
}

/** One compact axis number: K/M/B/T with one decimal, trimmed. */
export function compactChartNumber(value: number): string {
  if (!Number.isFinite(value)) return "—";
  return (value < 0 ? "-" : "") + compactMagnitude(value);
}

function currencyExponent(entry: CurrencyInfo): number {
  return Number.isFinite(entry.exponent) && entry.exponent > 0
    ? Math.trunc(entry.exponent)
    : 0;
}

/**
 * A compact money tick in the currency's own convention. The minor-unit value
 * is converted to major units first, so USD 123456 reads "$1.2K", not "$123.5K".
 * A null catalogue entry renders "—": no symbol is invented.
 */
export function compactChartMoney(
  entry: CurrencyInfo | null,
  value: number
): string {
  if (entry === null || !Number.isFinite(value)) return "—";
  const divisor = Math.pow(10, currencyExponent(entry));
  const bare = compactMagnitude(value / divisor);
  const negative = value < 0;
  const prefix =
    entry.position === "after" ? "" : entry.symbol + (entry.symbolSpaced ? " " : "");
  const suffix =
    entry.position === "after"
      ? (entry.symbolSpaced ? " " : "") + entry.symbol
      : "";
  return prefix + (negative ? "-" : "") + bare + suffix;
}

/** A percentage-axis tick, matching the PDF's compact extreme-value rule. */
export function formatPercentTick(value: number): string {
  if (!Number.isFinite(value)) return "—";
  const negative = value < 0;
  const magnitude = Math.abs(value);
  const body =
    magnitude >= 10_000
      ? compactMagnitude(magnitude)
      : Number.isInteger(magnitude)
        ? String(magnitude)
        : String(Number(magnitude.toFixed(6)));
  return (negative ? "-" : "") + body + "%";
}

/** A period's short chart label: "Nov 26", "Q2 26" or "2026". */
export function chartPeriodLabel(periodStart: string, bucket: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(periodStart.trim());
  if (match === null) return periodStart;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (
    date.getUTCFullYear() !== year ||
    date.getUTCMonth() !== month - 1 ||
    date.getUTCDate() !== day
  ) {
    return periodStart;
  }
  if (bucket === "year") return String(year);
  const shortYear = String(year % 100).padStart(2, "0");
  if (bucket === "quarter") {
    return `Q${Math.floor((month - 1) / 3) + 1} ${shortYear}`;
  }
  return `${MONTHS[month - 1]} ${shortYear}`;
}

/**
 * A stable identity for the chart's data. A parent re-render with the same
 * snapshot keeps it; a filter change, a retry that produced different figures,
 * or a changed projection starts a new one. Legend visibility is deliberately
 * not part of it: toggling a line rescales the percentage axis but is not a
 * data change.
 */
export function reportChartDataKey(report: PurchaseOrderReportData): string {
  const periods = report.periods.map((period) => [
    period.periodStart,
    period.partial ? 1 : 0,
    period.inProgress ? 1 : 0,
    period.poCount,
    period.amountMinor,
    period.costedMinor,
    period.uncostedMinor,
    period.capitalMinor,
    period.marginMinor,
    period.marginBps,
    period.coverageBps,
  ]);
  const series =
    report.filter.category === "all"
      ? report.categorySeries.map((entry) => [
          entry.businessCategory,
          ...entry.marginBps,
        ])
      : [];
  // Only the projection this view draws is part of the motion identity. A
  // yearly series changing under a monthly view (or any series under a
  // quarterly view) is not visible here and must not replay the marks.
  const projectionKinds =
    report.filter.bucket === "month"
      ? (["monthly"] as const)
      : report.filter.bucket === "year"
        ? (["yearly"] as const)
        : ([] as const);
  const projection = projectionKinds.map((kind) => {
    const value =
      kind === "monthly" ? report.projection.monthly : report.projection.yearly;
    return [
      kind,
      value.status,
      value.reason ?? "",
      value.buckets.map((bucket) => [
        bucket.periodStart,
        bucket.amountLowMinor,
        bucket.amountBaseMinor,
        bucket.amountHighMinor,
        bucket.capitalMinor,
        bucket.marginMinor,
        bucket.marginBps,
        bucket.sampleSize,
        bucket.source,
      ]),
    ];
  });
  return JSON.stringify([
    report.filter.from,
    report.filter.to,
    report.filter.bucket,
    report.filter.category,
    report.filter.currency,
    report.totals.poCount,
    report.totals.amountMinor,
    report.totals.costedMinor,
    report.totals.uncostedMinor,
    report.totals.capitalMinor,
    report.totals.marginMinor,
    report.totals.marginBps,
    report.totals.coverageBps,
    periods,
    series,
    projection,
  ]);
}

function bucketWord(count: number, bucket: string): string {
  switch (bucket) {
    case "year":
      return count === 1 ? "year" : "years";
    case "quarter":
      return count === 1 ? "quarter" : "quarters";
    default:
      return count === 1 ? "monthly period" : "monthly periods";
  }
}

function categoryRowLabel(report: PurchaseOrderReportData, code: string): string {
  const row = report.categories.find((entry) => entry.businessCategory === code);
  if (row !== undefined && row.label.trim() !== "") return row.label;
  if (code === "UNASSIGNED") return "Unassigned";
  return code;
}

function actualBucket(
  period: PurchaseOrderReportData["periods"][number],
  bucket: string
): ReportChartBucket {
  return {
    key: `actual:${period.periodStart}`,
    periodStart: period.periodStart,
    label: chartPeriodLabel(period.periodStart, bucket),
    projected: false,
    partial: period.partial,
    inProgress: period.inProgress,
    poCount: period.poCount,
    amountMinor: period.amountMinor,
    costedMinor: period.costedMinor,
    uncostedMinor: period.uncostedMinor,
    capitalMinor: period.capitalMinor,
    marginMinor: period.marginMinor,
    marginBps: period.marginBps,
    coverageBps: period.coverageBps,
    projectedDetail: null,
  };
}

function projectedBucket(
  bucket: PurchaseOrderReportProjectedBucket,
  viewBucket: string
): ReportChartBucket {
  return {
    key: `projected:${bucket.periodStart}`,
    periodStart: bucket.periodStart,
    label: chartPeriodLabel(bucket.periodStart, viewBucket),
    projected: true,
    partial: false,
    inProgress: false,
    poCount: null,
    amountMinor: bucket.amountBaseMinor,
    costedMinor: null,
    uncostedMinor: null,
    capitalMinor: bucket.capitalMinor,
    marginMinor: bucket.marginMinor,
    marginBps: bucket.marginBps,
    coverageBps: null,
    projectedDetail: bucket,
  };
}

function categoryModel(
  report: PurchaseOrderReportData,
  visible: ReadonlySet<string>
): Pick<ReportChartModel, "categories" | "categoryFootnote"> {
  if (report.filter.category !== "all") {
    return {
      categories: [],
      categoryFootnote: "Category margin lines appear only with All categories.",
    };
  }

  const present = new Map(
    report.categorySeries.map((entry) => [entry.businessCategory, entry])
  );
  const rows = new Set(
    report.categories.map((entry) => entry.businessCategory)
  );
  const codes = CHART_CATEGORY_ORDER.filter(
    (code) => present.has(code) || rows.has(code)
  );
  const available = codes.filter((code) => {
    const series = present.get(code);
    return (
      series !== undefined &&
      series.marginBps.some((value) => value !== null)
    );
  });
  const categories: ReportChartCategory[] = available.map((code) => {
    const series = present.get(code)!;
    const style = CATEGORY_STYLE[code] ?? { color: "#64748B", dash: "2 2" };
    return {
      code,
      label: categoryRowLabel(report, code),
      color: style.color,
      dash: style.dash,
      values: series.marginBps.map((value) =>
        value === null ? null : value / 100
      ),
      visible: visible.has(code),
    };
  });

  const omitted = codes.filter((code) => !available.includes(code));
  const omittedLabels = omitted
    .map((code) => categoryRowLabel(report, code))
    .join(", ");
  let footnote: string | null = null;
  if (categories.length === 0) {
    footnote =
      omittedLabels === ""
        ? "Category margin lines are omitted: the report carries no per-period category margin series."
        : `No per-period category margin series for ${omittedLabels}; category margin lines are omitted.`;
  } else if (omitted.length > 0) {
    footnote = `No per-period category margin series for ${omittedLabels}; those category margin lines are omitted.`;
  }

  return { categories, categoryFootnote: footnote };
}

/** Maps the normalized report to chart buckets and both axis domains. */
export function buildReportChart(
  report: PurchaseOrderReportData,
  visibleCategories: readonly string[] = []
): ReportChartModel {
  const bucket = report.filter.bucket;
  const visible = new Set(visibleCategories);
  const actual = report.periods.map((period) => actualBucket(period, bucket));

  let projectionKind: ReportChartModel["projectionKind"] = "none";
  let projectionNote: string | null = null;
  let projected: ReportChartBucket[] = [];

  if (bucket === "quarter") {
    projectionNote =
      "No quarterly projection is produced; the monthly and yearly forecasts remain in the Projection section below.";
  } else if (bucket === "month" || bucket === "year") {
    projectionKind = bucket === "month" ? "monthly" : "yearly";
    const series =
      bucket === "month" ? report.projection.monthly : report.projection.yearly;
    if (series.status !== "available") {
      projectionNote = `No projection is drawn: ${projectionReasonLabel(
        series.reason
      )}`;
    } else if (series.buckets.length === 0) {
      projectionNote = "No projected periods are available in this range.";
    } else {
      projected = series.buckets.map((entry) => projectedBucket(entry, bucket));
    }
  }

  const buckets = [...actual, ...projected];
  const moneyValues = buckets.flatMap((entry) => {
    if (entry.projected && entry.projectedDetail !== null) {
      return [
        entry.projectedDetail.amountLowMinor,
        entry.projectedDetail.amountBaseMinor,
        entry.projectedDetail.amountHighMinor,
        entry.capitalMinor,
        entry.marginMinor,
      ];
    }
    return [entry.amountMinor, entry.capitalMinor, entry.marginMinor];
  });
  const moneyScale = niceScale(
    moneyValues.reduce((min, value) => Math.min(min, value), 0),
    moneyValues.reduce((max, value) => Math.max(max, value), 0),
    CHART_TARGET_MONEY_TICKS
  );

  const { categories, categoryFootnote } = categoryModel(report, visible);
  const percentValues = buckets
    .filter((entry) => !entry.projected && entry.marginBps !== null)
    .map((entry) => entry.marginBps! / 100);
  for (const category of categories) {
    if (!category.visible) continue;
    for (const value of category.values) {
      if (value !== null) percentValues.push(value);
    }
  }
  const percentScale =
    percentValues.length === 0
      ? null
      : niceScale(
          percentValues.reduce((min, value) => Math.min(min, value), 0),
          percentValues.reduce((max, value) => Math.max(max, value), 0),
          CHART_TARGET_PERCENT_TICKS
        );

  const hasNegativeMargin = buckets.some((entry) => entry.marginMinor < 0);
  const hasUncosted = actual.some((entry) => (entry.uncostedMinor ?? 0) > 0);
  const hasPartial = actual.some((entry) => entry.partial || entry.inProgress);
  const summaryParts = [
    `${formatReportCount(report.totals.poCount)} POs across ${
      actual.length
    } actual ${bucketWord(actual.length, bucket)}.`,
  ];
  if (projected.length > 0) {
    summaryParts.push(
      `${projected.length} projected ${bucketWord(
        projected.length,
        bucket
      )} follow the dashed divider and are never included in actual totals.`
    );
  }

  return {
    dataKey: reportChartDataKey(report),
    bucket,
    buckets,
    actualCount: actual.length,
    projectedCount: projected.length,
    dividerIndex:
      projected.length > 0 && actual.length > 0 ? actual.length : null,
    moneyScale,
    percentScale,
    categories,
    categoryFootnote,
    projectionNote,
    projectionKind,
    hasNegativeMargin,
    hasUncosted,
    hasPartial,
    summary: summaryParts.join(" "),
  };
}

function moneyColumn(
  value: number,
  x: number,
  width: number,
  zeroY: number,
  scale: ChartScale,
  plotY: number,
  plotHeight: number
): ChartColumn {
  const y = scaleY(scale, value, plotY, plotHeight);
  return {
    x,
    y: Math.min(y, zeroY),
    width,
    height: Math.abs(y - zeroY),
    negative: value < 0,
    value,
  };
}

function linePath(
  points: Array<{ index: number; x: number; y: number }>
): string {
  let path = "";
  let previous = -2;
  for (const point of points) {
    const pair = `${point.x.toFixed(2)} ${point.y.toFixed(2)}`;
    path += point.index === previous + 1 ? ` L ${pair}` : ` M ${pair}`;
    previous = point.index;
  }
  return path.trim();
}

function percentPoints(
  values: Array<number | null>,
  actualCount: number,
  centers: number[],
  scale: ChartScale,
  plotY: number,
  plotHeight: number
): Array<{ index: number; x: number; y: number }> {
  const points: Array<{ index: number; x: number; y: number }> = [];
  const count = Math.min(actualCount, values.length);
  for (let index = 0; index < count; index += 1) {
    const value = values[index];
    if (value === null || value === undefined) continue;
    points.push({
      index,
      x: centers[index],
      y: scaleY(scale, value, plotY, plotHeight),
    });
  }
  return points;
}

/** Lays the model out for a measured container width. Pure and finite. */
export function layoutReportChart(
  model: ReportChartModel,
  width: number
): ReportChartLayout {
  const safeWidth =
    Number.isFinite(width) && width > 0 ? Math.round(width) : 960;
  const count = model.buckets.length;
  const chartWidth = Math.max(
    safeWidth,
    CHART_GUTTER_LEFT +
      CHART_GUTTER_RIGHT +
      (count > 0 ? count * CHART_MIN_GROUP_WIDTH : 0)
  );
  const plot = {
    x: CHART_GUTTER_LEFT,
    y: CHART_PLOT_TOP,
    width: Math.max(
      1,
      chartWidth - CHART_GUTTER_LEFT - CHART_GUTTER_RIGHT
    ),
    height: CHART_HEIGHT - CHART_PLOT_TOP - CHART_PLOT_BOTTOM,
  };
  const groupWidth = count > 0 ? plot.width / count : 0;
  const barGap = Math.max(1, Math.min(4, groupWidth * 0.035));
  const barWidth =
    count > 0
      ? Math.min(
          26,
          Math.max(3, (groupWidth * 0.72 - 2 * barGap) / 3)
        )
      : 0;
  const zeroY = scaleY(model.moneyScale, 0, plot.y, plot.height);
  const centers = model.buckets.map(
    (_, index) => plot.x + (index + 0.5) * groupWidth
  );

  const moneyTicks = model.moneyScale.ticks.map((value) => ({
    value,
    y: scaleY(model.moneyScale, value, plot.y, plot.height),
  }));
  const percentTicks =
    model.percentScale === null
      ? []
      : model.percentScale.ticks.map((value) => ({
          value,
          y: scaleY(model.percentScale!, value, plot.y, plot.height),
        }));

  const maxLabelLength = model.buckets.reduce(
    (longest, entry) => Math.max(longest, entry.label.length),
    0
  );
  const labelInterval =
    groupWidth > 0
      ? Math.max(1, Math.ceil((maxLabelLength * 6.2 + 10) / groupWidth))
      : 1;
  const labelIndices: number[] = [];
  for (let index = 0; index < count; index += labelInterval) {
    labelIndices.push(index);
  }
  const lastIndex = count - 1;
  if (
    count > 0 &&
    labelIndices[labelIndices.length - 1] !== lastIndex
  ) {
    while (
      labelIndices.length > 1 &&
      lastIndex - labelIndices[labelIndices.length - 1] < labelInterval
    ) {
      labelIndices.pop();
    }
    labelIndices.push(lastIndex);
  }
  const xLabels = labelIndices.map((index) => ({
    index,
    label: model.buckets[index].label,
    x: centers[index],
    anchor:
      count === 1
        ? ("middle" as const)
        : index === 0
          ? ("start" as const)
          : index === lastIndex
            ? ("end" as const)
            : ("middle" as const),
    partial:
      model.buckets[index].partial || model.buckets[index].inProgress,
  }));

  const columns: ReportChartBucketGeometry[] = model.buckets.map(
    (entry, index) => {
      const center = centers[index];
      const totalWidth = 3 * barWidth + 2 * barGap;
      const start = center - totalWidth / 2;
      const amountX = start;
      const capitalX = start + barWidth + barGap;
      const marginX = start + 2 * (barWidth + barGap);
      const amount = moneyColumn(
        entry.amountMinor,
        amountX,
        barWidth,
        zeroY,
        model.moneyScale,
        plot.y,
        plot.height
      );
      const capital = moneyColumn(
        entry.capitalMinor,
        capitalX,
        barWidth,
        zeroY,
        model.moneyScale,
        plot.y,
        plot.height
      );
      const margin = moneyColumn(
        entry.marginMinor,
        marginX,
        barWidth,
        zeroY,
        model.moneyScale,
        plot.y,
        plot.height
      );

      let uncosted: ReportChartBucketGeometry["uncosted"] = null;
      if (
        !entry.projected &&
        entry.costedMinor !== null &&
        (entry.uncostedMinor ?? 0) > 0
      ) {
        const costedY = scaleY(
          model.moneyScale,
          entry.costedMinor,
          plot.y,
          plot.height
        );
        const amountY = scaleY(
          model.moneyScale,
          entry.amountMinor,
          plot.y,
          plot.height
        );
        const top = Math.min(costedY, amountY);
        const height = Math.abs(costedY - amountY);
        if (height > 0.2) {
          uncosted = { x: amountX, y: top, width: barWidth, height };
        }
      }

      return {
        index,
        center,
        projected: entry.projected,
        amount,
        capital,
        margin,
        uncosted,
      };
    }
  );

  let bandPath: string | null = null;
  if (model.projectedCount > 0) {
    const projected = model.buckets.slice(model.actualCount);
    const low = projected.map((entry, index) => ({
      x: centers[model.actualCount + index],
      y: scaleY(
        model.moneyScale,
        entry.projectedDetail?.amountLowMinor ?? 0,
        plot.y,
        plot.height
      ),
    }));
    const high = projected.map((entry, index) => ({
      x: centers[model.actualCount + index],
      y: scaleY(
        model.moneyScale,
        entry.projectedDetail?.amountHighMinor ?? 0,
        plot.y,
        plot.height
      ),
    }));
    if (projected.length === 1) {
      const half = Math.max(3, groupWidth * 0.3);
      bandPath = `M ${(low[0].x - half).toFixed(2)} ${low[0].y.toFixed(
        2
      )} L ${(low[0].x + half).toFixed(2)} ${low[0].y.toFixed(2)} L ${(
        high[0].x + half
      ).toFixed(2)} ${high[0].y.toFixed(2)} L ${(
        high[0].x - half
      ).toFixed(2)} ${high[0].y.toFixed(2)} Z`;
    } else if (projected.length > 1) {
      const points = [
        ...low,
        ...[...high].reverse(),
      ];
      bandPath =
        points
          .map(
            (point, index) =>
              `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(
                2
              )}`
          )
          .join(" ") + " Z";
    }
  }

  const marginPoints =
    model.percentScale === null
      ? []
      : percentPoints(
          model.buckets
            .slice(0, model.actualCount)
            .map((entry) => entry.marginBps),
          model.actualCount,
          centers,
          model.percentScale,
          plot.y,
          plot.height
        );
  const marginPath =
    marginPoints.length === 0 ? null : linePath(marginPoints);

  const categoryPaths =
    model.percentScale === null
      ? []
      : model.categories
          .filter((category) => category.visible)
          .map((category) => {
            const points = percentPoints(
              category.values,
              model.actualCount,
              centers,
              model.percentScale!,
              plot.y,
              plot.height
            );
            return { code: category.code, path: linePath(points) };
          })
          .filter((entry) => entry.path !== "");

  return {
    width: chartWidth,
    height: CHART_HEIGHT,
    plot,
    groupWidth,
    barWidth,
    barGap,
    zeroY,
    centers,
    moneyTicks,
    percentTicks,
    xLabels,
    dividerX:
      model.dividerIndex === null
        ? null
        : plot.x + model.dividerIndex * groupWidth,
    bandPath,
    columns,
    marginPath,
    marginPoints,
    categoryPaths,
  };
}
