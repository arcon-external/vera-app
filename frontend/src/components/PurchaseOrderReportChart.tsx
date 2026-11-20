// Issue #631: the Purchase Order performance chart.
//
// One hand-written SVG, one pure geometry module, no charting dependency. The
// server's normalized report is the only source of every figure; this
// component derives pixels, not money. It owns its hover/touch/keyboard state
// so moving the cursor never re-renders the Reports page or its tables.
//
// Honesty rules encoded here:
// - amount, capital and margin share one zero-inclusive money axis and a
//   zero baseline, so a negative margin draws below zero;
// - the uncosted part is a hatch inside the amount column, never an extra
//   segment above it;
// - margin % is a straight line on its own right-hand percentage axis, and a
//   null is a gap, never 0%;
// - projected buckets are hatched, outlined, behind a labelled divider and in
//   a low-high amount band (never a fake margin band); the margin % line and
//   category lines stay on actual periods;
// - the table is the keyboard/screen-reader path, and the chart adds one
//   focus cursor rather than one tab stop per period.

import type {
  KeyboardEvent as ReactKeyboardEvent,
  PointerEvent as ReactPointerEvent,
  RefObject,
} from "react";
import {
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportProjectionSeries,
} from "../poReport";
import {
  buildReportChart,
  compactChartMoney,
  formatPercentTick,
  layoutReportChart,
  type ReportChartBucket,
  type ReportChartCategory,
  type ReportChartLayout,
  type ReportChartModel,
} from "../poReportChart";
import {
  formatReportBps,
  formatReportCount,
  formatReportDate,
  projectionSourceLabel,
  reportPeriodLabel,
} from "../poReportPage";
import { Badge, Card, SectionHeading } from "./ui";

export interface PurchaseOrderReportChartProps {
  report: PurchaseOrderReportData;
  /** Null when the app's catalogue cannot name the report currency. */
  currency: CurrencyInfo | null;
}

const SERIES = {
  amount: "#2e6866",
  capital: "#9c7337",
  marginPositive: "#387d59",
  marginNegative: "#ad453d",
  marginLine: "#1c1b19",
  grid: "#e6e3dc",
  rule: "#8a887e",
  axis: "#67655d",
  accent: "#295453",
  band: "#eef7f6",
  bandEdge: "#7fbfbc",
  paper: "#ffffff",
} as const;

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

// Before the first paint on the client, so the measured width and the entry
// class land without a flash of the 960px fallback or of the final marks.
// During SSR there is no layout pass, so the server uses the inert effect.
const useIsomorphicLayoutEffect =
  typeof window !== "undefined" ? useLayoutEffect : useEffect;

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return false;
    }
    return window.matchMedia(REDUCED_MOTION_QUERY).matches;
  });
  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }
    const query = window.matchMedia(REDUCED_MOTION_QUERY);
    const apply = () => setReduced(query.matches);
    apply();
    if (typeof query.addEventListener === "function") {
      query.addEventListener("change", apply);
      return () => query.removeEventListener("change", apply);
    }
    query.addListener(apply);
    return () => query.removeListener(apply);
  }, []);
  return reduced;
}

