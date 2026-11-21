// Issue #644: the cohesive upper Reports workspace.
//
// This is a pure view. It owns the report-wide controls, the actual KPI strip,
// the exact report's cost-coverage disclosure and the embedded Performance
// chart. The page keeps its heading, export, retry/error banners, notes and the
// independent detail tables. No figure is computed here: every value comes from
// the normalized report, and the chart receives that exact object reference so
// disclosure or unrelated parent state cannot remount or rebuild it.

import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import type { PurchaseOrderReportData } from "../poReport";
import {
  REPORT_BUCKET_OPTIONS,
  REPORT_KPI_GRID_CLASS,
  REPORT_KPI_VALUE_CLASS,
  REPORT_PRESET_OPTIONS,
  categoryFilterOptions,
  formatReportBps,
  formatReportCount,
  formatReportDate,
  reportCategoryLabel,
  reportCurrencyOptions,
  reportRangeLabel,
  type ReportBucket,
  type ReportFilterState,
  type ReportPeriodMode,
  type ReportPreset,
} from "../poReportPage";
import { PurchaseOrderReportChart } from "./PurchaseOrderReportChart";
import { PurchaseOrderReportPeriodToggle } from "./PurchaseOrderReportPeriod";
import { Select } from "./Select";
import { Badge, Card, StatusBanner, fieldClass } from "./ui";

export interface PurchaseOrderReportOverviewProps {
  filters: ReportFilterState;
  report: PurchaseOrderReportData | null;
  catalogue: readonly CurrencyInfo[] | null;
  loading: boolean;
  stale: boolean;
  validation: string;
  onPresetChange: (preset: ReportPreset) => void;
  onPeriodChange: (period: ReportPeriodMode | null) => void;
  onFromChange: (value: string) => void;
  onToChange: (value: string) => void;
  onBucketChange: (value: ReportBucket) => void;
  onCategoryChange: (value: string) => void;
  onCurrencyChange: (value: string) => void;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

/**
 * The period the *displayed* snapshot describes. When a filter change is still
 * in flight, the controls name the requested range but the KPIs and chart must
 * keep naming the previous snapshot they are actually showing.
 */
function displayedPeriod(
  report: PurchaseOrderReportData,
  filters: ReportFilterState,
  stale: boolean
): ReportPeriodMode | null {
  if (!stale) return filters.period;
  if (report.currentProjection.mode === "day") return "month";
  if (report.currentProjection.mode === "month") return "year";
  return null;
}

export function PurchaseOrderReportOverview({
  filters,
  report,
  catalogue,
  loading,
  stale,
  validation,
  onPresetChange,
  onPeriodChange,
  onFromChange,
  onToChange,
  onBucketChange,
  onCategoryChange,
  onCurrencyChange,
}: PurchaseOrderReportOverviewProps) {
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
  const showFigures = report !== null && report.totals.poCount > 0;
  const showEmpty = report !== null && report.totals.poCount === 0;
  const showSkeleton = report === null && loading;
  const unresolvedCurrency = report !== null && entry === null;
  const reportPeriod =
    report === null ? filters.period : displayedPeriod(report, filters, stale);
  const periodContext =
    reportPeriod === "month"
      ? "Current month to date"
      : reportPeriod === "year"
        ? "Current year to date"
        : "Selected range";
  const totals = report?.totals;

  const kpis =
    totals === undefined
      ? []
      : [
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
    <Card className="min-w-0" data-report-overview="">
      <div
        className="flex flex-col gap-4 p-5"
        data-report-overview-controls=""
      >
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-title font-semibold text-ink-950">
              Performance
            </h2>
            <p
              data-report-overview-scope=""
              className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-small text-ink-500"
            >
              {report === null ? (
                <span>Purchase order performance and actual figures</span>
              ) : (
                <>
                  <span className="tnum">{reportRangeLabel(report.filter)}</span>
                  <span aria-hidden="true">·</span>
                  <span>{reportCategoryLabel(report.filter.category)}</span>
                  <span aria-hidden="true">·</span>
                  <span className="tnum">{report.filter.currency}</span>
                  <span aria-hidden="true">·</span>
                  <span className="tnum">
                    As of {formatReportDate(report.asOf)}
                  </span>
                </>
              )}
            </p>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2">
            {stale && report !== null ? (
              <span
                data-report-overview-stale=""
                className="inline-flex items-center"
              >
                <Badge tone="neutral">Previous figures</Badge>
              </span>
            ) : null}
            <PurchaseOrderReportPeriodToggle
              period={filters.period}
              onPeriodChange={onPeriodChange}
            />
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          {filters.period === null ? (
            <>
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
                <span className="text-small font-medium text-ink-700">
                  Bucket
                </span>
                <Select
                  ariaLabel="Bucket"
                  value={filters.bucket}
                  options={REPORT_BUCKET_OPTIONS}
                  onChange={(value) => onBucketChange(value as ReportBucket)}
                />
              </label>
            </>
          ) : (
            <div
              data-report-current-period=""
              className="rounded-md border border-rule bg-paper-subtle/60 px-3 py-2 sm:col-span-2 xl:col-span-3"
            >
              <span className="text-small font-semibold text-ink-900">
                {filters.period === "month" ? "Current month" : "Current year"}
              </span>
              <span className="ml-2 text-small text-ink-600">
                {formatReportDate(filters.from)} to {formatReportDate(filters.to)}
                {" · "}
                {filters.period === "month" ? "daily" : "monthly"}
              </span>
            </div>
          )}
          <label className="grid gap-1.5">
            <span className="text-small font-medium text-ink-700">Category</span>
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
                {filters.currency || "-"}
              </span>
            </div>
          )}
        </div>
        {validation !== "" ? (
          <StatusBanner tone="danger">{validation}</StatusBanner>
        ) : null}
        {loading && report !== null ? (
          <p
            role="status"
            data-report-overview-updating=""
            className="text-small text-ink-500"
          >
            Updating the report…
          </p>
        ) : null}
      </div>

