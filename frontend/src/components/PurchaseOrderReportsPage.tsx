import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { GetPurchaseOrderReport } from "@backend/backend/Quotation";
import { errorMessage } from "../errors";
import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import {
  REPORT_BUCKET_OPTIONS,
  REPORT_KPI_GRID_CLASS,
  REPORT_KPI_VALUE_CLASS,
  REPORT_PRESET_OPTIONS,
  categoryFilterOptions,
  formatReportBps,
  formatReportCount,
  formatReportDate,
  initialReportFilters,
  projectionReasonLabel,
  projectionSourceLabel,
  reportCurrencyOptions,
  reportCategoryLabel,
  reportFilterKey,
  reportFilterValidation,
  reportMatchesFilter,
  reportPeriodLabel,
  reportPresetRange,
  reportRangeLabel,
  toReportQuery,
  type ReportBucket,
  type ReportFilterState,
  type ReportPreset,
} from "../poReportPage";
import {
  normalizePurchaseOrderReport,
  type PurchaseOrderReportData,
  type PurchaseOrderReportMetrics,
  type PurchaseOrderReportProjectionSeries,
  type PurchaseOrderReportQuery,
} from "../poReport";
import { todayWIB } from "../workspaceSettings";
import { useToast } from "../toast";
import { PurchaseOrderReportChart } from "./PurchaseOrderReportChart";
import { Select } from "./Select";
import { Badge, Button, Card, SectionHeading, StatusBanner, fieldClass } from "./ui";

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
  /** App's catalogue. A code missing here renders "—", never a fake IDR. */
  currencyCatalogue: readonly CurrencyInfo[] | null;
  /**
   * #632 owns the native PDF binding. Until App wires it after #630/#632
   * merge, the export button stays disabled and this page never guesses a
   * file path or success.
   */
  onExport?: (report: PurchaseOrderReportData) => Promise<string>;
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
  onFromChange: (value: string) => void;
  onToChange: (value: string) => void;
  onBucketChange: (value: ReportBucket) => void;
  onCategoryChange: (value: string) => void;
  onCurrencyChange: (value: string) => void;
  onRetry: () => void;
  onExport: () => void;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "—" : formatMoney(entry, minor);
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
      <td className="tnum whitespace-nowrap py-3 text-right align-middle text-ink-700">
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
    <Card className="overflow-hidden">
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
}: {
  report: PurchaseOrderReportData;
  entry: CurrencyInfo | null;
}) {
  return (
    <Card className="overflow-hidden">
      <div className="px-5 pt-5">
        <SectionTitle title="By period" />
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
            {report.periods.map((row) => (
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

function ProjectionTable({
  title,
  series,
  bucket,
  entry,
}: {
  title: string;
  series: PurchaseOrderReportProjectionSeries;
  bucket: "month" | "year";
  entry: CurrencyInfo | null;
}) {
  const basis = series.basis;
  return (
    <div className="border-t border-rule pt-5 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="text-lead font-medium text-ink-900">{title}</h3>
        <Badge tone={series.status === "available" ? "accent" : "neutral"}>
          {series.status === "available" ? "Projected" : "Unavailable"}
        </Badge>
      </div>
      <p className="mt-1 max-w-3xl text-small leading-relaxed text-ink-500">
        <span className="font-medium text-ink-700">Method:</span>{" "}
        {series.method}
      </p>
      <p className="mt-1 text-caption leading-relaxed text-ink-500">
        Basis: {basis.completeMonths} complete months ({basis.historyFrom} to{" "}
        {basis.historyTo}, exclusive), {basis.poCount} POs, as of{" "}
        {formatReportDate(basis.asOf)}.
      </p>
      {series.status !== "available" ? (
        <p className="mt-3 rounded-lg border border-rule bg-paper-subtle/60 px-4 py-3 text-small text-ink-600">
          {projectionReasonLabel(series.reason)}
        </p>
      ) : series.buckets.length === 0 ? (
        <p className="mt-3 text-small text-ink-500">
          No projected buckets in this range.
        </p>
      ) : (
        <div className="mt-3 overflow-x-auto scroll-thin">
          <table
            aria-label={`${title} projection`}
            className="w-full min-w-[720px] text-left text-body"
          >
            <thead>
              <tr className="border-b border-rule text-micro uppercase tracking-[0.1em] text-ink-500">
                <th className="py-2 pr-4 font-medium">Period</th>
                <th className="py-2 pr-4 text-right font-medium">Low</th>
                <th className="py-2 pr-4 text-right font-medium">Base</th>
                <th className="py-2 pr-4 text-right font-medium">High</th>
                <th className="py-2 pr-4 text-right font-medium">Capital</th>
                <th className="py-2 pr-4 text-right font-medium">Margin</th>
                <th className="py-2 pr-4 text-right font-medium">Margin %</th>
                <th className="py-2 text-right font-medium">n</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-rule">
              {series.buckets.map((row) => (
                <tr key={row.periodStart}>
                  <td className="tnum py-2 pr-4 align-middle text-ink-700">
                    {reportPeriodLabel(row.periodStart, bucket)}
                  </td>
                  <td className="tnum whitespace-nowrap py-2 pr-4 text-right align-middle text-ink-700">
                    {money(entry, row.amountLowMinor)}
                  </td>
                  <td className="tnum whitespace-nowrap py-2 pr-4 text-right align-middle font-medium text-ink-900">
                    {money(entry, row.amountBaseMinor)}
                  </td>
                  <td className="tnum whitespace-nowrap py-2 pr-4 text-right align-middle text-ink-700">
                    {money(entry, row.amountHighMinor)}
                  </td>
                  <td className="tnum whitespace-nowrap py-2 pr-4 text-right align-middle text-ink-700">
                    {money(entry, row.capitalMinor)}
                  </td>
                  <td
                    className={`tnum whitespace-nowrap py-2 pr-4 text-right align-middle ${negativeClass(
                      row.marginMinor
                    )}`}
                  >
                    {money(entry, row.marginMinor)}
                  </td>
                  <td className="tnum whitespace-nowrap py-2 pr-4 text-right align-middle text-ink-700">
                    {formatReportBps(row.marginBps)}
                  </td>
                  <td
                    className="tnum py-2 text-right align-middle text-ink-500"
                    title={projectionSourceLabel(row.source)}
                  >
                    {formatReportCount(row.sampleSize)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function ProjectionSection({
  report,
  entry,
}: {
  report: PurchaseOrderReportData;
  entry: CurrencyInfo | null;
}) {
  return (
    <Card className="p-5">
      <SectionHeading
        title="Projection"
        hint="Not included in actual totals"
      />
      <p className="mb-5 max-w-3xl text-small leading-relaxed text-ink-500">
        Projected from complete months before the range's end month and the
        report's as-of month. The history window can overlap the selected
        range; projected buckets are never counted in the KPIs or the actual
        tables above.
      </p>
      <div className="grid gap-5">
        <ProjectionTable
          title="Monthly"
          series={report.projection.monthly}
          bucket="month"
          entry={entry}
        />
        <ProjectionTable
          title="Yearly"
          series={report.projection.yearly}
          bucket="year"
          entry={entry}
        />
      </div>
    </Card>
  );
}

function KpiStrip({
  report,
  entry,
}: {
  report: PurchaseOrderReportData;
  entry: CurrencyInfo | null;
}) {
  const totals = report.totals;
  const kpis: { label: string; value: string; negative?: boolean }[] = [
    { label: "PO count", value: formatReportCount(totals.poCount) },
    { label: "PO amount", value: money(entry, totals.amountMinor) },
    { label: "Capital", value: money(entry, totals.capitalMinor) },
    {
      label: "PO margin",
      value: money(entry, totals.marginMinor),
      negative: totals.marginMinor < 0,
    },
    { label: "PO margin %", value: formatReportBps(totals.marginBps) },
  ];
  return (
    <Card className="p-5">
      <dl
        aria-label="Report KPIs"
        className={REPORT_KPI_GRID_CLASS}
      >
        {kpis.map((kpi) => (
          <div key={kpi.label}>
            <dt className="text-caption font-medium uppercase tracking-[0.12em] text-ink-500">
              {kpi.label}
            </dt>
            <dd
              className={`${REPORT_KPI_VALUE_CLASS} ${
                kpi.negative ? "text-danger-700" : "text-ink-950"
              }`}
            >
              {kpi.value}
            </dd>
          </div>
        ))}
      </dl>
      {totals.uncostedMinor > 0 ? (
        <p className="mt-4 border-t border-rule pt-3 text-small leading-relaxed text-ink-600">
          Coverage: {formatReportBps(totals.coverageBps)} of the amount is
          costed; {money(entry, totals.uncostedMinor)} of{" "}
          {money(entry, totals.amountMinor)} is uncosted.
        </p>
      ) : null}
    </Card>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-4" aria-label="Loading report">
      <div className="h-32 animate-pulse rounded-xl border border-rule bg-paper-muted" />
      <div className="h-64 animate-pulse rounded-xl border border-rule bg-paper-muted" />
      <div className="h-64 animate-pulse rounded-xl border border-rule bg-paper-muted" />
    </div>
  );
}

function EmptyReport({ report }: { report: PurchaseOrderReportData }) {
  return (
    <Card className="p-8">
      <h2 className="text-title font-semibold text-ink-950">
        No purchase orders in this range
      </h2>
      <p className="mt-1.5 max-w-2xl text-body leading-relaxed text-ink-500">
        {reportRangeLabel(report.filter)} ·{" "}
        {reportCategoryLabel(report.filter.category)} ·{" "}
        {report.filter.currency}. Choose a wider date range or a different
        category.
      </p>
    </Card>
  );
}

function ErrorCard({ error, onRetry }: { error: string; onRetry: () => void }) {
  return (
    <Card className="p-8">
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
  onFromChange,
  onToChange,
  onBucketChange,
  onCategoryChange,
  onCurrencyChange,
  onRetry,
  onExport,
}: PurchaseOrderReportsViewProps) {
  const reportCurrency = report?.filter.currency ?? filters.currency;
  const entry =
    catalogue?.find((item) => item.code === reportCurrency) ?? null;
  const categoryOptions = categoryFilterOptions(
    report?.population.hasUnassigned ?? false,
    filters.category
  );
  const currencyOptions = reportCurrencyOptions(
    report?.population.availableCurrencies ?? [],
    filters.currency
  );
  const showSkeleton = report === null && error === "" && validation === "";
  const showEmpty = report !== null && report.totals.poCount === 0;
  const showFigures = report !== null && report.totals.poCount > 0;
  const unresolvedCurrency = report !== null && entry === null;

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
          {report !== null ? (
            <span className="tnum text-caption text-ink-500">
              As of {formatReportDate(report.asOf)}
            </span>
          ) : null}
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

      <Card className="p-4">
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Date range
            </span>
            <Select
              ariaLabel="Date range preset"
              value={filters.preset}
              options={REPORT_PRESET_OPTIONS}
              onChange={(value) => onPresetChange(value as ReportPreset)}
            />
          </label>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              From date
            </span>
            <input
              type="date"
              aria-label="From date"
              className={fieldClass}
              value={filters.from}
              onChange={(event) => onFromChange(event.target.value)}
            />
          </label>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              To date
            </span>
            <input
              type="date"
              aria-label="To date"
              className={fieldClass}
              value={filters.to}
              onChange={(event) => onToChange(event.target.value)}
            />
          </label>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">Bucket</span>
            <Select
              ariaLabel="Bucket"
              value={filters.bucket}
              options={REPORT_BUCKET_OPTIONS}
              onChange={(value) => onBucketChange(value as ReportBucket)}
            />
          </label>
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">
              Category
            </span>
            <Select
              ariaLabel="Category"
              value={filters.category}
              options={categoryOptions}
              onChange={(value) => onCategoryChange(String(value))}
            />
          </label>
          {currencyOptions.length > 1 ? (
            <label className="grid gap-1.5">
              <span className="text-small font-medium text-ink-700">
                Currency
              </span>
              <Select
                ariaLabel="Currency"
                value={filters.currency}
                options={currencyOptions.map((code) => ({
                  value: code,
                  label: code,
                }))}
                onChange={(value) => onCurrencyChange(String(value))}
              />
            </label>
          ) : (
            <div className="grid gap-1.5">
              <span className="text-small font-medium text-ink-700">
                Currency
              </span>
              <span className="tnum flex min-h-10 items-center rounded-md border border-rule bg-paper-subtle px-3 text-body text-ink-700">
                {filters.currency || "—"}
              </span>
            </div>
          )}
        </div>
        {validation !== "" ? (
          <div className="mt-3">
            <StatusBanner tone="danger">{validation}</StatusBanner>
          </div>
        ) : null}
      </Card>

      {loading && report !== null ? (
        <p role="status" className="text-small text-ink-500">
          Updating the report…
        </p>
      ) : null}
      {!loading && stale && error === "" && validation === "" ? (
        <StatusBanner tone="info">
          Showing the previous filter's figures while this one has no report.
        </StatusBanner>
      ) : null}
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

      {unresolvedCurrency ? (
        <StatusBanner tone="info">
          {reportCurrency} is not in this app's currency catalogue, so monetary
          figures are shown as “—” and no other currency is substituted.
        </StatusBanner>
      ) : null}

      {showSkeleton ? <LoadingSkeleton /> : null}
      {showEmpty && report !== null ? <EmptyReport report={report} /> : null}
      {showFigures && report !== null ? (
        <>
          <KpiStrip report={report} entry={entry} />
          <PurchaseOrderReportChart report={report} currency={entry} />
          <CategoryTable report={report} entry={entry} />
          <PeriodTable report={report} entry={entry} />
          <ProjectionSection report={report} entry={entry} />
          {report.notes.length > 0 ? (
            <Card className="p-5">
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
    </section>
  );
}

/**
 * The live page. It owns exactly one request per filter change and keeps the
 * last successful snapshot visible while the new one is loading; a response
 * from a superseded request is ignored.
 */
export function PurchaseOrderReportsPage({
  defaultCurrency,
  currencyCatalogue,
  onExport,
}: PurchaseOrderReportsPageProps) {
  const { pushToast } = useToast();
  const [filters, setFilters] = useState<ReportFilterState>(() =>
    initialReportFilters(todayWIB(), defaultCurrency)
  );
  const [report, setReport] = useState<PurchaseOrderReportData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [retryToken, setRetryToken] = useState(0);
  const [exporting, setExporting] = useState(false);

  const validation = reportFilterValidation(filters);
  const query = useMemo<PurchaseOrderReportQuery | null>(
    () => (validation === "" ? toReportQuery(filters) : null),
    [filters.from, filters.to, filters.bucket, filters.category, filters.currency, validation]
  );
  const queryKey = query === null ? null : reportFilterKey(query);
  const requestSeq = useRef(0);
  const inFlightKey = useRef<string | null>(null);

  useEffect(() => {
    if (query === null || queryKey === null) {
      // An invalid range invalidates any late response; it must not repaint a
      // snapshot the controls no longer describe. Clearing the in-flight key
      // also lets a corrected filter re-request the same key.
      requestSeq.current += 1;
      inFlightKey.current = null;
      setLoading(false);
      return;
    }
    const key = `${queryKey}#${retryToken}`;
    if (inFlightKey.current === key) return;
    inFlightKey.current = key;
    const seq = ++requestSeq.current;
    setLoading(true);
    setError("");
    void (async () => {
      try {
        const raw = await GetPurchaseOrderReport(query);
        if (seq !== requestSeq.current) return;
        setReport(normalizePurchaseOrderReport(raw));
      } catch (caught) {
        if (seq !== requestSeq.current) return;
        setError(
          errorMessage(
            caught,
            "The purchase order report could not be loaded. Check the API connection and try again."
          )
        );
      } finally {
        if (inFlightKey.current === key) inFlightKey.current = null;
        if (seq === requestSeq.current) setLoading(false);
      }
    })();
  }, [queryKey, retryToken, query]);

  const changeFilters = useCallback((patch: Partial<ReportFilterState>) => {
    setFilters((current) => ({ ...current, ...patch }));
  }, []);

  const changePreset = useCallback((preset: ReportPreset) => {
    setFilters((current) => {
      if (preset === "custom") return { ...current, preset };
      return { ...current, preset, ...reportPresetRange(preset, todayWIB()) };
    });
  }, []);

  const stale =
    report !== null && (query === null || !reportMatchesFilter(report, query));
  const canExport =
    onExport !== undefined &&
    report !== null &&
    !loading &&
    !stale &&
    !exporting;

  const handleExport = useCallback(() => {
    if (!onExport || report === null || exporting || loading || stale) return;
    const snapshot = report;
    setExporting(true);
    void (async () => {
      try {
        const path = await onExport(snapshot);
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
        setExporting(false);
      }
    })();
  }, [exporting, loading, onExport, pushToast, report, stale]);

  return (
    <PurchaseOrderReportsView
      filters={filters}
      report={report}
      loading={loading}
      stale={stale}
      error={error}
      validation={validation}
      catalogue={currencyCatalogue}
      exporting={exporting}
      canExport={canExport}
      onPresetChange={changePreset}
      onFromChange={(value) => changeFilters({ preset: "custom", from: value })}
      onToChange={(value) => changeFilters({ preset: "custom", to: value })}
      onBucketChange={(value) => changeFilters({ bucket: value })}
      onCategoryChange={(value) => changeFilters({ category: value })}
      onCurrencyChange={(value) => changeFilters({ currency: value })}
      onRetry={() => setRetryToken((current) => current + 1)}
      onExport={handleExport}
    />
  );
}
