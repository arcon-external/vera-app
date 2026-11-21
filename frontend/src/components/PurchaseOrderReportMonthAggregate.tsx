// Issue #642: the All/Aggregate one-calendar-year comparison. It prints the
// annual snapshot's twelve rows in a wide six-metric table and its server-owned
// `totals` as the "Recorded total". Estimates stay separate marked lines; no
// row or estimate is ever summed into that total. Card/Individual layout reuses
// PurchaseOrderReportMonthTable's vertical metric table instead of this table.

import type { CurrencyInfo } from "../format";
import { formatMoney } from "../format";
import {
  monthLabel,
  monthUnavailableLabel,
  type PurchaseOrderMonthAggregate,
  type PurchaseOrderMonthAggregateRow,
} from "../poReportComparison";
import {
  formatReportBps,
  formatReportCount,
  formatReportDate,
  reportCategoryLabel,
} from "../poReportPage";
import type { PurchaseOrderReportMetrics } from "../poReport";
import { Badge, Button, Card, SectionHeading, StatusBanner } from "./ui";

export interface PurchaseOrderReportMonthAggregateProps {
  aggregate: PurchaseOrderMonthAggregate;
  entry: CurrencyInfo | null;
  /** Retries the year's one annual source. */
  onRetry: () => void;
}

function money(entry: CurrencyInfo | null, minor: number): string {
  return entry === null ? "-" : formatMoney(entry, minor);
}

function rowValues(
  row: PurchaseOrderMonthAggregateRow,
  entry: CurrencyInfo | null
): { label: string; value: string; negative?: boolean }[] {
  const metrics = row.metrics;
  return [
    { label: "POs", value: metrics ? formatReportCount(metrics.poCount) : "-" },
    { label: "Amount", value: metrics ? money(entry, metrics.amountMinor) : "-" },
    { label: "Costed", value: metrics ? money(entry, metrics.costedMinor) : "-" },
    { label: "Capital", value: metrics ? money(entry, metrics.capitalMinor) : "-" },
    {
      label: "Margin",
      value: metrics ? money(entry, metrics.marginMinor) : "-",
      negative: metrics !== null && metrics.marginMinor < 0,
    },
    {
      label: "Margin %",
      value: metrics ? formatReportBps(metrics.marginBps) : "-",
    },
  ];
}

function totalCells(
  total: PurchaseOrderReportMetrics,
  entry: CurrencyInfo | null
): { label: string; value: string; negative?: boolean }[] {
  return [
    { label: "POs", value: formatReportCount(total.poCount) },
    { label: "Amount", value: money(entry, total.amountMinor) },
    { label: "Costed", value: money(entry, total.costedMinor) },
    { label: "Capital", value: money(entry, total.capitalMinor) },
    {
      label: "Margin",
      value: money(entry, total.marginMinor),
      negative: total.marginMinor < 0,
    },
    { label: "Margin %", value: formatReportBps(total.marginBps) },
  ];
}

