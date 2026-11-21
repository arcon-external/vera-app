import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { errorMessage } from "../errors";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import {
  formatReportBps,
  formatReportCount,
  initialReportFilters,
  reportFilterKey,
  reportFilterValidation,
  reportPeriodBucket,
  reportPeriodLabel,
  reportPeriodRange,
  reportPresetRange,
  toReportQuery,
  type ReportBucket,
  type ReportFilterState,
  type ReportPeriodMode,
  type ReportPreset,
} from "../poReportPage";
import type {
  PurchaseOrderReportData,
  PurchaseOrderReportMetrics,
  PurchaseOrderReportQuery,
} from "../poReport";
import { msUntilNextWIBMidnight, todayWIB } from "../workspaceSettings";
import { useToast } from "../toast";
import {
  usePOReportCategoryComparison,
  usePOReportMonthComparison,
  usePOReportOverviewSource,
  usePOReportSourceRegistry,
} from "../usePOReportSources";
import {
  buildPurchaseOrderReportView,
  type PurchaseOrderReportView,
} from "../poReportExport";
import {
  allMonthsOfYear,
  annualMonthRequest,
  categorySourceRequest,
  monthLabel,
  overviewCanSupplyCurrentMonth,
  overviewIsAnnualForMonth,
  type PurchaseOrderMonthReadout,
} from "../poReportComparison";
import { PurchaseOrderReportMonthTable } from "./PurchaseOrderReportMonthTable";
import {
  PurchaseOrderReportMonthBrowser,
  type PurchaseOrderReportMonthBrowserProps,
} from "./PurchaseOrderReportMonthBrowser";
import {
  PurchaseOrderReportCategoryBrowser,
  type PurchaseOrderReportCategoryBrowserProps,
} from "./PurchaseOrderReportCategoryBrowser";
import { PurchaseOrderReportOverview } from "./PurchaseOrderReportOverview";
import { PurchaseOrderReportPicker } from "./PurchaseOrderReportPicker";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

// Issue #630: the global Reports workspace. It turns the #629 report endpoint
// into a screen and nothing more: one normalized snapshot is fetched per
// filter change, every figure is the server's, and the chart (#631) and the
// PDF renderer (#632) are later consumers of the same payload.
//
// "Reports" is a standalone workspace, not a Purchase Order tab. The page is
// kept mounted by App after its first visit, so its filter state survives
// navigation while the workspace remains mounted. Opening it never clears a
// document draft: App only changes its own page state.

export interface PurchaseOrderReportsPageProps {
  /** The company currency, used for the first request only. */
  defaultCurrency: string;
  /** App's catalogue. A code missing here renders "-", never a fake IDR. */
  currencyCatalogue: readonly CurrencyInfo[] | null;
  /**
   * #632 owns the native PDF binding. Until App wires it after #630/#632
   * merge, the export button stays disabled and this page never guesses a
   * file path or success.
   */
  onExport?: (view: PurchaseOrderReportView) => Promise<string>;
  /**
   * Issue #645: true while Reports is the visible workspace. A hidden page
   * fetches nothing; each false → true return revalidates the currently
   * required scopes once. Defaults to true for standalone tests.
   */
  active?: boolean;
  /**
   * Issue #645: App bumps this after a relevant successful mutation (saved PO,
   * correction, repair, link, deletion). It marks every cached snapshot stale
   * without fetching while hidden.
   */
  mutationEpoch?: number;
}