      {showFigures && report !== null && totals !== undefined ? (
        <div className="border-t border-rule" data-report-overview-actual="">
          <div className="px-5 pb-5 pt-4">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <p
                data-report-period-context={reportPeriod ?? "custom"}
                className="text-caption font-medium uppercase tracking-[0.12em] text-ink-500"
              >
                {periodContext}
              </p>
              <span className="text-caption text-ink-500">Actual totals</span>
            </div>
            <dl aria-label="Report KPIs" className={`${REPORT_KPI_GRID_CLASS} mt-3`}>
              {kpis.map((kpi) => (
                <div key={kpi.label}>
                  <dt className="text-caption font-medium uppercase tracking-[0.12em] text-ink-500">
                    {kpi.label}
                  </dt>
                  <dd
                    className={`${REPORT_KPI_VALUE_CLASS} ${
                      kpi.negative
                        ? "text-danger-700"
                        : "text-ink-950"
                    }`}
                  >
                    {kpi.value}
                  </dd>
                </div>
              ))}
            </dl>
            {totals.uncostedMinor > 0 ? (
              <p
                data-report-coverage=""
                className="mt-4 border-t border-rule pt-3 text-small leading-relaxed text-ink-600"
              >
                Coverage: {formatReportBps(totals.coverageBps)} of the amount is
                costed; {money(entry, totals.uncostedMinor)} of{" "}
                {money(entry, totals.amountMinor)} is uncosted.
              </p>
            ) : null}
            {unresolvedCurrency ? (
              <p
                data-report-overview-currency=""
                className="mt-2 text-small leading-relaxed text-ink-600"
              >
                {reportCurrency} is not in this app's currency catalogue, so
                monetary figures are shown as “-” and no other currency is
                substituted.
              </p>
            ) : null}
          </div>
        </div>
      ) : null}

      {showEmpty && report !== null ? (
        <div
          className="border-t border-rule px-5 py-6"
          data-report-overview-empty=""
        >
          <h3 className="text-title font-semibold text-ink-950">
            No purchase orders in this range
          </h3>
          <p className="mt-1.5 max-w-2xl text-body leading-relaxed text-ink-500">
            {reportRangeLabel(report.filter)} ·{" "}
            {reportCategoryLabel(report.filter.category)} ·{" "}
            {report.filter.currency}. Choose a wider date range or a different
            category.
          </p>
        </div>
      ) : null}

      {showSkeleton ? (
        <div
          className="border-t border-rule px-5 py-5"
          role="status"
          aria-label="Loading report"
          data-report-overview-loading=""
        >
          <div className="h-32 animate-pulse rounded-xl border border-rule bg-paper-muted motion-reduce:animate-none" />
          <div className="mt-4 h-64 animate-pulse rounded-xl border border-rule bg-paper-muted motion-reduce:animate-none" />
        </div>
      ) : null}

      {showFigures && report !== null ? (
        <div
          className="border-t border-rule px-5 pb-5 pt-4"
          data-report-overview-chart=""
        >
          <PurchaseOrderReportChart
            report={report}
            currency={entry}
            period={reportPeriod}
            embedded
          />
        </div>
      ) : null}
    </Card>
  );
}