export function PurchaseOrderReportMonthAggregate({
  aggregate,
  entry,
  onRetry,
}: PurchaseOrderReportMonthAggregateProps) {
  const heading = `${aggregate.year} monthly comparison`;
  return (
    <Card className="min-w-0 overflow-hidden" data-month-aggregate="">
      <div className="px-5 pt-5">
        <SectionHeading
          title={heading}
          hint="Recorded months only"
        />
        <p
          data-aggregate-scope=""
          className="text-small leading-relaxed text-ink-500"
        >
          {reportCategoryLabel(aggregate.category)} ·{" "}
          {aggregate.currency || "-"} · {aggregate.sourceLabel}
          {aggregate.asOf === ""
            ? null
            : ` · As of ${formatReportDate(aggregate.asOf)}`}
          {aggregate.generatedAt === "" ? null : (
            <span className="tnum"> · annual snapshot {aggregate.generatedAt}</span>
          )}
        </p>
      </div>

      <div className="px-5 pb-5 pt-3">
        {aggregate.status === "loading" ? (
          <p role="status" className="text-body text-ink-500">
            Loading the {aggregate.year} annual snapshot…
          </p>
        ) : null}
        {aggregate.status === "error" ? (
          <div className="grid gap-3">
            <StatusBanner tone="danger">{aggregate.error}</StatusBanner>
            <div>
              <Button aria-label="Retry month" onClick={onRetry}>
                Retry
              </Button>
            </div>
          </div>
        ) : null}
        {aggregate.status === "ready" ? (
          <div className="grid gap-4">
            {aggregate.stale ? (
              <p className="flex flex-wrap items-center gap-2 text-small leading-relaxed text-ink-500">
                <span
                  data-month-aggregate-stale=""
                  data-month-aggregate-refreshing={
                    aggregate.refreshing ? "true" : "false"
                  }
                  className="inline-flex items-center"
                >
                  <Badge tone="neutral">
                    {aggregate.refreshing ? "Updating…" : "Stale"}
                  </Badge>
                </span>
                <span>
                  Showing the previous {aggregate.year} annual snapshot until
                  the new one arrives.
                </span>
              </p>
            ) : null}
            {aggregate.error === "" ? null : (
              <div className="grid gap-3">
                <StatusBanner tone="danger">
                  This annual snapshot could not be refreshed:{" "}
                  {aggregate.error} Showing the previous figures.
                </StatusBanner>
                <div>
                  <Button aria-label="Retry month" onClick={onRetry}>
                    Retry
                  </Button>
                </div>
              </div>
            )}
            {aggregate.scopeMatchesOverview ? null : (
              <StatusBanner tone="info">
                This annual snapshot has a different business date from the
                overview. Export stays off until both are refreshed together.
              </StatusBanner>
            )}
            {aggregate.complete ? null : (
              <StatusBanner tone="info">
                This annual snapshot has no row for at least one month, so the
                comparison cannot be exported.
              </StatusBanner>
            )}
            <div className="overflow-x-auto scroll-thin">
              <table
                id="report-period-table"
                tabIndex={-1}
                aria-label={`${aggregate.year} monthly comparison`}
                className="w-full min-w-[820px] text-left text-body"
              >
                <thead>
                  <tr className="border-y border-rule bg-paper-subtle/60 text-micro uppercase tracking-[0.1em] text-ink-500">
                    <th className="px-4 py-3 font-medium">Month</th>
                    <th className="py-3 pr-4 text-right font-medium">POs</th>
                    <th className="py-3 pr-4 text-right font-medium">Amount</th>
                    <th className="py-3 pr-4 text-right font-medium">Costed</th>
                    <th className="py-3 pr-4 text-right font-medium">Capital</th>
                    <th className="py-3 pr-4 text-right font-medium">Margin</th>
                    <th className="py-3 pr-5 text-right font-medium">Margin %</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-rule">
                  {aggregate.rows.map((row) => (
                    <tr
                      key={row.month}
                      data-aggregate-row={row.month}
                      data-aggregate-recorded={row.recorded ? "true" : "false"}
                    >
                      <td className="px-4 py-3 align-middle font-medium text-ink-900">
                        <span className="tnum">{monthLabel(row.month)}</span>
                        {row.future && !row.recorded ? (
                          <span className="ml-2 text-caption text-ink-500">
                            No recorded POs
                          </span>
                        ) : row.recorded ? (
                          <span className="ml-2 text-caption text-ink-500">
                            Recorded
                          </span>
                        ) : null}
                      </td>
                      {rowValues(row, entry).map((value) => (
                        <td
                          key={value.label}
                          data-aggregate-metric={value.label}
                          className={`tnum whitespace-nowrap py-3 pr-4 text-right align-middle ${
                            value.negative
                              ? "text-danger-700"
                              : "text-ink-700"
                          }`}
                        >
                          {value.value}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr
                    data-aggregate-total=""
                    className="border-t border-rule-strong bg-paper-subtle/40 font-medium text-ink-900"
                  >
                    <td className="px-4 py-3">
                      Recorded total
                      <span className="ml-2 block text-caption font-normal text-ink-500">
                        Annual server total, not a row sum
                      </span>
                    </td>
                    {aggregate.total === null ? (
                      <td
                        colSpan={6}
                        className="tnum whitespace-nowrap py-3 pr-5 text-right align-middle text-ink-900"
                      >
                        -
                      </td>
                    ) : (
                      totalCells(aggregate.total, entry).map((value) => (
                        <td
                          key={value.label}
                          data-aggregate-total-metric={value.label}
                          className={`tnum whitespace-nowrap py-3 pr-4 text-right align-middle ${
                            value.negative ? "text-danger-700" : "text-ink-900"
                          }`}
                        >
                          {value.value}
                        </td>
                      ))
                    )}
                  </tr>
                </tfoot>
              </table>
            </div>

            {aggregate.forecasts.length === 0 ? null : (
              <div data-aggregate-forecasts="" className="grid gap-2">
                {aggregate.forecasts.map((forecast) => (
                  <div
                    key={forecast.month}
                    data-aggregate-estimate={forecast.month}
                    data-estimated="true"
                    className="rounded-md border border-dashed border-rule bg-paper-subtle/50 px-3 py-2"
                  >
                    <p className="text-small font-semibold text-ink-700">
                      Estimated forecast (not actual) —{" "}
                      {monthLabel(forecast.month)}
                    </p>
                    <p className="tnum mt-0.5 flex flex-wrap items-baseline gap-x-3 gap-y-1 text-small text-ink-600">
                      <span data-amount-part="base" className="whitespace-nowrap">
                        {money(entry, forecast.estimate.base)}
                      </span>
                      <span className="whitespace-nowrap">
                        Low {money(entry, forecast.estimate.low)} · High{" "}
                        {money(entry, forecast.estimate.high)}
                      </span>
                      <span className="whitespace-nowrap">
                        Margin {money(entry, forecast.estimate.margin)} ·{" "}
                        {formatReportBps(forecast.estimate.marginBps)}
                      </span>
                    </p>
                  </div>
                ))}
              </div>
            )}
            {aggregate.unavailableReason === "" ? null : (
              <div data-aggregate-unavailable="">
                <p className="text-small font-semibold text-ink-700">
                  Forecast unavailable
                </p>
                <p className="text-small leading-relaxed text-ink-500">
                  {monthUnavailableLabel(aggregate.unavailableReason)}
                </p>
              </div>
            )}

            {aggregate.includesFutureDated ? (
              <p
                data-aggregate-future-dated=""
                className="text-small leading-relaxed text-ink-600"
              >
                Future-dated purchase orders are included as recorded
                evidence, never as a forecast.
              </p>
            ) : null}
            <p className="text-caption leading-relaxed text-ink-500">
              Recorded total is the annual server snapshot's own total for
              January to December {aggregate.year}. It is not a sum of the
              rows above, and no estimate or forecast is ever included in it.
            </p>
          </div>
        ) : null}
      </div>
    </Card>
  );
}