function useMeasuredWidth(): {
  width: number;
  surfaceRef: RefObject<HTMLDivElement | null>;
} {
  const surfaceRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(960);
  useIsomorphicLayoutEffect(() => {
    const node = surfaceRef.current;
    if (node === null) return;
    const measure = () => {
      const next = Math.round(
        node.clientWidth > 0 ? node.clientWidth : node.getBoundingClientRect().width
      );
      if (next > 0) setWidth(next);
    };
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  return { width, surfaceRef };
}

function bucketTitle(bucket: string): string {
  if (bucket === "year") return "Yearly";
  if (bucket === "quarter") return "Quarterly";
  return "Monthly";
}

function money(currency: CurrencyInfo | null, minor: number): string {
  return currency === null ? "—" : formatMoney(currency, minor);
}

function nearestBucket(layout: ReportChartLayout, contentX: number): number | null {
  if (layout.centers.length === 0 || layout.groupWidth <= 0) return null;
  const relative = (contentX - layout.plot.x) / layout.groupWidth - 0.5;
  return Math.max(
    0,
    Math.min(layout.centers.length - 1, Math.round(relative))
  );
}

function projectionSeries(
  report: PurchaseOrderReportData,
  kind: ReportChartModel["projectionKind"]
): PurchaseOrderReportProjectionSeries | null {
  if (kind === "monthly") return report.projection.monthly;
  if (kind === "yearly") return report.projection.yearly;
  return null;
}

function ChartTooltip({
  bucket,
  model,
  series,
  currency,
  left,
  width,
}: {
  bucket: ReportChartBucket;
  model: ReportChartModel;
  series: PurchaseOrderReportProjectionSeries | null;
  currency: CurrencyInfo;
  left: number;
  width: number;
}) {
  const projected = bucket.projected;
  const rows: Array<{ label: string; value: string; negative?: boolean }> =
    projected && bucket.projectedDetail !== null
      ? [
          {
            label: "Amount low",
            value: money(currency, bucket.projectedDetail.amountLowMinor),
          },
          {
            label: "Amount base",
            value: money(currency, bucket.projectedDetail.amountBaseMinor),
          },
          {
            label: "Amount high",
            value: money(currency, bucket.projectedDetail.amountHighMinor),
          },
          { label: "Capital", value: money(currency, bucket.capitalMinor) },
          {
            label: "Margin",
            value: money(currency, bucket.marginMinor),
            negative: bucket.marginMinor < 0,
          },
          { label: "Margin %", value: formatReportBps(bucket.marginBps) },
          {
            label: "Sample",
            value: formatReportCount(bucket.projectedDetail.sampleSize),
          },
          {
            label: "Basis",
            value: projectionSourceLabel(bucket.projectedDetail.source),
          },
        ]
      : [
          { label: "PO count", value: formatReportCount(bucket.poCount ?? 0) },
          { label: "Amount", value: money(currency, bucket.amountMinor) },
          {
            label: "Costed",
            value: money(currency, bucket.costedMinor ?? 0),
          },
          {
            label: "Uncosted",
            value: money(currency, bucket.uncostedMinor ?? 0),
          },
          { label: "Capital", value: money(currency, bucket.capitalMinor) },
          {
            label: "Margin",
            value: money(currency, bucket.marginMinor),
            negative: bucket.marginMinor < 0,
          },
          { label: "Margin %", value: formatReportBps(bucket.marginBps) },
          { label: "Coverage", value: formatReportBps(bucket.coverageBps) },
        ];

  return (
    <div
      data-chart-tooltip
      aria-hidden="true"
      className="pointer-events-none absolute top-3 z-20 rounded-xl border border-rule bg-white/95 p-3 text-left shadow-pop backdrop-blur-sm"
      style={{ left, width }}
    >
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-small font-semibold text-ink-950">
          {reportPeriodLabel(bucket.periodStart, model.bucket)}
        </p>
        {projected ? <Badge tone="accent">Projected</Badge> : null}
        {bucket.inProgress ? <Badge tone="neutral">In progress</Badge> : null}
        {bucket.partial && !bucket.inProgress ? (
          <Badge tone="neutral">Partial</Badge>
        ) : null}
      </div>
      <dl className="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-caption">
        {rows.map((row) => (
          <div key={row.label} className="contents">
            <dt className="text-ink-500">{row.label}</dt>
            <dd
              className={`tnum whitespace-nowrap text-right font-medium ${
                row.negative ? "text-danger-700" : "text-ink-900"
              }`}
            >
              {row.value}
            </dd>
          </div>
        ))}
      </dl>
      {projected && series !== null ? (
        <p className="mt-2 border-t border-rule pt-2 text-micro leading-relaxed text-ink-500">
          Basis: {series.basis.completeMonths} complete months ·{" "}
          {formatReportCount(series.basis.poCount)} POs observed · as of{" "}
          {formatReportDate(series.basis.asOf)}. Estimate from complete
          history, never actual evidence.
        </p>
      ) : null}
    </div>
  );
}

function LegendSwatch({
  color,
  hatched = false,
  outlined = false,
}: {
  color: string;
  hatched?: boolean;
  outlined?: boolean;
}) {
  return (
    <span
      aria-hidden="true"
      className="inline-block h-2.5 w-3.5 shrink-0 rounded-[2px]"
      style={{
        backgroundColor: color,
        backgroundImage: hatched
          ? "repeating-linear-gradient(45deg, transparent 0 2px, rgba(255,255,255,0.85) 2px 3.5px)"
          : undefined,
        border: outlined ? `1px dashed ${color}` : undefined,
      }}
    />
  );
}

function categorySwatch(category: ReportChartCategory) {
  return (
    <svg width="18" height="8" aria-hidden="true" className="shrink-0">
      <line
        x1="0"
        y1="4"
        x2="18"
        y2="4"
        stroke={category.color}
        strokeWidth="2"
        strokeDasharray={category.dash === "0" ? undefined : category.dash}
      />
    </svg>
  );
}

function ChartLegend({
  model,
  onToggle,
}: {
  model: ReportChartModel;
  onToggle: (code: string) => void;
}) {
  const items = [
    { label: "Amount", color: SERIES.amount },
    { label: "Capital", color: SERIES.capital },
    { label: "Margin", color: SERIES.marginPositive },
  ];
  return (
    <div className="mb-3 flex flex-wrap items-center gap-x-4 gap-y-2">
      <span className="text-micro font-medium uppercase tracking-[0.12em] text-ink-500">
        Key
      </span>
      <ul aria-label="Chart legend" className="flex flex-wrap items-center gap-x-4 gap-y-2">
        {items.map((item) => (
          <li key={item.label} className="inline-flex items-center gap-1.5 text-caption text-ink-600">
            <LegendSwatch color={item.color} />
            {item.label}
          </li>
        ))}
        {model.hasNegativeMargin ? (
          <li className="inline-flex items-center gap-1.5 text-caption text-ink-600">
            <LegendSwatch color={SERIES.marginNegative} />
            Negative margin
          </li>
        ) : null}
        {model.hasUncosted ? (
          <li className="inline-flex items-center gap-1.5 text-caption text-ink-600">
            <LegendSwatch color={SERIES.amount} hatched />
            Uncosted
          </li>
        ) : null}
        {model.projectedCount > 0 ? (
          <li className="inline-flex items-center gap-1.5 text-caption text-ink-600">
            <LegendSwatch color={SERIES.accent} hatched outlined />
            Projected
          </li>
        ) : null}
        {model.projectedCount > 0 ? (
          <li className="inline-flex items-center gap-1.5 text-caption text-ink-600">
            <span
              aria-hidden="true"
              className="relative inline-block h-3 w-3.5 shrink-0"
            >
              <svg width="14" height="12" aria-hidden="true">
                <line x1="7" y1="1" x2="7" y2="11" stroke={SERIES.bandEdge} strokeWidth="1.2" strokeDasharray="3 2" />
                <line x1="3" y1="1" x2="11" y2="1" stroke={SERIES.bandEdge} strokeWidth="1.2" />
                <line x1="3" y1="11" x2="11" y2="11" stroke={SERIES.bandEdge} strokeWidth="1.2" />
              </svg>
            </span>
            Low-high range
          </li>
        ) : null}
        {model.percentScale !== null ? (
          <li className="inline-flex items-center gap-1.5 text-caption text-ink-600">
            <span aria-hidden="true" className="inline-block h-0.5 w-5 shrink-0 bg-ink-950" />
            Margin %
          </li>
        ) : null}
      </ul>
      {model.categories.length > 0 ? (
        <ul
          aria-label="Category margin lines"
          className="flex flex-wrap items-center gap-x-2 gap-y-1.5"
        >
          <li className="mr-1 text-caption text-ink-500">Category margin %</li>
          {model.categories.map((category) => (
            <li key={category.code}>
              <button
                type="button"
                data-category-toggle={category.code}
                aria-pressed={category.visible}
                aria-label={`Show or hide the ${category.label} (${category.code}) category margin line`}
                onClick={() => onToggle(category.code)}
                className={`inline-flex min-h-8 cursor-pointer items-center gap-1.5 rounded-md border px-2 text-caption transition-colors ${
                  category.visible
                    ? "border-accent-300 bg-accent-50 text-accent-800"
                    : "border-rule bg-white text-ink-600 hover:border-ink-300 hover:text-ink-900"
                }`}
              >
                {categorySwatch(category)}
                {category.label} ({category.code})
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function PurchaseOrderReportChart({
  report,
  currency,
}: PurchaseOrderReportChartProps) {
  const rawId = useId();
  const uid = rawId.replace(/[^a-zA-Z0-9_-]/g, "");
  const titleId = `chart-title-${uid}`;
  const captionId = `chart-caption-${uid}`;
  const descId = `chart-desc-${uid}`;
  const clipId = `chart-clip-${uid}`;
  const uncostedId = `chart-uncosted-${uid}`;
  const projectedAmountId = `chart-projected-amount-${uid}`;
  const projectedCapitalId = `chart-projected-capital-${uid}`;
  const projectedMarginPosId = `chart-projected-margin-pos-${uid}`;
  const projectedMarginNegId = `chart-projected-margin-neg-${uid}`;

  const reducedMotion = useReducedMotion();
  const [mounted, setMounted] = useState(false);
  useIsomorphicLayoutEffect(() => {
    setMounted(true);
  }, []);
  const animate = mounted && !reducedMotion;

  const { width: measuredWidth, surfaceRef } = useMeasuredWidth();
  const [visibleCategories, setVisibleCategories] = useState<string[]>([]);
  const visibleKey = useMemo(
    () => [...visibleCategories].sort().join(","),
    [visibleCategories]
  );
  const visibleCodes = useMemo(
    () => (visibleKey === "" ? [] : visibleKey.split(",")),
    [visibleKey]
  );
  const model = useMemo(
    () => buildReportChart(report, visibleCodes),
    [report, visibleCodes]
  );
  const layout = useMemo(
    () => layoutReportChart(model, measuredWidth),
    [model, measuredWidth]
  );

  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const [pointerX, setPointerX] = useState<number | null>(null);
  const [keyboardActive, setKeyboardActive] = useState(false);
  const [viewportScroll, setViewportScroll] = useState(0);

  useEffect(() => {
    setActiveIndex(null);
    setPointerX(null);
    setKeyboardActive(false);
  }, [model.dataKey]);

  // A keyboard cursor that walks off the visible slice of a 120-bucket chart
  // must bring its bucket into view. This scroll is a layout operation, not an
  // animation: the marks do not replay and the table is untouched.
  useEffect(() => {
    if (!keyboardActive || activeIndex === null) return;
    const node = surfaceRef.current;
    if (node === null || node.clientWidth <= 0) return;
    const center = layout.centers[activeIndex];
    if (!Number.isFinite(center)) return;
    const leftEdge = center - layout.groupWidth;
    const rightEdge = center + layout.groupWidth;
    let next = node.scrollLeft;
    if (leftEdge < node.scrollLeft) {
      next = Math.max(0, leftEdge - 8);
    } else if (rightEdge > node.scrollLeft + node.clientWidth) {
      next = Math.max(0, rightEdge - node.clientWidth + 8);
    }
    if (next !== node.scrollLeft) {
      node.scrollLeft = next;
      setViewportScroll(next);
    }
  }, [activeIndex, keyboardActive, layout, surfaceRef]);

  // A width change moves every bucket centre. A stationary pointer must still
  // point at the bucket under it, not at the index that used to be there.
  // Scrolling is handled by the surface; the resize case is this effect's job.
  useEffect(() => {
    if (pointerX === null) return;
    const node = surfaceRef.current;
    if (node === null) return;
    const index = nearestBucket(layout, pointerX + node.scrollLeft);
    setActiveIndex((current) => (current === index ? current : index));
  }, [layout, pointerX, surfaceRef]);

  const toggleCategory = (code: string) => {
    setVisibleCategories((current) =>
      current.includes(code)
        ? current.filter((entry) => entry !== code)
        : [...current, code]
    );
  };

  const activeBucket =
    activeIndex === null ? null : model.buckets[activeIndex] ?? null;
  const activeSeries = projectionSeries(report, model.projectionKind);
  const tooltipWidth = Math.max(
    180,
    Math.min(320, Math.max(1, measuredWidth) - 16)
  );
  const keyboardAnchor =
    activeIndex !== null
      ? layout.centers[activeIndex] - viewportScroll
      : 0;
  const anchorX = pointerX ?? keyboardAnchor;
  const tooltipLeft = Math.min(
    Math.max(anchorX - tooltipWidth / 2, 8),
    Math.max(8, measuredWidth - tooltipWidth - 8)
  );

  const selectAt = (event: ReactPointerEvent<HTMLDivElement>) => {
    const node = event.currentTarget;
    const rect = node.getBoundingClientRect();
    const contentX = event.clientX - rect.left + node.scrollLeft;
    const index = nearestBucket(layout, contentX);
    setActiveIndex(index);
    setPointerX(event.clientX - rect.left);
    setKeyboardActive(false);
  };

  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    // A touch drag pans a long chart horizontally. Selecting while the finger
    // moves would fight the scroll, so a touch reads the value on touch-down
    // and leaves movement to the surface.
    if (event.pointerType === "touch") return;
    selectAt(event);
  };

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.pointerType === "mouse" && event.button !== 0) return;
    selectAt(event);
  };

  const handlePointerLeave = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.pointerType === "touch") return;
    setActiveIndex(null);
    setPointerX(null);
    setKeyboardActive(false);
  };

  const moveCursor = (target: "first" | "last" | "next" | "previous") => {
    if (model.buckets.length === 0) return;
    setKeyboardActive(true);
    setPointerX(null);
    setActiveIndex((current) => {
      if (target === "first") return 0;
      if (target === "last") return model.buckets.length - 1;
      if (current === null) {
        return target === "next" ? 0 : model.buckets.length - 1;
      }
      return target === "next"
        ? Math.min(model.buckets.length - 1, current + 1)
        : Math.max(0, current - 1);
    });
  };

  const handleKeyDown = (event: ReactKeyboardEvent<SVGSVGElement>) => {
    switch (event.key) {
      case "ArrowRight":
        event.preventDefault();
        moveCursor("next");
        break;
      case "ArrowLeft":
        event.preventDefault();
        moveCursor("previous");
        break;
      case "Home":
        event.preventDefault();
        moveCursor("first");
        break;
      case "End":
        event.preventDefault();
        moveCursor("last");
        break;
      case "Escape":
        setActiveIndex(null);
        setPointerX(null);
        setKeyboardActive(false);
        break;
      default:
        break;
    }
  };

  if (currency === null) {
    return (
      <Card className="min-w-0 p-5" data-report-chart="">
        <SectionHeading
          title="Performance"
          hint={bucketTitle(report.filter.bucket)}
        />
        <p className="max-w-3xl text-small leading-relaxed text-ink-600">
          {report.filter.currency} is not in this app's currency catalogue, so
          the money chart cannot be labelled honestly. The period table below
          still lists the server's exact figures.
        </p>
      </Card>
    );
  }

  const series = projectionSeries(report, model.projectionKind);
  const notes: string[] = [];
  if (model.projectionNote !== null) notes.push(model.projectionNote);
  if (
    model.projectedCount === 0 &&
    model.projectionKind !== "none" &&
    series !== null &&
    series.status !== "available"
  ) {
    notes.push(
      `Basis: ${series.basis.completeMonths} complete months, ${formatReportCount(
        series.basis.poCount
      )} POs observed, as of ${formatReportDate(series.basis.asOf)}.`
    );
  }
  if (model.projectedCount > 0) {
    notes.push(
      "Projected periods follow the dashed divider: hatched, outlined and estimated from complete history. They are never included in actual totals; the margin % line and category lines cover actual periods only."
    );
  }
  if (model.hasUncosted) {
    notes.push(
      "The hatched part of an amount column is uncosted. It is part of the amount, not an extra segment above it."
    );
  }
  if (model.hasPartial) {
    notes.push("A dot under the axis marks a partial or in-progress period.");
  }
  if (model.percentScale === null) {
    notes.push(
      "No margin percentage is available in this range, so the right axis is omitted."
    );
  }
  if (model.categoryFootnote !== null) notes.push(model.categoryFootnote);

  const description =
    "Grouped columns show amount, capital and margin on the left money axis; the line shows margin percentage on the right axis. " +
    "The uncosted part of an amount is hatched inside that column. " +
    (model.projectedCount > 0
      ? "Projected periods after the dashed divider are estimates and are never included in actual totals. "
      : "") +
    "Use the Left and Right Arrow keys to inspect one period; the period table after this chart lists every exact value.";

  const announcement =
    activeBucket === null || !keyboardActive
      ? ""
      : activeBucket.projected && activeBucket.projectedDetail !== null
        ? `Projected ${reportPeriodLabel(
            activeBucket.periodStart,
            model.bucket
          )}: amount low ${money(
            currency,
            activeBucket.projectedDetail.amountLowMinor
          )}, base ${money(
            currency,
            activeBucket.projectedDetail.amountBaseMinor
          )}, high ${money(
            currency,
            activeBucket.projectedDetail.amountHighMinor
          )}, capital ${money(currency, activeBucket.capitalMinor)}, margin ${money(
            currency,
            activeBucket.marginMinor
          )}, margin ${formatReportBps(
            activeBucket.marginBps
          )}, sample ${formatReportCount(
            activeBucket.projectedDetail.sampleSize
          )}, basis ${projectionSourceLabel(
            activeBucket.projectedDetail.source
          )}.`
        : `${reportPeriodLabel(activeBucket.periodStart, model.bucket)}: ${
            activeBucket.poCount ?? 0
          } purchase orders, amount ${money(
            currency,
            activeBucket.amountMinor
          )}, capital ${money(currency, activeBucket.capitalMinor)}, margin ${money(
            currency,
            activeBucket.marginMinor
          )}, margin ${formatReportBps(
            activeBucket.marginBps
          )}, coverage ${formatReportBps(activeBucket.coverageBps)}.`;

  const lastMarginPoint = layout.marginPoints.at(-1);
  const latestBucket =
    lastMarginPoint === undefined ? null : model.buckets[lastMarginPoint.index];
  const latestLabel = latestBucket?.marginBps ?? null;
  const latestLabelWidth =
    latestLabel === null ? 0 : formatReportBps(latestLabel).length * 6.2;
  const latestAnchorEnd =
    lastMarginPoint !== undefined &&
    lastMarginPoint.x + 6 + latestLabelWidth > layout.plot.x + layout.plot.width;

  return (
    <Card className="min-w-0 p-5" data-report-chart="">
      <SectionHeading
        title="Performance"
        hint={bucketTitle(report.filter.bucket)}
      />
      <figure className="m-0">
        <figcaption
          id={captionId}
          className="max-w-4xl text-small leading-relaxed text-ink-600"
        >
          <span className="block">{model.summary}</span>
          <span className="mt-1 block text-caption text-ink-500">
            Money columns use the left axis; margin % uses the right. The hatch
            inside an amount column is uncosted, outlined and hatched periods
            are projected, and the dashed divider separates them from actuals.
          </span>
        </figcaption>
        <ChartLegend
          model={model}
          onToggle={toggleCategory}
        />
        <div className="mb-1 flex items-baseline justify-between gap-3 text-caption text-ink-600">
          <span>
            <span className="font-medium text-ink-700">Money</span> ·{" "}
            {currency.code}
          </span>
          <span>
            <span className="font-medium text-ink-700">Margin %</span>
            {model.percentScale === null ? " unavailable" : " · right axis"}
          </span>
        </div>
        <div className="relative">
          <div
            ref={surfaceRef}
            data-chart-surface
            className="overflow-x-auto scroll-thin"
            style={{ touchAction: "pan-x pan-y" }}
            onPointerMove={handlePointerMove}
            onPointerDown={handlePointerDown}
            onPointerLeave={handlePointerLeave}
            onScroll={(event) => {
              const next = event.currentTarget.scrollLeft;
              setViewportScroll((current) => (current === next ? current : next));
              if (pointerX !== null) {
                setActiveIndex(nearestBucket(layout, pointerX + next));
              }
            }}
          >
            <svg
              data-chart-svg
              width={layout.width}
              height={layout.height}
              viewBox={`0 0 ${layout.width} ${layout.height}`}
              role="img"
              tabIndex={0}
              aria-labelledby={`${titleId} ${captionId}`}
              aria-describedby={descId}
              onKeyDown={handleKeyDown}
              onFocus={() => {
                setKeyboardActive(true);
                setPointerX(null);
                setActiveIndex((current) =>
                  current ?? (model.buckets.length > 0 ? 0 : null)
                );
              }}
              onBlur={() => {
                setKeyboardActive(false);
                setActiveIndex(null);
                setPointerX(null);
              }}
              className="block max-w-none rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent-500"
            >
              <title id={titleId}>Purchase order performance chart</title>
              <desc id={descId}>{description}</desc>
              <defs>
                <clipPath id={clipId}>
                  <rect
                    x={layout.plot.x}
                    y={layout.plot.y}
                    width={layout.plot.width}
                    height={layout.plot.height}
                  />
                </clipPath>
                <pattern
                  id={uncostedId}
                  patternUnits="userSpaceOnUse"
                  width="6"
                  height="6"
                  patternTransform="rotate(45)"
                >
                  <line
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="6"
                    stroke={SERIES.paper}
                    strokeWidth="1.5"
                  />
                </pattern>
                <pattern
                  id={projectedAmountId}
                  patternUnits="userSpaceOnUse"
                  width="6"
                  height="6"
                  patternTransform="rotate(45)"
                >
                  <rect width="6" height="6" fill={SERIES.band} />
                  <line
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="6"
                    stroke={SERIES.amount}
                    strokeWidth="1.2"
                    opacity="0.55"
                  />
                </pattern>
                <pattern
                  id={projectedCapitalId}
                  patternUnits="userSpaceOnUse"
                  width="6"
                  height="6"
                  patternTransform="rotate(45)"
                >
                  <rect width="6" height="6" fill="#f6f4f0" />
                  <line
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="6"
                    stroke={SERIES.capital}
                    strokeWidth="1.2"
                    opacity="0.55"
                  />
                </pattern>
                <pattern
                  id={projectedMarginPosId}
                  patternUnits="userSpaceOnUse"
                  width="6"
                  height="6"
                  patternTransform="rotate(45)"
                >
                  <rect width="6" height="6" fill="#eef6f1" />
                  <line
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="6"
                    stroke={SERIES.marginPositive}
                    strokeWidth="1.2"
                    opacity="0.55"
                  />
                </pattern>
                <pattern
                  id={projectedMarginNegId}
                  patternUnits="userSpaceOnUse"
                  width="6"
                  height="6"
                  patternTransform="rotate(45)"
                >
                  <rect width="6" height="6" fill="#fbf0ef" />
                  <line
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="6"
                    stroke={SERIES.marginNegative}
                    strokeWidth="1.2"
                    opacity="0.55"
                  />
                </pattern>
              </defs>

              <g aria-hidden="true" data-chart-axis="money">
                {layout.moneyTicks.map((tick) => (
                  <g key={tick.value}>
                    <line
                      x1={layout.plot.x}
                      x2={layout.plot.x + layout.plot.width}
                      y1={tick.y}
                      y2={tick.y}
                      stroke={tick.value === 0 ? SERIES.rule : SERIES.grid}
                      strokeWidth={tick.value === 0 ? 1 : 0.7}
                      vectorEffect="non-scaling-stroke"
                    />
                    <text
                      x={layout.plot.x - 8}
                      y={tick.y + 3.5}
                      textAnchor="end"
                      fontSize={10.5}
                      fill={SERIES.axis}
                      className="tnum"
                    >
                      {compactChartMoney(currency, tick.value)}
                    </text>
                  </g>
                ))}
              </g>

              <g aria-hidden="true" data-chart-axis="percent">
                {layout.percentTicks.map((tick) => (
                  <g key={tick.value}>
                    <line
                      x1={layout.plot.x + layout.plot.width}
                      x2={layout.plot.x + layout.plot.width + 4}
                      y1={tick.y}
                      y2={tick.y}
                      stroke={SERIES.grid}
                      strokeWidth={0.8}
                      vectorEffect="non-scaling-stroke"
                    />
                    <text
                      x={layout.plot.x + layout.plot.width + 8}
                      y={tick.y + 3.5}
                      fontSize={10.5}
                      fill={SERIES.axis}
                      className="tnum"
                    >
                      {formatPercentTick(tick.value)}
                    </text>
                  </g>
                ))}
              </g>

              <g clipPath={`url(#${clipId})`}>
                <g
                  key={model.dataKey}
                  data-chart-marks
                  data-chart-motion={animate ? "run" : "static"}
                >
                  {layout.bandWhiskers.map((whisker, index) => (
                    <g
                      key={`range-${index}`}
                      data-chart-band
                      data-bucket-index={model.actualCount + index}
                      className={animate ? "report-chart-band-in" : undefined}
                    >
                      <line
                        x1={whisker.x}
                        x2={whisker.x}
                        y1={whisker.highY}
                        y2={whisker.lowY}
                        stroke={SERIES.bandEdge}
                        strokeWidth={1.2}
                        strokeDasharray="4 3"
                      />
                      <line
                        x1={whisker.x - 4}
                        x2={whisker.x + 4}
                        y1={whisker.highY}
                        y2={whisker.highY}
                        stroke={SERIES.bandEdge}
                        strokeWidth={1.2}
                      />
                      <line
                        x1={whisker.x - 4}
                        x2={whisker.x + 4}
                        y1={whisker.lowY}
                        y2={whisker.lowY}
                        stroke={SERIES.bandEdge}
                        strokeWidth={1.2}
                      />
                    </g>
                  ))}

                  {layout.columns.map((column) => {
                    const bucket = model.buckets[column.index];
                    const projected = bucket.projected;
                    const marginPattern =
                      bucket.marginMinor < 0
                        ? projectedMarginNegId
                        : projectedMarginPosId;
                    const delay = Math.min(column.index * 10, 120);
                    const showBar = (height: number) => height > 0.2;
                    return (
                      <g
                        key={bucket.key}
                        data-chart-bucket
                        data-projected={projected ? "true" : "false"}
                        data-bucket-index={column.index}
                        data-amount={bucket.amountMinor}
                        data-capital={bucket.capitalMinor}
                        data-margin={bucket.marginMinor}
                        className={
                          animate ? "report-chart-column-in" : undefined
                        }
                        style={
                          animate
                            ? {
                                animationDelay: `${delay}ms`,
                                transformOrigin: `0px ${layout.zeroY}px`,
                              }
                            : undefined
                        }
                      >
                        {showBar(column.amount.height) ? (
                          <rect
                            data-series="amount"
                            data-value={bucket.amountMinor}
                            data-negative={column.amount.negative ? "true" : "false"}
                            x={column.amount.x}
                            y={column.amount.y}
                            width={column.amount.width}
                            height={column.amount.height}
                            rx={column.amount.width > 6 ? 1.5 : 0}
                            fill={
                              projected
                                ? `url(#${projectedAmountId})`
                                : SERIES.amount
                            }
                            stroke={projected ? SERIES.amount : undefined}
                            strokeWidth={projected ? 1 : undefined}
                            strokeDasharray={projected ? "3 2" : undefined}
                          />
                        ) : null}
                        {column.uncosted !== null ? (
                          <rect
                            data-chart-uncosted
                            x={column.uncosted.x}
                            y={column.uncosted.y}
                            width={column.uncosted.width}
                            height={column.uncosted.height}
                            fill={`url(#${uncostedId})`}
                          />
                        ) : null}
                        {showBar(column.capital.height) ? (
                          <rect
                            data-series="capital"
                            data-value={bucket.capitalMinor}
                            data-negative={column.capital.negative ? "true" : "false"}
                            x={column.capital.x}
                            y={column.capital.y}
                            width={column.capital.width}
                            height={column.capital.height}
                            rx={column.capital.width > 6 ? 1.5 : 0}
                            fill={
                              projected
                                ? `url(#${projectedCapitalId})`
                                : SERIES.capital
                            }
                            stroke={projected ? SERIES.capital : undefined}
                            strokeWidth={projected ? 1 : undefined}
                            strokeDasharray={projected ? "3 2" : undefined}
                          />
                        ) : null}
                        {showBar(column.margin.height) ? (
                          <rect
                            data-series="margin"
                            data-value={bucket.marginMinor}
                            data-negative={column.margin.negative ? "true" : "false"}
                            x={column.margin.x}
                            y={column.margin.y}
                            width={column.margin.width}
                            height={column.margin.height}
                            rx={column.margin.width > 6 ? 1.5 : 0}
                            fill={
                              projected
                                ? `url(#${marginPattern})`
                                : column.margin.negative
                                  ? SERIES.marginNegative
                                  : SERIES.marginPositive
                            }
                            stroke={
                              projected
                                ? column.margin.negative
                                  ? SERIES.marginNegative
                                  : SERIES.marginPositive
                                : undefined
                            }
                            strokeWidth={projected ? 1 : undefined}
                            strokeDasharray={projected ? "3 2" : undefined}
                          />
                        ) : null}
                      </g>
                    );
                  })}

                  {layout.categoryPaths.map((line) => {
                    const category = model.categories.find(
                      (entry) => entry.code === line.code
                    );
                    if (category === undefined) return null;
                    return (
                      <path
                        key={line.code}
                        data-category-line={line.code}
                        d={line.path}
                        fill="none"
                        stroke={category.color}
                        strokeWidth={1.4}
                        strokeDasharray={
                          category.dash === "0" ? undefined : category.dash
                        }
                        strokeLinejoin="round"
                        strokeLinecap="round"
                      />
                    );
                  })}

                  {layout.marginPath !== null ? (
                    <g>
                      <path
                        data-chart-margin-line
                        d={layout.marginPath}
                        fill="none"
                        stroke={SERIES.marginLine}
                        strokeWidth={2.1}
                        strokeLinejoin="round"
                        strokeLinecap="round"
                        pathLength={1}
                        className={
                          animate ? "report-chart-line-in" : undefined
                        }
                      />
                      {layout.marginPoints.map((point) => (
                        <circle
                          key={point.index}
                          data-chart-margin-point
                          cx={point.x}
                          cy={point.y}
                          r={2.3}
                          fill={SERIES.paper}
                          stroke={SERIES.marginLine}
                          strokeWidth={1.4}
                        />
                      ))}
                      {lastMarginPoint !== undefined && latestLabel !== null ? (
                        <text
                          data-chart-latest-margin
                          x={
                            latestAnchorEnd
                              ? lastMarginPoint.x - 6
                              : lastMarginPoint.x + 6
                          }
                          y={Math.max(
                            layout.plot.y + 10,
                            Math.min(
                              layout.plot.y + layout.plot.height - 6,
                              lastMarginPoint.y - 6
                            )
                          )}
                          textAnchor={latestAnchorEnd ? "end" : "start"}
                          fontSize={10.5}
                          fontWeight={600}
                          fill={SERIES.marginLine}
                          stroke={SERIES.paper}
                          strokeWidth={3}
                          paintOrder="stroke"
                          className="tnum"
                        >
                          {formatReportBps(latestLabel)}
                        </text>
                      ) : null}
                    </g>
                  ) : null}
                </g>

                {activeIndex !== null && layout.centers[activeIndex] !== undefined ? (
                  <g data-chart-hover aria-hidden="true">
                    <rect
                      data-chart-hover-band
                      x={layout.centers[activeIndex] - layout.groupWidth / 2}
                      y={layout.plot.y}
                      width={layout.groupWidth}
                      height={layout.plot.height}
                      fill="#1c1b19"
                      fillOpacity={0.05}
                    />
                    <line
                      x1={layout.centers[activeIndex]}
                      x2={layout.centers[activeIndex]}
                      y1={layout.plot.y}
                      y2={layout.plot.y + layout.plot.height}
                      stroke="#1c1b19"
                      strokeOpacity={0.22}
                      strokeWidth={1}
                      strokeDasharray="3 3"
                    />
                  </g>
                ) : null}
              </g>

              {layout.dividerX !== null ? (
                <g aria-hidden="true">
                  <line
                    data-chart-divider
                    x1={layout.dividerX}
                    x2={layout.dividerX}
                    y1={layout.plot.y}
                    y2={layout.plot.y + layout.plot.height}
                    stroke={SERIES.accent}
                    strokeOpacity={0.55}
                    strokeWidth={1.2}
                    strokeDasharray="4 4"
                  />
                  <text
                    data-chart-divider-label
                    x={layout.dividerX + 6}
                    y={layout.plot.y + 12}
                    fontSize={10.5}
                    fontWeight={600}
                    fill={SERIES.accent}
                    stroke={SERIES.paper}
                    strokeWidth={3}
                    paintOrder="stroke"
                  >
                    Projected
                  </text>
                </g>
              ) : null}

              <g aria-hidden="true" data-chart-axis="x">
                {layout.xLabels.map((label) => (
                  <text
                    key={label.index}
                    x={label.x}
                    y={layout.plot.y + layout.plot.height + 16}
                    textAnchor={label.anchor}
                    fontSize={10.5}
                    fill={SERIES.axis}
                    className="tnum"
                  >
                    {label.label}
                  </text>
                ))}
                {model.buckets.map((bucket, index) =>
                  bucket.partial || bucket.inProgress ? (
                    <circle
                      key={`partial-${bucket.key}`}
                      data-chart-partial={bucket.key}
                      cx={layout.centers[index]}
                      cy={layout.plot.y + layout.plot.height + 5}
                      r={2}
                      fill={SERIES.accent}
                    />
                  ) : null
                )}
              </g>
            </svg>
          </div>
          {activeBucket !== null ? (
            <ChartTooltip
              bucket={activeBucket}
              model={model}
              series={activeSeries}
              currency={currency}
              left={tooltipLeft}
              width={tooltipWidth}
            />
          ) : null}
        </div>
        <p className="sr-only" aria-live="polite">
          {announcement}
        </p>
        <div className="mt-3">
          <a
            href="#report-period-table"
            className="text-small font-medium text-accent-700 underline decoration-accent-300 underline-offset-2 hover:text-accent-800"
          >
            Period table: every exact value
          </a>
        </div>
        {notes.length > 0 ? (
          <ul className="mt-2 grid gap-1 text-caption leading-relaxed text-ink-500">
            {notes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        ) : null}
      </figure>
    </Card>
  );
}