export interface PurchaseOrderReportsViewProps {
  filters: ReportFilterState;
  report: PurchaseOrderReportData | null;
  loading: boolean;
  stale: boolean;
  error: string;
  validation: string;
  catalogue: readonly CurrencyInfo[] | null;
  exporting: boolean;
  canExport: boolean;
  onPresetChange: (preset: ReportPreset) => void;
  onPeriodChange: (period: ReportPeriodMode | null) => void;
  onFromChange: (value: string) => void;
  onToChange: (value: string) => void;
  onBucketChange: (value: ReportBucket) => void;
  onCategoryChange: (value: string) => void;
  onCurrencyChange: (value: string) => void;
  onRetry: () => void;
  onExport: () => void;
  /** Issue #645: revalidates the currently visible required scopes. */
  onRefresh?: () => void;
  /** True while a visible required source is re-reading. */
  refreshing?: boolean;
  /** Optional so direct fixture/harness callers keep the pre-#641 table. */
  monthReadout?: PurchaseOrderMonthReadout | null;
  onPreviousMonth?: () => void;
  onNextMonth?: () => void;
  onRetryMonth?: () => void;
  /** The live #642 comparison; absent for direct fixture/harness callers. */
  monthBrowser?: Omit<PurchaseOrderReportMonthBrowserProps, "entry"> | null;
  /** The live #643 comparison; absent for direct fixture/harness callers. */
  categoryBrowser?: Omit<
    PurchaseOrderReportCategoryBrowserProps,
    "entry"
  > | null;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

function negativeClass(value: number): string {
  return value < 0 ? "text-danger-700" : "text-ink-900";
}

function SectionTitle({
  title,
  hint,
}: {
  title: string;
  hint?: string;
}) {
  return <SectionHeading title={title} hint={hint} />;
}

function MetricCells({
  metrics,
  entry,
}: {
  metrics: PurchaseOrderReportMetrics;
  entry: CurrencyInfo | null;
}) {
  return (
    <>
      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
        {formatReportCount(metrics.poCount)}
      </td>
      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
        {money(entry, metrics.amountMinor)}
      </td>
      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
        {money(entry, metrics.costedMinor)}
      </td>
      <td className="tnum whitespace-nowrap py-3 pr-4 text-right align-middle text-ink-700">
        {money(entry, metrics.capitalMinor)}
      </td>
      <td
        className={`tnum whitespace-nowrap py-3 pr-4 text-right align-middle font-medium ${negativeClass(
          metrics.marginMinor
        )}`}
      >
        {money(entry, metrics.marginMinor)}
      </td>
      <td className="tnum whitespace-nowrap py-3 pr-5 text-right align-middle text-ink-700">
        {formatReportBps(metrics.marginBps)}
      </td>
    </>
  );
}

function CategoryTable({
  report,
  entry,
}: {
  report: PurchaseOrderReportData;
  entry: CurrencyInfo | null;
}) {
  return (
    <Card className="min-w-0 overflow-hidden">
      <div className="px-5 pt-5">
        <SectionTitle title="By category" />
      </div>
      <div className="overflow-x-auto scroll-thin">
        <table
          aria-label="Category totals"
          className="w-full min-w-[760px] text-left text-body"
        >
          <thead>
            <tr className="border-y border-rule bg-paper-subtle/60 text-micro uppercase tracking-[0.1em] text-ink-500">
              <th className="px-5 py-3 font-medium">Category</th>
              <th className="py-3 pr-4 text-right font-medium">POs</th>
              <th className="py-3 pr-4 text-right font-medium">Amount</th>
              <th className="py-3 pr-4 text-right font-medium">Costed</th>
              <th className="py-3 pr-4 text-right font-medium">Capital</th>
              <th className="py-3 pr-4 text-right font-medium">Margin</th>
              <th className="py-3 pr-5 text-right font-medium">Margin %</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-rule">
            {report.categories.map((row) => (
              <tr key={row.businessCategory}>
                <td className="px-5 py-3 align-middle font-medium text-ink-900">
                  {row.label || row.businessCategory}
                </td>
                <MetricCells metrics={row} entry={entry} />
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="border-t border-rule-strong bg-paper-subtle/40 font-medium text-ink-900">
              <td className="px-5 py-3">Total</td>
              <MetricCells metrics={report.totals} entry={entry} />
            </tr>
          </tfoot>
        </table>
      </div>
    </Card>
  );
}

function PeriodTable({
  report,
  entry,
  periods,
}: {
  report: PurchaseOrderReportData;
  entry: CurrencyInfo | null;
  periods: PurchaseOrderReportData["periods"];
}) {
  return (
    <Card className="min-w-0 overflow-hidden">
      <div className="px-5 pt-5">
        <SectionTitle
          title="By period"
          hint={
            periods.length < report.periods.length
              ? "Actual periods to date. Projections are in the chart."
              : undefined
          }
        />
      </div>
      <div className="overflow-x-auto scroll-thin">
        <table
          id="report-period-table"
          tabIndex={-1}
          aria-label="Period totals"
          className="w-full min-w-[760px] scroll-mt-6 text-left text-body"
        >
          <thead>
            <tr className="border-y border-rule bg-paper-subtle/60 text-micro uppercase tracking-[0.1em] text-ink-500">
              <th className="px-5 py-3 font-medium">Period</th>
              <th className="py-3 pr-4 text-right font-medium">POs</th>
              <th className="py-3 pr-4 text-right font-medium">Amount</th>
              <th className="py-3 pr-4 text-right font-medium">Costed</th>
              <th className="py-3 pr-4 text-right font-medium">Capital</th>
              <th className="py-3 pr-4 text-right font-medium">Margin</th>
              <th className="py-3 pr-5 text-right font-medium">Margin %</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-rule">
            {periods.map((row) => (
              <tr key={row.periodStart}>
                <td className="px-5 py-3 align-middle font-medium text-ink-900">
                  <span className="tnum">
                    {reportPeriodLabel(row.periodStart, report.filter.bucket)}
                  </span>
                  {row.partial || row.inProgress ? (
                    <span className="ml-2 inline-flex align-middle">
                      <Badge tone="neutral">
                        {row.partial ? "Partial" : "In progress"}
                      </Badge>
                    </span>
                  ) : null}
                </td>
                <MetricCells metrics={row} entry={entry} />
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr className="border-t border-rule-strong bg-paper-subtle/40 font-medium text-ink-900">
              <td className="px-5 py-3">Total</td>
              <MetricCells metrics={report.totals} entry={entry} />
            </tr>
          </tfoot>
        </table>
      </div>
    </Card>
  );
}

function ErrorCard({ error, onRetry }: { error: string; onRetry: () => void }) {
  return (
    <Card className="min-w-0 p-8">
      <h2 className="text-title font-semibold text-ink-950">
        Report unavailable
      </h2>
      <p className="mt-1.5 max-w-2xl text-body leading-relaxed text-ink-500">
        {error}
      </p>
      <p className="mt-2 text-caption leading-relaxed text-ink-500">
        Adjust the filters, then retry. If the API is unreachable, try again
        when it is back.
      </p>
      <div className="mt-4">
        <Button aria-label="Retry report" onClick={onRetry}>
          Retry
        </Button>
      </div>
    </Card>
  );
}

/** The presentational surface, exported for the SSR snapshot fixture. */
export function PurchaseOrderReportsView({
  filters,
  report,
  loading,
  stale,
  error,
  validation,
  catalogue,
  exporting,
  canExport,
  onPresetChange,
  onPeriodChange,
  onFromChange,
  onToChange,
  onBucketChange,
  onCategoryChange,
  onCurrencyChange,
  onRetry,
  onExport,
  onRefresh,
  refreshing = false,
  monthReadout,
  onPreviousMonth,
  onNextMonth,
  onRetryMonth,
  monthBrowser,
  categoryBrowser,
}: PurchaseOrderReportsViewProps) {
  const reportCurrency = report?.filter.currency ?? filters.currency;
  const entry =
    catalogue?.find((item) => item.code === reportCurrency) ?? null;
  const showEmpty = report !== null && report.totals.poCount === 0;
  const showFigures = report !== null && report.totals.poCount > 0;
  // The report materializes the whole current month/year, including days and
  // months that have not happened. Those are ghost territory, not actual table
  // rows: the table stops at today and the chart carries the projection. This
  // fallback period table is used only by direct fixture/harness callers that
  // do not supply an independent month section.
  const asOfMonth = report === null ? "" : `${report.asOf.slice(0, 7)}-01`;
  const displayedPeriods =
    report === null
      ? []
      : filters.period === "month"
        ? report.periods.filter((period) => period.periodStart <= report.asOf)
        : filters.period === "year"
          ? report.periods.filter((period) => period.periodStart <= asOfMonth)
          : report.periods;
  // Issue #644 F5: the embedded chart's "Selected month" anchor always names
  // #report-period-table, but the section owns that id only on its ready
  // branch. A stable wrapper carries the id (and the fragment focus/scroll
  // contract) while the selected month is loading or errored, so a fetch or
  // month switch can never leave the anchor dead. The ready branch keeps the
  // table's own id, so the document never has two.
  const monthSectionReady = monthReadout?.status === "ready";
  const monthTable =
    monthReadout === undefined || monthReadout === null ? null : (
      <div
        id={monthSectionReady ? undefined : "report-period-table"}
        tabIndex={monthSectionReady ? undefined : -1}
        className={
          monthSectionReady ? "min-w-0" : "min-w-0 scroll-mt-6"
        }
      >
        <PurchaseOrderReportMonthTable
          readout={monthReadout}
          entry={entry}
          onPrevious={onPreviousMonth ?? (() => undefined)}
          onNext={onNextMonth ?? (() => undefined)}
          onRetry={onRetryMonth ?? onRetry}
        />
      </div>
    );
  // The #642 browser anchors its own table in the empty state, the single
  // card, the first grid card or the aggregate. Its loading and error cards
  // render no table, so the same stable wrapper carries the anchor there.
  const monthBrowserPresent = monthBrowser !== undefined && monthBrowser !== null;
  const monthBrowserOwnsAnchor =
    monthBrowserPresent &&
    (monthBrowser.months.length === 0 ||
      (monthBrowser.months.length === 1
        ? monthBrowser.readouts[0]?.status === "ready"
        : monthBrowser.allYear !== null &&
            monthBrowser.layout === "aggregate"
          ? monthBrowser.aggregate?.status === "ready"
          : monthBrowser.readouts[0]?.status === "ready"));
  const monthBrowserSection =
    monthBrowser === undefined || monthBrowser === null ? null : (
      <div
        id={monthBrowserOwnsAnchor ? undefined : "report-period-table"}
        tabIndex={monthBrowserOwnsAnchor ? undefined : -1}
        className={monthBrowserOwnsAnchor ? "min-w-0" : "min-w-0 scroll-mt-6"}
      >
        <PurchaseOrderReportMonthBrowser {...monthBrowser} entry={entry} />
      </div>
    );
  const monthSection = monthBrowserSection ?? monthTable;
  // #643: the category browser is its own section. It renders inside the
  // figures branch and is deliberately outside Performance's empty gate, so a
  // restricted overview with no orders still shows the all-category evidence.
  // Direct fixture/harness callers that pass no categoryBrowser keep the
  // pre-#643 CategoryTable exactly as it was.
  const categorySection =
    categoryBrowser === undefined || categoryBrowser === null ? null : (
      <PurchaseOrderReportCategoryBrowser
        {...categoryBrowser}
        entry={entry}
      />
    );

  return (
    <section aria-labelledby="reports-heading" className="flex flex-col gap-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1
            id="reports-heading"
            className="text-display font-semibold text-ink-950"
          >
            Reports
          </h1>
          <p className="mt-1.5 max-w-3xl text-body text-ink-500">
            Purchase order performance by category and period, in one currency.
            Internal use.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {onRefresh === undefined ? null : (
            <Button
              aria-label="Refresh report"
              data-report-refresh=""
              disabled={refreshing}
              title="Re-read the report scopes this screen is showing"
              onClick={onRefresh}
            >
              {refreshing ? "Refreshing…" : "Refresh"}
            </Button>
          )}
          <Button
            aria-label="Export PDF"
            disabled={!canExport || exporting}
            title={
              canExport
                ? "Save this visible report as a PDF"
                : "A valid, current report is required before export."
            }
            onClick={onExport}
          >
            {exporting ? "Exporting…" : "Export PDF"}
          </Button>
        </div>
      </header>

      {/*
        Root-owned notices keep a stable slot so a refresh error appearing or
        clearing cannot remount the Overview or its chart.
      */}
      <div
        className={error === "" ? "hidden" : ""}
        data-report-root-error=""
      >
        {error !== "" && report === null ? (
          <ErrorCard error={error} onRetry={onRetry} />
        ) : error !== "" ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <StatusBanner tone="danger">
              The report could not be refreshed: {error} Showing the previous
              figures.
            </StatusBanner>
            <Button aria-label="Retry report" onClick={onRetry}>
              Retry
            </Button>
          </div>
        ) : null}
      </div>

      <PurchaseOrderReportOverview
        filters={filters}
        report={report}
        catalogue={catalogue}
        loading={loading}
        stale={stale}
        validation={validation}
        onPresetChange={onPresetChange}
        onPeriodChange={onPeriodChange}
        onFromChange={onFromChange}
        onToChange={onToChange}
        onBucketChange={onBucketChange}
        onCategoryChange={onCategoryChange}
        onCurrencyChange={onCurrencyChange}
      />

      {showFigures && report !== null ? (
        <>
          {monthSection ?? (
            <PeriodTable
              report={report}
              entry={entry}
              periods={displayedPeriods}
            />
          )}
          {categorySection ?? <CategoryTable report={report} entry={entry} />}
          {report.notes.length > 0 ? (
            <Card className="min-w-0 p-5">
              <SectionTitle title="Notes" />
              <ul className="grid gap-2 text-small leading-relaxed text-ink-600">
                {report.notes.map((note) => (
                  <li key={note.code}>{note.text}</li>
                ))}
              </ul>
            </Card>
          ) : null}
        </>
      ) : null}

      {showEmpty && report !== null ? (
        <>
          {monthSection}
          {categorySection}
        </>
      ) : null}
    </section>
  );
}

/**
 * The live page. Performance, By period and By category share one bounded
 * source registry (#645): identical scopes reuse a cached snapshot, identical
 * in-flight reads are deduplicated, and a hidden page fetches nothing. The
 * page keeps the last successful overview visible while a new one loads; a
 * response from a superseded generation or scope is ignored.
 */
export function PurchaseOrderReportsPage({
  defaultCurrency,
  currencyCatalogue,
  onExport,
  active: activeProp = true,
  mutationEpoch = 0,
}: PurchaseOrderReportsPageProps) {
  const { pushToast } = useToast();
  const active = activeProp;
  const [filters, setFilters] = useState<ReportFilterState>(() =>
    initialReportFilters(todayWIB(), defaultCurrency)
  );
  const [exporting, setExporting] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerYear, setPickerYear] = useState("");
  const [categoryPickerOpen, setCategoryPickerOpen] = useState(false);
  const pickerTriggerRef = useRef<HTMLButtonElement | null>(null);
  const categoryTriggerRef = useRef<HTMLButtonElement | null>(null);

  const validation = reportFilterValidation(filters);
  const query = useMemo<PurchaseOrderReportQuery | null>(
    () => (validation === "" ? toReportQuery(filters) : null),
    [
      filters.from,
      filters.to,
      filters.bucket,
      filters.category,
      filters.currency,
      validation,
    ]
  );
  const queryKey = query === null ? null : reportFilterKey(query);

  // The registry lives with this authenticated page mount. Reports unmounts on
  // sign-out (App keys it by user id), so no source survives a different
  // session. One registry serves Performance and both comparisons, which is
  // what lets a category selection reuse the overview's own all-category
  // snapshot instead of issuing a second read.
  const sourceRegistry = usePOReportSourceRegistry({ active });
  const overview = usePOReportOverviewSource(query, sourceRegistry, active);
  const report = overview.report;
  const monthComparison = usePOReportMonthComparison(report, sourceRegistry, {
    active,
  });
  const categoryComparison = usePOReportCategoryComparison(
    report,
    sourceRegistry,
    { active }
  );
  const { invalidateAll, pinExport, pinVisible } = sourceRegistry;

  // Visible pinning: the scopes this screen is showing cannot be evicted by
  // the bounded cache while they are on screen (Q645-7).
  const monthSourceKeys = useMemo(() => {
    if (report === null) return [];
    const keys = new Set<string>();
    for (const month of monthComparison.months) {
      if (overviewCanSupplyCurrentMonth(report, month)) continue;
      if (overviewIsAnnualForMonth(report, month)) continue;
      const request = annualMonthRequest(
        month,
        report.filter.category,
        report.filter.currency,
        report.asOf
      );
      if (request === null) continue;
      keys.add(reportFilterKey(request.query));
    }
    return Array.from(keys);
  }, [monthComparison.months, report]);
  const categorySourceKey = useMemo(() => {
    if (report === null) return null;
    const request = categorySourceRequest(report);
    return reportFilterKey(request.query);
  }, [report]);

  // Issue #645: a month (or the annual aggregate) whose figures are the
  // displayed overview's own snapshot is not a separate registry source. While
  // that overview is stale or re-reading, the card's retained figures are
  // stale too, and the surface must say so rather than wait for a source it
  // does not own. The export gates below stay the hook's stricter ones.
  const viewReadouts = useMemo(() => {
    if (report === null) return monthComparison.readouts;
    return monthComparison.readouts.map((readout) => {
      if (readout.month === "" || readout.stale === true) return readout;
      const fromOverview =
        overviewCanSupplyCurrentMonth(report, readout.month) ||
        overviewIsAnnualForMonth(report, readout.month);
      if (!fromOverview) return readout;
      return { ...readout, stale: overview.stale, refreshing: overview.loading };
    });
  }, [
    monthComparison.readouts,
    overview.loading,
    overview.stale,
    report,
  ]);
  const viewAggregate = useMemo(() => {
    const aggregate = monthComparison.aggregate;
    if (aggregate === null || report === null || aggregate.stale === true) {
      return aggregate;
    }
    // The annual aggregate is its own registry source; only an exact annual
    // overview owns it. A current-month/day overview owning January (the
    // aggregate's first month) says nothing about the other eleven months
    // (NB1), so it must not label the aggregate stale.
    const fromOverview = overviewIsAnnualForMonth(
      report,
      `${aggregate.year}-01`
    );
    if (!fromOverview) return aggregate;
    return { ...aggregate, stale: overview.stale, refreshing: overview.loading };
  }, [
    monthComparison.aggregate,
    overview.loading,
    overview.stale,
    report,
  ]);

  useEffect(() => {
    pinVisible("overview", queryKey === null ? [] : [queryKey]);
  }, [pinVisible, queryKey]);
  useEffect(() => {
    pinVisible("months", monthSourceKeys);
  }, [monthSourceKeys, pinVisible]);
  useEffect(() => {
    pinVisible(
      "categories",
      categorySourceKey === null ? [] : [categorySourceKey]
    );
  }, [categorySourceKey, pinVisible]);

  const openPicker = useCallback(() => {
    setCategoryPickerOpen(false);
    setPickerYear(monthComparison.pickerStartYear());
    setPickerOpen(true);
  }, [monthComparison.pickerStartYear]);

  const openCategoryPicker = useCallback(() => {
    // Only one picker modal may stand at a time. Inertness already prevents
    // opening the second while the first is up; this keeps the invariant even
    // if both handlers are ever called in the same tick.
    setPickerOpen(false);
    setCategoryPickerOpen(true);
  }, []);

  const shiftPickerYear = useCallback((delta: number) => {
    setPickerYear((current) => {
      const year = Number(current);
      if (!Number.isInteger(year) || year < 1) return current;
      const next = year + delta;
      if (next < 1 || next > 9998) return current;
      return String(next);
    });
  }, []);

  const pickerItems = useMemo(
    () =>
      allMonthsOfYear(pickerYear).map((month) => ({
        key: month,
        label: monthLabel(month),
      })),
    [pickerYear]
  );

  // Category options come strictly from the ready all-category snapshot in its
  // supplied order; the picker never invents a value from the static filter map.
  const categoryPickerItems = useMemo(
    () =>
      categoryComparison.readout.supplied.map((item) => ({
        key: item.code,
        label: item.label,
      })),
    [categoryComparison.readout.supplied]
  );

  const changeFilters = useCallback((patch: Partial<ReportFilterState>) => {
    setFilters((current) => ({ ...current, ...patch }));
  }, []);

  const changePreset = useCallback((preset: ReportPreset) => {
    setFilters((current) => {
      if (preset === "custom") return { ...current, preset, period: null };
      return {
        ...current,
        preset,
        period: null,
        ...reportPresetRange(preset, todayWIB()),
      };
    });
  }, []);

  const changePeriod = useCallback((period: ReportPeriodMode | null) => {
    setFilters((current) =>
      period === null
        ? { ...current, period: null, preset: "custom" }
        : {
            ...current,
            period,
            preset: "custom",
            ...reportPeriodRange(period, todayWIB()),
            bucket: reportPeriodBucket(period),
          }
    );
  }, []);

  // Issue #645 Q645-5: one WIB date observer drives the midnight timeout, the
  // focus/visibility check and the hidden → active return. Only a changed WIB
  // date rebases the relative controls; explicit dates, month selections and
  // category selections are never rewritten.
  const observedWIBDate = useRef(todayWIB());
  const previousActive = useRef(active);
  const hasBeenActive = useRef(active);
  const invalidatedWhileHidden = useRef(false);
  const observedMutationEpoch = useRef(mutationEpoch);

  const rebaseRelativeFilters = useCallback((today: string) => {
    setFilters((current) => {
      if (current.period !== null) {
        return {
          ...current,
          ...reportPeriodRange(current.period, today),
          bucket: reportPeriodBucket(current.period),
        };
      }
      if (current.preset !== "custom") {
        return { ...current, ...reportPresetRange(current.preset, today) };
      }
      // A custom range is the user's own dates. Midnight never edits them.
      return current;
    });
  }, []);

  /** Returns true when the WIB date moved since the last observation. */
  const observeWIBDate = useCallback((): boolean => {
    const today = todayWIB();
    if (today === observedWIBDate.current) return false;
    observedWIBDate.current = today;
    rebaseRelativeFilters(today);
    return true;
  }, [rebaseRelativeFilters]);

  // A relevant mutation invalidates every cached evidence but never fetches:
  // the ensure effects revalidate only the currently required scopes while
  // active, and a hidden page waits for its return.
  useEffect(() => {
    if (mutationEpoch === observedMutationEpoch.current) return;
    observedMutationEpoch.current = mutationEpoch;
    invalidateAll();
    if (!active) invalidatedWhileHidden.current = true;
  }, [active, invalidateAll, mutationEpoch]);

  // One revalidation pass per false → true return, deduplicated with any
  // invalidation a hidden mutation already performed. A page mounted hidden
  // loads once on its first activation; the registry has nothing to refresh
  // yet, so it must not invalidate the read it just started.
  useEffect(() => {
    const wasActive = previousActive.current;
    previousActive.current = active;
    if (!active || wasActive) return;
    observeWIBDate();
    const firstActivation = !hasBeenActive.current;
    hasBeenActive.current = true;
    const alreadyInvalidated = invalidatedWhileHidden.current;
    invalidatedWhileHidden.current = false;
    if (firstActivation || alreadyInvalidated) return;
    invalidateAll();
  }, [active, invalidateAll, observeWIBDate]);

  // A single next-WIB-midnight timeout while active, never a polling interval.
  useEffect(() => {
    if (!active) return;
    let timer = 0;
    const arm = () => {
      timer = window.setTimeout(() => {
        if (observeWIBDate()) invalidateAll();
        arm();
      }, msUntilNextWIBMidnight());
    };
    arm();
    return () => window.clearTimeout(timer);
  }, [active, invalidateAll, observeWIBDate]);

  // Laptop sleep and a restored window do not fire the timeout; focus and a
  // visible tab compare the WIB date themselves.
  useEffect(() => {
    if (!active) return;
    const check = () => {
      if (observeWIBDate()) invalidateAll();
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") check();
    };
    window.addEventListener("focus", check);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      window.removeEventListener("focus", check);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [active, invalidateAll, observeWIBDate]);

  const refreshing =
    overview.loading ||
    monthComparison.readouts.some((readout) => readout.refreshing === true) ||
    monthComparison.aggregate?.refreshing === true ||
    categoryComparison.readout.refreshing === true;

  const canExport =
    onExport !== undefined &&
    report !== null &&
    !overview.loading &&
    !overview.stale &&
    overview.error === "" &&
    !exporting &&
    monthComparison.canExport &&
    categoryComparison.canExport;

  const refresh = useCallback(() => {
    invalidateAll();
  }, [invalidateAll]);

  const handleExport = useCallback(() => {
    // Defense in depth: the button is disabled while `canExport` is false, but
    // an explicit empty month/category selection can keep both selection
    // objects resolvable while the overview is stale or re-reading. Refuse at
    // the handler too, so a programmatic call can never export old evidence.
    if (!onExport || !canExport || report === null || exporting) return;
    const monthSelection = monthComparison.exportSelection;
    const categorySelection = categoryComparison.exportSelection;
    if (monthSelection === null || categorySelection === null) return;
    // Freeze the whole v1 envelope before the binding call. A later month,
    // category, layout, filter or Refresh cannot alter the document already
    // being saved; its sources stay pinned until the save settles.
    let view: PurchaseOrderReportView;
    try {
      view = buildPurchaseOrderReportView(report, {
        ...monthSelection,
        categories: categorySelection,
      });
    } catch (caught) {
      pushToast(
        "error",
        errorMessage(caught, "The selected comparison cannot be exported.")
      );
      return;
    }
    const releasePin = pinExport(
      view.sources.map((source) => reportFilterKey(source.filter))
    );
    setExporting(true);
    void (async () => {
      try {
        const path = await onExport(view);
        if (path.trim() !== "") {
          pushToast("success", `Report saved to ${path}`);
        }
      } catch (caught) {
        pushToast(
          "error",
          errorMessage(
            caught,
            "The PDF could not be saved. Choose another location and try again."
          )
        );
      } finally {
        releasePin();
        setExporting(false);
      }
    })();
  }, [
    canExport,
    categoryComparison.exportSelection,
    exporting,
    monthComparison.exportSelection,
    onExport,
    pinExport,
    pushToast,
    report,
  ]);

  return (
    <>
      <PurchaseOrderReportsView
        filters={filters}
        report={report}
        loading={overview.loading}
        stale={overview.stale}
        error={overview.error}
        validation={validation}
        catalogue={currencyCatalogue}
        exporting={exporting}
        canExport={canExport}
        onPresetChange={changePreset}
        onPeriodChange={changePeriod}
        onFromChange={(value) =>
          changeFilters({ preset: "custom", period: null, from: value })
        }
        onToChange={(value) =>
          changeFilters({ preset: "custom", period: null, to: value })
        }
        onBucketChange={(value) =>
          changeFilters({ period: null, bucket: value })
        }
        onCategoryChange={(value) => changeFilters({ category: value })}
        onCurrencyChange={(value) => changeFilters({ currency: value })}
        onRetry={overview.retry}
        onRefresh={refresh}
        refreshing={refreshing}
        onExport={handleExport}
        monthBrowser={{
          months: monthComparison.months,
          activeMonth: monthComparison.activeMonth,
          allYear: monthComparison.allYear,
          layout: monthComparison.layout,
          notice: monthComparison.notice,
          readouts: viewReadouts,
          aggregate: viewAggregate,
          triggerRef: pickerTriggerRef,
          onOpenPicker: openPicker,
          onLayoutChange: monthComparison.setLayout,
          onPrevious: monthComparison.selectPrevious,
          onNext: monthComparison.selectNext,
          onRetryMonth: monthComparison.retryMonth,
        }}
        categoryBrowser={{
          codes: categoryComparison.codes,
          activeCode: categoryComparison.activeCode,
          allSelected: categoryComparison.allSelected,
          layout: categoryComparison.layout,
          readout: categoryComparison.readout,
          triggerRef: categoryTriggerRef,
          onOpenPicker: openCategoryPicker,
          onLayoutChange: categoryComparison.setLayout,
          onPrevious: categoryComparison.selectPrevious,
          onNext: categoryComparison.selectNext,
          onRetry: categoryComparison.retry,
        }}
      />
      <PurchaseOrderReportPicker
        title={
          monthComparison.notice === ""
            ? "Choose months"
            : `Choose months — ${monthComparison.notice}`
        }
        open={pickerOpen}
        items={pickerItems}
        selectedKeys={monthComparison.months}
        allLabel={`All months in ${pickerYear}`}
        allSelected={
          pickerYear !== "" && monthComparison.allYear === pickerYear
        }
        onSelectAll={() => monthComparison.selectAll(pickerYear)}
        onToggle={monthComparison.toggleMonth}
        onClose={() => setPickerOpen(false)}
        returnFocusRef={pickerTriggerRef}
        pageLabel={pickerYear}
        onPreviousPage={() => shiftPickerYear(-1)}
        onNextPage={() => shiftPickerYear(1)}
      />
      <PurchaseOrderReportPicker
        title="Choose categories"
        open={categoryPickerOpen}
        items={categoryPickerItems}
        selectedKeys={categoryComparison.codes}
        allLabel="All categories"
        allSelected={categoryComparison.allSelected}
        onSelectAll={categoryComparison.selectAll}
        onToggle={categoryComparison.toggleCategory}
        onClose={() => setCategoryPickerOpen(false)}
        returnFocusRef={categoryTriggerRef}
      />
    </>
  );
}
