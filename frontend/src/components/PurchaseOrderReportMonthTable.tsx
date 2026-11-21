// Issue #641: one selected month, six vertical metrics. The actual,
// future-dated recorded and estimated quantities are separate blocks; the
// estimate alone gets the muted/hatched treatment and an accessible label, so
// a reader never mistakes it for recorded evidence. The server owns every
// figure and every percentage.

import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import {
  monthLabel,
  monthUnavailableLabel,
  type PurchaseOrderMonthEstimate,
  type PurchaseOrderMonthReadout,
} from "../poReportComparison";
import {
  formatReportBps,
  formatReportCount,
  formatReportDate,
  projectionSourceLabel,
  reportCategoryLabel,
} from "../poReportPage";
import type { PurchaseOrderReportMetrics } from "../poReport";
import { Button, Card, SectionHeading, StatusBanner } from "./ui";

export interface PurchaseOrderReportMonthTableProps {
  readout: PurchaseOrderMonthReadout;
  entry: CurrencyInfo | null;
  onPrevious: () => void;
  onNext: () => void;
  onRetry: () => void;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

function marginClass(value: number): string {
  return value < 0 ? "text-danger-700" : "text-ink-900";
}

function EvidenceGrid({
  metrics,
  entry,
  testId,
}: {
  metrics: PurchaseOrderReportMetrics;
  entry: CurrencyInfo | null;
  testId: string;
}) {
  const values: { label: string; value: string; negative?: boolean }[] = [
    { label: "POs", value: formatReportCount(metrics.poCount) },
    { label: "PO amount", value: money(entry, metrics.amountMinor) },
    { label: "Costed amount", value: money(entry, metrics.costedMinor) },
    { label: "Capital", value: money(entry, metrics.capitalMinor) },
    {
      label: "PO margin",
      value: money(entry, metrics.marginMinor),
      negative: metrics.marginMinor < 0,
    },
    { label: "PO margin %", value: formatReportBps(metrics.marginBps) },
  ];
  return (
    <dl
      data-month-evidence={testId}
      className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2 lg:grid-cols-3"
    >
      {values.map((value) => (
        <div
          key={value.label}
          data-month-recorded-metric={value.label}
          className="min-w-0"
        >
          <dt className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
            {value.label}
          </dt>
          <dd
            className={`tnum mt-0.5 whitespace-nowrap text-body ${
              value.negative ? "text-danger-700" : "text-ink-900"
            }`}
          >
            {value.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function EstimateBlock({
  estimate,
  entry,
}: {
  estimate: PurchaseOrderMonthEstimate;
  entry: CurrencyInfo | null;
}) {
  const label =
    estimate.kind === "remainder"
      ? "Estimated remainder (not actual)"
      : "Estimated forecast (not actual)";
  const values: {
    label: string;
    value?: string;
    amount?: { base: string; low: string; high: string };
    negative?: boolean;
  }[] = [
    { label: "POs", value: "-" },
    {
      label: "PO amount",
      amount: {
        base: money(entry, estimate.base),
        low: money(entry, estimate.low),
        high: money(entry, estimate.high),
      },
    },
    { label: "Costed amount", value: "-" },
    { label: "Capital", value: money(entry, estimate.capital) },
    {
      label: "PO margin",
      value: money(entry, estimate.margin),
      negative: estimate.margin < 0,
    },
    { label: "PO margin %", value: formatReportBps(estimate.marginBps) },
  ];
  return (
    <div
      role="group"
      data-month-evidence="estimated"
      data-estimated="true"
      aria-label={`${label}. Estimates are not actual evidence.`}
      className="rounded-md border border-dashed border-rule bg-paper-subtle/50 p-3"
    >
      <p className="mb-2 text-small font-semibold text-ink-600">{label}</p>
      <dl
        data-month-estimate-grid=""
        className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2 lg:grid-cols-3"
      >
        {values.map((value) => (
          <div
            key={value.label}
            data-month-estimate-metric={value.label}
            className={`min-w-0 ${
              value.amount === undefined ? "" : "sm:col-span-2 lg:col-span-3"
            }`}
          >
            <dt className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
              {value.label}
            </dt>
            {value.amount === undefined ? (
              <dd
                className={`tnum mt-0.5 whitespace-nowrap text-body ${
                  value.negative ? "text-danger-700" : "text-ink-600"
                }`}
              >
                {value.value}
              </dd>
            ) : (
              <dd
                data-month-estimate-amount=""
                className="mt-0.5 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-body text-ink-600"
              >
                <span
                  data-amount-part="base"
                  className="tnum whitespace-nowrap font-medium text-ink-700"
                >
                  {value.amount.base}
                </span>
                <span className="inline-flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                    Range
                  </span>
                  <span className="inline-flex items-baseline gap-1 whitespace-nowrap">
                    <span className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                      Low
                    </span>
                    <span data-amount-part="low" className="tnum whitespace-nowrap">
                      {value.amount.low}
                    </span>
                  </span>
                  <span className="inline-flex items-baseline gap-1 whitespace-nowrap">
                    <span className="text-caption font-medium uppercase tracking-[0.1em] text-ink-500">
                      High
                    </span>
                    <span data-amount-part="high" className="tnum whitespace-nowrap">
                      {value.amount.high}
                    </span>
                  </span>
                </span>
              </dd>
            )}
          </div>
        ))}
      </dl>
      <p className="mt-2 text-caption leading-relaxed text-ink-500">
        POs and Costed amount are not forecast. Sample {formatReportCount(
          estimate.sampleSize
        )}{" "}
        · {projectionSourceLabel(estimate.source)}
      </p>
    </div>
  );
}

export function PurchaseOrderReportMonthTable({
  readout,
  entry,
  onPrevious,
  onNext,
  onRetry,
}: PurchaseOrderReportMonthTableProps) {
  const heading = readout.month === "" ? "By period" : monthLabel(readout.month);
  const primary = readout.primary;
  const rows: { label: string; value: string; negative?: boolean }[] = [
    { label: "POs", value: primary ? formatReportCount(primary.poCount) : "-" },
    { label: "PO amount", value: primary ? money(entry, primary.amountMinor) : "-" },
    {
      label: "Costed amount",
      value: primary ? money(entry, primary.costedMinor) : "-",
    },
    { label: "Capital", value: primary ? money(entry, primary.capitalMinor) : "-" },
    {
      label: "PO margin",
      value: primary ? money(entry, primary.marginMinor) : "-",
      negative: primary !== null && primary.marginMinor < 0,
    },
    {
      label: "PO margin %",
      value: primary ? formatReportBps(primary.marginBps) : "-",
    },
  ];

  return (
    <Card className="min-w-0" data-month-browser="">
      <div className="flex flex-wrap items-start justify-between gap-3 px-5 pt-5">
        <div className="min-w-0">
          <SectionHeading
            title="By period"
            hint="One calendar month. Navigating it never changes Performance."
          />
          <h3 className="mt-2 text-title font-semibold text-ink-950">
            {heading}
          </h3>
          <p
            data-month-scope=""
            className="mt-1 text-small leading-relaxed text-ink-500"
          >
            {reportCategoryLabel(readout.category)} · {readout.currency || "-"}
            {readout.sourceLabel === "" ? null : ` · ${readout.sourceLabel}`}
            {readout.asOf === ""
              ? null
              : ` · As of ${formatReportDate(readout.asOf)}`}
            {readout.generatedAt === "" ? null : (
              <span className="tnum"> · snapshot {readout.generatedAt}</span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            aria-label="Previous month"
            data-month-nav="previous"
            onClick={onPrevious}
          >
            Previous
          </Button>
          <Button aria-label="Next month" data-month-nav="next" onClick={onNext}>
            Next
          </Button>
        </div>
      </div>

      <div className="px-5 pb-5 pt-3">
        {readout.status === "loading" ? (
          <p role="status" className="text-body text-ink-500">
            Loading {heading}…
          </p>
        ) : null}
        {readout.status === "error" ? (
          <div className="grid gap-3">
            <StatusBanner tone="danger">{readout.error}</StatusBanner>
            <div>
              <Button aria-label="Retry month" onClick={onRetry}>
                Retry
              </Button>
            </div>
          </div>
        ) : null}
        {readout.status === "ready" ? (
          <div className="grid gap-4">
            {readout.scopeMatchesOverview ? null : (
              <StatusBanner tone="info">
                This month's snapshot has a different business date from the
                overview. Export stays off until both are refreshed together.
              </StatusBanner>
            )}
            {readout.message === "" ? null : (
              <p className="text-small leading-relaxed text-ink-500">
                {readout.message}
              </p>
            )}
            <div>
              {readout.primaryLabel === "" ? null : (
                <p
                  data-month-primary-label={readout.primaryLabel}
                  className="mb-2 text-caption font-medium uppercase tracking-[0.1em] text-ink-500"
                >
                  {readout.primaryLabel}
                </p>
              )}
              <table
                id="report-period-table"
                tabIndex={-1}
                aria-label="Period totals"
                className="w-full scroll-mt-6 text-left text-body"
              >
                <thead>
                  <tr className="border-y border-rule bg-paper-subtle/60 text-micro uppercase tracking-[0.1em] text-ink-500">
                    <th className="py-2 pr-4 font-medium">Metric</th>
                    <th className="py-2 text-right font-medium">Value</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-rule">
                  {rows.map((row) => (
                    <tr key={row.label}>
                      <th
                        scope="row"
                        className="py-2.5 pr-4 text-left align-middle font-medium text-ink-700"
                      >
                        {row.label}
                      </th>
                      <td
                        data-month-metric={row.label}
                        className={`tnum whitespace-nowrap py-2.5 text-right align-middle ${
                          row.negative
                            ? "text-danger-700"
                            : "text-ink-900"
                        }`}
                      >
                        {row.value}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {readout.recordedFuture === null ? null : (
              <div data-month-evidence="recorded-future-label">
                <p className="mb-2 text-small font-semibold text-ink-900">
                  Future-dated recorded
                </p>
                <EvidenceGrid
                  metrics={readout.recordedFuture}
                  entry={entry}
                  testId="recorded-future"
                />
              </div>
            )}

            {readout.estimate === null ? (
              readout.unavailableReason === "" ? null : (
                <div data-month-evidence="unavailable">
                  <p className="text-small font-semibold text-ink-700">
                    Forecast unavailable
                  </p>
                  <p className="text-small leading-relaxed text-ink-500">
                    {monthUnavailableLabel(readout.unavailableReason)}
                  </p>
                </div>
              )
            ) : (
              <EstimateBlock estimate={readout.estimate} entry={entry} />
            )}

            <p className="text-caption leading-relaxed text-ink-500">
              Actual, future-dated recorded and estimated figures are separate.
              Estimates are never added to recorded evidence or to the month's
              actual figures.
            </p>
          </div>
        ) : null}
      </div>
    </Card>
  );
}
